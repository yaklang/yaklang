package aicommon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func evidenceMutationItemIDs(timeline *Timeline, key string) []int64 {
	var ids []int64
	timeline.mu.RLock()
	for _, id := range timeline.idToTimelineItem.Keys() {
		item, _ := timeline.idToTimelineItem.Get(id)
		if item == nil {
			continue
		}
		op, _ := item.value.(*PromotableTimelineItem)
		if op != nil && op.Kind == TimelinePromotedKindEvidence && op.Key == key {
			ids = append(ids, id)
		}
	}
	timeline.mu.RUnlock()
	return ids
}

func TestEvidenceBroadcastAllocatesLocalTimelineIDs(t *testing.T) {
	parent := newTimelineBroadcastConfig(t)
	fork, err := parent.Timeline.ForkForTask("evidence", "evidence", parent, parent)
	require.NoError(t, err)
	t.Cleanup(fork.Release)

	op, err := BuildSessionEvidenceUpsert("shared-fact", "FIRST_FACT")
	require.NoError(t, err)
	ops, changed, err := parent.BroadcastSessionEvidenceOps([]EvidenceOperation{op})
	require.NoError(t, err)
	require.True(t, changed)
	require.Len(t, ops, 1)

	parentStore, found := parent.Timeline.evidenceStore()
	require.True(t, found)
	branchStore, found := fork.Branch.evidenceStore()
	require.True(t, found)
	require.Contains(t, parentStore.Render(), "FIRST_FACT")
	require.Contains(t, branchStore.Render(), "FIRST_FACT")

	parentIDs := evidenceMutationItemIDs(parent.Timeline, "shared-fact")
	branchIDs := evidenceMutationItemIDs(fork.Branch, "shared-fact")
	require.Len(t, parentIDs, 1)
	require.Len(t, branchIDs, 1)
	require.NotEqual(t, parentIDs[0], branchIDs[0])

	parentMax, branchMax := parent.Timeline.GetMaxID(), fork.Branch.GetMaxID()
	_, changed, err = parent.BroadcastSessionEvidenceOps([]EvidenceOperation{op})
	require.NoError(t, err)
	require.False(t, changed, "identical evidence must not append another mutation")
	require.Equal(t, parentMax, parent.Timeline.GetMaxID())
	require.Equal(t, branchMax, fork.Branch.GetMaxID())

	updated, err := BuildSessionEvidenceUpsert("shared-fact", "SECOND_FACT")
	require.NoError(t, err)
	_, changed, err = parent.BroadcastSessionEvidenceOps([]EvidenceOperation{updated})
	require.NoError(t, err)
	require.True(t, changed)
	parentStore, _ = parent.Timeline.evidenceStore()
	branchStore, _ = fork.Branch.evidenceStore()
	require.Contains(t, parentStore.Render(), "SECOND_FACT")
	require.NotContains(t, parentStore.Render(), "FIRST_FACT")
	require.Contains(t, branchStore.Render(), "SECOND_FACT")
	require.Len(t, evidenceMutationItemIDs(parent.Timeline, "shared-fact"), 2)
	require.Len(t, evidenceMutationItemIDs(fork.Branch, "shared-fact"), 2)

	_, changed, err = parent.BroadcastSessionEvidenceOps([]EvidenceOperation{{ID: "shared-fact", Op: "delete"}})
	require.NoError(t, err)
	require.True(t, changed)
	parentStore, _ = parent.Timeline.evidenceStore()
	branchStore, _ = fork.Branch.evidenceStore()
	require.NotContains(t, parentStore.Render(), "SECOND_FACT")
	require.NotContains(t, branchStore.Render(), "SECOND_FACT")
	require.Len(t, evidenceMutationItemIDs(parent.Timeline, "shared-fact"), 3)
	require.Len(t, evidenceMutationItemIDs(fork.Branch, "shared-fact"), 3)
}
