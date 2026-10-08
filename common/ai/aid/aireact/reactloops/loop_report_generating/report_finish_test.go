package loop_report_generating

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	aicommon_testutil "github.com/yaklang/yaklang/common/ai/aid/aicommon/testutil"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/schema"
)

// Read one byte at a time, including inside UTF-8 characters and tag delimiters.
// Do not embed strings.Reader: its WriteTo would bypass fragmentation in io.Copy.
type fragmentedReportReader struct {
	reader *strings.Reader
}

func (r *fragmentedReportReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.reader.Read(p)
}

func TestReportOutputFilterPreservesAITagEdits(t *testing.T) {
	for _, internal := range []bool{false, true} {
		for _, literalNonce := range []bool{false, true} {
			t.Run(fmt.Sprintf("internal=%t/literal_nonce=%t", internal, literalNonce), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				workdir := t.TempDir()
				reportPath := filepath.Join(workdir, "report.md")
				initial := "# 格式回归报告\n\n原始说明。\n\n" +
					"```go\nraw := `第一行\n第二行\\n仍是字面量`\nquoted := \"引号\\\"与反斜杠\\\\\"\n```\n\n" +
					"```php\r\n$text = <<<'TEXT'\r\n\t中文 😀 / 'single' / \"double\" / \\n / \\t\r\nTEXT;\r\n```\n\n" +
					"| 字段 | 值 |\n| --- | --- |\n| 路径 | C:\\data\\report |\n\n" +
					strings.Repeat("保留行尾空格与缩进：  \n\t中文 😀、引号\"、反斜杠\\。\n\n", 100) +
					"## 结束\n"
				replacement := "更新说明：保留 `backtick`、\"双引号\"、'单引号'、\\n 字面量。\n\t第二行 😀。"
				inserted := "## 附加资料\n\n```text\n\t字面量 \\n 与真实换行\n引号 \" 和反斜杠 \\ 😀\n```\n\n"
				modified := strings.Replace(initial, "原始说明。", replacement, 1)
				final := "# 格式回归报告\n\n" + inserted + strings.TrimPrefix(modified, "# 格式回归报告\n\n")
				steps := []struct {
					action map[string]any
					body   string
					saved  string
				}{
					{map[string]any{"@action": "write_section"}, initial, initial},
					{map[string]any{"@action": "modify_section", "old_snippet": "原始说明。"}, replacement, modified},
					{map[string]any{"@action": "insert_section", "insert_line": 3}, inserted, final},
					{map[string]any{"@action": "finish"}, "", final},
				}
				var calls int
				var loop *reactloops.ReActLoop
				var mu sync.Mutex
				var events []*schema.AiOutputEvent
				cfg := aicommon.NewConfig(ctx,
					aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(workdir),
					aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if calls >= len(steps) {
							return nil, fmt.Errorf("unexpected AI call after report finish")
						}
						// The next turn must see the preceding edit completely parsed and saved.
						if calls > 0 {
							previous := steps[calls-1]
							written, err := os.ReadFile(reportPath)
							require.NoError(t, err)
							require.Equal(t, previous.saved, string(written))
							require.Equal(t, previous.saved, loop.Get("full_report_code"))
							require.Equal(t, previous.body, loop.Get("report_content"), "filtered streams must still complete the field callback")
						}
						step := steps[calls]
						calls++
						encoded, err := json.Marshal(step.action)
						require.NoError(t, err)
						raw := string(encoded)
						if step.body != "" {
							nonce := aicommon_testutil.MustExtractDynamicSectionNonce(t, request.GetPrompt())
							if literalNonce {
								nonce = aicommon.LiteralCurrentNoncePlaceholder
							}
							// AI Tag block formatting strips one framing newline at
							// either delimiter. Additional body newlines must survive.
							raw += "\n<|GEN_REPORT_" + nonce + "|>\n" + step.body + "\n<|GEN_REPORT_END_" + nonce + "|>"
						}
						response := c.NewAIResponse()
						response.EmitOutputStream(&fragmentedReportReader{reader: strings.NewReader(raw)})
						response.Close()
						return response, nil
					}),
					aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
						mu.Lock()
						defer mu.Unlock()
						events = append(events, e)
					}),
				)
				inv := mock.NewMockInvoker(ctx)
				inv.SetConfig(cfg)
				opts := []reactloops.ReActLoopOption{
					reactloops.WithAllowToolCall(false), reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
					reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
					reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true),
					reactloops.WithDisableIncreaseIteration(true), reactloops.WithMaxIterations(len(steps) + 1),
					reactloops.WithInitTask(func(l *reactloops.ReActLoop, _ aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
						l.Set("report_filename", reportPath)
						l.Set("user_requirements", "保留原始格式，完成写入、修改和插入")
						op.Continue()
					}),
				}
				if internal {
					opts = append(opts, WithInternalReportOutput())
				}
				var err error
				loop, err = reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_REPORT_GENERATING, inv, opts...)
				require.NoError(t, err)
				task := aicommon.NewStatefulTaskBase("report-tag-test", "生成格式回归报告", ctx, cfg.GetEmitter(), true)
				inv.SetCurrentTask(task)
				require.NoError(t, loop.ExecuteWithExistedTask(task))
				cfg.GetEmitter().WaitForStream()
				require.Equal(t, len(steps), calls)
				written, err := os.ReadFile(reportPath)
				require.NoError(t, err)
				require.Equal(t, final, string(written))
				require.Equal(t, final, loop.Get("full_report_code"))
				require.Equal(t, "true", loop.Get("report_finished"))
				mu.Lock()
				defer mu.Unlock()
				for _, event := range events {
					require.NotEqual(t, "report-content", event.NodeId)
					if event.NodeId == "stream-finished" {
						require.NotEqual(t, "report-content", event.GetContentJSONPath("$.node_id"))
					}
				}
			})
		}
	}
}

