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

type executeDetachedPlanRequest struct {
	CoordinatorID string
	SessionID     string
	ReactTaskID   string
	Plans         *aid.PlanResponse
	LegacyInput   *aicommon.ExecutePlanInput
}

func (r *ReAct) HandleSyncTypeExecuteDetachedPlanEvent(event *ypb.AIInputEvent) error {
	req, err := parseExecuteDetachedPlanRequest(event.SyncJsonInput)
	if err != nil {
		r.EmitSyncEventError("execute_detached_plan", err, event.SyncID)
		return nil
	}
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = r.config.PersistentSessionId
	}
	if req.CoordinatorID == "" {
		r.EmitSyncEventError("execute_detached_plan", errors.New("coordinator_id is empty"), event.SyncID)
		return nil
	}
	db := r.config.GetDB()
	if db == nil {
		r.EmitSyncEventError("execute_detached_plan", errors.New("db is nil"), event.SyncID)
		return nil
	}

	record, err := yakit.GetAISessionPlanAndExecByCoordinatorID(db, req.CoordinatorID)
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

	planPayload, approvedInput, reactTaskID, err := r.resolveExecuteDetachedPlanInput(record, req)
	if err != nil {
		r.EmitSyncEventError("execute_detached_plan", err, event.SyncID)
		return nil
	}

	rootTask, err := r.buildRootTaskForDetachedPlan(r.config.Ctx, planPayload, approvedInput)
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

	r.EmitSyncEvent("execute_detached_plan", map[string]any{
		"started":        true,
		"session_id":     record.SessionID,
		"coordinator_id": req.CoordinatorID,
		"react_task_id":  reactTaskID,
	}, event.SyncID)

	go r.AsyncRecoverPlanAndExecute(r.config.Ctx, req.CoordinatorID, "", func(err error) {
		if err != nil {
			log.Errorf("execute detached plan via recovery failed: coordinator=%s err=%v", req.CoordinatorID, err)
		}
	},
		WithInvokePlanAndExecutePlanPayload(planPayload),
		WithInvokePlanAndExecuteExecutePlanInput(approvedInput),
	)
	return nil
}

func parseExecuteDetachedPlanRequest(syncJSON string) (*executeDetachedPlanRequest, error) {
	if strings.TrimSpace(syncJSON) == "" {
		return nil, errors.New("sync json input is empty")
	}
	var params map[string]any
	if err := json.Unmarshal([]byte(syncJSON), &params); err != nil {
		return nil, fmt.Errorf("failed to parse execute detached plan params: %w", err)
	}

	req := &executeDetachedPlanRequest{
		CoordinatorID: utils.InterfaceToString(params["coordinator_id"]),
		SessionID:     utils.InterfaceToString(params["session_id"]),
		ReactTaskID:   utils.InterfaceToString(params["react_task_id"]),
	}
	if rawPlans, ok := params["plans"]; ok && rawPlans != nil {
		plansDTO, err := parseDetachedPlansDTO(rawPlans)
		if err != nil {
			return nil, err
		}
		req.Plans = &aid.PlanResponse{
			RootTask: detachedPlanTaskDTOToAiTask(&plansDTO.RootTask),
			Facts:    plansDTO.Facts,
			Document: plansDTO.Document,
		}
	}

	legacyInput := &aicommon.ExecutePlanInput{
		PlanPayload:  utils.InterfaceToString(params["plan_payload"]),
		PlanData:     utils.InterfaceToString(params["plan_data"]),
		PlanFacts:    utils.InterfaceToString(params["plan_facts"]),
		PlanDocument: utils.InterfaceToString(params["plan_document"]),
	}
	if strings.TrimSpace(legacyInput.PlanData) != "" ||
		strings.TrimSpace(legacyInput.PlanPayload) != "" ||
		strings.TrimSpace(legacyInput.PlanFacts) != "" ||
		strings.TrimSpace(legacyInput.PlanDocument) != "" {
		req.LegacyInput = legacyInput
	}
	return req, nil
}

func loadDetachedPlanProgress(raw string) *detachedPlanProgress {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return &detachedPlanProgress{}
	}
	progress := &detachedPlanProgress{}
	if err := json.Unmarshal([]byte(raw), progress); err != nil {
		return &detachedPlanProgress{}
	}
	return progress
}

type detachedPlanTaskDTO struct {
	TaskId             string                `json:"task_id"`
	Index              string                `json:"index"`
	Name               string                `json:"name"`
	Goal               string                `json:"goal"`
	SemanticIdentifier string                `json:"semantic_identifier"`
	DependsOn          []string              `json:"depends_on,omitempty"`
	Subtasks           []detachedPlanTaskDTO `json:"subtasks,omitempty"`
}

