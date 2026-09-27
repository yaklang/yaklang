package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
)

func TestTimelineCompressionSummaryPrompt(t *testing.T) {
	tl := compressionSnapshotFixture()
	replay := compressionSnapshotReplay(t, "historical assistant output")
	importFreezeItem(tl, 250, time.Unix(250, 0), &TextTimelineItem{ID: 250,
		Text: "[FUNCTION_CALL_ACTION_RESPONSE]:\naccepted", PromptText: replay})
	snapshot, err := tl.buildCompressionSnapshot()
	require.NoError(t, err)
	snapshot.RetainedContext = map[string]string{"user_query": "current task"}
	prompt, err := renderCompressionSummaryPrompt(snapshot)
	require.NoError(t, err)
	// Decode the actual source document: every byte of ordinary history occurs
	// in exactly one region; the old head is included, exact journals are not.
	var source struct {
		Previous string            `json:"previous_summary"`
		Older    string            `json:"history_to_summarize"`
		Retained map[string]string `json:"retained_context"`
	}
	require.NoError(t, json.Unmarshal([]byte(prompt[strings.Index(prompt, "{\n"):]), &source))
	require.Equal(t, snapshot.Head.Text, source.Previous)
	require.Equal(t, aiprojection.RedactNonce(renderCompressionSnapshotItems(snapshot.Items)), source.Older)
	require.Equal(t, "current task", source.Retained["user_query"])
	require.NotContains(t, prompt, "EXACT_EVIDENCE")
	require.NotContains(t, prompt, "EXACT_SCHEMA")
	require.NotContains(t, prompt, aiprojection.Nonce())
	// Historical native calls must not turn into assistant/tool messages in
	// the reducer's request. Only the outer user section is projected here.
	projected := aiprojection.ProjectAndObserve("timeline-compression-summary-test",
		aiprojection.CreateTag("PROMPT_SECTION", "timeline-open", prompt))
	require.NotNil(t, projected)
	require.NotEmpty(t, projected.Messages)
	for _, message := range projected.Messages {
		require.Equal(t, "user", message.Role)
	}
	require.Contains(t, source.Older, "call_a")
	require.Contains(t, source.Older, "call_b")
	_, err = renderCompressionSummaryPrompt(nil)
	require.Error(t, err)
	snapshot.Head, snapshot.Items = nil, nil
	_, err = renderCompressionSummaryPrompt(snapshot)
	require.Error(t, err)
}

