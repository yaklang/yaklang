package aid

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const maxPlanDAGValidationRepairAttempts = 3

func isPlanExecutableDAGValidationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "task executable dag") ||
		strings.Contains(message, "empty dag")
}

func (c *Coordinator) validatePlanExecutableDAG(root *AiTask) error {
	if c == nil {
		return utils.Error("coordinator is nil")
	}
	if root == nil {
		return utils.Error("root task is nil")
	}
	c.standardizeTaskTree(root)
	_, err := buildStrictExecutableTaskGraph(root)
	return err
}

func (c *Coordinator) recordPlanDAGValidationFailure(err error, attempt int) string {
	message := fmt.Sprintf(
		"当前计划的可执行 DAG 校验失败，不能进入执行阶段。错误: %v\n"+
			"原因: depends_on 只能引用真正的前置任务，不能引用任务自身，不能形成循环依赖，也不能让结构性父任务展开后间接依赖自身。\n"+
			"请重新生成完整 plan，修正 depends_on，保留用户原始目标和必要步骤。",
		err,
	)
	if attempt > 0 {
		message = fmt.Sprintf("第 %d 次自动校验失败。\n%s", attempt, message)
	}
	c.EmitError("plan executable DAG validation failed: %v", err)
	if c.Timeline != nil {
		c.Timeline.PushText(c.AcquireId(), "[plan_dag_validation_failed]\n%s", message)
	}
	return message
}

func (pr *planRequest) ensurePlanExecutableDAG(rsp *PlanResponse) (*PlanResponse, error) {
	if pr == nil || pr.cod == nil {
		return nil, utils.Error("plan request coordinator is nil")
	}
	current := rsp
	for attempt := 0; attempt <= maxPlanDAGValidationRepairAttempts; attempt++ {
		if current == nil || current.RootTask == nil {
			return nil, utils.Error("plan response root task is nil")
		}
		if err := pr.cod.validatePlanExecutableDAG(current.RootTask); err != nil {
			reason := pr.cod.recordPlanDAGValidationFailure(err, attempt+1)
			if attempt >= maxPlanDAGValidationRepairAttempts {
				return nil, utils.Errorf("coordinator: plan executable DAG validation failed after %d repair attempts: %v", maxPlanDAGValidationRepairAttempts, err)
			}
			pr.cod.planLoadingStatus("任务计划依赖无效，正在请求 AI 修正 / Invalid Plan DAG, Asking AI to Repair...")
			pr.cod.EmitInfo("request AI to repair invalid plan executable DAG: %v", err)
			repaired, repairErr := pr.generateNewPlan("incomplete", reason, current)
			if repairErr != nil {
				return nil, utils.Errorf("coordinator: repair invalid plan executable DAG failed: %v", repairErr)
			}
			current = repaired
			continue
		}
		if attempt > 0 {
			pr.cod.EmitInfo("plan executable DAG validation repaired")
			pr.cod.standardizeTaskTreeAndNotify(current.RootTask, "plan DAG validation repaired")
		}
		return current, nil
	}
	return nil, utils.Error("coordinator: plan executable DAG validation failed")
}
