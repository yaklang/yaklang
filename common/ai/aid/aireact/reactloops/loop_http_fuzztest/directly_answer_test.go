package loop_http_fuzztest

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

type httpFuzztestTestInvoker struct {
	base        *mock.MockInvoker
	config      aicommon.AICallerConfigIf
	currentTask aicommon.AIStatefulTask
	resultPayloads []string
	timelineEvents []string
	mu          sync.Mutex
}

func newHTTPFuzztestAICallbackInvoker(t *testing.T, cb aicommon.AICallbackType) *httpFuzztestTestInvoker {
	t.Helper()
	ctx := context.Background()
	return &httpFuzztestTestInvoker{
		base: mock.NewMockInvoker(ctx),
		config: aicommon.NewConfig(
			ctx,
			aicommon.WithAICallback(cb),
			aicommon.WithEnableSelfReflection(false),
			aicommon.WithDisallowMCPServers(true),
			aicommon.WithDisableSessionTitleGeneration(true),
			aicommon.WithDisableIntentRecognition(true),
			aicommon.WithDisableAutoSkills(true),
			aicommon.WithGenerateReport(false),
			aicommon.WithDisableDynamicPlanning(true),
		),
	}
}

func (i *httpFuzztestTestInvoker) GetBasicPromptInfo(tools []*aitool.Tool) (string, map[string]any, error) {
	return i.base.GetBasicPromptInfo(tools)
}

func (i *httpFuzztestTestInvoker) SetCurrentTask(task aicommon.AIStatefulTask) {
	i.currentTask = task
}

func (i *httpFuzztestTestInvoker) GetCurrentTask() aicommon.AIStatefulTask {
	return i.currentTask
}

func (i *httpFuzztestTestInvoker) GetCurrentTaskId() string {
	if i.currentTask == nil {
		return ""
	}
	return i.currentTask.GetIndex()
}

func (i *httpFuzztestTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	return i.config
}

func (i *httpFuzztestTestInvoker) ExecuteToolRequiredAndCall(ctx context.Context, name string) (*aitool.ToolResult, bool, error) {
	return i.base.ExecuteToolRequiredAndCall(ctx, name)
}

func (i *httpFuzztestTestInvoker) ExecuteToolRequiredAndCallWithoutRequired(ctx context.Context, toolName string, params aitool.InvokeParams) (*aitool.ToolResult, bool, error) {
	return i.base.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)
}

func (i *httpFuzztestTestInvoker) AskForClarification(ctx context.Context, question string, payloads []string) string {
	return i.base.AskForClarification(ctx, question, payloads)
}

func (i *httpFuzztestTestInvoker) DirectlyAnswer(ctx context.Context, query string, tools []*aitool.Tool, opts ...any) (string, error) {
	return i.base.DirectlyAnswer(ctx, query, tools, opts...)
}

func (i *httpFuzztestTestInvoker) CompressLongTextWithDestination(ctx context.Context, input any, destination string, targetByteSize int64) (string, error) {
	return i.base.CompressLongTextWithDestination(ctx, input, destination, targetByteSize)
}

func (i *httpFuzztestTestInvoker) EnhanceKnowledgeGetterEx(ctx context.Context, userQuery string, enhancePlans []string, collections ...string) (string, error) {
	return i.base.EnhanceKnowledgeGetterEx(ctx, userQuery, enhancePlans, collections...)
}

func (i *httpFuzztestTestInvoker) VerifyUserSatisfaction(ctx context.Context, query string, isToolCall bool, payload string) (*aicommon.VerifySatisfactionResult, error) {
	return i.base.VerifyUserSatisfaction(ctx, query, isToolCall, payload)
}

func (i *httpFuzztestTestInvoker) RequireAIForgeAndAsyncExecute(ctx context.Context, forgeName string, onFinish func(error)) {
	i.base.RequireAIForgeAndAsyncExecute(ctx, forgeName, onFinish)
}

