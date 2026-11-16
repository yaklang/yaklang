package sessionruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	_ "github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Only model responses are scripted. Session binding, Git checkout, input delivery,
// prompt assembly, tool dispatch and the built-in write_file tool run normally.
func TestCodingRuntimeFreeInputWritesOnlyManagedWorktree(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "textstream", true: "functioncall"}[native], func(t *testing.T) { testCodingRuntimeFreeInput(t, native) })
	}
}

func testCodingRuntimeFreeInput(t *testing.T, native bool) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	profile, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer profile.Close()
	project, err := utils.CreateTempTestDatabaseInMemory()
	require.NoError(t, err)
	defer project.Close()
	require.NoError(t, profile.AutoMigrate(&schema.AICodingWorkspace{}, &schema.AICodingSession{}, &schema.AICodingWorktree{}).Error)
	require.NoError(t, project.AutoMigrate(&schema.AISession{}).Error)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	source := t.TempDir()
	secondary := t.TempDir()
	filename := "hello.txt"
	require.NoError(t, os.WriteFile(filepath.Join(source, filename), []byte("committed\n"), 0600))
	for _, args := range [][]string{{"init"}, {"add", filename}, {"-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-m", "initial"}} {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", source}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	require.NoError(t, os.WriteFile(filepath.Join(source, filename), []byte("user's uncommitted change\n"), 0600))
	workspace, err := yakit.CreateAICodingWorkspace(ctx, profile, "vibe coding", source, secondary)
	require.NoError(t, err)
	script, err := yakscripttools.GetEmbedFS().ReadFile("yakscriptforai/fs/write_file.yak")
	require.NoError(t, err)
	toolDefinition := yakscripttools.LoadYakScriptToAiTools("write_file", string(script))
	require.NotNil(t, toolDefinition)
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{toolDefinition})
	require.Len(t, tools, 1)
	manager, _ := aicommon.NewMockEKManagerAndToken()
	runtime := New(func() *gorm.DB { return project }, func() *gorm.DB { return profile }).(*reActSessionRuntime)
	runtime.newReAct = aireact.NewTestReAct
	id := "coding-input-" + uuid.NewString()
	toolDone := make(chan struct{}, 1)
	modelErrors := make(chan error, 1)
	var cfg *aicommon.Config
	var exportOnce sync.Once
	var exportErr error
	connection, err := runtime.connectWithOptions(ctx, ConnectRequest{StartParams: &ypb.AIStartParams{
		TimelineSessionID: id, AgentMode: ypb.AgentModeCoding, CodingWorkspaceID: workspace.WorkspaceID,
		CodingCreateWorktree: true, DisableAISearchForge: true,
	}}, func(event *schema.AiOutputEvent) error {
		if event.Type == schema.EVENT_TOOL_CALL_DONE {
			select {
			case toolDone <- struct{}{}:
			default:
			}
		}
		return nil
	}, false, aicommon.WithEnhanceKnowledgeManager(manager), aicommon.WithTools(tools...),
		aicommon.WithEnableFunctionCallMode(native), aicommon.WithSingleAIModelMode(true), func(config *aicommon.Config) error { cfg = config; return nil },
		aicommon.WithAgreeYOLO(true), aicommon.WithEnablePETaskAnalyze(false),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := request.GetPrompt()
			wire := aispec.NewDefaultAIConfig(request.GetExtraSpecOpts()...)
			action := map[string]any{"@action": "finish", "human_readable_thought": "file updated"}
			if aicommon.IsPrimaryDecisionPrompt(prompt) {
				if cfg.AgentMode != ypb.AgentModeCoding || cfg.CodingWorktreeID == "" ||
					cfg.CodingBaseRef != "HEAD" || cfg.CodingBaseCommit == "" ||
					!strings.Contains(prompt, cfg.CodingBaseCommit) ||
					!strings.Contains(prompt, cfg.CodingWorkingDirectory) ||
					!strings.Contains(prompt, "Coding Agent 工作方法") ||
					!strings.Contains(prompt, "# Coding 主循环") ||
					(!native && !strings.Contains(prompt, "编程任务决策示例")) ||
					strings.Contains(prompt, "用union注入探测字段数") ||
					strings.Contains(prompt, "重放漏洞的关联假设") ||
					!strings.Contains(prompt, "detached HEAD") ||
					!strings.Contains(prompt, workspace.RootPaths[1]) {
					err := fmt.Errorf("free input lost coding mode or worktree prompt context")
					select {
					case modelErrors <- err:
					default:
					}
					return nil, err
				}
				exportOnce.Do(func() { exportErr = exportCodingPromptExample(native, prompt, wire.Tools) })
				if exportErr != nil {
					select {
					case modelErrors <- exportErr:
					default:
					}
					return nil, exportErr
				}
				target := filepath.Join(cfg.CodingWorkingDirectory, filename)
				data, _ := os.ReadFile(target)
				if string(data) != "updated in worktree\n" {
					action = map[string]any{"@action": "directly_call_tool", "directly_call_tool_name": "write_file",
						"directly_call_tool_params": map[string]any{"file": target, "content": "updated in worktree\n", "force": true},
						"human_readable_thought":    "edit the isolated checkout"}
				}
			} else if aicommon.IsVerifySatisfactionPrompt(prompt) {
				action = map[string]any{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "file written"}
			} else if aicommon.IsDirectAnswerPrompt(prompt) {
				action = map[string]any{"@action": "directly_answer", "answer_payload": "file written"}
			} else {
				return nil, fmt.Errorf("unexpected auxiliary model request")
			}
			actionName := action["@action"].(string)
			if wire.ToolCallCallback != nil {
				delete(action, "@action")
			}
			payload, err := json.Marshal(action)
			if err != nil {
				return nil, err
			}
			response := config.NewAIResponse()
			if wire.ToolCallCallback != nil {
				wire.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: uuid.NewString(), Type: "function", Function: aispec.FuncReturn{Name: actionName, Arguments: string(payload)}}})
				if wire.FinishReasonCallback != nil {
					wire.FinishReasonCallback("tool_calls", nil)
				}
			} else {
				response.EmitOutputStream(strings.NewReader(string(payload)))
			}
			response.Close()
			return response, nil
		}))
	require.NoError(t, err)
	defer connection.Close()
	defer cancel()
	require.NoError(t, connection.Send(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "Update hello.txt in this project"}))
	select {
	case <-toolDone:
	case err := <-modelErrors:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal("free input did not complete the file tool: ", ctx.Err())
	}
	saved, err := yakit.GetAISessionMetaStartParamsBySessionID(project, id)
	require.NoError(t, err)
	require.False(t, saved.CodingCreateWorktree)
	require.NotEmpty(t, saved.CodingWorktreeID)
	require.Equal(t, workspace.RootPaths[1], saved.CodingWorkspaceRoots[1])
	written, err := os.ReadFile(filepath.Join(saved.CodingWorkingDirectory, filename))
	require.NoError(t, err)
	require.Equal(t, "updated in worktree\n", string(written))
	original, err := os.ReadFile(filepath.Join(source, filename))
	require.NoError(t, err)
	require.Equal(t, "user's uncommitted change\n", string(original))
	require.NoFileExists(t, filepath.Join(secondary, filename))
}

// Optional inspection artifact, generated from the same request exercised by the
// integration test. Normal test runs do not write outside their temporary fixtures.
func exportCodingPromptExample(native bool, prompt string, tools []aispec.Tool) error {
	projected := aiprojection.Project(aiprojection.ProjectionInput{Prompt: prompt, ActionTools: tools})
	if !projected.Metadata.CacheProjected {
		return fmt.Errorf("coding prompt did not project cache sections")
	}
	if native && len(projected.Tools) == 0 {
		return fmt.Errorf("native coding prompt lost action schemas")
	}
	dir := os.Getenv("CODING_PROMPT_EXPORT_DIR")
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	name := map[bool]string{false: "coding-textstream", true: "coding-functioncall"}[native]
	if err := os.WriteFile(filepath.Join(dir, name+".txt"), []byte(aiprojection.RedactNonce(prompt)), 0600); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projected, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name+"-messages.json"), data, 0600)
}
