package aicommon

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/utils/linktable"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// timelineSerializable 用于序列化的 Timeline 结构体
type timelineSerializable struct {
	IdToTs                map[string]int64         `json:"id_to_ts"`
	TsToTimelineItem      map[string]*TimelineItem `json:"ts_to_timeline_item"`
	IdToTimelineItem      map[string]*TimelineItem `json:"id_to_timeline_item"`
	Summary               map[string]*TimelineItem `json:"summary"`  // 只保留最后一个值
	Reducers              map[string]string        `json:"reducers"` // 只保留最后一个值
	PerDumpContentLimit   int64                    `json:"per_dump_content_limit"`
	TotalDumpContentLimit int64                    `json:"total_dump_content_limit"`
}

// MarshalTimeline serializes a Timeline into a string.
// not include function and ai/config fields.
func MarshalTimeline(i *Timeline) (string, error) {
	if i == nil {
		return "", nil
	}

	// 转换omap为map，使用字符串键
	idToTsMap := make(map[string]int64)
	i.idToTs.ForEach(func(id int64, ts int64) bool {
		idToTsMap[fmt.Sprintf("%d", id)] = ts
		return true
	})

	tsToTimelineItemMap := make(map[string]*TimelineItem)
	i.tsToTimelineItem.ForEach(func(ts int64, item *TimelineItem) bool {
		tsToTimelineItemMap[fmt.Sprintf("%d", ts)] = item
		return true
	})

	idToTimelineItemMap := make(map[string]*TimelineItem)
	i.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		idToTimelineItemMap[fmt.Sprintf("%d", id)] = item
		return true
	})

	// 转换 summary 和 reducers 为可序列化的格式，只保留最后一个值
	summaryMap := make(map[string]*TimelineItem)
	i.summary.ForEach(func(id int64, lt *linktable.LinkTable[*TimelineItem]) bool {
		summaryMap[fmt.Sprintf("%d", id)] = lt.Value() // 只保留最后一个值
		return true
	})

	reducersMap := make(map[string]string)
	i.reducers.ForEach(func(id int64, lt *linktable.LinkTable[string]) bool {
		reducersMap[fmt.Sprintf("%d", id)] = lt.Value() // 只保留最后一个值
		return true
	})

	serializable := &timelineSerializable{
		IdToTs:                idToTsMap,
		TsToTimelineItem:      tsToTimelineItemMap,
		IdToTimelineItem:      idToTimelineItemMap,
		Summary:               summaryMap,
		Reducers:              reducersMap,
		PerDumpContentLimit:   i.perDumpContentLimit,
		TotalDumpContentLimit: i.totalDumpContentLimit,
	}

	data, err := json.Marshal(serializable)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func UnmarshalTimeline(s string) (*Timeline, error) {
	if s == "" {
		return NewTimeline(nil, nil), nil
	}

	var serializable timelineSerializable
	err := json.Unmarshal([]byte(s), &serializable)
	if err != nil {
		return nil, err
	}

	// 恢复 Timeline 结构体
	timeline := &Timeline{
		perDumpContentLimit:   serializable.PerDumpContentLimit,
		totalDumpContentLimit: serializable.TotalDumpContentLimit,
	}

	// 恢复 idToTs
	timeline.idToTs = omap.NewOrderedMap(map[int64]int64{})
	for key, value := range serializable.IdToTs {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		timeline.idToTs.Set(id, value)
	}

	// 恢复 tsToTimelineItem
	timeline.tsToTimelineItem = omap.NewOrderedMap(map[int64]*TimelineItem{})
	for key, value := range serializable.TsToTimelineItem {
		ts, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		timeline.tsToTimelineItem.Set(ts, value)
	}

	// 恢复 idToTimelineItem
	timeline.idToTimelineItem = omap.NewOrderedMap(map[int64]*TimelineItem{})
	for key, value := range serializable.IdToTimelineItem {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		timeline.idToTimelineItem.Set(id, value)
	}

	// 恢复 summary
	timeline.summary = omap.NewOrderedMap(map[int64]*linktable.LinkTable[*TimelineItem]{})
	for key, item := range serializable.Summary {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || item == nil {
			continue
		}
		// 从单个值重建 LinkTable
		lt := linktable.NewUnlimitedLinkTable(item)
		timeline.summary.Set(id, lt)
	}

	// 恢复 reducers
	timeline.reducers = omap.NewOrderedMap(map[int64]*linktable.LinkTable[string]{})
	for key, value := range serializable.Reducers {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || value == "" {
			continue
		}
		// 从单个值重建 LinkTable
		lt := linktable.NewUnlimitedStringLinkTable(value)
		timeline.reducers.Set(id, lt)
	}

	return timeline, nil
}
