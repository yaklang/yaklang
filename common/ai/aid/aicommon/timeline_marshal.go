package aicommon

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/log"
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
	ProjectionNonce       string                           `json:"projection_nonce,omitempty"`
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
	PromotedState         *TimelinePromotedState           `json:"promoted_state,omitempty"`
}

// MarshalTimeline serializes a Timeline into a string.
// not include function and ai/config fields.
func MarshalTimeline(i *Timeline) (string, error) {
	if i == nil {
		return "", nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return marshalTimelineUnlocked(i)
}

func marshalTimelineUnlocked(i *Timeline) (string, error) {
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
		ProjectionNonce:       aiprojection.Nonce(),
		IdToTs:                idToTsMap,
		TsToTimelineItem:      tsToTimelineItemMap,
		IdToTimelineItem:      idToTimelineItemMap,
		CompressedHead:        cloneTimelineCompressedHead(i.compressedHead),
		CompressedHistory:     cloneTimelineCompressedHistory(i.compressedHistory),
		ArchiveRefs:           archiveRefsMap,
		PerDumpContentLimit:   i.perDumpContentLimit,
		TotalDumpContentLimit: i.totalDumpContentLimit,
		PromotedState:         cloneTimelinePromotedState(i.promotedState),
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
	rebindTimelineProjectionNonce(&serializable)

	// 恢复 Timeline 结构体
	timeline := &Timeline{
		perDumpContentLimit:   serializable.PerDumpContentLimit,
		totalDumpContentLimit: serializable.TotalDumpContentLimit,
		compressing:           utils.NewOnce(),
		branchTimeline:        false,
		promotedState:         cloneTimelinePromotedState(serializable.PromotedState),
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

	// summary 仍参与 typed JSON 解码以兼容旧数据，但恢复时不消费其内容。

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

// Only the system-owned alternate prompt field is eligible for nonce migration.
// Both indexes are serialized independently; update both without touching Text,
// summaries, evidence, timestamps, ordering, or strings inside replay payloads.
func rebindTimelineProjectionNonce(saved *timelineSerializable) {
	updated := make(map[string]string)
	for _, index := range []map[string]*TimelineItem{saved.IdToTimelineItem, saved.TsToTimelineItem} {
		for _, item := range index {
			text, ok := timelineTextItem(item)
			if !ok || text.PromptText == "" {
				continue
			}
			category := normalizeTimelinePromptCategory(extractTextEntryType(text.Text))
			if category != "FUNCTION_CALL_ACTION_RESPONSE" && category != "MODEL_THINKING" {
				continue
			}
			if rebound, ok := updated[text.PromptText]; ok {
				text.PromptText = rebound
				continue
			}
			original := text.PromptText
			prefix, body := "", original
			if normalizeTimelinePromptCategory(extractTextEntryType(original)) == category {
				location := withTaskRegex.FindStringIndex(original)
				if location == nil {
					location = withoutTaskRegex.FindStringIndex(original)
				}
				if location != nil {
					prefix, body = original[:location[1]], original[location[1]:]
				}
			}
			rebound, err := aiprojection.RebindReplayNonce(body, saved.ProjectionNonce)
			if err != nil {
				// Keep the human-readable record; never upgrade arbitrary text or
				// leak an obsolete/invalid protocol envelope into a model request.
				log.Warnf("timeline replay restore skipped for item %d: %v", text.ID, err)
				rebound = ""
			} else {
				rebound = prefix + rebound
			}
			updated[original] = rebound
			text.PromptText = rebound
		}
	}
}
