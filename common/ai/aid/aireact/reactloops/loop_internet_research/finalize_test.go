package loop_internet_research

import (
	"context"
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

	invoker := &finalizeTestInvoker{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		artifactDir: t.TempDir(),
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

func TestGenerateAndOutputFinalReport_EmitsMarkdownWithoutDirectlyAnswer(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
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

	if len(invoker.directAnswers) != 0 {
		t.Fatalf("expected no DirectlyAnswer call, got %d", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected one result-after-stream payload, got %d", len(invoker.resultPayloads))
	}
	if !strings.Contains(invoker.resultPayloads[0], "# Internet Research Report") {
		t.Fatalf("expected final report heading in payload, got: %s", invoker.resultPayloads[0])
	}
	if !utils.InterfaceToBoolean(loop.Get(finalResearchReportDeliveredKey)) {
		t.Fatal("expected final report to be marked as delivered")
	}

	var sawAnswerStream bool
	for _, event := range invoker.events {
		if event.NodeId == "re-act-loop-answer-payload" && event.IsStream && event.ContentType == aicommon.TypeTextMarkdown {
			sawAnswerStream = true
			break
		}
	}
	if !sawAnswerStream {
		t.Fatal("expected final report to emit markdown stream events")
	}
}

func TestFinalSummaryAction_RegistersMarkdownStreamAndSkipsDirectlyAnswer(t *testing.T) {
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

	loop.Set("user_query", "What is Yaklang?")
	loop.Set("search_history", "#1 web_search: yaklang intro")
	loop.Set("search_results_summary", "Yaklang is a security toolkit.")
	loop.Set("search_count", "1")

	action, err := aicommon.ExtractAction(`{"@action":"final_summary","summary":"Yaklang is a security-oriented language and toolkit."}`, "final_summary")
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
	if len(invoker.directAnswers) != 0 {
		t.Fatalf("expected no DirectlyAnswer call, got %d", len(invoker.directAnswers))
	}
	if len(invoker.resultPayloads) != 1 {
		t.Fatalf("expected one result-after-stream payload, got %d", len(invoker.resultPayloads))
	}
	if !strings.Contains(invoker.resultPayloads[0], "# Internet Research Final Report") {
		t.Fatalf("expected final report heading in payload, got: %s", invoker.resultPayloads[0])
	}
	if !utils.InterfaceToBoolean(loop.Get(finalResearchReportDeliveredKey)) {
		t.Fatal("expected final summary action to mark report as delivered")
	}
}
