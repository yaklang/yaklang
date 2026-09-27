package aicommon

import (
	"encoding/json"
	"sort"
	"strconv"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// ReassignIDs remaps live entries AND compressed coverage boundaries. A fully
// compressed timeline has no ordinary entries left, but must still reserve its
// watermark so subsequently allocated IDs stay in Open. Build the entire new
// view before publication; errors never leave partially reassigned items.
func (m *Timeline) ReassignIDs(generator func() int64) int64 {
	if m == nil || generator == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make(map[int64]bool)
	items := make(map[int64]*TimelineItem)
	for _, id := range m.idToTimelineItem.Keys() {
		item, _ := m.idToTimelineItem.Get(id)
		if item == nil || item.deleted {
			continue
		}
		raw, err := json.Marshal(item)
		if err != nil {
			log.Errorf("remap timeline item %d: %v", id, err)
			return 0
		}
		var copy TimelineItem
		if err := json.Unmarshal(raw, &copy); err != nil {
			log.Errorf("remap timeline item %d: %v", id, err)
			return 0
		}
		items[id], ids[id] = &copy, true
	}
	add := func(id int64) {
		if id > 0 {
			ids[id] = true
		}
	}
	if m.compressedHead != nil {
		add(m.compressedHead.CoveredEndItemID)
	}
	for _, h := range m.compressedHistory {
		if h != nil {
			add(h.CoveredEndItemID)
		}
	}
	if m.freezeState != nil {
		for _, b := range m.freezeState.Batches {
			for _, id := range b.IDs {
				add(id)
			}
		}
	}
	ordered := make([]int64, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	remap := make(map[int64]int64, len(ids))
	var last int64
	for _, id := range ordered {
		next := generator()
		if next <= last {
			log.Error("timeline ID generator must be positive and strictly increasing")
			return 0
		}
		remap[id], last = next, next
	}
	newIDs := omap.NewOrderedMap(map[int64]*TimelineItem{})
	newTS := omap.NewOrderedMap(map[int64]*TimelineItem{})
	newIDToTS := omap.NewOrderedMap(map[int64]int64{})
	for _, id := range ordered {
		item := items[id]
		if item == nil {
			continue
		}
		next := remap[id]
		switch v := item.value.(type) {
		case *aitool.ToolResult:
			v.ID = next
		case *UserInteraction:
			v.ID = next
		case *TextTimelineItem:
			v.ID = next
		case *PromotableTimelineItem:
			v.ID = next
		default:
			log.Errorf("unsupported timeline item for remapping: %T", item.value)
			return 0
		}
		ts, _ := m.idToTs.Get(id)
		newIDs.OrderInsert(next, item, lessInt64)
		newTS.OrderInsert(ts, item, lessInt64)
		newIDToTS.Set(next, ts)
	}
	m.idToTimelineItem, m.tsToTimelineItem, m.idToTs = newIDs, newTS, newIDToTS
	if m.compressedHead != nil {
		m.compressedHead.CoveredEndItemID = remap[m.compressedHead.CoveredEndItemID]
	}
	for _, h := range m.compressedHistory {
		if h != nil {
			h.CoveredEndItemID = remap[h.CoveredEndItemID]
		}
	}
	if m.freezeState != nil {
		for _, b := range m.freezeState.Batches {
			for i, id := range b.IDs {
				b.IDs[i] = remap[id]
			}
		}
		m.freezeState.Version++
	}
	m.rebuildPromotedStateLocked(m.frozenThroughLocked())
	m.compressionLastFailure = ""
	return last
}

// restoreCompressionHistory migrates legacy reducer dumps once; new writes use only head/history.
func (timeline *Timeline) restoreCompressionHistory(serializable *timelineSerializable) {
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
		sort.Slice(legacyItems, func(i, j int) bool { return legacyItems[i].id < legacyItems[j].id })
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

}
