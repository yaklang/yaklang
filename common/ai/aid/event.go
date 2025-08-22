package aid

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (c *Config) emitBaseHandler(e *schema.AiOutputEvent) {
	select {
	case <-c.ctx.Done():
		return
	default:
	}
	c.m.Lock()
	defer c.m.Unlock()

	if c.saveEvent && e.ShouldSave() { // not save system and sync
		err := yakit.CreateAIEvent(consts.GetGormProjectDatabase(), e)
		if err != nil {
			log.Errorf("create AI event failed: %v", err)
		}
	}

	if c.guardian != nil {
		c.guardian.Feed(e)
	}

	if utils.StringArrayContains(c.disableOutputEventType, string(e.Type)) {
		return
	}

	if c.eventHandler == nil {
		if e.IsStream {
			if c.debugEvent {
				fmt.Print(string(e.StreamDelta))
			}
			return
		}

		if e.Type == schema.EVENT_TYPE_CONSUMPTION {
			if c.debugEvent {
				log.Info(e.String())
			}
			return
		}
		if c.debugEvent {
			log.Info(e.String())
		} else {
			//log.Info(utils.ShrinkString(e.String(), 200))
		}
		return
	}
	c.eventHandler(e)
}

func (r *Config) EmitRequireReviewForTask(task *AiTask, id string) {
	reqs := map[string]any{
		"id":            id,
		"selectors":     TaskReviewSuggestions,
		"task":          task,
		"short_summary": task.ShortSummary,
		"long_summary":  task.LongSummary,
	}
	if ep, ok := r.epm.LoadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.EmitInteractiveJSON(id, schema.EVENT_TYPE_TASK_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) EmitRequireReviewForPlan(rsp *PlanResponse, id string) {
	reqs := map[string]any{
		"id":        id,
		"selectors": r.getPlanReviewSuggestion(),
		"plans":     rsp,
		"plans_id":  uuid.New().String(),
	}
	if ep, ok := r.epm.LoadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.SubmitCheckpointRequest(ep.GetCheckpoint(), reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.EmitInteractiveJSON(id, schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) EmitCurrentConfigInfo() {
	r.EmitJSON(schema.EVENT_TYPE_AI_CONFIG, "system", r.SimpleInfoMap())
}

func (r *Config) EmitPushTask(task *AiTask) {
	r.EmitStructured("system", map[string]any{
		"type": "push_task",
		"task": map[string]any{
			"index": task.Index,
			"name":  task.Name,
			"goal":  task.Goal,
		},
	})
}

func (r *Config) EmitPopTask(task *AiTask) {
	r.EmitStructured("system", map[string]any{
		"type": "pop_task",
		"task": map[string]any{
			"index": task.Index,
			"name":  task.Name,
			"goal":  task.Goal,
		},
	})
}

func (r *Config) EmitRiskControlPrompt(id string, result *RiskControlResult) {
	r.EmitJSON(schema.EVENT_TYPE_RISK_CONTROL_PROMPT, `risk-control`, map[string]any{
		"id":     id,
		"score":  result.Score,
		"reason": result.Reason,
	})
}

func (r *Config) EmitUpdateTaskStatus(task *AiTask) {
	r.EmitStructured("system", map[string]any{
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

const (
	SYNC_TYPE_PLAN         string = "plan"
	SYNC_TYPE_CONSUMPTION  string = "consumption"
	SYNC_TYPE_CONFIG       string = "config"
	SYNC_TYPE_CURRENT_TASK string = "current_task"
	SYNC_TYPE_PING         string = "ping"
)

type InputEvent struct {
	Id string

	// 是否是同步信息
	IsSyncInfo bool
	// 同步类型 一般认为有 plan consumption
	SyncType string

	IsInteractive bool
	Params        aitool.InvokeParams
}

func ConvertAIInputEventToAIDInputEvent(event *ypb.AIInputEvent) (*InputEvent, error) {
	if event.IsSyncMessage {
		t := event.GetSyncType()
		var params = make(aitool.InvokeParams)
		err := json.Unmarshal([]byte(event.GetSyncJsonInput()), &params)
		if err != nil {
			log.Errorf("unmarshal interactive json input failed: %v", err)
		}
		return &InputEvent{
			IsSyncInfo: true,
			SyncType:   t,
			Params:     params,
		}, nil
	}

	if event.IsInteractiveMessage {
		var params = make(aitool.InvokeParams)
		err := json.Unmarshal([]byte(event.InteractiveJSONInput), &params)
		if err != nil {
			return nil, utils.Errorf("unmarshal interactive json input failed: %v", err)
		}
		return &InputEvent{
			IsInteractive: true,
			Id:            event.InteractiveId,
			Params:        params,
		}, nil
	}

	return nil, utils.Errorf("unknown input event type: %v", event)
}
