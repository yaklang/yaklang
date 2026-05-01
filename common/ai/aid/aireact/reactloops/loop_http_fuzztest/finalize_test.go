package loop_http_fuzztest

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type fuzzFinalizeTimelineRecord struct {
	entry   string
	content string
}

type fuzzFinalizeDirectlyAnswerCall struct {
	query string
	opts  []any
}

type fuzzFinalizeTestInvoker struct {
	*mock.MockInvoker
	artifactDir     string
	resultPayloads  []string
	events          []*schema.AiOutputEvent
	timelineEvents  []string
	timelineRecords []fuzzFinalizeTimelineRecord
	directAnswers   []fuzzFinalizeDirectlyAnswerCall

	directAnswerErr        error
	directAnswerReturnText string

	mu sync.Mutex
}

func newFuzzFinalizeTestInvoker(t *testing.T) *fuzzFinalizeTestInvoker {
	t.Helper()
	invoker := &fuzzFinalizeTestInvoker{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		artifactDir: t.TempDir(),
	}
	if cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig); ok {
		cfg.Emitter = aicommon.NewEmitter("http-fuzztest-finalize-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			invoker.mu.Lock()
			defer invoker.mu.Unlock()
			invoker.events = append(invoker.events, e)
			return e, nil
		})
	}
	return invoker
}

func (i *fuzzFinalizeTestInvoker) EmitResultAfterStream(result any) {
	i.mu.Lock()
	i.resultPayloads = append(i.resultPayloads, strings.TrimSpace(utils.InterfaceToString(result)))
	i.mu.Unlock()
	if cfg, ok := i.GetConfig().(*mock.MockedAIConfig); ok && cfg.Emitter != nil {
		_, _ = cfg.Emitter.EmitResultAfterStream("result", result, false)
	}
}

func (i *fuzzFinalizeTestInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	return filepath.Join(i.artifactDir, name+ext)
}

func (i *fuzzFinalizeTestInvoker) AddToTimeline(entry, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timelineEvents = append(i.timelineEvents, entry)
	i.timelineRecords = append(i.timelineRecords, fuzzFinalizeTimelineRecord{entry: entry, content: content})
}

// DirectlyAnswer 让测试可以单独控制 AI 总结路径的成功/失败行为。
// 关键词: DirectlyAnswer, finalize, AI summary
func (i *fuzzFinalizeTestInvoker) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	i.mu.Lock()
	i.directAnswers = append(i.directAnswers, fuzzFinalizeDirectlyAnswerCall{query: query, opts: opts})
	err := i.directAnswerErr
	text := i.directAnswerReturnText
	i.mu.Unlock()
	if err != nil {
		return "", err
	}
	return text, nil
}

// hasFuzzFinalizeReferenceMaterialOption 判断 opts 中是否包含 reference material。
// 关键词: WithDirectlyAnswerReferenceMaterial, finalize test
func hasFuzzFinalizeReferenceMaterialOption(opts []any) bool {
	cfg := aicommon.ApplyDirectlyAnswerOptions(opts)
	return strings.TrimSpace(cfg.ReferenceMaterial) != ""
}

// findFuzzFinalizeTimelineContent 在 timeline 中按 entry key 查找写入内容。
// 关键词: timeline, finalize test
func findFuzzFinalizeTimelineContent(records []fuzzFinalizeTimelineRecord, key string) (string, bool) {
	for _, record := range records {
		if record.entry == key {
			return record.content, true
		}
	}
	return "", false
}

func newFuzzFinalizeTestLoop(t *testing.T, invoker *fuzzFinalizeTestInvoker) *reactloops.ReActLoop {
	t.Helper()
	loop, err := reactloops.NewReActLoop(
		"http-fuzztest-finalize-test",
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
	)
	if err != nil {
		t.Fatalf("create loop: %v", err)
	}
	return loop
}

func collectMarkdownStreamContent(events []*schema.AiOutputEvent) string {
	var out strings.Builder
	for _, event := range events {
		if event.NodeId == "re-act-loop-answer-payload" && event.IsStream && len(event.StreamDelta) > 0 {
			out.Write(event.StreamDelta)
		}
	}
	return out.String()
}

