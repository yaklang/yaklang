package aireact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	detachedPlanPhasePendingApproval = "plan_pending_approval"
)

type detachedPlanProgress struct {
	Phase        string `json:"phase"`
	ReactTaskID  string `json:"react_task_id"`
	PlanPayload  string `json:"plan_payload"`
	PlanFacts    string `json:"plan_facts"`
	PlanDocument string `json:"plan_document"`
	UpdatedAt    int64  `json:"updated_at"`
}

// PublishDetachedPlan emits a non-blocking detached plan review panel and persists the plan into session storage.
func (r *ReAct) PublishDetachedPlan(ctx context.Context, input *aicommon.ExecutePlanInput, reactTaskID string) (string, error) {
	if input == nil {
		return "", utils.Error("execute plan input is nil")
	}
	if strings.TrimSpace(input.PlanData) == "" {
		return "", utils.Error("plan data is empty")
	}
	if strings.TrimSpace(r.config.PersistentSessionId) == "" {
		return "", utils.Error("persistent session id is empty")
	}
	if r.config.GetDB() == nil {
		return "", utils.Error("db is nil")
	}

	if ctx == nil {
		ctx = r.config.Ctx
	}

	coordinatorID := uuid.New().String()
	planPayload := enhancePlanPayloadWithTaskUserInput(input.PlanPayload, r.GetCurrentTask())

	rootTask, err := r.buildRootTaskForDetachedPlan(ctx, planPayload, input)
	if err != nil {
		return "", err
	}

	planRsp := &aid.PlanResponse{
		RootTask: rootTask,
		Facts:    input.PlanFacts,
		Document: input.PlanDocument,
	}
	if err := r.saveDetachedPlanSession(coordinatorID, reactTaskID, planPayload, rootTask, input); err != nil {
		return "", err
	}

	reqs := map[string]any{
		"id":             coordinatorID,
		"coordinator_id": coordinatorID,
		"session_id":     r.config.PersistentSessionId,
		"re-act_id":      r.config.Id,
		"re-act_task":    reactTaskID,
		"plan_payload":   planPayload,
		"detached":       true,
		"selectors":      detachedPlanSelectors(coordinatorID),
		"plans":          planRsp,
		"plans_id":       uuid.New().String(),
	}
	r.EmitJSON(schema.EVENT_TYPE_DETACHED_PLAN_REQUIRE, "detached-plan", reqs)
	log.Infof("detached plan published: coordinator=%s session=%s react_task=%s", coordinatorID, r.config.PersistentSessionId, reactTaskID)
	return coordinatorID, nil
}

func (r *ReAct) buildRootTaskForDetachedPlan(ctx context.Context, planPayload string, input *aicommon.ExecutePlanInput) (*aid.AiTask, error) {
	baseOpts := aicommon.ConvertConfigToOptions(r.config)
	baseOpts = append(baseOpts, aicommon.WithContext(ctx))
	cod, err := newCoordinatorContextForPlanExec(ctx, planPayload, baseOpts...)
	if err != nil {
		return nil, utils.Errorf("failed to create coordinator for detached plan: %v", err)
	}
	rootTask, err := cod.BuildRootTaskFromPlanData(input.PlanData, planPayload)
	if err != nil {
		return nil, err
	}
	return rootTask, nil
}

func (r *ReAct) saveDetachedPlanSession(
	coordinatorID, reactTaskID, planPayload string,
	rootTask *aid.AiTask,
	input *aicommon.ExecutePlanInput,
) error {
	progress := &detachedPlanProgress{
		Phase:        detachedPlanPhasePendingApproval,
		ReactTaskID:  reactTaskID,
		PlanPayload:  planPayload,
		PlanFacts:    input.PlanFacts,
		PlanDocument: input.PlanDocument,
		UpdatedAt:    time.Now().Unix(),
	}
	record := &schema.AISessionPlanAndExec{
		SessionID:     r.config.PersistentSessionId,
		CoordinatorID: coordinatorID,
		TaskTree:      string(utils.Jsonify(rootTask)),
		TaskProgress:  string(utils.Jsonify(progress)),
	}
	return yakit.CreateOrUpdateAISessionPlanAndExec(r.config.GetDB(), record)
}

func detachedPlanSelectors(coordinatorID string) []map[string]any {
	return []map[string]any{
		{
			"id":                 fmt.Sprintf("detached-plan-freedom-%s", coordinatorID),
			"value":              "freedom-review",
			"prompt":             "审阅模式",
			"prompt_english":     "User freely review the plan, can add more details or modify the plan",
			"allow_extra_prompt": true,
		},
		{
			"id":                 fmt.Sprintf("detached-plan-execute-%s", coordinatorID),
			"value":              "execute",
			"prompt":             "允许执行",
			"prompt_english":     "Allow plan execution",
			"allow_extra_prompt": false,
		},
		{
			"id":                 fmt.Sprintf("detached-plan-close-%s", coordinatorID),
			"value":              "close",
			"prompt":             "关闭",
			"prompt_english":     "Close review panel",
			"allow_extra_prompt": false,
		},
	}
}

