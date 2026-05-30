package aireact

import (
	"bytes"
	"fmt"
	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func mockedFreeInputOutput(config aicommon.AICallerConfigIf, flag string) (*aicommon.AIResponse, error) {
	rsp := config.NewAIResponse()
	rs := bytes.NewBufferString(`
{"@action": "object", "next_action": {
	"type": "directly_answer",
	"answer_payload": "..[mocked_answer` + flag + `]..",
}, "cumulative_summary": "..cumulative-mocked` + flag + `.."}
`)
	rsp.EmitOutputStream(rs)
	rsp.Close()
	return rsp, nil
}

func addScopedVerificationTodo(config aicommon.AICallerConfigIf, task aicommon.AIStatefulTask, todoID, content string) {
	config.ApplyVerificationTodoOps(
		aicommon.BuildVerificationTodoScope(task),
		false,
		[]aicommon.VerifyNextMovement{
			{Op: "add", ID: todoID, Content: content},
		},
	)
}

func mockedLoopDirectlyAnswerOutput(config aicommon.AICallerConfigIf, payload string) (*aicommon.AIResponse, error) {
	rsp := config.NewAIResponse()
	rsp.EmitOutputStream(bytes.NewBufferString(payload))
	rsp.Close()
	return rsp, nil
}

// mockedFinishOutput 产出唯一终结器 finish 的决策响应. 去 Exit 化后 directly_answer
// 只发答复并继续循环, 真正收尾必须由 finish 完成, 测试 mock 在答复后追加 finish.
// 关键词: finish 唯一终结器, directly_answer 答复后追加 finish 收尾
func mockedFinishOutput(config aicommon.AICallerConfigIf) (*aicommon.AIResponse, error) {
	return mockedLoopDirectlyAnswerOutput(config, `{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"finish after answer delivered","cumulative_summary":"summary"}`)
}

var timelineNextEntryPattern = regexp.MustCompile(`\n\d{2}:\d{2}:\d{2} \[`)

func extractBlockedTodoSnippet(timeline string) string {
	// 去 Exit 化后, TODO 闸门从 directly_answer 移到唯一终结器 finish 上,
	// 面包屑标记相应变为 [FINISH_BLOCKED_BY_TODO].
	// 关键词: TODO 闸门移到 finish, FINISH_BLOCKED_BY_TODO 面包屑
	start := strings.Index(timeline, "[[FINISH_BLOCKED_BY_TODO]]")
	if start < 0 {
		return ""
	}
	rest := timeline[start:]
	if loc := timelineNextEntryPattern.FindStringIndex(rest[1:]); loc != nil {
		return rest[:1+loc[0]]
	}
	return rest
}

func TestReAct_FreeInput(t *testing.T) {
	flag := ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedFreeInputOutput(i, flag)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 1; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
		close(in)
	}()
	after := time.After(5 * time.Second)

	haveResult := false

LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.NodeId == "result" {
				result := jsonpath.FindFirst(e.GetContent(), "$..result")
				if strings.Contains(utils.InterfaceToString(result), flag) {
					haveResult = true
				}
			}
			if e.NodeId == "react_task_status_changed" {
				result := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(result) == "completed" {
					break LOOP
				}
			}
		case <-after:
			break LOOP
		}
	}

	if !haveResult {
		t.Fatal("Expected to have at least one result event, but got none")
	}
	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, flag) {
		t.Fatal("timeline does not contain flag", flag)
	}
	fmt.Println(timeline)
}

func TestReAct_FreeInput_MultiCalls(t *testing.T) {
	flag := ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)
	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedFreeInputOutput(i, flag)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = ins
	go func() {
		for i := 0; i < 3; i++ {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "abc",
			}
		}
		close(in)
	}()
	after := time.After(5 * time.Second)

	haveResult := false

	count := 0
LOOP:
	for {
		select {
		case e := <-out:
			fmt.Println(e.String())
			if e.NodeId == "result" {
				result := jsonpath.FindFirst(e.GetContent(), "$..result")
				if strings.Contains(utils.InterfaceToString(result), flag) {
					haveResult = true
					count++
					if count >= 3 {
						break LOOP
					}
				}
			}
		case <-after:
			break LOOP
		}
	}

	if !haveResult {
		t.Fatal("Expected to have at least one result event, but got none")
	}

	fmt.Println(ins.DumpTimeline())
}

