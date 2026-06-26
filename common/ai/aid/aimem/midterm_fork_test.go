package aimem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestForkMidtermArchiveStore_SharedBaseNodesBranchOnlyWrites(t *testing.T) {
	db, err := getTestDatabase()
	require.NoError(t, err)
	parent, err := NewAIMemoryForQuery("timeline-midterm:test-parent-fork", WithDatabase(db))
	require.NoError(t, err)
	defer func() {
		_ = parent.Close()
	}()

	parentEntity := &aicommon.MemoryEntity{
		Id:                 "parent-node-1",
		Content:            "parent archived chunk",
		Tags:               []string{midtermMemoryKindTag},
		PotentialQuestions: []string{"parent question"},
		C_Score:            0.8,
		O_Score:            0.8,
		R_Score:            0.8,
		E_Score:            0.5,
		P_Score:            0.2,
		A_Score:            0.6,
		T_Score:            0.8,
		CorePactVector:     []float32{0.8, 0.8, 0.8, 0.5, 0.2, 0.6, 0.8},
	}
	require.NoError(t, parent.SaveMemoryEntities(parentEntity))
	require.True(t, parent.hnswBackend.HasMemoryID(parentEntity.Id))

	fork, err := ForkMidtermArchiveStore(parent, "1-1", "task-a", "persistent-session-1")
	require.NoError(t, err)
	require.NotNil(t, fork)
	require.True(t, fork.BranchStore.hnswBackend.HasMemoryID(parentEntity.Id))

	branchEntity := &aicommon.MemoryEntity{
		Id:                 "branch-node-1",
		Content:            "branch archived chunk",
		Tags:               []string{midtermMemoryKindTag},
		PotentialQuestions: []string{"branch question"},
		C_Score:            0.7,
		O_Score:            0.7,
		R_Score:            0.7,
		E_Score:            0.5,
		P_Score:            0.2,
		A_Score:            0.6,
		T_Score:            0.7,
		CorePactVector:     []float32{0.7, 0.7, 0.7, 0.5, 0.2, 0.6, 0.7},
	}
	require.NoError(t, fork.BranchStore.SaveMemoryEntities(branchEntity))
	require.True(t, fork.BranchStore.hnswBackend.HasMemoryID(branchEntity.Id))
	require.False(t, parent.hnswBackend.HasMemoryID(branchEntity.Id))

	mergeResult, err := fork.MergeBack()
	require.NoError(t, err)
	require.NotNil(t, mergeResult)
	require.Equal(t, 1, mergeResult.NodesMerged)
	require.True(t, parent.hnswBackend.HasMemoryID(branchEntity.Id))

	var row struct {
		SessionID string
	}
	require.NoError(t, parent.GetDB().Table("ai_memory_entities_v1").
		Select("session_id").Where("memory_id = ?", branchEntity.Id).Scan(&row).Error)
	require.Equal(t, parent.GetSessionID(), row.SessionID)
}

func TestMidtermMemoryFork_SearchIncludesBaseAndBranch(t *testing.T) {
	db, err := getTestDatabase()
	require.NoError(t, err)
	parent, err := NewAIMemoryForQuery("timeline-midterm:test-search-fork", WithDatabase(db))
	require.NoError(t, err)
	defer func() {
		_ = parent.Close()
	}()

	parentEntity := &aicommon.MemoryEntity{
		Id:                 "shared-search-node",
		Content:            "shared keyword alpha archive",
		Tags:               []string{midtermMemoryKindTag},
		PotentialQuestions: []string{"alpha archive"},
		C_Score:            0.8,
		O_Score:            0.8,
		R_Score:            0.8,
		E_Score:            0.5,
		P_Score:            0.2,
		A_Score:            0.6,
		T_Score:            0.8,
		CorePactVector:     []float32{0.8, 0.8, 0.8, 0.5, 0.2, 0.6, 0.8},
	}
	require.NoError(t, parent.SaveMemoryEntities(parentEntity))

	fork, err := ForkMidtermArchiveStore(parent, "1-2", "task-b", "persistent-session-2")
	require.NoError(t, err)

	branchEntity := &aicommon.MemoryEntity{
		Id:                 "branch-search-node",
		Content:            "branch keyword beta archive",
		Tags:               []string{midtermMemoryKindTag},
		PotentialQuestions: []string{"beta archive"},
		C_Score:            0.7,
		O_Score:            0.7,
		R_Score:            0.7,
		E_Score:            0.5,
		P_Score:            0.2,
		A_Score:            0.6,
		T_Score:            0.7,
		CorePactVector:     []float32{0.7, 0.7, 0.7, 0.5, 0.2, 0.6, 0.7},
	}
	require.NoError(t, fork.BranchStore.SaveMemoryEntities(branchEntity))

	result, err := fork.SearchArchivedBatches(context.Background(), &aicommon.TimelineArchiveSearchQuery{
		Query:                 "alpha",
		BytesLimit:            4096,
		DisableSemanticSearch: true,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.SelectedMemory)

	resultBeta, err := fork.SearchArchivedBatches(context.Background(), &aicommon.TimelineArchiveSearchQuery{
		Query:                 "beta",
		BytesLimit:            4096,
		DisableSemanticSearch: true,
	})
	require.NoError(t, err)
	require.NotNil(t, resultBeta)
	require.Len(t, resultBeta.SelectedMemory, 1)
	require.Equal(t, branchEntity.Id, resultBeta.SelectedMemory[0].Id)
}
