package loop_knowledge_enhance

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type finalizeTestInvoker struct {
	*mock.MockInvoker
	artifactDir    string
	directAnswers  []string
	timelineEvents []string
}

func newFinalizeTestInvoker(t *testing.T) *finalizeTestInvoker {
	t.Helper()

	return &finalizeTestInvoker{
		MockInvoker: mock.NewMockInvoker(context.Background()),
		artifactDir: t.TempDir(),
	}
}

func (i *finalizeTestInvoker) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	i.directAnswers = append(i.directAnswers, query)
	return "ok", nil
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

	if len(invoker.directAnswers) != 1 {
		t.Fatalf("expected exactly one fallback direct answer, got %d", len(invoker.directAnswers))
	}
	if !strings.Contains(invoker.directAnswers[0], "# 知识增强查询报告") {
		t.Fatalf("expected fallback answer to contain final report heading, got: %s", invoker.directAnswers[0])
	}
	if !strings.Contains(invoker.directAnswers[0], "poc.HTTP()") {
		t.Fatalf("expected fallback answer to include final summary content, got: %s", invoker.directAnswers[0])
	}
	if !utils.InterfaceToBoolean(loop.Get(finalSummaryDirectAnsweredKey)) {
		t.Fatal("expected loop to record that the final summary was directly answered")
	}
	if loop.Get("final_knowledge_document") == "" {
		t.Fatal("expected final knowledge document path to be recorded")
	}
}

func TestFinalizeKnowledgeEnhanceLoop_DirectlyAnswersInsufficientReportOnMaxIterations(t *testing.T) {
	invoker := newFinalizeTestInvoker(t)
	loop := newFinalizeTestLoop(t, invoker)

	loop.Set("user_query", "Find undocumented Yaklang packet APIs")
	loop.Set("search_results_summary", "Only partial references to TCP helpers were found.")
	loop.Set("search_history", "[10:00:00] #1 semantic: yaklang packet api -> 128 bytes")
	loop.Set("search_count", "1")
	loop.Set("compressed_result_round_1_1", "Partial TCP helper notes")
	loop.Set("artifact_round_1_1", filepath.Join(invoker.artifactDir, "round-1.md"))

	finalContent := generateInsufficientDataReport(loop, invoker)
	deliverFinalAnswerFallback(loop, invoker, finalContent)

	if len(invoker.directAnswers) != 1 {
		t.Fatalf("expected exactly one fallback direct answer, got %d", len(invoker.directAnswers))
	}
	if !strings.Contains(invoker.directAnswers[0], "资料不足") {
		t.Fatalf("expected insufficient report in fallback answer, got: %s", invoker.directAnswers[0])
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
}