// TestReAct_DirectlyAnswer_ChecksCurrentTaskTodo 去 Exit 化后的新语义:
// directly_answer 即便当前任务仍有未关闭 TODO 也照常 emit 答复 (不再被拦),
// 且 directly_answer 不再终结循环; 真正终结由唯一终结器 finish 完成, 而
// TODO 闸门已从 directly_answer 移到 finish 上 (open TODO 时 finish 被拦,
// 关闭后才放行). 本测试验证: 答复照常发出 + finish 受当前任务 TODO 闸门约束.
// 关键词: directly_answer 永不 Exit 不再被 TODO 拦, finish 唯一终结器 + TODO 闸门
func TestReAct_DirectlyAnswer_ChecksCurrentTaskTodo(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)
	var primaryAttempts int32
	var ins *ReAct
	var err error
	ins, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			if isPrimaryDecisionPrompt(prompt) {
				switch atomic.AddInt32(&primaryAttempts, 1) {
				case 1:
					currentTask := ins.GetCurrentTask()
					if currentTask == nil {
						return nil, utils.Error("current task is nil in callback")
					}
					addScopedVerificationTodo(ins.GetConfig(), currentTask, "current_open_todo", "当前任务未完成待办")
					// directly_answer 即便有未关闭 TODO 也照常 emit, 不再被拦
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"final answer"},"human_readable_thought":"directly answer","cumulative_summary":"summary"}`)
				case 2:
					// finish 是被 TODO 闸门拦截的唯一终结器: 此时仍有 open TODO
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"try finish","cumulative_summary":"summary"}`)
				case 3:
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"adjust_todolist","next_movements":[{"op":"done","id":"current_open_todo"}]},"human_readable_thought":"close todo","cumulative_summary":"todo updated"}`)
				case 4:
					// TODO 已关闭, finish 放行收口
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"finish","cumulative_summary":"summary"}`)
				default:
					return nil, utils.Errorf("unexpected primary prompt attempt: %d", atomic.LoadInt32(&primaryAttempts))
				}
			}
			if isVerifySatisfactionPrompt(prompt) {
				return mockedLoopDirectlyAnswerOutput(i, `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"todo gate satisfied"}`)
			}
			return mockedLoopDirectlyAnswerOutput(i, `{"@action":"directly_answer","answer_payload":"fallback"}`)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	in <- &ypb.AIInputEvent{IsFreeInput: true, FreeInput: "abc"}
	timeout := time.After(15 * time.Second)
	var drain <-chan time.Time
	results := make([]string, 0)
	taskCompleted := false
