package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestEvidenceOpToItem(t *testing.T) {
	t.Run("upsert", func(t *testing.T) {
		op, err := BuildSessionEvidenceUpsert("", "  confirmed result  ")
		require.NoError(t, err)

		item, err := evidenceOpToItem(op)
		require.NoError(t, err)
		require.Zero(t, item.GetID())

		promoted, ok := item.value.(*PromotableTimelineItem)
		require.True(t, ok)
		require.Zero(t, promoted.ID)
		require.Equal(t, TimelinePromotedKindEvidence, promoted.Kind)
		require.Equal(t, TimelinePromotedTargetSemiDynamic1, promoted.TargetSection)
		require.Equal(t, TimelinePromotedOperationUpsert, promoted.Operation)
		require.Equal(t, op.ID, promoted.Key)
		require.Equal(t, promotedPayloadHash(promoted.Payload), promoted.PayloadHash)
	})

	t.Run("delete", func(t *testing.T) {
		item, err := evidenceOpToItem(EvidenceOperation{ID: "saved-fact", Op: "delete"})
		require.NoError(t, err)
		require.Zero(t, item.GetID())

		promoted, ok := item.value.(*PromotableTimelineItem)
		require.True(t, ok)
		require.Equal(t, TimelinePromotedOperationDelete, promoted.Operation)
		require.Equal(t, "saved-fact", promoted.Key)
		require.Empty(t, promoted.Payload)
	})

	t.Run("invalid", func(t *testing.T) {
		_, err := evidenceOpToItem(EvidenceOperation{ID: "bad id", Op: "upsert", Content: "content"})
		require.Error(t, err)
		_, err = evidenceOpToItem(EvidenceOperation{ID: "saved-fact", Op: "delete", Content: "unexpected"})
		require.Error(t, err)
		_, err = evidenceOpToItem(EvidenceOperation{ID: "saved-fact", Op: "unknown"})
		require.Error(t, err)
	})
}

func TestItemToEvidenceOp(t *testing.T) {
	original, err := BuildSessionEvidenceUpsert("saved-fact", "confirmed content")
	require.NoError(t, err)
	item, err := evidenceOpToItem(original)
	require.NoError(t, err)

	op, err := itemToEvidenceOp(item)
	require.NoError(t, err)
	require.Equal(t, EvidenceOperation{
		ID:      original.ID,
		Op:      "add",
		Content: original.Content,
	}, op)

	deleteItem, err := evidenceOpToItem(EvidenceOperation{ID: original.ID, Op: "delete"})
	require.NoError(t, err)
	op, err = itemToEvidenceOp(deleteItem)
	require.NoError(t, err)
	require.Equal(t, EvidenceOperation{ID: original.ID, Op: "delete"}, op)

	_, err = itemToEvidenceOp(NewTimelineItem(&TextTimelineItem{Text: "not evidence"}))
	require.Error(t, err)

	promoted := item.value.(*PromotableTimelineItem)
	invalidHash := *promoted
	invalidHash.PayloadHash = "invalid"
	_, err = itemToEvidenceOp(NewTimelineItem(&invalidHash))
	require.Error(t, err)
}

func TestIsEvidenceOpNeeded(t *testing.T) {
	timeline := newEvidenceOpTestTimeline(t, EvidenceItem{ID: "saved-fact", Content: "first"})

	require.False(t, timeline.isEvidenceOpNeeded(EvidenceOperation{
		ID: "saved-fact", Op: "add", Content: "first",
	}))
	require.True(t, timeline.isEvidenceOpNeeded(EvidenceOperation{
		ID: "saved-fact", Op: "update", Content: "second",
	}))
	require.True(t, timeline.isEvidenceOpNeeded(EvidenceOperation{
		ID: "saved-fact", Op: "delete",
	}))
	require.False(t, timeline.isEvidenceOpNeeded(EvidenceOperation{
		ID: "missing-fact", Op: "delete",
	}))
	require.True(t, timeline.isEvidenceOpNeeded(EvidenceOperation{
		ID: "new-fact", Op: "upsert", Content: "new",
	}))
}

func TestFilterEvidenceOps(t *testing.T) {
	timeline := newEvidenceOpTestTimeline(t, EvidenceItem{ID: "existing", Content: "A"})
	ops := []EvidenceOperation{
		{ID: "existing", Op: "add", Content: "A"},
		{ID: "new", Op: "add", Content: "B"},
		{ID: "new", Op: "upsert", Content: "B"},
		{ID: "missing", Op: "delete"},
		{ID: "new", Op: "delete"},
		{ID: "new", Op: "delete"},
		{ID: "existing", Op: "update", Content: "C"},
		{ID: "existing", Op: "unknown", Content: "D"},
	}

	filtered := timeline.filterEvidenceOps(ops)
	require.Equal(t, []EvidenceOperation{
		{ID: "new", Op: "add", Content: "B"},
		{ID: "new", Op: "delete"},
		{ID: "existing", Op: "update", Content: "C"},
	}, filtered)
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
