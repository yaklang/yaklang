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
	if receipt, ok := result.(PlanEditReceipt); ok {
		if actionErr == nil && receipt.Status == "unchanged" {
			op.Feedback(name + " unchanged；当前计划无需更新。")
			return true
		}
		if actionErr != nil {
			receipt.Status = "rejected"
			receipt.Components = nil
			receipt.TaskIDs = nil
			result = receipt
		}
		if receipt.PatchArtifact != "" && loop.GetEmitter() != nil {
			loop.GetEmitter().EmitPinFilename(receipt.PatchArtifact)
		}
	}
	params := map[string]any{}
	arguments := a.GetParams()
	for _, key := range []string{"task_ids", "task_id", "attempt_id", "decision", "reason", "mode", "timeout_seconds"} {
		if value, ok := arguments[key]; ok {
			params[key] = value
		}
	}
	// 不重复保存计划正文、任务书或报告正文；保留完整结果和实际产物引用。
	resultReferences := result
	if actionErr == nil {
		resultReferences = taskResultReferences(loop, result)
	}
	outcome := struct {
		Action     string         `json:"action"`
		Status     string         `json:"status"`
		Parameters map[string]any `json:"parameters,omitempty"`
		Worker     any            `json:"worker,omitempty"`
		Result     any            `json:"result,omitempty"`
		Error      string         `json:"error,omitempty"`
	}{Action: name, Status: "completed", Parameters: params, Result: resultReferences}

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
	// 操作按结果寻址，保留组件变更与验收结论。
	// 循环共享同一 session，但不同协调实例、任务尝试不会覆盖彼此的记录。
	scope := string(data)
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

// 动作回执只引用结果，不能写入或降级 Controller 发布的最终结果。
func taskResultReferences(loop *reactloops.ReActLoop, value any) any {
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
			tasks = []Attempt{{Task: Task{ID: ref.TaskID}, ID: ref.AttemptID, State: Running, Result: v}}
		} else {
			return value
		}
	default:
		return value
	}
	refs := make([]string, 0, len(tasks))
	for _, task := range taskResultRecords(tasks) {
		// Controller alone publishes the canonical result after the worker has
		// exited and Timeline merge has finished; receipts contain references.
		id, _ := taskResultEvidence(loop.GetConfig(), task)
		refs = append(refs, id)
	}
	return struct {
		Reason           string   `json:"reason,omitempty"`
		ResultReferences []string `json:"result_refs"`
	}{reason, refs}
}