LOOP:
	for {
		select {
		case e := <-out:
			if e == nil {
				continue
			}
			if e.NodeId == "result" {
				results = append(results, strings.TrimSpace(utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..result"))))
			}
			if e.NodeId == "react_task_status_changed" {
				status := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(status) == "completed" {
					taskCompleted = true
					if drain == nil {
						drain = time.After(300 * time.Millisecond)
					}
				}
			}
		case <-drain:
			break LOOP
		case <-timeout:
			break LOOP
		}
	}

	if !taskCompleted {
		t.Fatal("task should complete after current task todo is closed and finish is allowed")
	}
	if got := atomic.LoadInt32(&primaryAttempts); got != 4 {
		t.Fatalf("expected 4 primary decision attempts, got %d", got)
	}
	haveFinalAnswer := false
	for _, result := range results {
		if result == "final answer" {
			haveFinalAnswer = true
			break
		}
	}
	if !haveFinalAnswer {
		t.Fatalf("expected directly_answer to emit final answer even with open todo, got %#v", results)
	}
	timeline := ins.DumpTimeline()
	// 新语义: directly_answer 不再产生 blocked 面包屑
	if strings.Contains(timeline, "[DIRECT_ANSWER_BLOCKED_BY_TODO]") {
		t.Fatal("directly_answer must NOT be blocked by todo anymore (no implicit exit gate)")
	}
	// TODO 闸门现在落在 finish 上
	if !strings.Contains(timeline, "[FINISH_BLOCKED_BY_TODO]") {
		t.Fatal("expected finish blocked breadcrumb in timeline while current task todo is open")
	}
	if !strings.Contains(timeline, "current_open_todo") {
		t.Fatal("expected current task todo id in finish blocked breadcrumb")
	}
}

// TestReAct_DirectlyAnswer_IgnoresSessionTodoFromOtherTask 验证 TODO 闸门的
// 作用域: 仅当前任务的 open TODO 才会拦 finish; 别的任务 (session 兄弟任务)
// 残留的 TODO 不影响本任务的 finish 收口. 同时 directly_answer 照常 emit.
// 关键词: TODO 闸门作用域当前任务, finish 不被兄弟任务 TODO 拦, directly_answer 照常 emit
func TestReAct_DirectlyAnswer_IgnoresSessionTodoFromOtherTask(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)
	var primaryAttempts int32
	var ins *ReAct
	var err error
	ins, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			if isPrimaryDecisionPrompt(prompt) {
				switch atomic.AddInt32(&primaryAttempts, 1) {
				case 1:
					siblingTask := aicommon.NewStatefulTaskBase("session-sibling-task", "other task", i.GetContext(), i.GetEmitter(), true)
					addScopedVerificationTodo(ins.GetConfig(), siblingTask, "session_only_open_todo", "别的任务残留待办")
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"final answer"},"human_readable_thought":"directly answer","cumulative_summary":"summary"}`)
				case 2:
					// 仅兄弟任务有 open TODO, 本任务的 finish 不应被拦, 直接收口
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"finish","cumulative_summary":"summary"}`)
				default:
					return nil, utils.Errorf("unexpected primary prompt attempt: %d", atomic.LoadInt32(&primaryAttempts))
				}
			}
			if isVerifySatisfactionPrompt(prompt) {
				return mockedLoopDirectlyAnswerOutput(i, `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"sibling todo ignored"}`)
			}
			return mockedLoopDirectlyAnswerOutput(i, `{"@action":"directly_answer","answer_payload":"fallback"}`)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	in <- &ypb.AIInputEvent{IsFreeInput: true, FreeInput: "abc"}
	timeout := time.After(15 * time.Second)
	var drain <-chan time.Time
	results := make([]string, 0)
	taskCompleted := false
