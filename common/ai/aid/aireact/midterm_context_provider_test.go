package aireact

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

type mockMidtermArchiveStore struct {
	result *aicommon.TimelineArchiveSearchResult
}

func (m *mockMidtermArchiveStore) ArchiveCompressedBatch(context.Context, *aicommon.TimelineArchiveBatch) (*aicommon.TimelineArchiveRef, error) {
	return nil, nil
}

func (m *mockMidtermArchiveStore) SearchArchivedBatches(context.Context, *aicommon.TimelineArchiveSearchQuery) (*aicommon.TimelineArchiveSearchResult, error) {
	if m.result == nil {
		return &aicommon.TimelineArchiveSearchResult{}, nil
	}
	return m.result, nil
}

type midtermQueryTestTask struct {
	id        string
	index     string
	name      string
	userInput string
	origin    string
	summary   string
	info      *aicommon.AITaskRetrievalInfo
}

func (m *midtermQueryTestTask) GetIndex() string                            { return m.index }
func (m *midtermQueryTestTask) GetName() string                             { return m.name }
func (m *midtermQueryTestTask) GetSemanticIdentifier() string               { return "midterm_query_task" }
func (m *midtermQueryTestTask) SetSemanticIdentifier(string)                {}
func (m *midtermQueryTestTask) PushToolCallResult(*aitool.ToolResult)       {}
func (m *midtermQueryTestTask) GetAllToolCallResults() []*aitool.ToolResult { return nil }
func (m *midtermQueryTestTask) GetSummary() string                          { return m.summary }
func (m *midtermQueryTestTask) SetSummary(summary string)                   { m.summary = summary }
func (m *midtermQueryTestTask) GetId() string                               { return m.id }
func (m *midtermQueryTestTask) GetTaskRetrievalInfo() *aicommon.AITaskRetrievalInfo {
	return m.info.Clone()
}
func (m *midtermQueryTestTask) SetTaskRetrievalInfo(info *aicommon.AITaskRetrievalInfo) {
	m.info = info.Clone()
}
func (m *midtermQueryTestTask) SetAsyncDeferCallback(func(error))              {}
func (m *midtermQueryTestTask) CallAsyncDeferCallback(error)                   {}
func (m *midtermQueryTestTask) SetResult(string)                               {}
func (m *midtermQueryTestTask) GetResult() string                              { return "" }
func (m *midtermQueryTestTask) GetContext() context.Context                    { return context.Background() }
func (m *midtermQueryTestTask) Cancel()                                        {}
func (m *midtermQueryTestTask) IsFinished() bool                               { return false }
func (m *midtermQueryTestTask) GetUserInput() string                           { return m.userInput }
func (m *midtermQueryTestTask) GetOriginUserInput() string                     { return m.origin }
func (m *midtermQueryTestTask) SetUserInput(input string)                      { m.userInput = input }
func (m *midtermQueryTestTask) SetAttachedDatas([]*aicommon.AttachedResource)  {}
func (m *midtermQueryTestTask) GetAttachedDatas() []*aicommon.AttachedResource { return nil }
func (m *midtermQueryTestTask) GetStatus() aicommon.AITaskState {
	return aicommon.AITaskState_Processing
}
func (m *midtermQueryTestTask) SetStatus(aicommon.AITaskState)     {}
func (m *midtermQueryTestTask) AppendErrorToResult(error)          {}
func (m *midtermQueryTestTask) GetCreatedAt() time.Time            { return time.Now() }
func (m *midtermQueryTestTask) Finish(error)                       {}
func (m *midtermQueryTestTask) SetAsyncMode(bool)                  {}
func (m *midtermQueryTestTask) IsAsyncMode() bool                  { return false }
func (m *midtermQueryTestTask) GetEmitter() *aicommon.Emitter      { return nil }
func (m *midtermQueryTestTask) SetEmitter(*aicommon.Emitter)       {}
func (m *midtermQueryTestTask) SetReActLoop(aicommon.ReActLoopIF)  {}
func (m *midtermQueryTestTask) GetReActLoop() aicommon.ReActLoopIF { return nil }
func (m *midtermQueryTestTask) SetDB(*gorm.DB)                     {}
func (m *midtermQueryTestTask) GetRisks() []*schema.Risk           { return nil }
func (m *midtermQueryTestTask) GetUUID() string                    { return m.id }
func (m *midtermQueryTestTask) GetFocusMode() string               { return "" }
func (m *midtermQueryTestTask) SetFocusMode(string)                {}

func TestBuildMidtermRecallQuery_IncludesCurrentTaskDetails(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	cfg.SetUserInputHistory([]schema.AIAgentUserInputRecord{{
		Round:     1,
		Timestamp: time.Now(),
		UserInput: "session level request",
	}})

	react := &ReAct{config: cfg}
	react.setCurrentTask(&midtermQueryTestTask{
		id:        "task-1",
		index:     "1-2",
		name:      "verify http flow",
		userInput: "focus on malformed headers",
		origin:    "collect and verify malformed header behavior",
		summary:   "need reproduce with retry",
		info: &aicommon.AITaskRetrievalInfo{
			Target:    "http fuzz regression",
			Questions: []string{"which malformed headers failed"},
			Tags:      []string{"http", "fuzz"},
		},
	})

	query := buildMidtermRecallQuery(react)

	for _, expected := range []string{
		"1-2",
		"verify http flow",
		"collect and verify malformed header behavior",
		"focus on malformed headers",
		"need reproduce with retry",
		"http fuzz regression",
		"which malformed headers failed",
		"http",
		"fuzz",
		"session level request",
	} {
		if !strings.Contains(query, expected) {
			t.Fatalf("expected query to contain %q, got: %s", expected, query)
		}
	}
}

func TestBuildTimelineDumpWithMidtermMemory_PrependsMidtermMemory(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background())
	cfg.TimelineArchiveStore = &mockMidtermArchiveStore{
		result: &aicommon.TimelineArchiveSearchResult{
			TotalContent: "important archived clue\nsecond line",
		},
	}

	timeline := aicommon.NewTimeline(nil, nil)
	timeline.PushText(1, "live timeline item")
	cfg.Timeline = timeline

	react := &ReAct{config: cfg}
	react.setCurrentTask(&midtermQueryTestTask{
		id:        "task-1",
		index:     "1-2",
		name:      "verify http flow",
		userInput: "focus on malformed headers",
		origin:    "collect and verify malformed header behavior",
		summary:   "need reproduce with retry",
	})

	dump := buildTimelineDumpWithMidtermMemory(react, timeline)

	require.True(t, strings.HasPrefix(dump, "timeline:\n--["))
	require.Contains(t, dump, "midterm-memory:")
	require.Contains(t, dump, "search-query:")
	require.Contains(t, dump, "important archived clue")
	require.Contains(t, dump, "live timeline item")
	require.Less(t, strings.Index(dump, "midterm-memory:"), strings.Index(dump, "live timeline item"))
}
