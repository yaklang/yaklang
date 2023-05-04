package yaklib

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type TagValue struct {
	Name  string
	Count int
}

type TagAndTypeValue struct {
	Value string
	Count int
}

type TagStat struct {
	data      []*TagValue
	mapStruct map[string]int
}

func (t *TagStat) GetCount(val string) int {
	if t == nil {
		return 0
	}
	if t.mapStruct == nil {
		t.mapStruct = make(map[string]int)
		for _, v := range t.data {
			t.mapStruct[v.Name] = v.Count
		}
	}
	count, ok := t.mapStruct[val]
	if ok {
		return count
	}
	return 0
}

func (t *TagStat) TopN(n int) []*TagValue {
	if len(t.data) <= n {
		return t.data[:]
	}
	return t.data[:n]
}

func (t *TagStat) All() []*TagValue {
	return t.data
}

func NewTagStat() (*TagStat, error) {
	stat, err := updateTags()
	if err != nil {
		return nil, err
	}
	return &TagStat{data: stat}, nil
}

func (t *TagStat) ForceUpdate() error {
	forceData, err := forceUpdateTags()
	if err != nil {
		return err
	}
	t.data = forceData
	return nil
}

const YAKIT_TAG_STATS = "YAKIT_TAG_STATS"

func forceUpdateTags() ([]*TagValue, error) {
	var db = consts.GetGormProfileDatabase()
	if db == nil {
		log.Error("cannot found database config")
		return nil, utils.Error("empty database")
	}

	yakit.DelKey(db, YAKIT_TAG_STATS)
	return updateTags()
}

func updateTags() ([]*TagValue, error) {
	var db = consts.GetGormProfileDatabase()
	if db == nil {
		log.Error("cannot found database config")
		return nil, utils.Error("empty database")
	}

	value := yakit.GetKey(db, YAKIT_TAG_STATS)
	if value != "" {
		var tags []*TagValue
		_ = json.Unmarshal([]byte(value), &tags)
		if len(tags) > 0 {
			return tags, nil
		}
	}

	//log.Info("start to execute updating tags")
	db = db.Raw(`SELECT value, count(t.id) as count
from (WITH RECURSIVE split(value, str) AS (
    SELECT null, tags || ','
    from yak_scripts WHERE type in ('nuclei', 'port-scan', 'mitm') AND (local_path not like '%-workflow.y%ml')
    UNION ALL
    SELECT substr(str, 0, instr(str, ',')),
           substr(str, instr(str, ',') + 1)
    FROM split
    WHERE str != ''
)
      SELECT DISTINCT value
      FROM split
      WHERE value is not NULL
        and value != '')
         join yak_scripts t on ( type in ('nuclei', 'port-scan', 'mitm') AND (local_path not like '%-workflow.y%ml') AND tags LIKE '%' || value || '%')
group by value order by count desc;`)
	rows, err := db.Rows()
	if err != nil {
		return nil, utils.Errorf("rows failed: %s", err)
	}

	var tags = make([]*TagValue, 0)
	for rows.Next() {
		var tagName string
		var count int
		err = rows.Scan(&tagName, &count)
		if err != nil {
			log.Errorf("scan tag stats failed: %s", err)
			continue
		}
		tags = append(tags, &TagValue{
			Name:  tagName,
			Count: count,
		})
	}

	raw, _ := json.Marshal(tags)
	if len(raw) > 0 {
		//log.Infof("start to cache tags[%v]", len(raw))
		yakit.SetKey(consts.GetGormProfileDatabase(), YAKIT_TAG_STATS, string(raw))
	}
	return tags, nil
}
