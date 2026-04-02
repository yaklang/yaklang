package loop_knowledge_enhance

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type finalizeTestInvoker struct {
	*mock.MockInvoker
	artifactDir    string
	directAnswers  []string
	resultPayloads []string
	timelineEvents []string
	events         []*schema.AiOutputEvent
	mu             sync.Mutex
}

func newFinalizeTestInvoker(t *testing.T) *finalizeTestInvoker {
	t.Helper()

	return &finalizeTestInvoker{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		artifactDir: t.TempDir(),
	}
}

func (i *finalizeTestInvoker) installEmitter() {
	if cfg, ok := i.GetConfig().(*mock.MockedAIConfig); ok {
		cfg.Emitter = aicommon.NewEmitter("knowledge-enhance-finalize-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			i.mu.Lock()
			defer i.mu.Unlock()
			i.events = append(i.events, e)
			return e, nil
		})
	}
}

func (i *finalizeTestInvoker) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.directAnswers = append(i.directAnswers, query)
	return "ok", nil
}

func (i *finalizeTestInvoker) EmitResultAfterStream(result any) {
	i.mu.Lock()
	i.resultPayloads = append(i.resultPayloads, utils.InterfaceToString(result))
	i.mu.Unlock()
	if cfg, ok := i.GetConfig().(*mock.MockedAIConfig); ok && cfg.Emitter != nil {
		_, _ = cfg.Emitter.EmitResultAfterStream("result", result, false)
	}
}

func (i *finalizeTestInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	return filepath.Join(i.artifactDir, name+ext)
}

func (i *finalizeTestInvoker) AddToTimeline(entry, content string) {
	i.timelineEvents = append(i.timelineEvents, entry)
}

