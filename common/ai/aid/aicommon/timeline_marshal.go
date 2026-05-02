package aicommon

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"

	"github.com/yaklang/yaklang/common/utils/linktable"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// timelineSerializable 用于序列化的 Timeline 结构体
// 关键词: timelineSerializable, summary 向后兼容, reducerTs 序列化
// 历史说明：
//   - Summary 字段已废弃（dead code），新数据不再写入；仅在反序列化老数据时容忍其存在并静默忽略。
//   - ReducerTs 字段是新增的稳定时间戳，对应 reducers 中每个 key 的原始 unix 毫秒时间。
type timelineSerializable struct {
	IdToTs                map[string]int64               `json:"id_to_ts"`
	TsToTimelineItem      map[string]*TimelineItem       `json:"ts_to_timeline_item"`
	IdToTimelineItem      map[string]*TimelineItem       `json:"id_to_timeline_item"`
	Summary               map[string]*TimelineItem       `json:"summary,omitempty"` // deprecated: 仅做向后兼容反序列化
	Reducers              map[string]string              `json:"reducers"`          // 只保留最后一个值
	ReducerTs             map[string]int64               `json:"reducer_ts,omitempty"`
	ArchiveRefs           map[string]*TimelineArchiveRef `json:"archive_refs"`
	PerDumpContentLimit   int64                          `json:"per_dump_content_limit"`
	TotalDumpContentLimit int64                          `json:"total_dump_content_limit"`
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

	// 转换 reducers 为可序列化的格式，只保留最后一个值
	reducersMap := make(map[string]string)
	i.reducers.ForEach(func(id int64, lt *linktable.LinkTable[string]) bool {
		reducersMap[fmt.Sprintf("%d", id)] = lt.Value() // 只保留最后一个值
		return true
	})

	// 关键词: MarshalTimeline, reducerTs 序列化
	reducerTsMap := make(map[string]int64)
	if i.reducerTs != nil {
		i.reducerTs.ForEach(func(id int64, ts int64) bool {
			reducerTsMap[fmt.Sprintf("%d", id)] = ts
			return true
		})
	}

	archiveRefsMap := make(map[string]*TimelineArchiveRef)
	i.archiveRefs.ForEach(func(id int64, ref *TimelineArchiveRef) bool {
		archiveRefsMap[fmt.Sprintf("%d", id)] = ref
		return true
	})

	serializable := &timelineSerializable{
		IdToTs:                idToTsMap,
		TsToTimelineItem:      tsToTimelineItemMap,
		IdToTimelineItem:      idToTimelineItemMap,
		Reducers:              reducersMap,
		ReducerTs:             reducerTsMap,
		ArchiveRefs:           archiveRefsMap,
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
		compressing:           utils.NewOnce(),
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
		timeline.OrderInsertId(id, value)
	}

	// 关键词: UnmarshalTimeline, summary 向后兼容
	// summary 字段已弃用：读到老数据中的 summary 内容时直接忽略，不再写入 Timeline
	// （此处不需要解析 serializable.Summary，json.Unmarshal 已经把内容放到 serializable 里了，但我们不消费它）
	_ = serializable.Summary

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

	// 关键词: UnmarshalTimeline, reducerTs 反序列化
	timeline.reducerTs = omap.NewOrderedMap(map[int64]int64{})
	for key, value := range serializable.ReducerTs {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || value <= 0 {
			continue
		}
		timeline.reducerTs.Set(id, value)
	}

	timeline.archiveRefs = omap.NewOrderedMap(map[int64]*TimelineArchiveRef{})
	for key, value := range serializable.ArchiveRefs {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil || value == nil {
			continue
		}
		timeline.archiveRefs.Set(id, value)
	}

	return timeline, nil
}
