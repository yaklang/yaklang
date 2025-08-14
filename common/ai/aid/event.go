package aid

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

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

	if c.eventProcessHandler != nil {
		e = c.callEventBeforeSave(e)
	}

	if c.saveEvent && e.ShouldSave() { // not save system and sync
		err := yakit.CreateAIEvent(consts.GetGormProjectDatabase(), e)
		if err != nil {
			log.Errorf("create AI event failed: %v", err)
		}
	}

	if c.guardian != nil {
		c.guardian.feed(e)
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
	if ep, ok := r.epm.loadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.submitCheckpointRequest(ep.checkpoint, reqs)
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
	if ep, ok := r.epm.loadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.submitCheckpointRequest(ep.checkpoint, reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.EmitInteractiveJSON(id, schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) EmitRequireReviewForToolUse(tool *aitool.Tool, params aitool.InvokeParams, id string) {
	reqs := map[string]any{
		"id":               id,
		"selectors":        ToolUseReviewSuggestions,
		"tool":             tool.Name,
		"tool_description": tool.Description,
		"params":           params,
	}
	if ep, ok := r.epm.loadEndpoint(id); ok {
		ep.SetReviewMaterials(reqs)
		err := r.submitCheckpointRequest(ep.checkpoint, reqs)
		if err != nil {
			log.Errorf("submit request reivew to db for task failed: %v", err)
		}
	}
	r.EmitInteractiveJSON(id, schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE, "review-require", reqs)
}

func (r *Config) EmitCurrentConfigInfo() {
	r.EmitJSON(schema.EVENT_TYPE_AID_CONFIG, "system", r.SimpleInfoMap())
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

func (c *Config) pushProcess(newProcess *schema.AiProcess) *Config {
	err := yakit.CreateAIProcess(consts.GetGormProjectDatabase(), newProcess)
	if err != nil {
		return nil
	}
	callBack := func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		event.Processes = append(event.Processes, newProcess)
		return event
	}
	return c.pushEventBeforeSave(callBack)
}

func (c *Config) pushEventBeforeSave(newHandler func(event *schema.AiOutputEvent) *schema.AiOutputEvent) *Config {
	var subConfig = new(Config)
	*subConfig = *c
	if subConfig.eventProcessHandler == nil {
		subConfig.eventProcessHandler = utils.NewStack[func(event *schema.AiOutputEvent) *schema.AiOutputEvent]()
	}
	subConfig.eventProcessHandler.Push(newHandler)
	return subConfig
}

func (c *Config) popEventBeforeSave() *Config {
	var subConfig = new(Config)
	*subConfig = *c
	if subConfig.eventProcessHandler == nil {
		return subConfig
	}
	subConfig.eventProcessHandler.Pop()
	return subConfig
}

func (c *Config) callEventBeforeSave(event *schema.AiOutputEvent) *schema.AiOutputEvent {
	if c.eventProcessHandler == nil || c.eventProcessHandler.Len() == 0 {
		return event
	}
	c.eventProcessHandler.ForeachStack(func(f func(e *schema.AiOutputEvent) *schema.AiOutputEvent) bool {
		event = f(event)
		return true
	})
	return event
}

type SyncType string

const (
	SYNC_TYPE_PLAN          SyncType = "plan"
	SYNC_TYPE_CONSUMPTION   SyncType = "consumption"
	SYNC_TYPE_PING          SyncType = "ping"
	SYNC_TYPE_PROCESS_EVENT SyncType = "sync_process_event"

	ProcessID           string = "process_id"
	SyncProcessEeventID        = "sync_process_event_id"
)

func ParseSyncType(s string) (SyncType, bool) {
	for _, t := range []SyncType{
		SYNC_TYPE_PLAN, SYNC_TYPE_CONSUMPTION, SYNC_TYPE_PING, SYNC_TYPE_PROCESS_EVENT,
	} {
		if string(t) == s {
			return t, true
		}
	}
	return "", false
}

type InputEvent struct {
	Id string

	// 是否是同步信息
	IsSyncInfo bool
	// 同步类型 一般认为有 plan consumption
	SyncType SyncType

	IsInteractive bool
	Params        aitool.InvokeParams
}
