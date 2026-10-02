package aireact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator_legacy"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	detachedPlanPhasePendingApproval = aicommon.PlanExecPhaseDetachedPendingApproval
)

type detachedPlanProgress struct {
	PlanEngine   string `json:"plan_engine,omitempty"`
	Phase        string `json:"phase"`
	ReactTaskID  string `json:"react_task_id"`
	PlanPayload  string `json:"plan_payload"`
	PlanDocument string `json:"plan_document"`
	UpdatedAt    int64  `json:"updated_at"`
}

// PublishDetachedPlan emits a non-blocking detached plan review panel and persists the plan into session storage.
func (r *ReAct) publishLegacyDetachedPlan(ctx context.Context, input *aicommon.ExecutePlanInput, reactTaskID string) (string, error) {
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

	planRsp := &coordinator_legacy.PlanResponse{
		RootTask: rootTask,
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
	r.AddToTimeline("DETACHED_PLAN", coordinator_legacy.FormatDetachedPlanTimelineContent(
		coordinatorID,
		r.config.PersistentSessionId,
		reactTaskID,
		rootTask,
		input,
	))
	log.Infof("detached plan published: coordinator=%s session=%s react_task=%s", coordinatorID, r.config.PersistentSessionId, reactTaskID)
	return coordinatorID, nil
}

func (r *ReAct) buildRootTaskForDetachedPlan(ctx context.Context, planPayload string, input *aicommon.ExecutePlanInput) (*coordinator_legacy.AiTask, error) {
	baseOpts := aicommon.ConvertConfigToOptions(r.config)
	baseOpts = append(baseOpts, aicommon.WithContext(ctx), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithLiteForgeExecutor(nil))
	cod, err := newCoordinatorContextForPlanExec(ctx, planPayload, baseOpts...)
	if err != nil {
		return nil, utils.Errorf("failed to create coordinator for detached plan: %v", err)
	}
	return cod.BuildRootTaskFromPlanData(input.PlanData, planPayload)
}

func (r *ReAct) saveDetachedPlanSession(
	coordinatorID, reactTaskID, planPayload string,
	rootTask *coordinator_legacy.AiTask,
	input *aicommon.ExecutePlanInput,
) error {
	progress := &detachedPlanProgress{
		PlanEngine:   "coordinator_legacy",
		Phase:        detachedPlanPhasePendingApproval,
		ReactTaskID:  reactTaskID,
		PlanPayload:  planPayload,
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
			"id":                 fmt.Sprintf("detached-plan-execute-%s", coordinatorID),
			"value":              "continue",
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
	reject := func(err error) {
		log.Warnf("detached plan approval rejected: session=%s coordinator=%s sync=%s error=%v", r.config.PersistentSessionId, coordinatorID, event.SyncID, err)
		r.EmitSyncEventError("execute_detached_plan", err, event.SyncID)
		// Existing clients close the review panel before sending approval and
		// do not display errors carried only by a sync acknowledgement.
		r.EmitError("计划未能开始执行：%v", err)
	}
	if err != nil {
		reject(err)
		return nil
	}
	if sessionID == "" {
		sessionID = r.config.PersistentSessionId
	}
	if coordinatorID == "" {
		reject(errors.New("coordinator_id is empty"))
		return nil
	}
	db := r.config.GetDB()
	if db == nil {
		reject(errors.New("db is nil"))
		return nil
	}

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, coordinatorID)
	if err != nil || record == nil {
		if err == nil {
			err = errors.New("detached plan session record not found")
		}
		reject(err)
		return nil
	}
	if sessionID != "" && record.SessionID != "" && record.SessionID != sessionID {
		reject(errors.New("session_id mismatch for detached plan"))
		return nil
	}
	channel, err := r.coordinatorChannel(coordinatorID)
	if err != nil {
		reject(err)
		return nil
	}

	var detectPlan detachedPlanProgress
	json.Unmarshal([]byte(record.TaskProgress), &detectPlan)
	if channel == coordinator.Name && detectPlan.PlanDocument == "" {
		var native coordinator.Progress
		if json.Unmarshal([]byte(record.TaskProgress), &native) == nil && native.CoordinatorState != nil {
			plan := native.CoordinatorState.Approved
			if plan == nil {
				plan = native.CoordinatorState.Draft
			}
			if plan != nil {
				detectPlan.PlanDocument = plan.Document
			}
		}
	}
	if detectPlan.Phase != detachedPlanPhasePendingApproval && detectPlan.Phase != coordinator_legacy.Phase_PlanReady {
		reject(errors.New("plan is already executing or is no longer pending approval"))
		return nil
	}

	if sessionID != "" {
		record.SessionID = sessionID
	}
	if input.PlanPayload == "" {
		input.PlanPayload = detectPlan.PlanPayload
	}
	if input.PlanDocument == "" {
		input.PlanDocument = detectPlan.PlanDocument
	}
	if input.PlanData == "" {
		input.PlanData = record.TaskTree
	}

	approvedInput := &aicommon.ExecutePlanInput{
		PlanPayload:  input.PlanPayload,
		PlanData:     input.PlanData,
		PlanDocument: input.PlanDocument,
	}
	// Yakit submits the edited plans.root_task. Validate it before changing the
	// persisted phase or acknowledging execution, and keep the approved edit.
	var root any
	if channel == coordinator.Name {
		var plan *coordinator.Plan
		plan, err = coordinator.ParseReviewedPlan(approvedInput.PlanData, approvedInput.PlanDocument, nil)
		if err == nil {
			root = plan.Tree
			approvedInput.PlanData = string(plan.Tree)
		}
	} else {
		root, err = r.buildRootTaskForDetachedPlan(r.config.GetContext(), input.PlanPayload, approvedInput)
	}
	if err != nil {
		reject(err)
		return nil
	}
	oldTree, oldProgress := record.TaskTree, record.TaskProgress
	tree, err := json.Marshal(root)
	if err != nil {
		reject(err)
		return nil
	}
	record.TaskTree = string(tree)
	record.TaskProgress = string(utils.Jsonify(map[string]any{"plan_engine": channel, "phase": coordinator_legacy.Phase_NotCompleted, "updated_at": time.Now().Unix()}))
	if err := yakit.CreateOrUpdateAISessionPlanAndExec(db, record); err != nil {
		reject(err)
		return nil
	}

	// Reuse the execution queue to bypass planning for the approved plan.
	// This is its first execution, not a recovery of interrupted work.
	userInput := formatPlanExecutionTaskInput(record, "执行已批准计划")
	recoveryTask := aicommon.NewStatefulTaskBase(
		formatRecoveryTaskID(coordinatorID),
		userInput,
		r.config.GetContext(),
		r.Emitter,
	)
	recoveryTask.SetTaskKind(aicommon.AITaskKind_Recovery)
	recoveryTask.SetRecoveryData(&aicommon.RecoveryTaskData{
		CoordinatorID:    coordinatorID,
		StartTaskID:      "",
		ExecutePlanInput: approvedInput,
	})
	recoveryTask.SetStatus(aicommon.AITaskState_Queueing)
	if err := r.taskQueue.Append(recoveryTask); err != nil {
		record.TaskTree, record.TaskProgress = oldTree, oldProgress
		_ = yakit.CreateOrUpdateAISessionPlanAndExec(db, record)
		reject(err)
		return nil
	}
	log.Infof("detached plan approval queued: session=%s coordinator=%s sync=%s task=%s", record.SessionID, coordinatorID, event.SyncID, recoveryTask.GetId())
	r.EmitSyncEvent("execute_detached_plan", map[string]any{"started": true, "session_id": record.SessionID, "coordinator_id": coordinatorID, "react_task_id": reactTaskID}, event.SyncID)
	return nil
}

// extractRecoveryTaskUserInput builds a human-readable description for a
// recovery task from the persisted plan record.  It tries the root task
// Name/Goal stored in TaskTree first, then falls back to PlanPayload from
// the detached-plan progress, and finally to a generic message.
func extractRecoveryTaskUserInput(record *schema.AISessionPlanAndExec) string {
	return formatPlanExecutionTaskInput(record, "恢复执行计划")
}

func formatPlanExecutionTaskInput(record *schema.AISessionPlanAndExec, action string) string {
	// Both coordinator versions persist the root name/goal in TaskTree.
	if record != nil && strings.TrimSpace(record.TaskTree) != "" {
		var root struct {
			Name string `json:"name"`
			Goal string `json:"goal"`
		}
		if err := json.Unmarshal([]byte(record.TaskTree), &root); err == nil {
			name := strings.TrimSpace(root.Name)
			goal := strings.TrimSpace(root.Goal)
			if goal != "" {
				if name != "" {
					return fmt.Sprintf("%s: %s — %s", action, name, goal)
				}
				return fmt.Sprintf("%s: %s", action, goal)
			}
			if name != "" {
				return fmt.Sprintf("%s: %s", action, name)
			}
		}
	}
	// Fall back to PlanPayload from TaskProgress (detachedPlanProgress)
	if record != nil && strings.TrimSpace(record.TaskProgress) != "" {
		var prog detachedPlanProgress
		if err := json.Unmarshal([]byte(record.TaskProgress), &prog); err == nil {
			if payload := strings.TrimSpace(prog.PlanPayload); payload != "" {
				return fmt.Sprintf("%s: %s", action, payload)
			}
		}
	}
	return action
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
		PlanDocument: utils.InterfaceToString(params["plan_document"]),
	}
	if plans, ok := params["plans"].(map[string]any); ok {
		if raw, exists := plans["document"]; exists {
			var valid bool
			input.PlanDocument, valid = raw.(string)
			if !valid {
				return "", "", "", nil, errors.New("plans.document must be a string")
			}
		}
		if root, exists := plans["root_task"]; exists {
			object, ok := root.(map[string]any)
			if !ok || len(object) == 0 {
				return "", "", "", nil, errors.New("plans.root_task must be a nonempty object")
			}
			data, marshalErr := json.Marshal(object)
			if marshalErr != nil {
				return "", "", "", nil, marshalErr
			}
			input.PlanData = string(data)
		}
	}
	return coordinatorID, sessionID, reactTaskID, input, nil
}
