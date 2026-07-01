package aireact

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// mockedMaxIterationLoopForever 让 ReAct 永远选择继续调用工具且从不满意, 从而必然
// 撞上最大迭代上限. 在满意度校验里通过 next_movements 落一条 TODO, 用来验证软性
// 中断会把它 SKIP 回收. directly_answer(即 loop 的 finalize 收尾总结)会正常应答.
func mockedMaxIterationLoopForever(i aicommon.AICallerConfigIf, req *aicommon.AIRequest, toolName string) (*aicommon.AIResponse, error) {
	prompt := req.GetPrompt()
	if isPrimaryDecisionPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "require_tool", "tool_require_payload": "` + toolName + `" },
"human_readable_thought": "keep probing with the tool", "cumulative_summary": "..still working.."}
`))
		rsp.Close()
		return rsp, nil
	}

	if isToolParamGenerationPrompt(prompt, toolName) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "call-tool", "params": { "seconds" : 0.05 }}`))
		rsp.Close()
		return rsp, nil
	}

	if isVerifySatisfactionPrompt(prompt) {
		rsp := i.NewAIResponse()
		// 永不满意, 并顺带落一条待办, 让它在撞上迭代上限时仍处于活跃状态.
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": false, "reasoning": "still-not-done-mock", "next_movements":[{"op":"add","id":"pending_probe","content":"继续排查剩余的可疑流量"}]}`))
		rsp.Close()
		return rsp, nil
	}

	if isDirectAnswerPrompt(prompt) {
		rsp := i.NewAIResponse()
		rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "directly_answer", "answer_payload": "本轮到达迭代上限，这是一次自然结束（不是错误）。已把没做完的排查项标记为 SKIP，你可以回复继续，或开启新话题。"}`))
		rsp.Close()
		return rsp, nil
	}

	return nil, utils.Errorf("unexpected prompt")
}

// TestReAct_MaxIteration_NaturalEnd 覆盖"最大迭代上限 = 软性中断 = 自然结束"这条
// 系统框架路径的对客户端表现 (对应用户提出的响应测试要点):
//  1. 客户端表现为"自然结束": 收到 success_react_task 终止事件, 任务状态 completed;
//  2. 不出现错误: 全程不出现 fail_react_task 事件;
//  3. TODO 都被清掉: 撞上限时把活跃 TODO 批量 SKIP, timeline 记录 "marked as SKIP";
//  4. 退出原因入 timeline: timeline 记录 iteration_limit_interrupt (退出=超出最大迭代).
//
// 关键词: max iteration 自然结束, 无 fail, 待办 SKIP, timeline 退出原因
func TestReAct_MaxIteration_NaturalEnd(t *testing.T) {
	_ = ksuid.New().String()
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 10)

	sleepTool, err := aitool.New(
		"sleep",
		aitool.WithNumberParam("seconds"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			s := params.GetFloat("seconds", 0.05)
			if s <= 0 {
				s = 0.05
			}
			time.Sleep(time.Duration(s*1000) * time.Millisecond)
			return "done", nil
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	ins, err := NewTestReAct(
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, r *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return mockedMaxIterationLoopForever(i, r, "sleep")
		}),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithTools(sleepTool),
		aicommon.WithMaxIterationCount(3),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "分析这批 HTTP 流量里有没有敏感信息泄露",
		}
	}()

	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(15)
	}
	after := time.After(du * time.Second)

	var (
		gotSuccessTerminal bool
		gotFailTerminal    bool
		gotAnswerPayload   bool
		taskCompleted      bool
		iid                string
	)

LOOP:
	for {
		select {
		case e := <-out:
			switch e.Type {
			case string(schema.EVENT_TYPE_SUCCESS_REACT):
				gotSuccessTerminal = true
			case string(schema.EVENT_TYPE_FAIL_REACT):
				gotFailTerminal = true
			case string(schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE):
				iid = utils.InterfaceToString(jsonpath.FindFirst(string(e.Content), "$.id"))
				in <- &ypb.AIInputEvent{
					IsInteractiveMessage: true,
					InteractiveId:        iid,
					InteractiveJSONInput: `{"suggestion": "continue"}`,
				}
			}

			// loop 的 finalize 收尾总结通过 "re-act-loop-answer-payload" 流式节点下发,
			// 内容是分片流出的, 因此按节点 id 判定"答复已下发", 不按单片内容匹配.
			if e.Type == string(schema.EVENT_TYPE_STREAM) && e.NodeId == "re-act-loop-answer-payload" {
				gotAnswerPayload = true
			}

			if e.NodeId == "react_task_status_changed" {
				status := utils.InterfaceToString(jsonpath.FindFirst(e.GetContent(), "$..react_task_now_status"))
				if status == "completed" {
					taskCompleted = true
				}
			}

			// 收到自然结束终止事件且任务已 completed 即可结束观测.
			if gotSuccessTerminal && taskCompleted {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	// 要点2: 全程不得出现 fail 终止事件 (与硬中断形成对比).
	if gotFailTerminal {
		t.Fatal("max-iteration soft interrupt must NOT emit a fail_react_task event")
	}
	// 要点1: 客户端表现为自然结束 (success 终止事件 + 任务 completed).
	if !gotSuccessTerminal {
		t.Fatal("expected a success_react_task terminal event (natural end), but got none")
	}
	if !taskCompleted {
		t.Fatal("expected task status to become completed, but it did not")
	}
	// 要点1(续): loop 的 finalize 收尾总结正常应答 (AI 介绍情况 / 下一步).
	if !gotAnswerPayload {
		t.Fatal("expected the finalize summary answer to be delivered, but got none")
	}

	tl := ins.DumpTimeline()
	// 要点4: 退出原因入 timeline (退出 = 超出最大迭代限制).
	if !strings.Contains(tl, "iteration_limit_interrupt") {
		t.Fatalf("timeline must record the iteration-limit interrupt exit reason, got:\n%s", tl)
	}
	// 要点3: 活跃 TODO 被 SKIP 回收.
	if !strings.Contains(tl, "marked as SKIP") {
		t.Fatalf("timeline must record that unfinished TODOs were marked as SKIP, got:\n%s", tl)
	}
	fmt.Println("--- max-iteration natural-end timeline ---")
	fmt.Println(tl)
}
