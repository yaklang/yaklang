package aimem

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/schema"
)

// Timeline tests exercise real persistence/indexing/retrieval with explicit,
// distinct timestamps. Reverse insertion order makes the sorting assertion
// meaningful; no wall-clock sleeps or AI-generated duplicate fixtures are needed.
func timelineFixture(t *testing.T, contents []string) (*AIMemoryTriage, map[string]time.Time) {
	t.Helper()
	memory, err := CreateTestAIMemory(t, "timeline-"+uuid.NewString(), WithInvoker(mock.NewMockInvoker(context.Background())))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, memory.Close()) })
	times := make(map[string]time.Time)
	base := time.Now().Add(-time.Minute)
	for i, content := range contents {
		id := fmt.Sprintf("timeline-%d", i)
		entity := &aicommon.MemoryEntity{Id: id, Content: content, Tags: []string{"timeline"}, PotentialQuestions: []string{content},
			C_Score: .9, O_Score: .9, R_Score: .9, E_Score: .9, P_Score: .9, A_Score: .9, T_Score: 1,
			CorePactVector: []float32{.9, .9, .9, .9, .9, .9, 1}}
		require.NoError(t, memory.SaveMemoryEntities(entity))
		createdAt := base.Add(-time.Duration(i) * time.Second)
		updated := memory.GetDB().Model(&schema.AIMemoryEntity{}).Where("session_id = ? AND memory_id = ?", memory.GetSessionID(), id).UpdateColumn("created_at", createdAt)
		require.NoError(t, updated.Error)
		require.EqualValues(t, 1, updated.RowsAffected)
		times[id] = createdAt
	}
	return memory, times
}

func assertTimeline(t *testing.T, result *aicommon.SearchMemoryResult, times map[string]time.Time) {
	t.Helper()
	require.NotNil(t, result)
	require.GreaterOrEqual(t, len(result.Memories), 2, "sorting needs at least two distinct memories")
	seen := make(map[string]bool)
	for i, entry := range result.Memories {
		expected, ok := times[entry.Id]
		require.True(t, ok, "unexpected memory %s", entry.Id)
		require.False(t, seen[entry.Id], "duplicate search result %s", entry.Id)
		seen[entry.Id] = true
		require.True(t, entry.CreatedAt.Equal(expected), "persisted timestamp was lost")
		if i > 0 {
			require.True(t, result.Memories[i-1].CreatedAt.Before(entry.CreatedAt), "search must sort by timestamp, not insertion order or relevance")
		}
	}
}

func TestSearchMemory_TimelineOrdering(t *testing.T) {
	memory, times := timelineFixture(t, []string{"Go基础语法", "Go并发编程", "Go内存模型", "Go微服务"})
	result, err := memory.SearchMemory("Go", 5000)
	require.NoError(t, err)
	assertTimeline(t, result, times)
}

func TestSearchMemoryWithoutAI_TimelineOrdering(t *testing.T) {
	memory, times := timelineFixture(t, []string{"Go基础语法", "Go并发编程", "Go内存模型", "Go微服务"})
	result, err := memory.SearchMemoryWithoutAI("Go", 5000)
	require.NoError(t, err)
	assertTimeline(t, result, times)
	// Keyword retrieval must retain every prepared record and the same order.
	result, err = memory.SearchMemoryWithoutAIAndSemantics("Go", 5000)
	require.NoError(t, err)
	require.Len(t, result.Memories, len(times))
	assertTimeline(t, result, times)
}

func TestSearchMemory_TimelineWithMixedContent(t *testing.T) {
	memory, times := timelineFixture(t, []string{"Go协程", "Python数据分析", "Go channel", "JavaScript异步编程", "Go垃圾回收", "Rust所有权", "Go性能调优"})
	result, err := memory.SearchMemory("Go", 4000)
	require.NoError(t, err)
	assertTimeline(t, result, times)
}

func TestSearchMemory_TimelineEmptyResult(t *testing.T) {
	memory, _ := timelineFixture(t, nil)
	result, err := memory.SearchMemory("不存在的记忆", 1000)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Memories)
	require.Empty(t, result.TotalContent)
	require.Zero(t, result.ContentTokens)
}

func TestSearchMemory_TimestampPresence(t *testing.T) {
	memory, _ := timelineFixture(t, nil)
	before := time.Now()
	require.NoError(t, memory.HandleMemory("Go并发编程与测试"))
	after := time.Now()
	result, err := memory.SearchMemory("Go", 5000)
	require.NoError(t, err)
	require.NotEmpty(t, result.Memories, "HandleMemory must really save searchable data")
	for _, entry := range result.Memories {
		require.False(t, entry.CreatedAt.IsZero())
		require.False(t, entry.CreatedAt.Before(before))
		require.False(t, entry.CreatedAt.After(after))
	}
}
