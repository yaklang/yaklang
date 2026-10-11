package reactloops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

const manageAsyncRequestKey = "manage_async_execution_runtime_request"

type manageAsyncRequest struct {
	op      string
	id      string
	timeout time.Duration
}

// A core action: registration does not depend on tools, MCP, or forge support.
// It controls only the manager owned by this loop, through its existing API.
var loopAction_ManageAsyncExecutionRuntime = &LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_MANAGE_ASYNC_EXECUTION_RUNTIME,
	Description: "这是始终可用的顶层 action，用于管理当前循环所属的后台执行。" +
		"直接选择 manage_async_execution_runtime 并提供 op 等参数；它不是业务工具，不能作为 directly_call_tool 的 tool_name，也不需要 require_tool 加载。" +
		"异步提交返回 execute_id 只表示已登记，不代表业务成功；状态 Dump 只是元信息，不能代替结果核验。" +
		"list 列出执行 ID、状态等元信息；inspect 通过资源的观察器读取当前 stdout、stderr、中间结果和最终结果。" +
		"stop 请求取消指定执行，stop_all 请求取消当前循环的全部执行；stopping 表示已请求取消，需继续 wait 确认执行和清理已结束。" +
		"wait 等待执行及清理结束并返回元信息，等待超时不会取消后台执行；需要输出时再调用 inspect。" +
		"stop_all 后仍可提交新的执行；本 action 仅管理已有执行，不启动新工作。" +
		"需要结束任务时，先 wait 确认所需执行结束，再 inspect 核对结果；主任务结束会取消尚未完成的后台执行。避免连续轮询完整输出，状态足够时使用 list 或一次有界 wait。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam("op", aitool.WithParam_EnumString("list", "inspect", "stop", "wait", "stop_all"),
			aitool.WithParam_Default("list"), aitool.WithParam_Description("管理操作：list 列表、inspect 查看输出与结果、stop 请求停止、wait 等待结束、stop_all 请求停止全部执行。默认 list。")),
		aitool.WithStringParam("execute_id",
			aitool.WithParam_Description("提交回执或 list 返回的执行 ID；inspect、stop、wait 操作必填。")),
		aitool.WithNumberParam("timeout_seconds", aitool.WithParam_Default(10), aitool.WithParam_Min(0), aitool.WithParam_Max(60),
			aitool.WithParam_Description("仅用于 wait：最长等待秒数，默认 10 秒，范围 0–60 秒；0 表示立即查询。等待超时不会取消执行。")),
	},
	ActionVerifier: func(loop *ReActLoop, action *aicommon.Action) error {
		if loop == nil || action == nil {
			return errors.New("manage_async_execution_runtime requires a loop and parsed action")
		}
		// Keep validation per invocation: native batches may contain several
		// management calls. GetParams waits for the complete canonical object.
		loop.SetActionExecutionValue(action, manageAsyncRequestKey, nil)
		params := action.GetParams()
		if nested := params.GetObject("next_action"); len(nested) > 0 {
			params = nested
		}
		request := manageAsyncRequest{
			op: strings.ToLower(strings.TrimSpace(params.GetString("op"))),
			id: strings.TrimSpace(params.GetString("execute_id")), timeout: 10 * time.Second,
		}
		if request.op == "" {
			request.op = "list"
		}
		switch request.op {
		case "list", "stop_all":
		case "inspect", "stop", "wait":
			if request.id == "" {
				return fmt.Errorf("execute_id is required for op=%s", request.op)
			}
		default:
			return fmt.Errorf("unsupported async operation %q; use list, inspect, stop, wait or stop_all", request.op)
		}
		if raw, exists := params["timeout_seconds"]; request.op == "wait" && exists && raw != nil {
			seconds, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(raw)), 64)
			if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds > 60 {
				return errors.New("timeout_seconds must be a number between 0 and 60")
			}
			request.timeout = time.Duration(seconds * float64(time.Second))
		}
		loop.SetActionExecutionValue(action, manageAsyncRequestKey, request)
		return nil
	},
	ActionHandler: func(loop *ReActLoop, action *aicommon.Action, operator *LoopActionHandlerOperator) {
		request, ok := loop.GetActionExecutionValue(action, manageAsyncRequestKey).(manageAsyncRequest)
		if !ok {
			operator.Fail("validated async management request is unavailable")
			return
		}
		feedback := map[string]any{"op": request.op}
		manager, err := loop.GetAsyncExecutionManager()
		if err == nil {
			switch request.op {
			case "list":
				feedback["executions"] = manager.List()
			case "inspect":
				snapshot, inspectErr := manager.Inspect(operator.GetContext(), request.id)
				err = inspectErr
				if snapshot.ID != "" {
					feedback["execution"] = snapshot
				}
			case "stop":
				summary, stopErr := manager.Stop(request.id)
				err = stopErr
				if summary.ID != "" {
					feedback["execution"] = summary
				}
			case "wait":
				ctx, cancel := context.WithTimeout(operator.GetContext(), request.timeout)
				summary, waitErr := manager.Wait(ctx, request.id)
				cancel()
				err = waitErr
				if summary.ID != "" {
					feedback["execution"] = summary
				}
				feedback["wait_timed_out"] = errors.Is(waitErr, context.DeadlineExceeded)
				// Bounded waiting is a normal outcome; it must not trigger retries.
				if errors.Is(waitErr, context.DeadlineExceeded) {
					err = nil
				}
			case "stop_all":
				feedback["executions"] = manager.StopAll()
			}
		}
		if err != nil {
			feedback["error"] = err.Error()
		}
		encoded, err := json.Marshal(feedback)
		if err != nil {
			encoded, _ = json.Marshal(map[string]any{"op": request.op,
				"error": fmt.Sprintf("async management response could not be encoded: %v", err)})
		}
		operator.Feedback(string(encoded))
		operator.Continue()
	},
}
