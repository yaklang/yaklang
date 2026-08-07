package aireact

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	aicommonmock "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type mockMidtermArchiveStore struct {
	result        *aicommon.TimelineArchiveSearchResult
	results       map[string]*aicommon.TimelineArchiveSearchResult
	queries       []string
	searchQueries []*aicommon.TimelineArchiveSearchQuery
}

func (m *mockMidtermArchiveStore) ArchiveCompressedBatch(context.Context, *aicommon.TimelineArchiveBatch) (*aicommon.TimelineArchiveRef, error) {
	return nil, nil
}

func (m *mockMidtermArchiveStore) SearchArchivedBatches(ctx context.Context, query *aicommon.TimelineArchiveSearchQuery) (*aicommon.TimelineArchiveSearchResult, error) {
	_ = ctx
	if query != nil {
		m.queries = append(m.queries, query.Query)
		cloned := *query
		m.searchQueries = append(m.searchQueries, &cloned)
		if m.results != nil {
			if result, ok := m.results[query.Query]; ok {
				return result, nil
			}
		}
	}
	if m.result == nil {
		return &aicommon.TimelineArchiveSearchResult{}, nil
	}
	return m.result, nil
}

type midtermQueryTestTask struct {
	*aicommonmock.MockStatefulTask
}

func newMidtermQueryTestTask(id, index, name, userInput, origin, summary string, info *aicommon.AITaskRetrievalInfo) *midtermQueryTestTask {
	task := &midtermQueryTestTask{
		MockStatefulTask: aicommonmock.NewMockStatefulTask(context.Background(), id, userInput),
	}
	task.SetIndex(index)
	task.SetName(name)
	task.SetOriginUserInput(origin)
	task.SetSummary(summary)
	task.SetSemanticIdentifier("midterm_query_task")
	task.SetTaskRetrievalInfo(info)
	task.SetStatus(aicommon.AITaskState_Processing)
	return task
}

func TestConsumeAndSearchMidtermMemory_NoPendingSnapshot(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	cfg.TimelineArchiveStore = &mockMidtermArchiveStore{}

	react := &ReAct{config: cfg}
	react.setCurrentTask(newMidtermQueryTestTask(
		"task-1", "1-2", "verify http flow",
		"focus on malformed headers",
		"collect and verify malformed header behavior",
		"need reproduce with retry",
		nil,
	))

	// No pending perception snapshot → empty result, no searches.
	result := react.ConsumeAndSearchMidtermMemory()
	require.Empty(t, result)
}

func TestConsumeAndSearchMidtermMemory_UsesPendingPerceptionSnapshot(t *testing.T) {
	store := &mockMidtermArchiveStore{
		results: map[string]*aicommon.TimelineArchiveSearchResult{
			"perception summary about malformed headers": {
				TotalContent: "important archived clue\nsecond line",
			},
			"http fuzzing malformed headers": {
				TotalContent: "topic based memory",
			},
			"header": {
				TotalContent: "keyword based memory from header",
			},
			"malformed": {
				TotalContent: "keyword based memory from malformed",
			},
		},
	}
	cfg := aicommon.NewConfig(context.Background())
	cfg.TimelineArchiveStore = store

	react := &ReAct{config: cfg}
	react.setCurrentTask(newMidtermQueryTestTask(
		"task-1", "1-2", "verify http flow",
		"focus on malformed headers",
		"collect and verify malformed header behavior",
		"need reproduce with retry",
		nil,
	))

	// Before scheduling: empty.
	require.Empty(t, react.ConsumeAndSearchMidtermMemory())

	react.ScheduleMidtermTimelineRecallFromPerception(
		"perception summary about malformed headers",
		[]string{"http fuzzing", "malformed headers"},
		[]string{"header", "malformed"},
	)
	result := react.ConsumeAndSearchMidtermMemory()

	require.Contains(t, result, "midterm-memory:")
	require.Contains(t, result, "search-queries:")
	require.Contains(t, result, "perception summary about malformed headers")
	require.Contains(t, result, "http fuzzing malformed headers")
	require.Contains(t, result, "header")
	require.Contains(t, result, "malformed")
	require.Contains(t, result, "topic based memory")
	require.Contains(t, result, "keyword based memory from header")
	require.Contains(t, result, "keyword based memory from malformed")
	require.Contains(t, result, "important archived clue")
	require.Equal(t, []string{
		"perception summary about malformed headers",
		"http fuzzing malformed headers",
		"header",
		"malformed",
	}, store.queries)
	require.Len(t, store.searchQueries, 4)
	require.False(t, store.searchQueries[0].DisableSemanticSearch)
	require.False(t, store.searchQueries[1].DisableSemanticSearch)
	require.True(t, store.searchQueries[2].DisableSemanticSearch)
	require.True(t, store.searchQueries[3].DisableSemanticSearch)

	// Consume-once: second call returns empty.
	require.Empty(t, react.ConsumeAndSearchMidtermMemory())
	require.Len(t, store.queries, 4)
}

func TestConsumeAndSearchMidtermMemory_NoArchiveStore(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	// No TimelineArchiveStore set.

	react := &ReAct{config: cfg}
	react.ScheduleMidtermTimelineRecallFromPerception("summary", nil, nil)
	result := react.ConsumeAndSearchMidtermMemory()
	require.Empty(t, result)
}

func TestConsumeAndSearchMidtermMemory_EmptySnapshot(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	cfg.TimelineArchiveStore = &mockMidtermArchiveStore{}

	react := &ReAct{config: cfg}
	// All empty → pending flag cleared, no search.
	react.ScheduleMidtermTimelineRecallFromPerception("", nil, nil)
	require.Empty(t, react.ConsumeAndSearchMidtermMemory())
}

func TestScheduleMidtermTimelineRecallFromPerception_DeduplicatesTopicsAndKeywords(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	store := &mockMidtermArchiveStore{
		results: map[string]*aicommon.TimelineArchiveSearchResult{
			"summary":       {TotalContent: "summary memory"},
			"topic1 topic2": {TotalContent: "topic memory"},
			"keyword1":      {TotalContent: "keyword1 memory"},
		},
	}
	cfg.TimelineArchiveStore = store

	react := &ReAct{config: cfg}
	react.ScheduleMidtermTimelineRecallFromPerception(
		"summary",
		[]string{"topic1", "topic2", "topic1"}, // dedup
		[]string{"keyword1", "keyword1"},       // dedup
	)
	result := react.ConsumeAndSearchMidtermMemory()

	require.Contains(t, result, "summary memory")
	require.Contains(t, result, "topic memory")
	require.Contains(t, result, "keyword1 memory")
	// topic1 appears only once in "topic1 topic2" query
	require.Equal(t, []string{"summary", "topic1 topic2", "keyword1"}, store.queries)
}