func (i *httpFuzztestTestInvoker) AsyncPlanAndExecute(ctx context.Context, planPayload string, onFinish func(error)) {
	i.base.AsyncPlanAndExecute(ctx, planPayload, onFinish)
}

func (i *httpFuzztestTestInvoker) InvokeLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	return i.base.InvokeLiteForge(ctx, actionName, prompt, outputs, opts...)
}

func (i *httpFuzztestTestInvoker) InvokeSpeedPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	return i.base.InvokeSpeedPriorityLiteForge(ctx, actionName, prompt, outputs, opts...)
}

func (i *httpFuzztestTestInvoker) InvokeQualityPriorityLiteForge(ctx context.Context, actionName string, prompt string, outputs []aitool.ToolOption, opts ...aicommon.GeneralKVConfigOption) (*aicommon.Action, error) {
	return i.base.InvokeQualityPriorityLiteForge(ctx, actionName, prompt, outputs, opts...)
}

func (i *httpFuzztestTestInvoker) SelectKnowledgeBase(ctx context.Context, originQuery string) (*aicommon.SelectedKnowledgeBaseResult, error) {
	return i.base.SelectKnowledgeBase(ctx, originQuery)
}

func (i *httpFuzztestTestInvoker) ExecuteLoopTaskIF(taskTypeName string, task aicommon.AIStatefulTask, options ...any) (bool, error) {
	return i.base.ExecuteLoopTaskIF(taskTypeName, task, options...)
}

func (i *httpFuzztestTestInvoker) AddToTimeline(entry, content string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timelineEvents = append(i.timelineEvents, entry)
}

func (i *httpFuzztestTestInvoker) EmitFileArtifactWithExt(name, ext string, data any) string {
	return i.base.EmitFileArtifactWithExt(name, ext, data)
}

func (i *httpFuzztestTestInvoker) EmitResultAfterStream(v any) {
	i.mu.Lock()
	i.resultPayloads = append(i.resultPayloads, strings.TrimSpace(utils.InterfaceToString(v)))
	i.mu.Unlock()
}

func (i *httpFuzztestTestInvoker) EmitResult(v any) {
	i.base.EmitResult(v)
}

func newHTTPFuzztestLoopForDirectAnswerTest(t *testing.T) *reactloops.ReActLoop {
	t.Helper()
	invoker := mock.NewMockInvoker(context.Background())
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
		t.Fatalf("create http_fuzztest loop: %v", err)
	}
	return loop
}

func TestLoopHTTPFuzztestFactory_ConfiguresDirectlyAnswerAITag(t *testing.T) {
	loop := newHTTPFuzztestLoopForDirectAnswerTest(t)

	action, err := loop.GetActionHandler("directly_answer")
	if err != nil {
		t.Fatalf("get directly_answer action: %v", err)
	}
	if action != loopActionDirectlyAnswerHTTPFuzztest {
		t.Fatal("expected http_fuzztest loop to use the custom directly_answer action override")
	}
	if len(action.AITagStreamFields) != 1 {
		t.Fatalf("expected one AITag stream field, got %d", len(action.AITagStreamFields))
	}
	field := action.AITagStreamFields[0]
	if field.TagName != "FINAL_ANSWER" {
		t.Fatalf("expected FINAL_ANSWER tag, got %q", field.TagName)
	}
	if field.VariableName != "tag_final_answer" {
		t.Fatalf("expected tag_final_answer variable, got %q", field.VariableName)
	}
	if field.AINodeId != "re-act-loop-answer-payload" {
		t.Fatalf("expected markdown answer node, got %q", field.AINodeId)
	}
	if field.ContentType != aicommon.TypeTextMarkdown {
		t.Fatalf("expected markdown content type, got %q", field.ContentType)
	}
	if !strings.Contains(action.Description, "FINAL_ANSWER AITAG") {
		t.Fatalf("expected action description to mention FINAL_ANSWER AITAG, got: %s", action.Description)
	}
}

