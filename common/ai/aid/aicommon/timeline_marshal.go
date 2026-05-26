package aicommon

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"strconv"

	"github.com/yaklang/yaklang/common/utils/omap"
)

// timelineSerializable 用于序列化的 Timeline 结构体
// 关键词: timelineSerializable, summary 向后兼容
// 历史说明：
//   - Summary 字段已废弃（dead code），新数据不再写入；仅在反序列化老数据时容忍其存在并静默忽略。
//   - Reducers/ReducerTs 字段已废弃，新数据不再写入；仅在反序列化老数据时做一次性迁移为 CompressedHead。
type timelineSerializable struct {
	IdToTs                map[string]int64                 `json:"id_to_ts"`
	TsToTimelineItem      map[string]*TimelineItem         `json:"ts_to_timeline_item"`
	IdToTimelineItem      map[string]*TimelineItem         `json:"id_to_timeline_item"`
	Summary               map[string]*TimelineItem         `json:"summary,omitempty"` // deprecated: 仅做向后兼容反序列化
	CompressedHead        *TimelineCompressedHead          `json:"compressed_head,omitempty"`
	CompressedHistory     []*TimelineCompressedHistoryNode `json:"compressed_history,omitempty"`
	Reducers              map[string]string                `json:"reducers,omitempty"`   // legacy read only: migrated to CompressedHead on unmarshal
	ReducerTs             map[string]int64                 `json:"reducer_ts,omitempty"` // legacy read only
	ArchiveRefs           map[string]*TimelineArchiveRef   `json:"archive_refs"`
	PerDumpContentLimit   int64                            `json:"per_dump_content_limit"`
	TotalDumpContentLimit int64                            `json:"total_dump_content_limit"`
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
		item, ok := i.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			return true
		}
		idToTsMap[fmt.Sprintf("%d", id)] = ts
		return true
	})

	tsToTimelineItemMap := make(map[string]*TimelineItem)
	i.tsToTimelineItem.ForEach(func(ts int64, item *TimelineItem) bool {
		if item == nil || item.deleted {
			return true
		}
		tsToTimelineItemMap[fmt.Sprintf("%d", ts)] = item
		return true
	})

	idToTimelineItemMap := make(map[string]*TimelineItem)
	i.idToTimelineItem.ForEach(func(id int64, item *TimelineItem) bool {
		if item == nil || item.deleted {
			return true
		}
		idToTimelineItemMap[fmt.Sprintf("%d", id)] = item
		return true
	})

	archiveRefsMap := make(map[string]*TimelineArchiveRef)
	i.archiveRefs.ForEach(func(id int64, ref *TimelineArchiveRef) bool {
		archiveRefsMap[fmt.Sprintf("%d", id)] = ref
		return true
	})

	serializable := &timelineSerializable{
		IdToTs:                idToTsMap,
		TsToTimelineItem:      tsToTimelineItemMap,
		IdToTimelineItem:      idToTimelineItemMap,
		CompressedHead:        cloneTimelineCompressedHead(i.compressedHead),
		CompressedHistory:     cloneTimelineCompressedHistory(i.compressedHistory),
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

	timeline.compressedHead = cloneTimelineCompressedHead(serializable.CompressedHead)
	timeline.compressedHistory = cloneTimelineCompressedHistory(serializable.CompressedHistory)

	// Legacy migration: if no compressed_head but old reducers data exists, migrate to head+history view
	if timeline.compressedHead == nil && len(serializable.Reducers) > 0 {
		type legacyReducerItem struct {
			id   int64
			text string
			ts   int64
		}
		var legacyItems []legacyReducerItem
		for key, value := range serializable.Reducers {
			if value == "" {
				continue
			}
			id, err := strconv.ParseInt(key, 10, 64)
			if err != nil {
				continue
			}
			var ts int64
			if v, ok := serializable.ReducerTs[key]; ok {
				ts = v
			}
			legacyItems = append(legacyItems, legacyReducerItem{id: id, text: value, ts: ts})
		}
		// sort by id ascending
		for i := 0; i < len(legacyItems); i++ {
			for j := i + 1; j < len(legacyItems); j++ {
				if legacyItems[i].id > legacyItems[j].id {
					legacyItems[i], legacyItems[j] = legacyItems[j], legacyItems[i]
				}
			}
		}
		if len(legacyItems) > 0 {
			for idx, item := range legacyItems {
				version := int64(idx + 1)
				if idx == len(legacyItems)-1 {
					timeline.compressedHead = &TimelineCompressedHead{
						Text:             item.text,
						CoveredEndItemID: item.id,
						CoveredEndAtMs:   item.ts,
						Version:          version,
					}
					break
				}
				timeline.compressedHistory = append(timeline.compressedHistory, &TimelineCompressedHistoryNode{
					Version:          version,
					PrevVersion:      version - 1,
					Text:             item.text,
					CoveredEndItemID: item.id,
					CoveredEndAtMs:   item.ts,
					CreatedAtMs:      item.ts,
				})
			}
		}
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