func TestLoopHTTPFuzztestFinalize_DeliversFallbackSummary(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)

	loop.Set("original_request", "GET /health HTTP/1.1\r\nHost: example.com\r\n\r\n")
	loop.Set("original_request_summary", "URL: http://example.com/health BODY: [(0) bytes]")
	loop.Set("current_request_summary", "URL: http://example.com/health BODY: [(0) bytes]")
	loop.Set("diff_result_compressed", "发现 500 响应与回显差异。")
	loop.Set("verification_result", "Satisfied: false\nReasoning: 仍需继续验证")
	loop.Set("representative_httpflow_hidden_index", "flow-123")
	recordLoopHTTPFuzzAction(loop, "fuzz_header", "header_name=X-Test", "共执行 2 次测试，保存 2 条 HTTPFlow。代表性响应状态：HTTP/1.1 500 Internal Server Error", "未达到当前目标；仍需继续验证", "flow-123", []string{"' OR '1'='1", "../../etc/passwd"})

	finalContent := generateLoopHTTPFuzzFinalizeSummary(loop, context.DeadlineExceeded)
	deliverLoopHTTPFuzzFinalizeSummary(loop, invoker, finalContent)
	if emitter := loop.GetEmitter(); emitter != nil {
		emitter.WaitForStream()
	}

	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected one fallback result payload, got %d", len(invoker.resultPayloads))
	}
	if strings.Contains(invoker.resultPayloads[0], "# HTTP Fuzz Test 阶段总结") {
		t.Fatalf("expected concise fallback summary without markdown heading, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "已执行") {
		t.Fatalf("expected fallback summary to mention executed actions, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "fuzz_header") {
		t.Fatalf("expected fallback summary to include action name, got: %s", invoker.resultPayloads[0])
	}
	if !hasLoopHTTPFuzzFinalAnswerDelivered(loop) {
		t.Fatal("expected fallback summary to mark final answer delivered")
	}
	if getLoopHTTPFuzzLastAction(loop) != "finalize_summary" {
		t.Fatalf("expected last action to be finalize_summary, got %q", getLoopHTTPFuzzLastAction(loop))
	}
	var sawMarkdownStream bool
	for _, event := range invoker.events {
		if event.NodeId == "re-act-loop-answer-payload" && event.IsStream {
			sawMarkdownStream = true
			break
		}
	}
	if !sawMarkdownStream {
		t.Fatal("expected fallback summary to emit markdown stream")
	}
}

// TestGenerateLoopHTTPFuzzFinalizeSummary_LiteSummaryOmitsDiffAndPayload 校验
// lite 版总结只展示动作清单 / 代表性 HTTPFlow / 一句话验证结论，
// 不再复述 diff_result_*、payload、当前数据包等冗长内容。
// 关键词: lite_summary, finalize, 简化总结
func TestGenerateLoopHTTPFuzzFinalizeSummary_LiteSummaryOmitsDiffAndPayload(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)

	loop.Set("original_request", "GET /health HTTP/1.1\r\nHost: example.com\r\n\r\n")
	loop.Set("original_request_summary", "URL: http://example.com/health BODY: [(0) bytes]")
	loop.Set("current_request_summary", "URL: http://example.com/health BODY: [(0) bytes]")
	loop.Set("diff_result_analysis", "analysis-only result")
	loop.Set("diff_result_compressed", "compressed result")
	loop.Set("verification_result", "Satisfied: true\nReasoning: 已验证目标")
	loop.Set("representative_httpflow_hidden_index", "flow-200")
	recordLoopHTTPFuzzAction(loop, "fuzz_get_params", "param_name=id", "共执行 2 次测试", "已达到当前目标", "flow-200", []string{"1", "2"})
	recordLoopHTTPFuzzAction(loop, "fuzz_header", "header_name=X-Test", "共执行 1 次测试", "已达到当前目标", "flow-200", []string{"hello"})

	finalContent := generateLoopHTTPFuzzFinalizeSummary(loop, "done")

	if strings.Contains(finalContent, "compressed result") {
		t.Fatalf("expected lite summary to drop diff_result_compressed, got: %s", finalContent)
	}
	if strings.Contains(finalContent, "analysis-only result") {
		t.Fatalf("expected lite summary to drop diff_result_analysis, got: %s", finalContent)
	}
	for _, keyword := range []string{"已测试 Payload", "当前发现", "当前有效请求", "代表性样本", "用户目标", "退出原因"} {
		if strings.Contains(finalContent, keyword) {
			t.Fatalf("expected lite summary to drop section %q, got: %s", keyword, finalContent)
		}
	}

	if strings.Contains(finalContent, "# HTTP Fuzz Test 阶段总结") {
		t.Fatalf("expected lite summary to drop markdown heading, got: %s", finalContent)
	}
	if strings.Contains(finalContent, "详细测试记录请查看") {
		t.Fatalf("expected lite summary to drop boilerplate follow-up hint, got: %s", finalContent)
	}
	if !strings.Contains(finalContent, "已执行 2 个动作 (fuzz_get_params / fuzz_header)") {
		t.Fatalf("expected lite summary to list deduped action names in concise form, got: %s", finalContent)
	}
	if !strings.Contains(finalContent, "代表性 HTTPFlow: flow-200") {
		t.Fatalf("expected lite summary to keep representative HTTPFlow, got: %s", finalContent)
	}
	if !strings.Contains(finalContent, "已达到当前安全测试目标") {
		t.Fatalf("expected lite summary to render verdict from Satisfied flag, got: %s", finalContent)
	}
}

