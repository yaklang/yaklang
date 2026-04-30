package loop_internet_research

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
	artifactDir     string
	directAnswers   []directlyAnswerCall
	resultPayloads  []string
	timelineEntries []timelineEntry
	events          []*schema.AiOutputEvent

	// 可配置 DirectlyAnswer 行为：err 非 nil 触发失败回退；returnText 控制成功返回值
	directAnswerErr        error
	directAnswerReturnText string

	mu sync.Mutex
}

type directlyAnswerCall struct {
	query string
	opts  []any
}

type timelineEntry struct {
	entry   string
	content string
}

func newFinalizeTestInvoker(t *testing.T) *finalizeTestInvoker {
	t.Helper()

	invoker := &finalizeTestInvoker{
		MockInvoker:            mock.NewMockInvoker(context.Background()),
		artifactDir:            t.TempDir(),
		directAnswerReturnText: "ok",
	}
	if cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig); ok {
		cfg.Emitter = aicommon.NewEmitter("internet-research-finalize-test", func(e *schema.AiOutputEvent) (*schema.AiOutputEvent, error) {
			invoker.mu.Lock()
			defer invoker.mu.Unlock()
			invoker.events = append(invoker.events, e)
			return e, nil
		})
	}
	return invoker
}

func (i *finalizeTestInvoker) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.directAnswers = append(i.directAnswers, directlyAnswerCall{query: query, opts: opts})
	if i.directAnswerErr != nil {
		return "", i.directAnswerErr
	}
	return i.directAnswerReturnText, nil
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
	i.mu.Lock()
	i.timelineEntries = append(i.timelineEntries, timelineEntry{entry: entry, content: content})
	i.mu.Unlock()
}

// hasReferenceMaterialOption 判断 opts 中是否包含 aicommon.WithDirectlyAnswerReferenceMaterial 选项。
func hasReferenceMaterialOption(opts []any) bool {
	cfg := aicommon.ApplyDirectlyAnswerOptions(opts)
	return cfg.ReferenceMaterial != ""
}

// hasTimelineEntry 判断 timeline 中是否有指定 entry key。
func hasTimelineEntry(entries []timelineEntry, key string) bool {
	for _, e := range entries {
		if e.entry == key {
			return true
		}
	}
	return false
}

func newFinalizeTestLoop(t *testing.T, invoker *finalizeTestInvoker, opts ...reactloops.ReActLoopOption) *reactloops.ReActLoop {
	t.Helper()

	baseOpts := []reactloops.ReActLoopOption{
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
	}
	baseOpts = append(baseOpts, opts...)

	loop, err := reactloops.NewReActLoop("internet-research-finalize-test", invoker, baseOpts...)
	if err != nil {
		t.Fatalf("create loop: %v", err)
	}
	return loop
}

func TestGenerateAndOutputFinalReport_DeliversConversationalAnswerViaDirectlyAnswer(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	loop := newFinalizeTestLoop(t, invoker)

	const userQuery = "What is Yaklang?"
	loop.Set("user_query", userQuery)
	loop.Set("final_summary", "Yaklang is a security-oriented language and toolkit.")
	loop.Set("search_results_summary", "Yaklang docs describe it as a security language and integrated toolkit.")
	loop.Set("search_history", "#1 web_search: yaklang intro")
	loop.Set("search_count", "1")
	loop.Set("compressed_result_round_1_1", "Yaklang integrates scanning, scripting, and analysis capabilities.")
	loop.Set("artifact_round_1_1", filepath.Join(invoker.artifactDir, "round-1.md"))

	generateAndOutputFinalReport(loop, invoker, false)
	time.Sleep(200 * time.Millisecond)

	if len(invoker.directAnswers) != 1 {
		t.Fatalf("expected exactly one DirectlyAnswer call, got %d", len(invoker.directAnswers))
	}
	if invoker.directAnswers[0].query != userQuery {
		t.Fatalf("expected DirectlyAnswer to use user query %q, got %q", userQuery, invoker.directAnswers[0].query)
	}
	if !hasReferenceMaterialOption(invoker.directAnswers[0].opts) {
		t.Fatalf("expected DirectlyAnswer to receive reference material option, got %+v", invoker.directAnswers[0].opts)
	}
	if !hasTimelineEntry(invoker.timelineEntries, "internet_research_content") {
		t.Fatalf("expected internet_research_content to be added to timeline, got %+v", invoker.timelineEntries)
	}
	if len(invoker.resultPayloads) != 0 {
		t.Fatalf("expected no raw report emit on DirectlyAnswer success, got %d", len(invoker.resultPayloads))
	}
	if !utils.InterfaceToBoolean(loop.Get(finalResearchReportDeliveredKey)) {
		t.Fatal("expected final report to be marked as delivered")
	}
}

