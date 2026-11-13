package aicommon

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func newEvidenceTestItem(t *testing.T, id, content string) *TimelineItem {
	t.Helper()
	payload, err := json.Marshal(EvidenceItem{ID: id, Content: content})
	require.NoError(t, err)
	return NewTimelineItem(&PromotableTimelineItem{
		Kind:          TimelinePromotedKindEvidence,
		TargetSection: TimelinePromotedTargetSemiDynamic1,
		Key:           id,
		Operation:     TimelinePromotedOperationUpsert,
		Payload:       string(payload),
		PayloadHash:   promotedPayloadHash(string(payload)),
	})
}

func newEvidenceDeleteItem(id string) *TimelineItem {
	return NewTimelineItem(&PromotableTimelineItem{
		Kind:          TimelinePromotedKindEvidence,
		TargetSection: TimelinePromotedTargetSemiDynamic1,
		Key:           id,
		Operation:     TimelinePromotedOperationDelete,
		PayloadHash:   promotedPayloadHash(""),
	})
}

func newEvidenceOpTestTimeline(t *testing.T, items ...EvidenceItem) *Timeline {
	t.Helper()
	timeline := NewTimeline(nil, nil)
	if len(items) == 0 {
		return timeline
	}
	var id int64
	require.NoError(t, timeline.replaceEvidence(
		&EvidenceStore{Items: append([]EvidenceItem(nil), items...)},
		func() int64 {
			id++
			return id
		},
	))
	return timeline
}

func TestItemToEvidenceOp(t *testing.T) {
	item := newEvidenceTestItem(t, "saved-fact", "confirmed content")

	op, err := itemToEvidenceOp(item)
	require.NoError(t, err)
	require.Equal(t, EvidenceOperation{
		ID:      "saved-fact",
		Op:      "add",
		Content: "confirmed content",
	}, op)

	op, err = itemToEvidenceOp(newEvidenceDeleteItem("saved-fact"))
	require.NoError(t, err)
	require.Equal(t, EvidenceOperation{ID: "saved-fact", Op: "delete"}, op)

	_, err = itemToEvidenceOp(NewTimelineItem(&TextTimelineItem{Text: "not evidence"}))
	require.Error(t, err)

	promoted := item.value.(*PromotableTimelineItem)
	invalidHash := *promoted
	invalidHash.PayloadHash = "invalid"
	_, err = itemToEvidenceOp(NewTimelineItem(&invalidHash))
	require.Error(t, err)
}

func TestNewEvidenceTimelineItem(t *testing.T) {
	item, err := newEvidenceTimelineItem(EvidenceOperation{
		ID:      "saved-fact",
		Op:      "update",
		Content: "confirmed content",
	})
	require.NoError(t, err)
	require.Zero(t, item.GetID())
	require.NotEmpty(t, item.historyID)
	require.False(t, item.createdAt.IsZero())

	promoted, ok := item.value.(*PromotableTimelineItem)
	require.True(t, ok)
	require.Zero(t, promoted.ID)
	require.Equal(t, TimelinePromotedKindEvidence, promoted.Kind)
	require.Equal(t, TimelinePromotedTargetSemiDynamic1, promoted.TargetSection)
	require.Equal(t, "saved-fact", promoted.Key)
	require.Equal(t, TimelinePromotedOperationUpsert, promoted.Operation)

	op, err := itemToEvidenceOp(item)
	require.NoError(t, err)
	require.Equal(t, EvidenceOperation{ID: "saved-fact", Op: "add", Content: "confirmed content"}, op)

	item, err = newEvidenceTimelineItem(EvidenceOperation{ID: "saved-fact", Op: "delete"})
	require.NoError(t, err)
	promoted = item.value.(*PromotableTimelineItem)
	require.Equal(t, TimelinePromotedOperationDelete, promoted.Operation)
	require.Empty(t, promoted.Payload)

	_, err = newEvidenceTimelineItem(EvidenceOperation{ID: "", Op: "bad"})
	require.Error(t, err)
}

func TestIsEvidenceOpNeeded(t *testing.T) {
	timeline := newEvidenceOpTestTimeline(t, EvidenceItem{ID: "saved-fact", Content: "first"})
	needed := func(op EvidenceOperation) bool {
		timeline.mu.RLock()
		defer timeline.mu.RUnlock()
		return timeline.isEvidenceOpNeededLocked(op)
	}

	require.False(t, needed(EvidenceOperation{
		ID: "saved-fact", Op: "add", Content: "first",
	}))
	require.True(t, needed(EvidenceOperation{
		ID: "saved-fact", Op: "update", Content: "second",
	}))
	require.True(t, needed(EvidenceOperation{
		ID: "saved-fact", Op: "delete",
	}))
	require.False(t, needed(EvidenceOperation{
		ID: "missing-fact", Op: "delete",
	}))
	require.True(t, needed(EvidenceOperation{
		ID: "new-fact", Op: "upsert", Content: "new",
	}))
}

func TestNormalizeEvidenceOpAndChangesStore(t *testing.T) {
	normalized, ok := normalizeEvidenceOp(EvidenceOperation{ID: " fact ", Op: " UPDATE ", Content: "value"})
	require.True(t, ok)
	require.Equal(t, "fact", normalized.ID)
	require.Equal(t, "add", normalized.Op)
	require.Equal(t, "value", normalized.Content)

	store := &EvidenceStore{Items: []EvidenceItem{{ID: "fact", Content: "value"}}}
	require.False(t, opChangesStore(store, normalized))

	normalized.Content = "changed"
	require.True(t, opChangesStore(store, normalized))
	require.True(t, opChangesStore(store, EvidenceOperation{ID: "fact", Op: "delete"}))
	require.False(t, opChangesStore(store, EvidenceOperation{ID: "missing", Op: "delete"}))
}

func TestApplyEvidenceOperationsUsesChangeCheck(t *testing.T) {
	timeline := newEvidenceOpTestTimeline(t, EvidenceItem{ID: "existing", Content: "same"})
	nextID := int64(1)
	acquireID := func() int64 {
		nextID++
		return nextID
	}

	store, changed, err := timeline.applyEvidenceOperationsWithChange([]EvidenceOperation{
		{ID: "existing", Op: "add", Content: "same"},
		{ID: "missing", Op: "delete"},
		{ID: "existing", Op: "update", Content: "updated"},
		{ID: "new", Op: "add", Content: "new"},
	}, "", acquireID)
	require.NoError(t, err)
	require.True(t, changed)
	require.Contains(t, store.Render(), "updated")
	require.Contains(t, store.Render(), "new")
	require.Equal(t, 3, timeline.idToTimelineItem.Len())

	_, changed, err = timeline.applyEvidenceOperationsWithChange([]EvidenceOperation{
		{ID: "existing", Op: "add", Content: "updated"},
		{ID: "new", Op: "add", Content: "new"},
	}, "", acquireID)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, 3, timeline.idToTimelineItem.Len())
}
