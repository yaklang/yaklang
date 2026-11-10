package aicommon

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func normalizeEvidenceBroadcastOperation(op EvidenceOperation) (EvidenceOperation, error) {
	id := strings.TrimSpace(op.ID)
	switch strings.ToLower(strings.TrimSpace(op.Op)) {
	case "add", "update", "upsert":
		return BuildSessionEvidenceUpsert(id, op.Content)
	case "delete":
		if !sessionEvidenceIDPattern.MatchString(id) {
			return EvidenceOperation{}, fmt.Errorf("invalid session evidence id %q", id)
		}
		return EvidenceOperation{ID: id, Op: "delete"}, nil
	default:
		return EvidenceOperation{}, fmt.Errorf("unsupported session evidence operation %q", op.Op)
	}
}

func newEvidenceBroadcastTimelineItem(op EvidenceOperation) (*TimelineItem, error) {
	normalized, err := normalizeEvidenceBroadcastOperation(op)
	if err != nil {
		return nil, err
	}
	control := &PromotableTimelineItem{
		Kind:          TimelinePromotedKindEvidence,
		TargetSection: TimelinePromotedTargetSemiDynamic1,
		Key:           normalized.ID,
	}
	switch normalized.Op {
	case "delete":
		control.Operation = TimelinePromotedOperationDelete
	default:
		item := newEvidenceItem(normalized.ID, normalized.Content, time.Now().Unix())
		payload, err := json.Marshal(item)
		if err != nil {
			return nil, err
		}
		control.Operation = TimelinePromotedOperationUpsert
		control.Payload = string(payload)
		control.PayloadHash = promotedPayloadHash(control.Payload)
	}
	item := NewTimelineItem(control)
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"session-evidence", normalized.Op, normalized.ID, normalized.Content,
	}, "\x00")))
	item.historyID = fmt.Sprintf("evidence:%x", sum[:])
	return item, nil
}

// BroadcastSessionEvidenceOps publishes evidence writes to every live Timeline
// in the session. Private ApplySessionEvidenceOps mutations remain local.
func (c *Config) BroadcastSessionEvidenceOps(ops []EvidenceOperation) ([]EvidenceOperation, bool, error) {
	if c == nil {
		return nil, false, fmt.Errorf("config is nil")
	}
	if len(ops) == 0 {
		return nil, false, nil
	}

	normalized := make([]EvidenceOperation, 0, len(ops))
	items := make([]*TimelineItem, 0, len(ops))
	for _, op := range ops {
		canonical, err := normalizeEvidenceBroadcastOperation(op)
		if err != nil {
			return nil, false, err
		}
		item, err := newEvidenceBroadcastTimelineItem(canonical)
		if err != nil {
			return nil, false, err
		}
		normalized = append(normalized, canonical)
		items = append(items, item)
	}

	timeline := c.GetTimeline()
	before := ""
	if timeline != nil {
		if store, _ := timeline.evidenceStore(); store != nil {
			before = store.Render()
		}
	}
	for _, item := range items {
		if err := c.BroadcastTimelineItem(item); err != nil {
			return normalized, false, err
		}
	}
	after := ""
	if timeline != nil {
		if store, _ := timeline.evidenceStore(); store != nil {
			after = store.Render()
		}
	}
	return normalized, before != after, nil
}

func isEvidenceBroadcastItem(item *TimelineItem) bool {
	if item == nil {
		return false
	}
	op, ok := item.value.(*PromotableTimelineItem)
	return ok && op != nil && op.Kind == TimelinePromotedKindEvidence
}

func (c *Config) initializeEvidenceBroadcastObserver() {
	if c == nil {
		return
	}
	timeline, broadcaster := c.timelineBroadcastState()
	if timeline == nil || broadcaster == nil {
		return
	}
	key := "zzz-evidence:" + configTimelineBroadcastID(c)
	broadcaster.Register(key, func(item *TimelineItem) error {
		if isEvidenceBroadcastItem(item) {
			c.persistBroadcastEvidence(timeline)
		}
		return nil
	})
	if c.Ctx != nil {
		context.AfterFunc(c.Ctx, func() { broadcaster.Unregister(key) })
	}
}
