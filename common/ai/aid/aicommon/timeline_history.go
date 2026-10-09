package aicommon

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

// HistorySnapshot captures independent live originals, including promoted items.
// Their stable identities also tell history queries what is already visible.
func (m *Timeline) HistorySnapshot() ([]schema.AITimelineHistory, error) {
	if m == nil {
		return nil, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.historySnapshotLocked()
}

func (m *Timeline) historySnapshotLocked() ([]schema.AITimelineHistory, error) {
	rows := make([]schema.AITimelineHistory, 0)
	for _, id := range m.idToTimelineItem.Keys() {
		item, ok := m.idToTimelineItem.Get(id)
		if !ok || item == nil || item.deleted {
			continue
		}
		raw, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		row, err := yakit.AITimelineHistoryItem(id, raw, originalTimelineItemText(item))
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// Capture before unlocking so compression/replacement cannot change the receipt.
// Persist outside Timeline.mu, before Push returns. Checkpoints retry all live
// receipts and fail closed before compression can retire any original.
func (m *Timeline) collectHistoryWriteLocked(notifications *[]func(), item *TimelineItem) {
	if item.historyID == "" {
		item.historyID = uuid.NewString()
	}
	if notifications == nil {
		return
	}
	config := m.config
	if m.sessionMemory != nil {
		m.sessionMemory.mu.Lock()
		if owner := m.sessionMemory.persistenceConfig; owner != nil {
			config = owner
		}
		m.sessionMemory.mu.Unlock()
	}
	cfg, ok := config.(*Config)
	if !ok || cfg.DisableCreateDBRuntime || cfg.PersistentSessionId == "" {
		return
	}
	raw, err := json.Marshal(item)
	if err != nil {
		log.Errorf("serialize timeline history item: %v", err)
		return
	}
	row, err := yakit.AITimelineHistoryItem(item.GetID(), raw, originalTimelineItemText(item))
	if err != nil {
		log.Errorf("capture timeline history item: %v", err)
		return
	}
	db, session := cfg.GetDB(), cfg.PersistentSessionId
	*notifications = append(*notifications, func() {
		m.persistMu.Lock()
		defer m.persistMu.Unlock()
		if err := yakit.EnsureAITimelineHistory(db, false); err != nil {
			log.Errorf("prepare timeline history: %v", err)
			return
		}
		if err := db.Transaction(func(tx *gorm.DB) error {
			return yakit.ArchiveAITimelineItems(tx, session, []schema.AITimelineHistory{row})
		}); err != nil {
			log.Errorf("persist timeline history item: %v", err)
		}
	})
}

// ToolResult.String normally honors shrink projections. History must always
// render/search the original result; copy it so live prompt state is untouched.
func originalTimelineItemText(item *TimelineItem) string {
	if op, ok := item.value.(*PromotableTimelineItem); ok {
		return fmt.Sprintf("promotion kind=%s target_section=%s key=%s operation=%s\n%s", op.Kind, op.TargetSection, op.Key, op.Operation, op.Payload)
	}
	if tool, ok := item.value.(*aitool.ToolResult); ok {
		original := *tool
		original.ShrinkResult, original.ShrinkSimilarResult = "", ""
		// Restore the captured execution envelope after JSON storage so native
		// dumping preserves combined stdout/stderr ordering and raw text.
		if data, ok := original.Data.(map[string]any); ok {
			_, hasResult := data["result"]
			_, hasStdout := data["stdout"]
			_, hasStderr := data["stderr"]
			_, hasCombined := data["combined_output"]
			if hasResult && hasStdout && hasStderr && hasCombined {
				if raw, err := json.Marshal(data); err == nil {
					var envelope aitool.ToolExecutionResult
					if json.Unmarshal(raw, &envelope) == nil {
						original.Data = &envelope
					}
				}
			}
		}
		return original.String()
	}
	return item.String()
}

// DumpAITimelineHistory renders only the selected original items with the normal
// Timeline dump renderer. JSON is storage only: decode it, call the original
// value's String, and retain literal quotes/newlines in the stdout text.
func DumpAITimelineHistory(rows []*schema.AITimelineHistory, offset, limit int) (string, error) {
	tl := NewTimeline(nil, nil)
	for _, row := range rows {
		raw, err := json.Marshal(timelineItemSerializable{HistoryID: row.HistoryID,
			CreatedAt: row.Timestamp, Type: row.Type, Value: json.RawMessage(row.ValueJSON)})
		if err != nil {
			return "", err
		}
		var original TimelineItem
		if err := json.Unmarshal(raw, &original); err != nil {
			return "", err
		}
		content := []rune(originalTimelineItemText(&original))
		start := min(offset, len(content))
		end := min(start+limit, len(content))
		metadata := fmt.Sprintf("history item_id=%d timeline_item_id=%d type=%s content_offset=%d next_content_offset=%d truncated=%t\n",
			row.ID, row.ItemID, row.Type, start, end, end < len(content))
		// An unshrunk text view keeps tool/user formatting and also makes archived
		// promotion journals readable instead of reapplying their control effects.
		item := &TimelineItem{historyID: row.HistoryID, createdAt: row.Timestamp,
			value: &TextTimelineItem{ID: int64(row.ID), Text: metadata + string(content[start:end])}}
		tl.OrderInsertId(int64(row.ID), item)
		tl.OrderInsertTs(row.Timestamp.UnixMilli(), item)
	}
	return tl.Dump(), nil
}