func TestReportGeneratingOutputLifecycle(t *testing.T) {
	for _, internal := range []bool{false, true} {
		name := "standalone"
		if internal {
			name = "internal"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			workdir := t.TempDir()
			referencePath := filepath.Join(workdir, "reference.md")
			reportPath := filepath.Join(workdir, "report.md")
			overview := "这是一个 Go 项目，提供多个服务入口。"
			report := "# 项目探索报告\n\n## 项目概览\n\n" + overview +
				"\n\n## 目录结构\n\n```text\n" + strings.Repeat("internal/module/\n", 200) +
				"```\n\n## 关键配置\n\n| 文件 | 说明 |\n| --- | --- |\n| go.mod | 依赖声明 |"
			require.NoError(t, os.WriteFile(referencePath, []byte("# 参考资料\n项目使用 Go。"), 0o600))
			responses := []string{
				`{"@action":"read_reference_file","identifier":"read_reference","file_path":"` + filepath.ToSlash(referencePath) + `"}` +
					"\n<|GEN_REPORT_CURRENT_NONCE|>placeholder<|GEN_REPORT_END_CURRENT_NONCE|>",
				`{"@action":"write_section","identifier":"write_report"}` +
					"\n<|GEN_REPORT_CURRENT_NONCE|>" + report + "<|GEN_REPORT_END_CURRENT_NONCE|>",
				`{"@action":"finish","identifier":"finish_report"}` +
					"\n<|GEN_REPORT_CURRENT_NONCE|>placeholder<|GEN_REPORT_END_CURRENT_NONCE|>",
			}
			var calls int
			var mu sync.Mutex
			var events []*schema.AiOutputEvent
			cfg := aicommon.NewConfig(ctx,
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithWorkdir(workdir),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if calls >= len(responses) {
						return nil, context.Canceled
					}
					raw := responses[calls]
					calls++
					response := c.NewAIResponse()
					response.EmitOutputStream(strings.NewReader(raw))
					response.Close()
					return response, nil
				}),
				aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
					mu.Lock()
					defer mu.Unlock()
					if e.Type == schema.EVENT_TYPE_REPORT_FINISH || e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME {
						written, err := os.ReadFile(reportPath)
						if err != nil || string(written) != report {
							t.Errorf("report published before being saved: %v", err)
						}
					}
					events = append(events, e)
				}),
			)
			inv := mock.NewMockInvoker(ctx)
			inv.SetConfig(cfg)
			opts := []reactloops.ReActLoopOption{
				reactloops.WithAllowToolCall(false), reactloops.WithAllowRAG(false),
				reactloops.WithAllowAIForge(false), reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
				reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithDisableIncreaseIteration(true), reactloops.WithMaxIterations(5),
				reactloops.WithInitTask(func(l *reactloops.ReActLoop, _ aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
					l.Set("report_filename", reportPath)
					l.Set("user_requirements", "读取参考资料后生成项目探索报告")
					op.Continue()
				}),
			}
			if internal {
				opts = append(opts, WithInternalReportOutput())
			}
			loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_REPORT_GENERATING, inv, opts...)
			require.NoError(t, err)
			task := aicommon.NewStatefulTaskBase("report-task", "生成项目探索报告", ctx, cfg.GetEmitter(), true)
			inv.SetCurrentTask(task)
			require.NoError(t, loop.ExecuteWithExistedTask(task))
			cfg.GetEmitter().WaitForStream()
			require.Equal(t, len(responses), calls)
			written, err := os.ReadFile(reportPath)
			require.NoError(t, err)
			require.Equal(t, report, string(written))
			require.Equal(t, "true", loop.Get("report_finished"))
			mu.Lock()
			defer mu.Unlock()
			var finished []reportFinishEvent
			var pins int
			for _, e := range events {
				require.NotEqual(t, "report-content", e.NodeId, "edit payloads and placeholders must not become chat cards")
				require.NotEqual(t, "infra-code-verify", e.NodeId, "Markdown reports are not code verification")
				if e.Type == schema.EVENT_TYPE_REPORT_FINISH {
					var payload reportFinishEvent
					require.NoError(t, json.Unmarshal(e.Content, &payload))
					finished = append(finished, payload)
				}
				if e.Type == schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME {
					pins++
				}
				if internal {
					require.NotEqual(t, schema.EVENT_TYPE_STREAM_START, e.Type)
					require.NotEqual(t, schema.EVENT_TYPE_STREAM, e.Type)
					require.NotEqual(t, schema.EVENT_TYPE_REFERENCE_MATERIAL, e.Type)
					require.NotEqual(t, "status", e.NodeId)
				}
			}
			if internal {
				require.Empty(t, finished)
				require.Zero(t, pins)
			} else {
				require.Len(t, finished, 1)
				require.Equal(t, 1, pins)
				require.Equal(t, overview+"\n\n完整内容见报告文件。", finished[0].SummaryMarkdown)
				require.NotContains(t, finished[0].SummaryMarkdown, "go.mod")
				require.Equal(t, reportPath, finished[0].ReportPath)
				require.Contains(t, task.GetResult(), overview)
			}
		})
	}
}