// TestGenerateLoopHTTPFuzzFinalizeSummary_AppendsExitReasonOnError 校验
// 仅在 reason 是 error 时 lite 版总结才追加 "退出原因" 行。
// 关键词: lite_summary, exit_reason, error_reason
func TestGenerateLoopHTTPFuzzFinalizeSummary_AppendsExitReasonOnError(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)

	loop.Set("verification_result", "Satisfied: false")
	recordLoopHTTPFuzzAction(loop, "fuzz_get_params", "param_name=id", "共执行 1 次测试", "未达到当前目标", "flow-err", []string{"1"})

	errReason := generateLoopHTTPFuzzFinalizeSummary(loop, context.DeadlineExceeded)
	if !strings.Contains(errReason, "(退出原因: context deadline exceeded)") {
		t.Fatalf("expected error reason in summary, got: %s", errReason)
	}

	noReason := generateLoopHTTPFuzzFinalizeSummary(loop, "done")
	if strings.Contains(noReason, "退出原因") {
		t.Fatalf("expected non-error reason to be omitted, got: %s", noReason)
	}
}

func TestLoopHTTPFuzztestFinalize_SkipsWhenAlreadyDirectlyAnswered(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)
	markLoopHTTPFuzzDirectlyAnswered(loop)

	deliverLoopHTTPFuzzFinalizeSummary(loop, invoker, "# HTTP Fuzz Test 阶段总结\n\n无需再次输出")

	if len(invoker.resultPayloads) != 0 {
		t.Fatalf("expected no fallback payload after directly_answer, got %d", len(invoker.resultPayloads))
	}
}

func TestLoopHTTPFuzztestFinalize_PreservesComplexMarkdown(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)

	finalContent := strings.TrimSpace("# HTTP Fuzz Test 阶段总结\n\n" +
		"## 当前发现\n\n" +
		"> 目前没有直接报错型注入证据，但对象切换和回显差异值得继续追。\n\n" +
		"| 观察项 | 现象 | 含义 |\n" +
		"| --- | --- | --- |\n" +
		"| 状态码 | 始终 200 | 需要依赖内容差异继续判断 |\n" +
		"| 订单号遍历 | 返回不同用户摘要 | 疑似 IDOR |\n\n" +
		"## 下一步建议\n\n" +
		"1. 扩大对象编号遍历范围。\n" +
		"2. 切换 Cookie 和 Authorization 头验证权限边界。\n\n" +
		"## 代表性命令\n\n" +
		"```http\n" +
		"GET /orders?id=1002 HTTP/1.1\n" +
		"Host: example.com\n" +
		"Cookie: role=user\n" +
		"```")

	deliverLoopHTTPFuzzFinalizeSummary(loop, invoker, finalContent)
	if emitter := loop.GetEmitter(); emitter != nil {
		emitter.WaitForStream()
	}

	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected one finalize payload, got %d", len(invoker.resultPayloads))
	}
	if got := strings.TrimSpace(invoker.resultPayloads[0]); got != finalContent {
		t.Fatalf("expected finalize payload to preserve markdown exactly\nexpected:\n%s\n\ngot:\n%s", finalContent, got)
	}
	streamed := strings.TrimSpace(collectMarkdownStreamContent(invoker.events))
	if streamed != finalContent {
		t.Fatalf("expected markdown stream to preserve content exactly\nexpected:\n%s\n\ngot:\n%s", finalContent, streamed)
	}
	var sawMarkdownStream bool
	for _, event := range invoker.events {
		if event.NodeId == "re-act-loop-answer-payload" && event.IsStream && event.ContentType == aicommon.TypeTextMarkdown {
			sawMarkdownStream = true
			break
		}
	}
	if !sawMarkdownStream {
		t.Fatal("expected complex finalize summary to emit markdown stream events")
	}
}