type detachedPlansDTO struct {
	RootTask detachedPlanTaskDTO `json:"root_task"`
	Facts    string              `json:"facts"`
	Document string              `json:"document"`
}

func parseDetachedPlansDTO(raw any) (*detachedPlansDTO, error) {
	if raw == nil {
		return nil, errors.New("plans is nil")
	}
	plans := &detachedPlansDTO{}
	if err := json.Unmarshal(utils.Jsonify(raw), plans); err != nil {
		return nil, fmt.Errorf("failed to parse plans: %w", err)
	}
	if strings.TrimSpace(plans.RootTask.Name) == "" && len(plans.RootTask.Subtasks) == 0 {
		return nil, errors.New("plans.root_task is empty")
	}
	return plans, nil
}

func parseDetachedPlanRootTaskDTO(raw string) (*detachedPlanTaskDTO, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("task tree is empty")
	}
	rootTask := &detachedPlanTaskDTO{}
	if err := json.Unmarshal([]byte(raw), rootTask); err != nil {
		return nil, fmt.Errorf("failed to parse task tree: %w", err)
	}
	if strings.TrimSpace(rootTask.Name) == "" && len(rootTask.Subtasks) == 0 {
		return nil, errors.New("task tree is empty")
	}
	return rootTask, nil
}

func detachedPlanTaskDTOToAiTask(dto *detachedPlanTaskDTO) *aid.AiTask {
	if dto == nil {
		return nil
	}
	task := &aid.AiTask{
		TaskId:             strings.TrimSpace(dto.TaskId),
		Index:              dto.Index,
		Name:               dto.Name,
		Goal:               dto.Goal,
		SemanticIdentifier: dto.SemanticIdentifier,
		DependsOn:          dto.DependsOn,
	}
	if task.TaskId == "" && task.Index != "" {
		task.TaskId = fmt.Sprintf("pe-task-%s", task.Index)
	}
	for i := range dto.Subtasks {
		task.Subtasks = append(task.Subtasks, detachedPlanTaskDTOToAiTask(&dto.Subtasks[i]))
	}
	return task
}

func (r *ReAct) resolveExecuteDetachedPlanInput(
	record *schema.AISessionPlanAndExec,
	req *executeDetachedPlanRequest,
) (planPayload string, input *aicommon.ExecutePlanInput, reactTaskID string, err error) {
	if record == nil {
		return "", nil, "", errors.New("detached plan session record is nil")
	}
	if req == nil {
		return "", nil, "", errors.New("execute detached plan request is nil")
	}

	progress := loadDetachedPlanProgress(record.TaskProgress)
	planPayload = strings.TrimSpace(progress.PlanPayload)
	reactTaskID = strings.TrimSpace(req.ReactTaskID)
	if reactTaskID == "" {
		reactTaskID = strings.TrimSpace(progress.ReactTaskID)
	}

	if req.LegacyInput != nil && strings.TrimSpace(req.LegacyInput.PlanData) != "" {
		if strings.TrimSpace(req.LegacyInput.PlanPayload) != "" {
			planPayload = strings.TrimSpace(req.LegacyInput.PlanPayload)
		}
		return planPayload, req.LegacyInput, reactTaskID, nil
	}

	var rootTask *aid.AiTask
	var planFacts, planDocument string
	switch {
	case req.Plans != nil && req.Plans.RootTask != nil:
		rootTask = req.Plans.RootTask
		planFacts = strings.TrimSpace(req.Plans.Facts)
		planDocument = strings.TrimSpace(req.Plans.Document)
	default:
		rootDTO, err := parseDetachedPlanRootTaskDTO(record.TaskTree)
		if err != nil {
			return "", nil, "", err
		}
		rootTask = detachedPlanTaskDTOToAiTask(rootDTO)
		planFacts = strings.TrimSpace(progress.PlanFacts)
		planDocument = strings.TrimSpace(progress.PlanDocument)
	}
	if planFacts == "" {
		planFacts = strings.TrimSpace(progress.PlanFacts)
	}
	if planDocument == "" {
		planDocument = strings.TrimSpace(progress.PlanDocument)
	}

	planData := strings.TrimSpace(aid.SerializeRootTaskToPlanData(rootTask))
	if planData == "" {
		return "", nil, "", errors.New("plans is empty")
	}
	if len(rootTask.Subtasks) <= 0 {
		return "", nil, "", errors.New("plan has no subtasks")
	}

	return planPayload, &aicommon.ExecutePlanInput{
		PlanPayload:  planPayload,
		PlanData:     planData,
		PlanFacts:    planFacts,
		PlanDocument: planDocument,
	}, reactTaskID, nil
}
