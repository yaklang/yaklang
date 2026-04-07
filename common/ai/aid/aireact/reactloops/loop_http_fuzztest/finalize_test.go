package loop_http_fuzztest

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type fuzzFinalizeTestInvoker struct {
	*mock.MockInvoker
	artifactDir    string
	resultPayloads []string
	events         []*schema.AiOutputEvent
	timelineEvents []string
	mu             sync.Mutex
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
	if !strings.Contains(invoker.resultPayloads[0], "HTTP Fuzz Test 阶段总结") {
		t.Fatalf("expected fallback summary heading, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "fuzz_header") {
		t.Fatalf("expected fallback summary to include action history, got: %s", invoker.resultPayloads[0])
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

func TestLoopHTTPFuzztestFinalize_SkipsWhenAlreadyDirectlyAnswered(t *testing.T) {
	invoker := newFuzzFinalizeTestInvoker(t)
	loop := newFuzzFinalizeTestLoop(t, invoker)
	markLoopHTTPFuzzDirectlyAnswered(loop)

	deliverLoopHTTPFuzzFinalizeSummary(loop, invoker, "# HTTP Fuzz Test 阶段总结\n\n无需再次输出")

	if len(invoker.resultPayloads) != 0 {
		t.Fatalf("expected no fallback payload after directly_answer, got %d", len(invoker.resultPayloads))
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