func TestReportFinishRequiresSavedArtifact(t *testing.T) {
	for _, source := range []string{"missing", "empty", "unsaved", "saved"} {
		t.Run(source, func(t *testing.T) {
			inv := mock.NewMockInvoker(context.Background())
			cfg := inv.GetConfig().(*mock.MockedAIConfig)
			var finishes int
			cfg.Emitter = aicommon.NewEmitter("report-finish-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
				if e.Type == schema.EVENT_TYPE_REPORT_FINISH {
					finishes++
				}
				return e, nil
			})
			loop := reactloops.NewMinimalReActLoop(cfg, inv)
			path := filepath.Join(t.TempDir(), "report.md")
			loop.Set("report_filename", path)
			content := "# 项目报告\n\n项目使用 Go。"
			loop.Set("full_report_code", content)
			if source != "missing" {
				written := content
				if source == "empty" {
					written = ""
				}
				if source == "unsaved" {
					written = "旧草稿"
				}
				require.NoError(t, os.WriteFile(path, []byte(written), 0o600))
			}
			emitReportFinish(loop)
			if source == "saved" {
				require.Equal(t, 1, finishes)
			} else {
				require.Zero(t, finishes)
			}
		})
	}
}

func TestReportFinishPreviewKeepsCompleteParagraphs(t *testing.T) {
	content := "# 测试报告\n\n## 概览\n\n项目用于展示结构。\n\n```text\n" + strings.Repeat("目录/\n", 200) + "```\n\n| 文件 | 说明 |\n| --- | --- |\n| tail.md | 尾部配置 |"
	title, summary := buildReportFinishPreview(content)
	require.Equal(t, "测试报告", title)
	require.Equal(t, "项目用于展示结构。\n\n完整内容见报告文件。", summary)
	_, fallback := buildReportFinishPreview("# 标题\n\n```text\nproject/\n\nmodule/\n```\n\n| 文件 | 说明 |\n| --- | --- |")
	require.Equal(t, "报告已生成，完整内容见报告文件。", fallback)
}

func TestInternalReportOutputLeavesCallerEventsVisible(t *testing.T) {
	inv := mock.NewMockInvoker(context.Background())
	cfg := inv.GetConfig().(*mock.MockedAIConfig)
	var mu sync.Mutex
	var streams []string
	var diagnostic bool
	cfg.Emitter = aicommon.NewEmitter("scope-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
		mu.Lock()
		defer mu.Unlock()
		if e.Type == schema.EVENT_TYPE_STREAM_START {
			streams = append(streams, e.NodeId)
		}
		if e.NodeId == "loop_marker" {
			diagnostic = true
		}
		return e, nil
	})
	loop := reactloops.NewMinimalReActLoop(cfg, inv)
	withReportOutput()(loop)
	WithInternalReportOutput()(loop)
	// References used to dereference a nil stream event when the processor hid it.
	reactloops.EmitActionLog(loop, "report-read-reference", "内部笔记", "参考资料内容")
	_, err := loop.GetEmitter().EmitStructured("loop_marker", map[string]any{"marker": "enter"})
	require.NoError(t, err)
	_, err = cfg.GetEmitter().EmitTextStreamWithTaskIndex("caller-progress", "正在探索项目", "parent")
	require.NoError(t, err)
	cfg.GetEmitter().WaitForStream()
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []string{"caller-progress"}, streams)
	require.True(t, diagnostic, "retain execution diagnostics while hiding internal chat output")
}