LOOP:
	for {
		select {
		case e := <-out:
			if e == nil {
				continue
			}
			if e.NodeId == "result" {
				results = append(results, strings.TrimSpace(utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..result"))))
			}
			if e.NodeId == "react_task_status_changed" {
				status := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(status) == "completed" {
					taskCompleted = true
					if drain == nil {
						drain = time.After(300 * time.Millisecond)
					}
				}
			}
		case <-drain:
			break LOOP
		case <-timeout:
			break LOOP
		}
	}

	if !taskCompleted {
		t.Fatal("task should complete when only sibling task owns unfinished todo")
	}
	if got := atomic.LoadInt32(&primaryAttempts); got != 2 {
		t.Fatalf("expected exactly 2 primary decision attempts, got %d", got)
	}
	haveFinalAnswer := false
	for _, result := range results {
		if result == "final answer" {
			haveFinalAnswer = true
			break
		}
	}
	if !haveFinalAnswer {
		t.Fatalf("expected directly_answer to emit final answer, got %#v", results)
	}
	timeline := ins.DumpTimeline()
	if strings.Contains(timeline, "[FINISH_BLOCKED_BY_TODO]") {
		t.Fatal("finish should not be blocked by session TODOs from another task")
	}
	if strings.Contains(timeline, "session_only_open_todo") {
		t.Fatal("sibling task todo should not leak into current task finish timeline")
	}
}

// TestReAct_DirectlyAnswer_PrefersCurrentTaskTodoOverSessionTodo 验证当前任务
// 与兄弟任务同时有 open TODO 时, finish 闸门只看当前任务的 TODO: 关闭当前任务
// TODO 后 finish 即可收口, 兄弟任务 TODO 不参与拦截. directly_answer 始终照常 emit.
// 关键词: TODO 闸门只看当前任务, finish 收口不被兄弟任务 TODO 拦, 面包屑只列当前任务 TODO
func TestReAct_DirectlyAnswer_PrefersCurrentTaskTodoOverSessionTodo(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 200)
	var primaryAttempts int32
	var ins *ReAct
	var err error
	ins, err = NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			prompt := req.GetPrompt()
			if isPrimaryDecisionPrompt(prompt) {
				switch atomic.AddInt32(&primaryAttempts, 1) {
				case 1:
					currentTask := ins.GetCurrentTask()
					if currentTask == nil {
						return nil, utils.Error("current task is nil in callback")
					}
					addScopedVerificationTodo(ins.GetConfig(), currentTask, "current_blocking_todo", "当前任务待完成")
					siblingTask := aicommon.NewStatefulTaskBase("session-other-task", "other task", i.GetContext(), i.GetEmitter(), true)
					addScopedVerificationTodo(ins.GetConfig(), siblingTask, "session_sibling_todo", "兄弟任务待完成")
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"directly_answer","answer_payload":"final answer"},"human_readable_thought":"directly answer","cumulative_summary":"summary"}`)
				case 2:
					// 当前任务仍有 open TODO, finish 被拦
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"try finish","cumulative_summary":"summary"}`)
				case 3:
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"adjust_todolist","next_movements":[{"op":"done","id":"current_blocking_todo"}]},"human_readable_thought":"close todo","cumulative_summary":"todo updated"}`)
				case 4:
					// 当前任务 TODO 已关闭, 兄弟任务 TODO 不拦, finish 放行
					return mockedLoopDirectlyAnswerOutput(i, `{"@action":"object","next_action":{"type":"finish"},"human_readable_thought":"finish","cumulative_summary":"summary"}`)
				default:
					return nil, utils.Errorf("unexpected primary prompt attempt: %d", atomic.LoadInt32(&primaryAttempts))
				}
			}
			if isVerifySatisfactionPrompt(prompt) {
				return mockedLoopDirectlyAnswerOutput(i, `{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"current todo cleared, sibling todo should not block"}`)
			}
			return mockedLoopDirectlyAnswerOutput(i, `{"@action":"directly_answer","answer_payload":"fallback"}`)
		}),
		aicommon.WithDebug(false),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	in <- &ypb.AIInputEvent{IsFreeInput: true, FreeInput: "abc"}
	timeout := time.After(15 * time.Second)
	var drain <-chan time.Time
	results := make([]string, 0)
	taskCompleted := false
LOOP:
	for {
		select {
		case e := <-out:
			if e == nil {
				continue
			}
			if e.NodeId == "result" {
				results = append(results, strings.TrimSpace(utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..result"))))
			}
			if e.NodeId == "react_task_status_changed" {
				status := jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status")
				if utils.InterfaceToString(status) == "completed" {
					taskCompleted = true
					if drain == nil {
						drain = time.After(300 * time.Millisecond)
					}
				}
			}
		case <-drain:
			break LOOP
		case <-timeout:
			break LOOP
		}
	}

	if !taskCompleted {
		t.Fatal("task should complete after closing only the current task todo")
	}
	if got := atomic.LoadInt32(&primaryAttempts); got != 4 {
		t.Fatalf("expected 4 primary decision attempts, got %d", got)
	}
	haveFinalAnswer := false
	for _, result := range results {
		if result == "final answer" {
			haveFinalAnswer = true
			break
		}
	}
	if !haveFinalAnswer {
		t.Fatalf("expected final answer result after unblocking current todo, got %#v", results)
	}
	timeline := ins.DumpTimeline()
	if !strings.Contains(timeline, "[FINISH_BLOCKED_BY_TODO]") {
		t.Fatal("expected finish blocked breadcrumb in timeline while current task todo is open")
	}
	blockedSnippet := extractBlockedTodoSnippet(timeline)
	if !strings.Contains(blockedSnippet, "current_blocking_todo") {
		t.Fatal("expected current task todo id in finish blocked breadcrumb")
	}
	if strings.Contains(blockedSnippet, "session_sibling_todo") {
		t.Fatal("blocked breadcrumb should only list current task TODOs, not sibling session TODOs")
	}
}