func TestTimelineCompressionSummarySingleRequest(t *testing.T) {
	for _, mode := range []string{"success", "large_input", "empty", "over_budget", "wrong_type", "nested_summary", "request_error", "invalid_json", "control_token"} {
		t.Run(mode, func(t *testing.T) {
			registerTimelineTestLiteForge(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			calls := 0
			cfg := NewTestConfig(ctx, WithDisableAutoSkills(true), WithDisableCreateDBRuntime(true),
				WithAIAutoRetry(1), WithAITransactionAutoRetry(1),
				WithSpeedPriorityAICallback(func(_ AICallerConfigIf, request *AIRequest) (*AIResponse, error) {
					calls++
					require.Equal(t, CallerLabelTimelineCompress, request.GetCallerLabel())
					require.Equal(t, 1, strings.Count(request.GetPrompt(), `"$schema"`))
					require.Contains(t, request.GetPrompt(), "OLD_SUMMARY")
					if mode == "large_input" {
						require.Contains(t, request.GetPrompt(), "LARGE_HISTORY_BEGIN")
						require.Contains(t, request.GetPrompt(), "LARGE_HISTORY_END")
						require.Greater(t, len(request.GetPrompt()), 80*1024)
					}
					if mode == "request_error" {
						return nil, errors.New("provider context limit")
					}
					var summary any = "已完成初步调查；验证仍未完成，继续检查边界输入。"
					switch mode {
					case "empty":
						summary = " \n "
					case "over_budget":
						summary = strings.Repeat("unbounded summary ", 10000)
					case "wrong_type":
						summary = []string{"not a paragraph"}
					case "control_token":
						summary = aiprojection.CreateTag("FUNCTION_CALL_ACTION_RESPONSE", "", "[]")
					}
					payload, err := json.Marshal(map[string]any{"@action": "timeline-summary", "summary": summary})
					require.NoError(t, err)
					if mode == "invalid_json" {
						payload = []byte("no structured result")
					}
					if mode == "nested_summary" {
						payload = []byte(`{"@action":"timeline-summary","metadata":{"summary":"not a root summary"}}`)
					}
					response := NewUnboundAIResponse()
					response.EmitOutputStream(strings.NewReader(string(payload)))
					response.Close()
					return response, nil
				}))
			tl := compressionSnapshotFixture()
			tl.SoftBindConfig(cfg, nil)
			if mode == "large_input" {
				item, _ := tl.idToTimelineItem.Get(10)
				item.value.(*TextTimelineItem).Text = "LARGE_HISTORY_BEGIN " + strings.Repeat("complete original observation ", 4000) + " LARGE_HISTORY_END"
			}
			snapshot, err := tl.buildCompressionSnapshot()
			require.NoError(t, err)
			before, err := MarshalTimeline(tl)
			require.NoError(t, err)
			summary, err := tl.summarizeCompressionSnapshot(snapshot, TimelineCompressionOptions{MaxInputTokens: 100000, MaxSummaryTokens: 1024})
			if mode == "success" || mode == "large_input" {
				require.NoError(t, err)
				require.Contains(t, summary, "验证仍未完成")
				require.LessOrEqual(t, MeasureTokens(summary), 1024)
			} else {
				require.Error(t, err)
				require.Empty(t, summary)
			}
			require.Equal(t, 1, calls, "no batching, refinement, or retry on rejected summary")
			after, err := MarshalTimeline(tl)
			require.NoError(t, err)
			require.Equal(t, before, after, "generation alone must not publish or delete history")
		})
	}
	_, err := (*Timeline)(nil).summarizeCompressionSnapshot(nil)
	require.Error(t, err)
	_, err = NewTimeline(nil, nil).summarizeCompressionSnapshot(nil)
	require.Error(t, err)
}

// These are synthetic review examples, not recorded production runs or model
// quality evaluations. Export the real renderer's output only when explicitly
// requested; ordinary test runs leave no files in the repository.
func TestTimelineCompressionSummaryExamples(t *testing.T) {
	for _, example := range []struct {
		name, previous string
		history        []string
	}{
		{"first-compression", "", []string{
			"用户要求调查配置加载失败，只允许修改内部实现，公开接口与默认行为必须保持兼容。先定位根因，再验证，不能仅凭编译通过认定问题解决。",
			"已经阅读配置入口与默认值合并逻辑。配置来源包括默认值、文件和显式覆盖。当前假设是空字符串处理有误，但尚未获得足够证据，不能直接修改。",
			"第一次测试命令因当前工作目录不正确而失败，未实际运行目标测试。已确认仓库位置，准备修正工作目录重新执行；该失败不说明被测逻辑错误。",
			"在正确目录执行后复现：文件中显式设置的空值被默认值覆盖。正常非空输入行为符合预期。定位到 merge.go 的空值判断，需要区分未设置与显式空值。",
			"已修改内部合并条件，保持配置结构与导出函数签名不变。变更只影响显式空值，不改其他来源的优先级；还需要测试空值、缺失值和非空值三类输入。",
			"测试结果确认显式空值和正常非空值均符合预期，缺失值正确使用默认值。尚未验证嵌套配置，不应声称所有兼容性验证已经完成。测试输出保存于 artifacts/config-tests.txt。",
			"用户补充：嵌套配置也需要覆盖，完成后报告实际验证范围与剩余风险，不要顺便重构无关模块。此前公开接口保持兼容的约束仍然生效。",
			"当前准备检查嵌套配置。",
		}},
		{"merge-old-summary", "此前确认显式空值被默认值覆盖；公开接口不可修改，嵌套配置尚未验证。", []string{
			"已读取旧摘要，继续处理嵌套配置，保持公开接口不变。此次范围仅为配置合并，不调整配置文件格式。原先显式空值的修复需要保留。",
			"构造嵌套配置复现用例后发现，同类空值判断也存在于子配置路径。顶层测试通过不能证明嵌套路径正确，先补充失败测试并记录输入。",
			"已补充两层嵌套和空父节点的测试。两层嵌套用例失败，空父节点用例通过。失败定位到子配置递归时没有保留字段是否显式设置的信息。",
			"修改递归合并时的字段状态传递，只在内部增加显式设置标记。导出结构与入口参数保持不变。代码完成不等于验证完成，接下来执行受影响测试。",
			"执行受影响测试后，顶层与嵌套配置测试均通过，包括缺失值、显式空值、非空值和空父节点。该结果更新了旧摘要中嵌套配置尚未验证的状态。",
			"检查配置加载的其他调用方，没有发现依赖旧错误行为的测试。当前证据仅覆盖相关包的测试，不代表整个仓库都已验证。结果位于 artifacts/nested-config-tests.txt。",
			"准备向用户汇报根因、修复范围和已执行测试。还没有提交或推送代码，用户未要求发布，不能在总结中声称已经上线。",
			"当前准备生成最终说明。",
		}},
		{"pending-tool-result", "当前调查配置问题，所有修改都需验证；工具已受理不代表执行完成。", []string{
			"用户要求同时检查配置解析与错误处理，先只读调查。不得提前修改文件，也不要把工具请求被受理当成实际拿到了调查结果。",
			"检索定位到配置读取入口和错误包装函数，准备读取两处实现。目录信息只说明文件存在，并不能推断具体错误处理是否正确。",
			"检查历史记录发现此前一次读取超时，不能复用该次读取不存在的输出。保留超时状态，重新请求目标文件，并限制读取范围避免无关内容干扰。",
			"模型发起两个调用：call_config 读取配置加载实现，call_errors 读取错误包装实现。两者都有 accepted 回执，仅表示调用已被系统接收，等待后续结果。",
			"call_config 的执行结果已到达，配置加载在文件不存在时使用默认值，在内容格式错误时返回错误。这个分支的行为已经从代码中确认，但尚未执行测试。",
			"call_errors 尚无执行结果。不能据此认定错误包装正确，也不能声称两个检查都已经完成。后续需要按该调用 ID 关联结果，避免误把其他工具的输出当成它的回执。",
			"当前已确认配置读取分支，仍等待错误包装的输出。没有执行写操作，用户只读调查的约束仍有效；也没有足够信息安排修复或宣称任务完成。",
			"当前等待 call_errors 的结果。",
		}},
	} {
		t.Run(example.name, func(t *testing.T) {
			tl := NewTimeline(nil, nil)
			if example.previous != "" {
				tl.compressedHead = &TimelineCompressedHead{Text: example.previous, CoveredEndItemID: 1, Version: 1}
			}
			for i, body := range example.history {
				id := int64(i + 2)
				importFreezeItem(tl, id, time.Date(2026, 9, 27, 10, i, 0, 0, time.UTC), &TextTimelineItem{ID: id, Text: body})
			}
			snapshot, err := tl.buildCompressionSnapshot()
			require.NoError(t, err)
			prompt, err := renderCompressionSummaryPrompt(snapshot)
			require.NoError(t, err)
			t.Logf("%s: complete input=%d tokens, no fixed output ratio", example.name, snapshot.InputTokens)
			if dir := os.Getenv("YAK_TIMELINE_COMPRESSION_EXAMPLES_DIR"); dir != "" {
				require.NoError(t, os.MkdirAll(dir, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(dir, example.name+".prompt.txt"), []byte(prompt+"\n# 输出 Schema\n"+timelineCompressionSchema), 0644))
			}
		})
	}
}
