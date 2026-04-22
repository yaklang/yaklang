package reactloops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	mockcfg "github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

type workspaceDebugTestInvoker struct {
	*mockcfg.MockInvoker
	cfg aicommon.AICallerConfigIf
}

func (i *workspaceDebugTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	return i.cfg
}

func TestAIWorkspaceDebugFlagFromEnv(t *testing.T) {
	t.Setenv(envAIWorkspaceDebugPrimary, "true")
	require.True(t, IsAIWorkspaceDebugEnabled())

	t.Setenv(envAIWorkspaceDebugPrimary, "false")
	t.Setenv(envAIWorkspaceDebugSecondary, "true")
	require.False(t, IsAIWorkspaceDebugEnabled(), "primary env should take precedence when explicitly set")

	t.Setenv(envAIWorkspaceDebugPrimary, "")
	t.Setenv(envAIWorkspaceDebugSecondary, "true")
	require.True(t, IsAIWorkspaceDebugEnabled())
}

func TestWriteIntentRecognitionDebugMarkdown(t *testing.T) {
	t.Setenv(envAIWorkspaceDebugPrimary, "true")

	workdir := t.TempDir()
	cfg := &aicommon.Config{Workdir: workdir}
	invoker := &workspaceDebugTestInvoker{
		MockInvoker: mockcfg.NewMockInvoker(context.Background()),
		cfg:         cfg,
	}

	loop := NewMinimalReActLoop(cfg, invoker)
	loop.loopName = "intent-debug-loop"
	task := aicommon.NewStatefulTaskBase("intent-task", "debug intent", context.Background(), nil, true)
	task.SetTaskRetrievalInfo(&aicommon.AITaskRetrievalInfo{
		Target:    "target-repo",
		Tags:      []string{"tag1", "tag2"},
		Questions: []string{"how to use tool1?"},
	})
	loop.SetCurrentTask(task)
	loop.Set("search_results", "### Matched Tools\n- tool1")
	loop.Set("matched_capabilities_details", `[{"capability_name":"tool1","capability_type":"tool","description":"demo tool"}]`)

	result := &DeepIntentResult{
		IntentAnalysis:    "用户想找合适的工具和蓝图",
		RecommendedTools:  "tool1",
		RecommendedForges: "forge1",
		ContextEnrichment: "### Recommended Capabilities\n- tool1",
		MatchedToolNames:  "tool1,tool2",
		MatchedForgeNames: "forge1",
		MatchedSkillNames: "skill1",
	}

	filePath := writeIntentRecognitionDebugMarkdown(invoker, loop, result)
	require.NotEmpty(t, filePath)
	require.FileExists(t, filePath)
	require.Contains(t, filePath, filepath.Join(workdir, "debug", "intent"))

	raw, err := os.ReadFile(filePath)
	require.NoError(t, err)
	content := string(raw)
	require.Contains(t, content, "# Intent Recognition Debug")
	require.Contains(t, content, result.IntentAnalysis)
	require.Contains(t, content, "tool1,tool2")
	require.Contains(t, content, "target-repo")
	require.Contains(t, content, "### Matched Tools")
}

func TestWritePerceptionDebugMarkdown(t *testing.T) {
	t.Setenv(envAIWorkspaceDebugPrimary, "true")

	workdir := t.TempDir()
	cfg := &aicommon.Config{Workdir: workdir}
	loop := NewMinimalReActLoop(cfg, nil)
	loop.loopName = "perception-debug-loop"

	state := &PerceptionState{
		OneLinerSummary: "用户正在排查 HTTP 请求问题",
		Topics:          []string{"http", "debugging"},
		Keywords:        []string{"header", "timeout"},
		Changed:         true,
		ConfidenceLevel: 0.88,
		Epoch:           3,
		LastTrigger:     PerceptionTriggerForced,
	}
	input := CapabilitySearchInput{
		Query:   "用户正在排查 HTTP 请求问题",
		Queries: []string{"http", "debugging", "header", "timeout"},
	}
	result := &CapabilitySearchResult{
		SearchResultsMarkdown:   "### Matched Tools\n- web_search",
		ContextEnrichment:       "### Recommended Capabilities\n- web_search",
		MatchedToolNames:        []string{"web_search"},
		MatchedForgeNames:       []string{"forge_http"},
		MatchedSkillNames:       []string{"skill_http"},
		MatchedFocusModeNames:   []string{"internet_research"},
		RecommendedCapabilities: []string{"web_search"},
	}

	filePath := writePerceptionDebugMarkdown(loop, state, input, result, nil)
	require.NotEmpty(t, filePath)
	require.FileExists(t, filePath)
	require.Contains(t, filePath, filepath.Join(workdir, "debug", "perception"))

	raw, err := os.ReadFile(filePath)
	require.NoError(t, err)
	content := string(raw)
	require.Contains(t, content, "# Perception Debug")
	require.Contains(t, content, state.OneLinerSummary)
	require.Contains(t, content, strings.Join(state.Topics, ", "))
	require.Contains(t, content, "web_search")
	require.Contains(t, content, "internet_research")
}