func TestLoopHTTPFuzztestDirectlyAnswerVerifier_AllowsTagOnly(t *testing.T) {
	loop := newHTTPFuzztestLoopForDirectAnswerTest(t)
	loop.Set("tag_final_answer", "## 当前阶段结论\n\n- 已测试方面：参数 id")

	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","identifier":"phase_status"}`, "directly_answer")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	if err := loopActionDirectlyAnswerHTTPFuzztest.ActionVerifier(loop, action); err != nil {
		t.Fatalf("expected FINAL_ANSWER tag-only payload to pass verifier, got: %v", err)
	}
	if got := loop.Get("directly_answer_payload"); !strings.Contains(got, "## 当前阶段结论") {
		t.Fatalf("expected verifier to store FINAL_ANSWER markdown payload, got: %q", got)
	}
}

func TestLoopHTTPFuzztestDirectlyAnswerVerifier_RejectsPayloadAndTagTogether(t *testing.T) {
	loop := newHTTPFuzztestLoopForDirectAnswerTest(t)
	loop.Set("tag_final_answer", "## 当前阶段结论")

	action, err := aicommon.ExtractAction(`{"@action":"directly_answer","identifier":"phase_status","answer_payload":"short answer"}`, "directly_answer")
	if err != nil {
		t.Fatalf("extract action: %v", err)
	}
	err = loopActionDirectlyAnswerHTTPFuzztest.ActionVerifier(loop, action)
	if err == nil {
		t.Fatal("expected verifier to reject answer_payload and FINAL_ANSWER together, got nil")
	}
	if !strings.Contains(err.Error(), "exactly one of answer_payload or FINAL_ANSWER") {
		t.Fatalf("unexpected verifier error: %v", err)
	}
}

func TestLoopHTTPFuzztestExecute_DirectlyAnswerWithFinalAnswerAITag(t *testing.T) {
	var prompts []string
	invoker := newHTTPFuzztestAICallbackInvoker(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompts = append(prompts, req.GetPrompt())
			nonceRe := regexp.MustCompile(`<\|FINAL_ANSWER_(\w{4})\|>`)
			matches := nonceRe.FindStringSubmatch(req.GetPrompt())
			if len(matches) != 2 {
				t.Fatalf("expected FINAL_ANSWER nonce in prompt, got: %s", req.GetPrompt())
			}

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(
				`{"@action":"directly_answer","identifier":"phase_status"}` + "\n" +
					"<|FINAL_ANSWER_" + matches[1] + "|>\n" +
					"## 当前阶段结论\n\n" +
					"### 已测试方面\n- 已测试 q 参数的基础注入。\n\n" +
					"### 结果与发现\n- 暂未发现直接报错回显。\n\n" +
					"### 下一步建议\n1. 继续做上下文打断测试。\n" +
					"<|FINAL_ANSWER_END_" + matches[1] + "|>",
			))
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
		t.Fatalf("create http_fuzztest loop: %v", err)
	}

	err = loop.Execute("http-fuzztest-direct-answer", context.Background(), "现在总结一下当前阶段进展")
	if err != nil {
		t.Fatalf("execute loop: %v", err)
	}
	if len(prompts) == 0 {
		t.Fatal("expected AI callback to receive at least one prompt")
	}
	if !strings.Contains(prompts[0], "<|FINAL_ANSWER_") {
		t.Fatalf("expected generated prompt to include FINAL_ANSWER guidance, got: %s", prompts[0])
	}
	if !hasLoopHTTPFuzzDirectlyAnswered(loop) {
		t.Fatal("expected loop to be marked as directly answered")
	}
	if getLoopHTTPFuzzLastAction(loop) != "directly_answer" {
		t.Fatalf("expected last action to be directly_answer, got %q", getLoopHTTPFuzzLastAction(loop))
	}
	if got := loop.Get("tag_final_answer"); !strings.Contains(got, "## 当前阶段结论") {
		t.Fatalf("expected FINAL_ANSWER markdown to be captured, got: %q", got)
	}
	if got := loop.Get("directly_answer_payload"); !strings.Contains(got, "### 结果与发现") {
		t.Fatalf("expected directly_answer payload to use markdown block content, got: %q", got)
	}
}