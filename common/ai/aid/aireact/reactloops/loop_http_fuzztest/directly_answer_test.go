package loop_http_fuzztest

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/utils"
)

type httpFuzztestTestInvoker struct {
	*mock.MockInvoker
	mu             sync.Mutex
	resultPayloads []string
}

func newHTTPFuzztestAICallbackInvoker(t *testing.T, cb aicommon.AICallbackType) *httpFuzztestTestInvoker {
	t.Helper()
	ctx := context.Background()
	base := mock.NewMockInvoker(ctx)
	base.SetConfig(aicommon.NewConfig(
		ctx,
		aicommon.WithAICallback(cb),
		aicommon.WithEnableSelfReflection(false),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithDisableSessionTitleGeneration(true),
		aicommon.WithDisableIntentRecognition(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithGenerateReport(false),
		aicommon.WithDisableDynamicPlanning(true),
	))
	return &httpFuzztestTestInvoker{MockInvoker: base}
}

func (i *httpFuzztestTestInvoker) GetCurrentTaskId() string {
	if i == nil {
		return ""
	}
	if task := i.GetCurrentTask(); task != nil {
		return task.GetIndex()
	}
	return ""
}

func (i *httpFuzztestTestInvoker) EmitResultAfterStream(v any) {
	i.mu.Lock()
	i.resultPayloads = append(i.resultPayloads, strings.TrimSpace(utils.InterfaceToString(v)))
	i.mu.Unlock()
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

// TestLoopHTTPFuzztestExecute_DirectlyAnswerEmptyPayloadRetryWithAITAGHint
// 回归: directly_answer ActionVerifier 报错 (空 payload 且无 FINAL_ANSWER tag)
// 时, reactloops.WrapDirectlyAnswerError 必须把当前 nonce 化的 AITAG 模板注入
// 错误信息, 让 CallAITransaction 重试时把 hint 喂给 AI 的下一轮 prompt, AI
// 才有依据用 FINAL_ANSWER tag 自纠正, 避免上一轮 hostscan 长跑里 5 次同款空
// payload 重试黑洞 + fatal abort.
//
// 这是上一轮归因里 P0 修复的关键回归点: 之前 reactloops 内置 directly_answer
// 的 ActionVerifier 只抛纯文字 "answer_payload is required for ActionDirectlyAnswer
// but empty", 5 次重试都同样错下去 -> fatal abort, 浪费 14% 时间 + ~1.2MB token.
// 本测试主要验证"hint 已被注入到下一轮 prompt 里", AI 是否真自纠正属于上游策略,
// 此处不强求 (loop 内部状态机/stream 处理顺序在 retry 路径下有自身复杂度).
//
// 关键词: directly_answer 5 次重试黑洞修复 回归测试, AITAG retry hint 注入,
// CallAITransaction 重试, ReAct Loop directly_answer
func TestLoopHTTPFuzztestExecute_DirectlyAnswerEmptyPayloadRetryWithAITAGHint(t *testing.T) {
	var (
		prompts  []string
		promptMu sync.Mutex
		attempts int32
	)

	invoker := newHTTPFuzztestAICallbackInvoker(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()
		promptMu.Lock()
		prompts = append(prompts, prompt)
		promptMu.Unlock()
		atomic.AddInt32(&attempts, 1)

		// 始终返回空 payload 让 verifier 持续失败, 这样可以严格抓到"第二轮 prompt
		// 必须包含 AITAG retry hint"这一关键回归点. 即便 retry 全部用尽, 测试只关心
		// hint 是否被注入到下一轮 prompt, 不关心最终是否成功 (那是上游 AI 行为).
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(
			`{"@action":"directly_answer","identifier":"phase_status"}`,
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

	// 故意忽略 Execute 返回值: 全部 attempts 都失败时它会返回 error, 这里关心
	// 的是 retry 链路是否正确把 hint 注入下一轮 prompt, 不关心最终是否成功.
	_ = loop.Execute("http-fuzztest-direct-answer-retry", context.Background(), "总结一下当前阶段进展")

	got := atomic.LoadInt32(&attempts)
	if got < 2 {
		t.Fatalf("expected at least 2 AI attempts (1 fail + 1 retry with hint), got %d", got)
	}

	promptMu.Lock()
	captured := append([]string(nil), prompts...)
	promptMu.Unlock()

	if len(captured) < 2 {
		t.Fatalf("expected at least 2 captured prompts, got %d", len(captured))
	}
	// 第一条 prompt 是干净的 (没有任何重试 hint 注入).
	if strings.Contains(captured[0], "AITAG retry hint") {
		t.Fatalf("first prompt should not contain retry hint, got: %s", captured[0])
	}
	// 第二条 prompt 必须包含我们注入的 AITAG retry hint, 且包含 nonce 化模板,
	// AI 才能照抄正确格式. 这是修 5 次重试黑洞的核心修复点.
	nonce := aicommon.MustExtractDynamicSectionNonce(t, captured[1])
	if !strings.Contains(captured[1], "AITAG retry hint") {
		t.Fatalf("second prompt MUST contain 'AITAG retry hint' (no hint = no self-correction), got prompt[1]: %s", captured[1])
	}
	if !strings.Contains(captured[1], "<|FINAL_ANSWER_"+nonce+"|>") {
		t.Fatalf("second prompt MUST contain nonce-tagged FINAL_ANSWER template (nonce=%s), got prompt[1]: %s", nonce, captured[1])
	}
	if !strings.Contains(captured[1], "MUST emit AITAG block") {
		t.Fatalf("second prompt MUST instruct AI to emit AITAG block, got prompt[1]: %s", captured[1])
	}
	if !strings.Contains(captured[1], "answer_payload is required for ActionDirectlyAnswer but empty") &&
		!strings.Contains(captured[1], "directly_answer requires answer_payload or FINAL_ANSWER tag") {
		t.Fatalf("second prompt MUST preserve original ActionVerifier error text for diagnosability, got prompt[1]: %s", captured[1])
	}
}

func TestLoopHTTPFuzztestExecute_DirectlyAnswerWithFinalAnswerAITag(t *testing.T) {
	var prompts []string
	var answered int32
	invoker := newHTTPFuzztestAICallbackInvoker(t, func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompts = append(prompts, req.GetPrompt())
		nonce := aicommon.MustExtractDynamicSectionNonce(t, req.GetPrompt())

		// directly_answer 永不 Exit: 发完答复后循环继续, 必须由显式 finish
		// 终结整个 loop. mock 先发一次结构化答复, 之后用 finish 收口.
		// 关键词: directly_answer 永不 Exit, finish 唯一终结器, mock 迁移
		if atomic.AddInt32(&answered, 1) > 1 {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action":"finish"}`))
			rsp.Close()
			return rsp, nil
		}

		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(
			`{"@action":"directly_answer","identifier":"phase_status"}` + "\n" +
				"<|FINAL_ANSWER_" + nonce + "|>\n" +
				"## 当前阶段结论\n\n" +
				"### 已测试方面\n- 已测试 q 参数的基础注入。\n\n" +
				"### 结果与发现\n- 暂未发现直接报错回显。\n\n" +
				"### 下一步建议\n1. 继续做上下文打断测试。\n" +
				"<|FINAL_ANSWER_END_" + nonce + "|>",
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
