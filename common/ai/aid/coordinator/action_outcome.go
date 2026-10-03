package coordinator

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

// 动作观测复用 session Evidence 的持久化、去重、冻结和提升通道。
// 不强制冻结，不写时间戳/调用 ID；相同查询不会扰动稳定前缀。
// 这是历史观测而非实时状态，实时调度仍以 PLAN STATUS 为准。
func recordActionOutcome(loop *reactloops.ReActLoop, a *aicommon.Action, op *reactloops.LoopActionHandlerOperator, name string, result any, actionErr error) bool {
	params := map[string]any{}
	arguments := a.GetParams()
	for _, key := range []string{"plan_version", "task_ids", "task_id", "attempt_id", "decision", "reason", "mode", "timeout_seconds"} {
		if value, ok := arguments[key]; ok {
			params[key] = value
		}
	}
	// 不重复保存计划正文、任务书或报告正文；保留完整结果和实际产物引用。
	observations := result
	if actionErr == nil {
		var err error
		observations, err = saveTaskObservations(loop, result)
		if err != nil {
			op.Fail(fmt.Errorf("%s 已处理，但任务观测保存失败：%w；请核对状态，勿盲目重试", name, err))
			return false
		}
	}
	outcome := struct {
		Action     string            `json:"action"`
		Status     string            `json:"status"`
		Parameters map[string]any    `json:"parameters,omitempty"`
		Plan       map[string]uint64 `json:"plan,omitempty"`
		Worker     any               `json:"worker,omitempty"`
		Result     any               `json:"result,omitempty"`
		Error      string            `json:"error,omitempty"`
	}{Action: name, Status: "completed", Parameters: params, Result: observations}
	if c := controller(loop); c != nil && name != "inspect_plan" && name != "submit_plan" {
		receipt := c.planReceipt()
		outcome.Plan = map[string]uint64{"draft_version": receipt.DraftVersion, "approved_version": receipt.ApprovedVersion}
	}
	if worker := loop.GetVariable("coordinator_worker_attempt"); worker != nil {
		outcome.Worker = worker
	} else if controller(loop) == nil {
		outcome.Worker = map[string]string{"task_id": op.GetTask().GetId()}
	}
	if actionErr != nil {
		outcome.Status, outcome.Error = "rejected", actionErr.Error()
	}
	data, err := json.Marshal(outcome)
	if err != nil {
		op.Fail(fmt.Errorf("%s 已执行，但无法编码动作观测：%w；请核对状态，勿盲目重试", name, err))
		return false
	}
	// 查询复用固定槽位；有副作用的操作按结果寻址，保留每次版本/验收结论。
	// 循环共享同一 session，但不同协调实例、任务尝试不会覆盖彼此的记录。
	scope := string(data)
	if actionErr == nil && (name == "inspect_plan" || name == "inspect_tasks" || name == "wait_tasks") {
		scope = "latest"
	}
	key := fmt.Sprintf("%s:%s:%s", loop.GetConfig().GetRuntimeId(), name, scope)
	digest := sha256.Sum256([]byte(key))
	id := fmt.Sprintf("coordinator.%s.%x", name, digest[:16])
	content := "协调动作历史观测（实时状态以 PLAN STATUS 为准）：\n" + string(data)
	if _, err := reactloops.SaveSessionEvidence(loop.GetConfig(), id, content); err != nil {
		op.Fail(fmt.Errorf("%s 已处理，但 session 动作观测保存失败：%w；请核对状态，勿盲目重试", name, err))
		return false
	}
	// feedback 仅负责一轮的导航；事实保存在 Timeline Open / 提升后的 Evidence。
	op.Feedback(fmt.Sprintf("%s %s；见 Timeline Evidence [id: %s]", name, outcome.Status, id))
	return true
}

// 一个任务尝试一个观测槽位，选择 a、b 或 a+b 不会覆盖其他任务的结果，
// inspect/wait/start 也不反复复制同一份结果。验收理由另存为不可变动作记录。
func saveTaskObservations(loop *reactloops.ReActLoop, value any) (any, error) {
	var tasks []Attempt
	var reason string
	switch v := value.(type) {
	case []Attempt:
		tasks = v
	case WaitResult:
		tasks, reason = v.Tasks, v.Reason
	case Result:
		if ref, ok := loop.GetVariable("coordinator_worker_attempt").(workerAttemptRef); ok {
			// 提交结果时 worker 仍在运行；最终状态由 Controller 随后结算。
			tasks = []Attempt{{Task: Task{ID: ref.TaskID}, ID: ref.AttemptID, PlanVersion: ref.PlanVersion, State: Running, Result: v}}
		} else {
			return value, nil
		}
	default:
		return value, nil
	}
	refs := make([]string, 0, len(tasks))
	for _, task := range taskObservations(tasks) {
		key := fmt.Sprintf("%s:%s:%d:%d", loop.GetConfig().GetRuntimeId(), task.TaskID, task.PlanVersion, task.AttemptID)
		digest := sha256.Sum256([]byte(key))
		id := fmt.Sprintf("coordinator.task.%x", digest[:16])
		data, err := json.Marshal(task)
		if err != nil {
			return nil, err
		}
		if _, err := reactloops.SaveSessionEvidence(loop.GetConfig(), id, "任务尝试历史观测（实时状态以 PLAN STATUS 为准）：\n"+string(data)); err != nil {
			return nil, err
		}
		refs = append(refs, id)
	}
	return struct {
		Reason       string   `json:"reason,omitempty"`
		Observations []string `json:"observations"`
	}{reason, refs}, nil
}
