package loop_dir_explore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

// Run note writes on disk while keeping model calls deterministic.
type exploreOutputRuntime struct{ *mock.MockInvoker }

func (r *exploreOutputRuntime) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, name string, params aitool.InvokeParams, opts ...aicommon.ToolCallerOption) (*aitool.ToolResult, bool, error) {
	if name != "write_file" {
		return r.MockInvoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, name, params, opts...)
	}
	err := os.WriteFile(params.GetString("file"), []byte(params.GetString("content")), 0o600)
	return &aitool.ToolResult{Success: err == nil, Data: "笔记已写入"}, false, err
}

func TestDirExploreOutputLifecycle(t *testing.T) {
	for _, outcome := range []string{"success", "missing_report", "empty_report", "cancelled"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			workdir := t.TempDir()
			reportPath := filepath.Join(workdir, "final.md")
			if outcome == "empty_report" {
				require.NoError(t, os.WriteFile(reportPath, nil, 0o600))
			}
			notePath := filepath.Join(workdir, "dir_structure.md")
			projectOverview := "这是一个用于提供账户服务的 Go 项目。"
			report := "# demo 项目探索报告\n\n" + projectOverview + "\n\n## 目录结构\n\n```text\ndemo/\n  cmd/\n  internal/\n```\n\n## 技术栈\n\nGo, SQLite\n\n## 入口\n\ncmd/server/main.go\n\n## 阅读建议\n\n阅读 `x{NNN}_*_test.go`。\n"
			noteJSON, err := json.Marshal(map[string]any{
				"@action": "write_file", "identifier": "write_dirs", "human_readable_thought": "内部：写探索笔记", "file": notePath,
				"content": "# 目录结构\n\ndemo/\n  cmd/\n  internal/\n", "force": true,
			})
			require.NoError(t, err)
			completeJSON, err := json.Marshal(map[string]any{
				"@action": "complete_explore", "identifier": "complete_project", "human_readable_thought": "内部：启动报告子流程",
				"project_name": "demo", "project_overview": projectOverview, "tech_stack": "Go, SQLite",
				"entry_points": "cmd/server/main.go（账户服务）", "modules_summary": "internal/accounts（账户逻辑）",
				"reading_guide": "独立概览：先读 cmd/server/main.go，再读 x{NNN}_*_test.go。",
			})
			require.NoError(t, err)
			referenceJSON, err := json.Marshal(map[string]any{"@action": "read_reference_file", "identifier": "read_dirs", "file_path": notePath})
			require.NoError(t, err)
			responses := []string{
				string(noteJSON), string(noteJSON), string(completeJSON),
				string(referenceJSON) + "\n<|GEN_REPORT_CURRENT_NONCE|>placeholder<|GEN_REPORT_END_CURRENT_NONCE|>",
			}
			if outcome != "missing_report" && outcome != "empty_report" {
				responses = append(responses, `{"@action":"write_section","identifier":"write_report","human_readable_thought":"内部：撰写报告"}`+
					"\n<|GEN_REPORT_CURRENT_NONCE|>"+report+"<|GEN_REPORT_END_CURRENT_NONCE|>")
			}
			responses = append(responses, `{"@action":"finish","identifier":"finish_report"}`)
			var calls int
			var mu sync.Mutex
			var events []*schema.AiOutputEvent
			writeTool := aitool.NewWithoutCallback("write_file",
				aitool.WithStringParam("file", aitool.WithParam_Required(true)),
				aitool.WithStringParam("content", aitool.WithParam_Required(true)),
				aitool.WithBoolParam("force"))
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithTools(writeTool),
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(workdir),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if outcome == "cancelled" && calls == len(responses)-1 {
						cancel()
						return nil, ctx.Err()
					}
					if calls >= len(responses) {
						return nil, context.Canceled
					}
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(responses[calls]))
					calls++
					response.Close()
					return response, nil
				}),
				aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
					mu.Lock()
					defer mu.Unlock()
					if e.Type == schema.EVENT_TYPE_REPORT_FINISH || e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME && e.GetContentJSONPath("$.path") == reportPath {
						written, readErr := os.ReadFile(reportPath)
						if outcome != "success" || readErr != nil || strings.TrimSpace(string(written)) != strings.TrimSpace(report) {
							t.Errorf("published an incomplete report: %v", readErr)
						}
					}
					events = append(events, e)
				}),
			)
			inv := &exploreOutputRuntime{mock.NewMockInvoker(ctx)}
			inv.SetConfig(cfg)
			loop, err := BuildDirExploreLoop(inv,
				reactloops.WithVar("target_path", workdir), reactloops.WithVar("output_report_path", reportPath),
				reactloops.WithFunctionCallMode(false), reactloops.WithDisableLoopPerception(true),
				reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableIncreaseIteration(true), reactloops.WithMaxIterations(8),
			)
			require.NoError(t, err)
			task := aicommon.NewStatefulTaskBase("explore-task", "帮我探索这个项目", ctx, cfg.GetEmitter(), true)
			inv.SetCurrentTask(task)
			err = loop.ExecuteWithExistedTask(task)
			cfg.GetEmitter().WaitForStream()
			if outcome == "success" {
				require.NoError(t, err)
				require.Equal(t, aicommon.AITaskState_Completed, task.GetStatus())
				require.Equal(t, reportPath, loop.Get("result_report_path"))
				require.Contains(t, task.GetResult(), projectOverview)
				require.Contains(t, task.GetResult(), "internal/accounts")
			} else {
				require.Error(t, err)
				require.Empty(t, loop.Get("result_report_path"))
				require.NotContains(t, task.GetResult(), "报告已生成")
				require.NotContains(t, task.GetResult(), "见报告文件")
			}
			mu.Lock()
			defer mu.Unlock()
			var phases []string
			var finishes, reportPins, noteMilestones int
			var fullReference, failureResult bool
			reportingIndex, finishIndex, completedIndex := -1, -1, -1
			for i, e := range events {
				require.NotEqual(t, "report-content", e.NodeId)
				require.NotEqual(t, "report-read-reference", e.NodeId)
				require.NotEqual(t, "re-act-loop-thought", e.NodeId)
				require.NotEqual(t, "infra-code-verify", e.NodeId)
				if e.Type == schema.EVENT_TYPE_STREAM {
					require.NotContains(t, string(e.StreamDelta), "placeholder")
				}
				if e.NodeId == "status" {
					var status aicommon.StatusPayload
					require.NoError(t, json.Unmarshal(e.Content, &status))
					code := status.Code
					if strings.HasPrefix(code, "dir_explore.") {
						phases = append(phases, code)
					}
					if code == "dir_explore.reporting" {
						reportingIndex = i
					}
					if code == "dir_explore.completed" {
						completedIndex = i
					}
				}
				if e.Type == schema.EVENT_TYPE_STREAM_START && e.NodeId == "dir-explore-progress" {
					// Two writes of the same note should produce only one milestone.
					// The target, reporting and terminal phase each produce their own stream.
					noteMilestones++
				}
				if e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME && e.GetContentJSONPath("$.path") == reportPath {
					reportPins++
				}
				if e.Type == schema.EVENT_TYPE_REPORT_FINISH {
					finishes++
					finishIndex = i
					saved, readErr := os.ReadFile(reportPath)
					require.NoError(t, readErr)
					require.Equal(t, string(saved), e.GetContentJSONPath("$.summary_markdown"), "the completed card must match the saved report, including Markdown formatting")
					require.NotContains(t, e.GetContentJSONPath("$.summary_markdown"), "独立概览")
				}
				if e.Type == schema.EVENT_TYPE_REFERENCE_MATERIAL && strings.TrimSpace(e.GetContentJSONPath("$.payload")) == strings.TrimSpace(report) {
					fullReference = true
				}
				if e.Type == schema.EVENT_TYPE_RESULT && e.GetContentJSONPath("$.after_stream") == "false" {
					require.Contains(t, e.GetContentJSONPath("$.result"), projectOverview)
					failureResult = true
				}
			}
			expectedMilestones := 4
			if outcome == "cancelled" || outcome == "success" {
				expectedMilestones = 3
			} // Stream producers stop with their context.
			require.Equal(t, expectedMilestones, noteMilestones, "duplicate writes must not repeat a committed-note milestone")
			if outcome == "success" {
				require.Equal(t, []string{"dir_explore.exploring", "dir_explore.exploring", "dir_explore.reporting", "dir_explore.completed"}, phases)
				require.Equal(t, 1, finishes)
				require.Equal(t, 1, reportPins)
				require.True(t, fullReference, "the reference viewer must open the complete report")
				require.Greater(t, finishIndex, reportingIndex)
				require.Greater(t, completedIndex, finishIndex)
			} else {
				require.Zero(t, finishes)
				require.Zero(t, reportPins)
				require.Equal(t, -1, completedIndex)
				if outcome == "cancelled" {
					require.Equal(t, "cancelled", loop.Get("explore_phase"))
					require.False(t, failureResult)
				} else {
					require.Equal(t, "dir_explore.failed", phases[len(phases)-1])
					require.True(t, failureResult, "failed report generation must leave a visible explanation")
				}
			}
		})
	}
}
