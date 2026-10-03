package coordinator

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Accept extra action fields for compatibility, but validate every declared
// parameter before getters coerce it (e.g. null task_ids must not cancel all tasks).
func validateActionParameters(name string, options []aitool.ToolOption) reactloops.LoopActionVerifierFunc {
	tool := aitool.NewWithoutCallback(name, options...)
	return func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
		params := declaredActionParameters(action, tool)
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
		if !c.actionAllowed(d.name) {
			err := fmt.Errorf("当前阶段不允许 %s", d.name)
			recordDecision(loop, d.name, err)
			if recordActionOutcome(loop, a, op, d.name, nil, err) {
				op.Continue()
			}
			return
		}
		if err := validateActionParameters(d.name, d.options)(loop, a); err != nil {
			recordDecision(loop, d.name, err)
			if recordActionOutcome(loop, a, op, d.name, nil, err) {
				op.Continue()
			}
			return
		}
		value, err := d.execute(c, loop, a, op)
		recordDecision(loop, d.name, err)
		if recordActionOutcome(loop, a, op, d.name, value, err) {
			op.Continue()
		}
	})
}
func actionOptions() []reactloops.ReActLoopOption {
	definitions := []coordinatorAction{actionCreatePlan(), actionModifyPlan(), actionSubmitPlan(), actionWaitMessages(), actionInspectTask(), actionReviewTask(), actionRetryTask(), actionCancelTasks(), actionCreateReport(), actionModifyReport(), actionSubmitReport()}
	opts := make([]reactloops.ReActLoopOption, 0, len(definitions))
	for _, d := range definitions {
		opts = append(opts, d.option())
	}
	return opts
}

func (c *Controller) actionAllowed(name string) bool {
	c.mu.Lock()
	phase := c.state.Phase
	manual := c.automatic && c.manualReview
	c.mu.Unlock()
	switch name {
	case "create_plan", "submit_plan":
		return phase == PhasePlan
	case "modify_plan":
		return phase == PhasePlan || phase == PhaseExec
	case "create_report", "modify_report", "submit_report":
		return phase == PhaseExec && c.ReportReady()
	case "review_task":
		return phase == PhaseExec && !manual
	case "wait_messages", "inspect_task", "retry_task", "cancel_tasks":
		return phase == PhaseExec
	default:
		return false
	}
}

// Unknown fields remain available to the shared loop (e.g. todo_delta), but
// never reach component edits or change their cardinality/atomicity checks.
func declaredActionParameters(a *aicommon.Action, tool *aitool.Tool) map[string]any {
	params := make(map[string]any)
	for key, value := range a.GetParams() {
		if tool.Params().Have(key) {
			params[key] = value
		}
	}
	return params
}
func modifyArguments(a *aicommon.Action, options []aitool.ToolOption) map[string]any {
	return declaredActionParameters(a, aitool.NewWithoutCallback("modify", options...))
}
func requiredString(name, description string) aitool.ToolOption {
	return aitool.WithStringParam(name, aitool.WithParam_Description(description), aitool.WithParam_Required())
}

func taskIDsParameter() aitool.ToolOption {
	return aitool.WithStringArrayParam("task_ids", aitool.WithParam_Description("待取消的稳定任务 ID 列表；省略时取消全部当前任务，同时明确处理受影响后继。"))
}
func attemptParameter() aitool.ToolOption {
	return aitool.WithIntegerParam("attempt_id", aitool.WithParam_Description("PLAN STATUS 与 Timeline 执行结果中的当前 attempt_id，必须精确匹配。"), aitool.WithParam_Required())
}