func newFinalizeTestLoop(t *testing.T, invoker *finalizeTestInvoker) *reactloops.ReActLoop {
	t.Helper()

	loop, err := reactloops.NewReActLoop(
		"knowledge-enhance-finalize-test",
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

func TestFinalizeKnowledgeEnhanceLoop_DirectlyAnswersWhenSummaryActionWasSkipped(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	invoker.installEmitter()
	loop := newFinalizeTestLoop(t, invoker)

	loop.Set("user_query", "How does Yaklang send HTTP requests?")
	loop.Set("final_summary", "Use poc.HTTP() to send HTTP requests with custom headers and body.")
	loop.Set("search_results_summary", "poc.HTTP supports headers, body, and method configuration.")
	loop.Set("search_history", "[10:00:00] #1 semantic: yaklang http request -> 256 bytes")
	loop.Set("search_count", "1")
	loop.Set("compressed_result_round_1_1", "Yaklang HTTP request sample")
	loop.Set("artifact_round_1_1", filepath.Join(invoker.artifactDir, "round-1.md"))

	finalContent := generateFinalKnowledgeDocument(loop, invoker)
	deliverFinalAnswerFallback(loop, invoker, finalContent)
	time.Sleep(200 * time.Millisecond)

	if len(invoker.directAnswers) != 0 {
		t.Fatalf("expected no fallback DirectlyAnswer call, got %d", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected exactly one result-after-stream payload, got %d", len(invoker.resultPayloads))
	}
	if !strings.Contains(invoker.resultPayloads[0], "# 知识增强查询报告") {
		t.Fatalf("expected fallback answer to contain final report heading, got: %s", invoker.resultPayloads[0])
	}
	if !strings.Contains(invoker.resultPayloads[0], "poc.HTTP()") {
		t.Fatalf("expected fallback answer to include final summary content, got: %s", invoker.resultPayloads[0])
	}
	if !utils.InterfaceToBoolean(loop.Get(finalSummaryDirectAnsweredKey)) {
		t.Fatal("expected loop to record that the final summary was directly answered")
	}
	if loop.Get("final_knowledge_document") == "" {
		t.Fatal("expected final knowledge document path to be recorded")
	}

	var sawAnswerStream bool
	for _, event := range invoker.events {
		if event.NodeId == "re-act-loop-answer-payload" && event.IsStream && event.ContentType == aicommon.TypeTextMarkdown {
			sawAnswerStream = true
			break
		}
	}
	if !sawAnswerStream {
		t.Fatal("expected fallback final report to emit markdown stream events")
	}
}

func TestFinalizeKnowledgeEnhanceLoop_DirectlyAnswersInsufficientReportOnMaxIterations(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	invoker.installEmitter()
	loop := newFinalizeTestLoop(t, invoker)

	loop.Set("user_query", "Find undocumented Yaklang packet APIs")
	loop.Set("search_results_summary", "Only partial references to TCP helpers were found.")
	loop.Set("search_history", "[10:00:00] #1 semantic: yaklang packet api -> 128 bytes")
	loop.Set("search_count", "1")
	loop.Set("compressed_result_round_1_1", "Partial TCP helper notes")
	loop.Set("artifact_round_1_1", filepath.Join(invoker.artifactDir, "round-1.md"))

	finalContent := generateInsufficientDataReport(loop, invoker)
	deliverFinalAnswerFallback(loop, invoker, finalContent)
	time.Sleep(200 * time.Millisecond)

	if len(invoker.directAnswers) != 0 {
		t.Fatalf("expected no fallback DirectlyAnswer call, got %d", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected exactly one result-after-stream payload, got %d", len(invoker.resultPayloads))
	}
	if !strings.Contains(invoker.resultPayloads[0], "资料不足") {
		t.Fatalf("expected insufficient report in fallback answer, got: %s", invoker.resultPayloads[0])
	}
	if loop.Get("knowledge_search_status") != "insufficient" {
		t.Fatalf("expected insufficient knowledge search status, got %q", loop.Get("knowledge_search_status"))
	}
}

func TestFinalizeKnowledgeEnhanceLoop_SkipsDuplicateFallbackDirectAnswer(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	loop := newFinalizeTestLoop(t, invoker)

	loop.Set("user_query", "Yaklang example")
	loop.Set("final_summary", "Summary already delivered.")
	loop.Set("compressed_result_round_1_1", "Example knowledge")
	markFinalSummaryDirectAnswered(loop)

	finalizeKnowledgeErr := errors.New("max iterations reached")
	var finalContent string
	if strings.Contains(finalizeKnowledgeErr.Error(), "max iterations") {
		finalContent = generateInsufficientDataReport(loop, invoker)
	} else {
		finalContent = generateFinalKnowledgeDocument(loop, invoker)
	}
	deliverFinalAnswerFallback(loop, invoker, finalContent)

	if len(invoker.directAnswers) != 0 {
		t.Fatalf("expected no duplicate fallback answer, got %d", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 0 {
		t.Fatalf("expected no duplicate fallback result payload, got %d", len(invoker.resultPayloads))
	}
}

func TestFinalSummaryAction_RegistersMarkdownStreamAndSkipsDirectlyAnswer(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	invoker.installEmitter()

	loop, err := reactloops.NewReActLoop(
		"knowledge-enhance-final-summary-test",
		invoker,
		finalSummaryAction(invoker),
	)
	if err != nil {
		t.Fatalf("create loop: %v", err)
	}

	actionHandler, err := loop.GetActionHandler("final_summary")
	if err != nil {
		t.Fatalf("get action handler: %v", err)
	}
	if len(actionHandler.StreamFields) != 1 {
		t.Fatalf("expected one stream field, got %d", len(actionHandler.StreamFields))
	}
	field := actionHandler.StreamFields[0]
	if field.FieldName != "summary" || field.AINodeId != "re-act-loop-answer-payload" || field.ContentType != aicommon.TypeTextMarkdown {
		t.Fatalf("unexpected stream field: %+v", field)
	}

	loop.Set("user_query", "Yaklang HTTP 请求怎么发送？")
	loop.Set("search_history", "#1 semantic: yaklang http request")
	loop.Set("search_results_summary", "poc.HTTP 支持 headers、body、method 配置")
	loop.Set("search_count", "1")

	action, err := aicommon.ExtractAction(`{"@action":"final_summary","summary":"Use poc.HTTP() to send HTTP requests."}`, "final_summary")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := actionHandler.ActionVerifier(loop, action); err != nil {
		t.Fatalf("verify action: %v", err)
	}

	task := aicommon.NewStatefulTaskBase("knowledge-final-summary-task", "question", context.Background(), loop.GetEmitter())
	op := reactloops.NewActionHandlerOperator(task)
	actionHandler.ActionHandler(loop, action, op)

	terminated, opErr := op.IsTerminated()
	if opErr != nil {
		t.Fatalf("unexpected operator error: %v", opErr)
	}
	if !terminated {
		t.Fatal("expected final_summary handler to terminate the loop")
	}
	if len(invoker.directAnswers) != 0 {
		t.Fatalf("expected no DirectlyAnswer call, got %d", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected one result-after-stream payload, got %d", len(invoker.resultPayloads))
	}
	if !strings.Contains(invoker.resultPayloads[0], "# 知识收集最终报告") {
		t.Fatalf("expected result payload to contain final report heading, got: %s", invoker.resultPayloads[0])
	}
	if !utils.InterfaceToBoolean(loop.Get(finalSummaryDirectAnsweredKey)) {
		t.Fatal("expected final summary to be marked as delivered")
	}
}