func (r *ReAct) HandleSyncTypeExecuteDetachedPlanEvent(event *ypb.AIInputEvent) error {
	coordinatorID, sessionID, reactTaskID, input, err := parseExecuteDetachedPlanParams(event.SyncJsonInput)
	if err != nil {
		r.EmitSyncEventError("execute_detached_plan", err, event.SyncID)
		return nil
	}
	if sessionID == "" {
		sessionID = r.config.PersistentSessionId
	}
	if coordinatorID == "" {
		r.EmitSyncEventError("execute_detached_plan", errors.New("coordinator_id is empty"), event.SyncID)
		return nil
	}
	if input == nil || strings.TrimSpace(input.PlanData) == "" {
		r.EmitSyncEventError("execute_detached_plan", errors.New("plan_data is empty"), event.SyncID)
		return nil
	}
	db := r.config.GetDB()
	if db == nil {
		r.EmitSyncEventError("execute_detached_plan", errors.New("db is nil"), event.SyncID)
		return nil
	}

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	if err != nil || record == nil {
		if err == nil {
			err = errors.New("detached plan session record not found")
		}
		r.EmitSyncEventError("execute_detached_plan", err, event.SyncID)
		return nil
	}
	if sessionID != "" && record.SessionID != "" && record.SessionID != sessionID {
		r.EmitSyncEventError("execute_detached_plan", errors.New("session_id mismatch for detached plan"), event.SyncID)
		return nil
	}

	planPayload := strings.TrimSpace(input.PlanPayload)
	rootTask, err := r.buildRootTaskForDetachedPlan(r.config.Ctx, planPayload, input)
	if err != nil {
		r.EmitSyncEventError("execute_detached_plan", err, event.SyncID)
		return nil
	}

	record.TaskTree = string(utils.Jsonify(rootTask))
	record.TaskProgress = string(utils.Jsonify(&aid.PlanAndExecProgress{
		Phase:     aid.Phase_PlanReady,
		UpdatedAt: time.Now().Unix(),
	}))
	if sessionID != "" {
		record.SessionID = sessionID
	}
	if err := yakit.CreateOrUpdateAISessionPlanAndExec(db, record); err != nil {
		r.EmitSyncEventError("execute_detached_plan", err, event.SyncID)
		return nil
	}

	approvedInput := &aicommon.ExecutePlanInput{
		PlanPayload:  planPayload,
		PlanData:     input.PlanData,
		PlanFacts:    input.PlanFacts,
		PlanDocument: input.PlanDocument,
	}

	r.EmitSyncEvent("execute_detached_plan", map[string]any{
		"started":        true,
		"session_id":     record.SessionID,
		"coordinator_id": coordinatorID,
		"react_task_id":  reactTaskID,
	}, event.SyncID)

	go r.AsyncRecoverPlanAndExecute(r.config.Ctx, coordinatorID, "", func(err error) {
		if err != nil {
			log.Errorf("execute detached plan via recovery failed: coordinator=%s err=%v", coordinatorID, err)
		}
	},
		WithInvokePlanAndExecutePlanPayload(planPayload),
		WithInvokePlanAndExecuteExecutePlanInput(approvedInput),
	)
	return nil
}

func parseExecuteDetachedPlanParams(syncJSON string) (coordinatorID, sessionID, reactTaskID string, input *aicommon.ExecutePlanInput, err error) {
	if strings.TrimSpace(syncJSON) == "" {
		return "", "", "", nil, errors.New("sync json input is empty")
	}
	var params map[string]any
	if err = json.Unmarshal([]byte(syncJSON), &params); err != nil {
		return "", "", "", nil, fmt.Errorf("failed to parse execute detached plan params: %w", err)
	}
	coordinatorID = utils.InterfaceToString(params["coordinator_id"])
	sessionID = utils.InterfaceToString(params["session_id"])
	reactTaskID = utils.InterfaceToString(params["react_task_id"])
	input = &aicommon.ExecutePlanInput{
		PlanPayload:  utils.InterfaceToString(params["plan_payload"]),
		PlanData:     utils.InterfaceToString(params["plan_data"]),
		PlanFacts:    utils.InterfaceToString(params["plan_facts"]),
		PlanDocument: utils.InterfaceToString(params["plan_document"]),
	}
	return coordinatorID, sessionID, reactTaskID, input, nil
}
