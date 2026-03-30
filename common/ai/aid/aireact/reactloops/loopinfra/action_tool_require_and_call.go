package loopinfra

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

var loopAction_toolRequireAndCall = &reactloops.LoopAction{
	ActionType:  schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
	Description: "申请工具调用，执行这个 @action 会进入工具申请流程，查看工具教程以及文档，来生成参数。仅当目标工具不在 CACHE_TOOL_CALL 最近缓存中时使用；如果缓存里已经有该工具，优先 directly_call_tool。",
	Options: []aitool.ToolOption{
		aitool.WithStringParam(
			"tool_require_payload",
			aitool.WithParam_Description(`MUST set in {"@action": "require_tool", ... }. 根据上下文信息，提供你想要申请的工具名，只说明工具名即可，严禁包含参数.`),
		),
	},
	ActionVerifier: func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
		payload := action.GetString("tool_require_payload")
		if payload == "" {
			payload = action.GetInvokeParams("next_action").GetString("tool_require_payload")
		}
		if payload == "" {
			return utils.Error("tool_require_payload is required for ActionRequireTool but empty")
		}
		loop.Set("tool_require_payload", payload)
		return nil
	},
	ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
		toolPayload := loop.Get("tool_require_payload")
		if toolPayload == "" {
			operator.Feedback(utils.Error("tool_require_payload is required for ActionRequireTool but empty"))
			return
		}
		invoker := loop.GetInvoker()
		ctx := invoker.GetConfig().GetContext()
		t := loop.GetCurrentTask()
		if t != nil {
			ctx = t.GetContext()
		}

		// loading file or tool
		pr, pw := utils.NewPipe()
		pw.WriteString("loading tool: ")
		pw.WriteString(toolPayload)
		pw.WriteString("...")
		closeOnce := new(sync.Once)
		closeStatusPipe := func() {
			closeOnce.Do(func() {
				pw.Close()
			})
		}
		loop.GetEmitter().EmitDefaultStreamEvent("load_tool", pr, operator.GetTask().GetId())
		defer closeStatusPipe()

		toolIns, err := loop.GetConfig().GetAiToolManager().GetToolByName(toolPayload)
		if err != nil {
			pw.WriteString(fmt.Sprintf("Error: %v", err.Error()))
		} else {
			pw.WriteString(utils.MustRenderTemplate(
				`done! {{ .Name }}{{ if .VerboseName}}({{.VerboseName}}){{ end }} is prepared`,
				map[string]interface{}{
					"Name":        toolIns.GetName(),
					"VerboseName": toolIns.GetVerboseName(),
				}),
			)
		}

		result, directly, callErr := invoker.ExecuteToolRequiredAndCall(ctx, toolPayload)

		// cache tool on successful execution (before satisfaction check)
		if callErr == nil && result != nil {
			if cachedTool, lookupErr := loop.GetConfig().GetAiToolManager().GetToolByName(toolPayload); lookupErr == nil {
				loop.GetConfig().GetAiToolManager().AddRecentlyUsedTool(cachedTool)
				if realCfg, ok := loop.GetConfig().(*aicommon.Config); ok {
					realCfg.SaveRecentToolCache()
				}
			}
		}

		handleToolCallResult(loop, ctx, invoker, toolPayload, result, directly, callErr, operator)
	},
}
