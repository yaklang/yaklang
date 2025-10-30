package aid

import (
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func (c *Coordinator) EmitRequireReviewForTask(task *AiTask, id string) {
	reqs := map[string]any{
		"id":            id,
		"selectors":     TaskReviewSuggestions,
		"task":          task,
		"short_summary": task.ShortSummary,
		"long_summary":  task.LongSummary,
	}
	if ep, ok := c.Epm.LoadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := c.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	c.EmitInteractiveJSON(id, schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, "review-require", reqs)
}

func (c *Coordinator) EmitRequireReviewForPlan(rsp *PlanResponse, id string) {
	reqs := map[string]any{
		"id":        id,
		"selectors": c.getPlanReviewSuggestion(),
		"plans":     rsp,
		"plans_id":  uuid.New().String(),
	}
	if ep, ok := c.Epm.LoadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := c.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	c.EmitInteractiveJSON(id, schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, "review-require", reqs)
}

func (c *Coordinator) EmitPushTask(task *AiTask) {
	c.EmitStructured("system", map[string]any{
		"type": "push_task",
		"task": map[string]any{
			"index": task.Index,
			"name":  task.Name,
			"goal":  task.Goal,
		},
	})
}

func (c *Coordinator) EmitPopTask(task *AiTask) {
	c.EmitStructured("system", map[string]any{
		"type": "pop_task",
		"task": map[string]any{
			"index": task.Index,
			"name":  task.Name,
			"goal":  task.Goal,
		},
	})
}

//func (c *Coordinator) EmitRiskControlPrompt(id string, result *RiskControlResult) {
//	c.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN, `risk-control`, map[string]any{
//		"id":     id,
//		"score":  result.Score,
//		"reason": result.Reason,
//	})
//}

func (c *Coordinator) EmitUpdateTaskStatus(task *AiTask) {
	c.EmitStructured("system", map[string]any{
		"type": "update_task_status",
		"task": map[string]any{
			"index":        task.Index,
			"name":         task.Name,
			"goal":         task.Goal,
			"summary":      task.ShortSummary,
			"long_summary": task.LongSummary,
			"executing":    task.executing,
			"executed":     task.executed,
		},
	})
}