func TestLoopHTTPFuzztestFinalize_PostIterationHookDeliversSummary(t *testing.T) {
	invoker := newHTTPFuzztestAICallbackInvoker(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(strings.NewReader(`{"@action":"finish","identifier":"stop_now","answer":"结束当前轮次"}`))
		rsp.Close()
		return rsp, nil
	})
	loop, err := reactloops.CreateLoopByName(
		LoopHTTPFuzztestName,
		invoker,
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
			operator.Continue()
		}),
	)
	if err != nil {
		t.Fatalf("create loop with finalize hook: %v", err)
	}

	loop.Set("current_request_summary", "URL: http://example.com/orders?id=1002 BODY: [(0) bytes]")
	loop.Set("diff_result_compressed", "订单号切换后返回了不同用户摘要，存在越权读取线索。")
	loop.Set("verification_result", "Satisfied: false\nReasoning: 仍需确认是否可以稳定越权读取")
	loop.Set("representative_httpflow_hidden_index", "flow-789")
	recordLoopHTTPFuzzAction(loop, "fuzz_get_params", "param_name=id", "共执行 3 次测试，保存 3 条 HTTPFlow。代表性响应状态：HTTP/1.1 200 OK", "未达到当前目标；继续测试", "flow-789", []string{"1001", "1002", "1003"})

	err = loop.Execute("http-fuzztest-hook-task", context.Background(), "帮我继续验证订单接口是否存在越权")
	if err != nil {
		t.Fatalf("execute loop with finalize hook: %v", err)
	}
	if emitter := loop.GetEmitter(); emitter != nil {
		emitter.WaitForStream()
	}

	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected finalize hook to emit one payload, got %d", len(invoker.resultPayloads))
	}
	if strings.Contains(invoker.resultPayloads[0], "# HTTP Fuzz Test 阶段总结") {
		t.Fatalf("expected concise finalize hook payload without markdown heading, got: %s", invoker.resultPayloads[0])
	}
	if strings.Contains(invoker.resultPayloads[0], "详细测试记录请查看") {
		t.Fatalf("expected concise finalize hook payload to drop boilerplate hint, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "fuzz_get_params") {
		t.Fatalf("expected finalize hook payload to include action name, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "代表性 HTTPFlow: flow-789") {
		t.Fatalf("expected finalize hook payload to include representative HTTPFlow, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "未达到当前安全测试目标") {
		t.Fatalf("expected finalize hook payload to include verification verdict, got: %s", invoker.resultPayloads[0])
	}
	if strings.Contains(invoker.resultPayloads[0], "帮我继续验证订单接口是否存在越权") {
		t.Fatalf("expected lite finalize hook payload to drop user goal section, got: %s", invoker.resultPayloads[0])
	}
	if strings.Contains(invoker.resultPayloads[0], "订单号切换后返回了不同用户摘要") {
		t.Fatalf("expected lite finalize hook payload to drop diff content, got: %s", invoker.resultPayloads[0])
	}
	if !hasLoopHTTPFuzzFinalAnswerDelivered(loop) {
		t.Fatal("expected finalize hook to mark final answer delivered")
	}
	if getLoopHTTPFuzzLastAction(loop) != "finalize_summary" {
		t.Fatalf("expected finalize hook to record finalize_summary, got %q", getLoopHTTPFuzzLastAction(loop))
	}
}

// TestTryDeliverLoopHTTPFuzzFinalizeViaAI_DeliversConversationalSummary 校验
// AI 总结路径在 DirectlyAnswer 成功时会注入参考资料、调用 DirectlyAnswer 并标记交付完成，
// 不再退回到 lite 模板的 raw markdown emit。
// 关键词: AI summary, finalize, DirectlyAnswer, reference material
func TestTryDeliverLoopHTTPFuzzFinalizeViaAI_DeliversConversationalSummary(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	invoker.directAnswerReturnText = "本轮针对 id 参数发送了 5 次模糊测试请求，状态码差异显著，疑似存在 SQL 注入。代表性 HTTPFlow: flow-ai-1"
	loop := newFuzzFinalizeTestLoop(t, invoker)

	loop.Set("representative_httpflow_hidden_index", "flow-ai-1")
	loop.Set("diff_result_compressed", "状态码与响应长度均出现显著差异")
	loop.Set("verification_result", "Satisfied: true\nReasoning: 已确认存在差异")
	recordLoopHTTPFuzzAction(loop, "fuzz_get_params", "param_name=id", "共执行 5 次测试，保存 5 条 HTTPFlow", "已达到当前目标", "flow-ai-1", []string{"1", "2", "3"})

	ok := tryDeliverLoopHTTPFuzzFinalizeViaAI(loop, invoker, nil)
	if !ok {
		t.Fatal("expected AI summary path to succeed when DirectlyAnswer returns non-empty answer")
	}

	if len(invoker.directAnswers) != 1 {
		t.Fatalf("expected DirectlyAnswer to be called once, got %d", len(invoker.directAnswers))
	}
	if !hasFuzzFinalizeReferenceMaterialOption(invoker.directAnswers[0].opts) {
		t.Fatalf("expected DirectlyAnswer to receive reference material option, got opts=%+v", invoker.directAnswers[0].opts)
	}
	if !strings.Contains(invoker.directAnswers[0].query, "HTTP 安全模糊测试") {
		t.Fatalf("expected summarization query to mention HTTP 安全模糊测试 context, got: %s", invoker.directAnswers[0].query)
	}

	content, found := findFuzzFinalizeTimelineContent(invoker.timelineRecords, "http_fuzztest_finalize_context")
	if !found {
		t.Fatalf("expected finalize reference material to be injected into timeline, got %+v", invoker.timelineRecords)
	}
	if !strings.Contains(content, "fuzz_get_params") || !strings.Contains(content, "param_name=id") {
		t.Fatalf("expected finalize timeline content to include action context, got: %s", content)
	}

	if !hasLoopHTTPFuzzFinalAnswerDelivered(loop) {
		t.Fatal("expected AI summary path to mark final answer delivered")
	}
	if getLoopHTTPFuzzLastAction(loop) != "finalize_summary" {
		t.Fatalf("expected last action to be finalize_summary after AI summary, got %q", getLoopHTTPFuzzLastAction(loop))
	}
	if len(invoker.resultPayloads) != 0 {
		t.Fatalf("expected AI summary success path to not emit raw fallback markdown, got %d payloads", len(invoker.resultPayloads))
	}
}

// TestTryDeliverLoopHTTPFuzzFinalizeViaAI_FailureReturnsFalseForFallback 校验
// 当 DirectlyAnswer 返回错误时，AI 路径返回 false 留给上层走 lite 模板回退，
// 并且不会错误地标记 final_answer_delivered。
// 关键词: AI summary, fallback, finalize, DirectlyAnswer error
func TestTryDeliverLoopHTTPFuzzFinalizeViaAI_FailureReturnsFalseForFallback(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	invoker.directAnswerErr = errors.New("simulated DirectlyAnswer failure")
	loop := newFuzzFinalizeTestLoop(t, invoker)

	recordLoopHTTPFuzzAction(loop, "fuzz_get_params", "param_name=id", "共执行 1 次测试", "未达到当前目标", "flow-fail", []string{"1"})

	ok := tryDeliverLoopHTTPFuzzFinalizeViaAI(loop, invoker, nil)
	if ok {
		t.Fatal("expected AI summary path to return false when DirectlyAnswer fails")
	}
	if len(invoker.directAnswers) != 1 {
		t.Fatalf("expected DirectlyAnswer to be attempted once, got %d", len(invoker.directAnswers))
	}
	if hasLoopHTTPFuzzFinalAnswerDelivered(loop) {
		t.Fatal("expected AI summary failure to leave final_answer_delivered untouched")
	}
}

// TestTryDeliverLoopHTTPFuzzFinalizeViaAI_EmptyAnswerReturnsFalse 校验
// 当 DirectlyAnswer 返回空字符串时（典型 mock 行为），AI 路径同样视为不可用，
// 让上层走 lite 模板回退。
// 关键词: AI summary, fallback, finalize, DirectlyAnswer empty
func TestTryDeliverLoopHTTPFuzzFinalizeViaAI_EmptyAnswerReturnsFalse(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)

	recordLoopHTTPFuzzAction(loop, "fuzz_get_params", "param_name=id", "共执行 1 次测试", "未达到当前目标", "flow-empty", []string{"1"})

	ok := tryDeliverLoopHTTPFuzzFinalizeViaAI(loop, invoker, nil)
	if ok {
		t.Fatal("expected AI summary path to return false on empty DirectlyAnswer answer")
	}
	if hasLoopHTTPFuzzFinalAnswerDelivered(loop) {
		t.Fatal("expected empty answer path to leave final_answer_delivered untouched")
	}
}

func TestLoopHTTPFuzzSessionContext_RestoresActionHistoryAndPayloads(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)
	rawRequest := "GET /items?id=1 HTTP/1.1\r\nHost: example.com\r\n\r\n"
	fuzzReq, err := newLoopFuzzRequest(context.Background(), invoker, []byte(rawRequest), false)
	if err != nil {
		t.Fatalf("new fuzz request: %v", err)
	}
	storeLoopFuzzRequestState(loop, fuzzReq, []byte(rawRequest), false)
	loop.Set("representative_httpflow_hidden_index", "flow-456")
	loop.Set("diff_result_compressed", "测试发现回显差异")
	loop.Set("verification_result", "Satisfied: false")
	recordLoopHTTPFuzzAction(loop, "fuzz_get_params", "param_name=id", "共执行 1 次测试，保存 1 条 HTTPFlow。代表性响应状态：HTTP/1.1 200 OK", "未达到当前目标；继续测试", "flow-456", []string{"1", "2", "1"})
	markLoopHTTPFuzzLastAction(loop, "fuzz_get_params")
	ctx := captureLoopHTTPFuzzSessionContext(loop, "unit_test")
	if ctx == nil {
		t.Fatal("expected session context to be captured")
	}

	restoredInvoker := newFuzzFinalizeTestInvoker(t)
	restoredLoop := newFuzzFinalizeTestLoop(t, restoredInvoker)
	if !applyLoopHTTPFuzzSessionContext(restoredLoop, restoredInvoker, ctx) {
		t.Fatal("expected session context restore to succeed")
	}
	records := getLoopHTTPFuzzRecentActions(restoredLoop)
	if len(records) != 1 {
		t.Fatalf("expected one restored action record, got %d", len(records))
	}
	if records[0].ActionName != "fuzz_get_params" {
		t.Fatalf("expected restored action fuzz_get_params, got %q", records[0].ActionName)
	}
	tested := getLoopHTTPFuzzTestedPayloads(restoredLoop)
	if len(tested["fuzz_get_params"]) != 2 {
		t.Fatalf("expected two deduplicated payloads, got %v", tested["fuzz_get_params"])
	}
	if getLoopHTTPFuzzLastAction(restoredLoop) != "fuzz_get_params" {
		t.Fatalf("expected restored last action, got %q", getLoopHTTPFuzzLastAction(restoredLoop))
	}
	if strings.TrimSpace(restoredLoop.Get("representative_httpflow_hidden_index")) != "flow-456" {
		t.Fatalf("expected restored representative hidden index, got %q", restoredLoop.Get("representative_httpflow_hidden_index"))
	}
}