func TestGenerateAndOutputFinalReport_FallsBackToRawReportWhenDirectlyAnswerFails(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	invoker.directAnswerErr = errors.New("simulated DirectlyAnswer failure")
	loop := newFinalizeTestLoop(t, invoker)

	loop.Set("user_query", "What is Yaklang?")
	loop.Set("final_summary", "Yaklang is a security-oriented language and toolkit.")
	loop.Set("search_results_summary", "Yaklang docs describe it as a security language and integrated toolkit.")
	loop.Set("search_history", "#1 web_search: yaklang intro")
	loop.Set("search_count", "1")
	loop.Set("compressed_result_round_1_1", "Yaklang integrates scanning, scripting, and analysis capabilities.")
	loop.Set("artifact_round_1_1", filepath.Join(invoker.artifactDir, "round-1.md"))

	generateAndOutputFinalReport(loop, invoker, false)
	time.Sleep(200 * time.Millisecond)

	if len(invoker.directAnswers) != 1 {
		t.Fatalf("expected DirectlyAnswer to be attempted once, got %d", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected raw report fallback emit on DirectlyAnswer failure, got %d", len(invoker.resultPayloads))
	}
	if !strings.Contains(invoker.resultPayloads[0], "# Internet Research Report") {
		t.Fatalf("expected raw report fallback to contain final report heading, got: %s", invoker.resultPayloads[0])
	}
	if !utils.InterfaceToBoolean(loop.Get(finalResearchReportDeliveredKey)) {
		t.Fatal("expected fallback path to mark final report as delivered")
	}

	var sawAnswerStream bool
	for _, event := range invoker.events {
		if event.NodeId == "re-act-loop-answer-payload" && event.IsStream && event.ContentType == aicommon.TypeTextMarkdown {
			sawAnswerStream = true
			break
		}
	}
	if !sawAnswerStream {
		t.Fatal("expected raw report fallback to emit markdown stream events")
	}
}

// TestFinalSummaryAction_CommitsAIStreamedSummaryWithoutExtraDirectlyAnswer 验证：
// final_summary 动作路径下，AI 的 summary 已经通过 stream field 实时投递到 re-act-loop-answer-payload，
// 因此 ActionHandler 必须只 commit 一次 result-after-stream，**不能** 再次调 DirectlyAnswer，
// 否则会在 UI 上出现「两篇」。
func TestFinalSummaryAction_CommitsAIStreamedSummaryWithoutExtraDirectlyAnswer(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	loop := newFinalizeTestLoop(t, invoker, finalSummaryAction(invoker))

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

	const userQuery = "What is Yaklang?"
	const summaryContent = "Yaklang is a security-oriented language and toolkit."
	loop.Set("user_query", userQuery)
	loop.Set("search_history", "#1 web_search: yaklang intro")
	loop.Set("search_results_summary", "Yaklang is a security toolkit.")
	loop.Set("search_count", "1")

	action, err := aicommon.ExtractAction(`{"@action":"final_summary","summary":"`+summaryContent+`"}`, "final_summary")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := actionHandler.ActionVerifier(loop, action); err != nil {
		t.Fatalf("verify action: %v", err)
	}

	task := aicommon.NewStatefulTaskBase("internet-final-summary-task", "question", context.Background(), loop.GetEmitter())
	op := reactloops.NewActionHandlerOperator(task)
	actionHandler.ActionHandler(loop, action, op)

	terminated, opErr := op.IsTerminated()
	if opErr != nil {
		t.Fatalf("unexpected operator error: %v", opErr)
	}
	if !terminated {
		t.Fatal("expected final_summary handler to terminate the loop")
	}
	// 关键断言：避免「两篇」回归
	if len(invoker.directAnswers) != 0 {
		t.Fatalf("expected NO DirectlyAnswer call on final_summary action path, got %d (this would cause double-article bug)", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected exactly one EmitResultAfterStream call to commit AI summary, got %d", len(invoker.resultPayloads))
	}
	if invoker.resultPayloads[0] != summaryContent {
		t.Fatalf("expected committed result to be the AI summary %q, got %q", summaryContent, invoker.resultPayloads[0])
	}
	if !utils.InterfaceToBoolean(loop.Get(finalResearchReportDeliveredKey)) {
		t.Fatal("expected final summary action to mark report as delivered")
	}
}
