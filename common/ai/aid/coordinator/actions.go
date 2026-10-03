package coordinator

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Providers can return malformed native arguments. Validate the advertised
// types before getters coerce them (e.g. null task_ids must not become all tasks).
func validateActionParameters(name string, options []aitool.ToolOption) reactloops.LoopActionVerifierFunc {
	tool := aitool.NewWithoutCallback(name, options...)
	return func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
		params := action.GetParams()
		delete(params, "@action")
		if valid, problems := tool.ValidateParams(params); !valid {
			return fmt.Errorf("invalid %s arguments: %s", name, strings.Join(problems, "; "))
		}
		return nil
	}
}

// registerAction supplies separate text-stream and native function definitions,
// with one Chinese parameter contract and one execution/validation path.
func registerAction(name, description string, options []aitool.ToolOption, handler reactloops.LoopActionHandlerFunc) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(name, description+"文本流模式通过 @action 选择本动作，参数遵循当前 JSON Schema。", options, nil, validateActionParameters(name, options), handler, func(action *reactloops.LoopAction) {
		action.NativeDescription = description + "原生调用通过函数名选择本动作，参数写入 arguments。"
		action.NativeOptions = append([]aitool.ToolOption{}, options...)
	})
}

type coordinatorAction struct {
	name, description string
	options           []aitool.ToolOption
	execute           func(*Controller, *reactloops.ReActLoop, *aicommon.Action, *reactloops.LoopActionHandlerOperator) (any, error)
}

func (d coordinatorAction) option() reactloops.ReActLoopOption {
	return registerAction(d.name, d.description, d.options, func(loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
		c := controller(loop)
		if c == nil {
			op.Fail("coordinator controller missing")
			return
		}
		value, err := d.execute(c, loop, a, op)
		if recordActionOutcome(loop, a, op, d.name, value, err) {
			op.Continue()
		}
	})
}
func actionOptions() []reactloops.ReActLoopOption {
	definitions := []coordinatorAction{actionCreatePlan(), actionModifyPlan(), actionSubmitPlan(), actionStartTasks(), actionWaitTasks(), actionReviewTask(), actionRetryTask(), actionCancelTasks(), actionWriteReport()}
	opts := make([]reactloops.ReActLoopOption, 0, len(definitions))
	for _, d := range definitions {
		opts = append(opts, d.option())
	}
	return opts
}
func requiredString(name, description string) aitool.ToolOption {
	return aitool.WithStringParam(name, aitool.WithParam_Description(description), aitool.WithParam_Required())
}
func planVersionParameter() aitool.ToolOption {
	return aitool.WithIntegerParam("plan_version", aitool.WithParam_Description("create_plan 或 modify_plan 返回的当前草案版本，必须精确匹配。"), aitool.WithParam_Required())
}
func taskIDsParameter() aitool.ToolOption {
	return aitool.WithStringArrayParam("task_ids", aitool.WithParam_Description("逻辑任务 ID 列表；start_tasks 省略时选择全部可派发任务，wait/cancel 省略时选择全部已批准任务。"))
}
func attemptParameter() aitool.ToolOption {
	return aitool.WithIntegerParam("attempt_id", aitool.WithParam_Description("PLAN STATUS 与 Timeline 执行结果中的当前 attempt_id，必须精确匹配。"), aitool.WithParam_Required())
}
