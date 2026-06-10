package aireact

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type planCoordinatorSession struct {
	r                   *ReAct
	cod                 *aid.Coordinator
	uid                 string
	planPayload         string
	planInput           *aicommon.ExecutePlanInput
	forceManualReview   bool
	eventParams         map[string]any
	planCtx             context.Context
	cancel              context.CancelFunc
	unregisterMirror    func()
	unsubscribeHotpatch func()
	approvedInput       *aicommon.ExecutePlanInput
	endOnce             sync.Once
}

var _ aicommon.PlanCoordinatorSession = (*planCoordinatorSession)(nil)

func (s *planCoordinatorSession) CoordinatorID() string {
	return s.uid
}

func (s *planCoordinatorSession) ApprovedPlanInput() *aicommon.ExecutePlanInput {
	return s.approvedInput
}

func (s *planCoordinatorSession) ReviewPlan(ctx context.Context) error {
	if s == nil || s.cod == nil || s.planInput == nil {
		return utils.Error("plan coordinator session is not initialized")
	}
	if strings.TrimSpace(s.planInput.PlanData) == "" {
		return utils.Error("plan data is empty")
	}

	rootTask, err := s.cod.BuildRootTaskFromPlanData(s.planInput.PlanData, s.planPayload)
	if err != nil {
		s.fail(err)
		return utils.Errorf("failed to build root task for plan review: %v", err)
	}

	planRsp := &aid.PlanResponse{
		RootTask: rootTask,
		Facts:    s.planInput.PlanFacts,
		Document: s.planInput.PlanDocument,
	}

	if ctx == nil {
		ctx = s.planCtx
	}
	approvedRsp, err := s.cod.ReviewPlanThroughUser(ctx, s.planPayload, planRsp)
	if err != nil {
		s.fail(err)
		return err
	}
	if approvedRsp == nil || approvedRsp.RootTask == nil {
		err = utils.Error("approved plan root task is nil")
		s.fail(err)
		return err
	}

	if err := s.cod.CommitApprovedPlan(approvedRsp.RootTask, approvedRsp.Facts, approvedRsp.Document); err != nil {
		s.fail(err)
		return utils.Errorf("failed to commit approved plan: %v", err)
	}

	reviewed := executePlanInputFromPlanResponse(s.planPayload, approvedRsp, s.planInput)
	if reviewed == nil || strings.TrimSpace(reviewed.PlanData) == "" {
		err = utils.Error("approved plan data is empty after review")
		s.fail(err)
		return err
	}
	s.approvedInput = reviewed
	log.Infof("plan review approved on coordinator %s, ready to execute asynchronously", s.uid)
	return nil
}

func (s *planCoordinatorSession) Close() {
	if s == nil {
		return
	}
	if s.unregisterMirror != nil {
		s.unregisterMirror()
		s.unregisterMirror = nil
	}
	if s.unsubscribeHotpatch != nil {
		s.unsubscribeHotpatch()
		s.unsubscribeHotpatch = nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *planCoordinatorSession) fail(err error) {
	if s == nil || err == nil {
		return
	}
	s.endOnce.Do(func() {
		if s.r != nil {
			s.r.EmitPlanExecFail(err.Error())
			s.r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, s.r.config.Id, s.eventParams)
		}
	})
}

// BeginPlanCoordinatorSession creates a plan-exec coordinator from external plan loop output,
// emits start_plan_and_execution for task-channel UI routing, and keeps the coordinator alive
// until ReviewPlan completes.
func (r *ReAct) BeginPlanCoordinatorSession(
	ctx context.Context,
	input *aicommon.ExecutePlanInput,
	forceManualReview bool,
) (aicommon.PlanCoordinatorSession, error) {
	if input == nil {
		return nil, utils.Error("execute plan input is nil")
	}
	if strings.TrimSpace(input.PlanData) == "" {
		return nil, utils.Error("plan data is empty")
	}

	if ctx == nil {
		ctx = r.config.Ctx
	}
	planCtx, cancel := context.WithCancel(ctx)

	task := r.GetCurrentTask()
	planPayload := enhancePlanPayloadWithTaskUserInput(input.PlanPayload, task)

	uid := uuid.New().String()
	reactTaskID := ""
	if task != nil {
		reactTaskID = task.GetId()
	}
	eventParams := map[string]any{
		"re-act_id":      r.config.Id,
		"re-act_task":    reactTaskID,
		"coordinator_id": uid,
		"mode":           "request_plan",
	}
	r.EmitJSON(schema.EVENT_TYPE_START_PLAN_AND_EXECUTION, r.config.Id, eventParams)

	inputChannel, unregisterMirror := r.registerPlanExecInputMirror(uid)
	hotpatchChan := r.config.HotPatchBroadcaster.Subscribe()

	baseOpts := buildPlanExecBaseOptions(r, uid, planCtx, inputChannel, hotpatchChan)
	if forceManualReview {
		baseOpts = append(baseOpts, aicommon.WithForceManualPlanReview(true))
	}

	cod, err := newCoordinatorContextForPlanExec(planCtx, planPayload, baseOpts...)
	if err != nil {
		cancel()
		unregisterMirror()
		r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)
		r.EmitPlanExecFail(err.Error())
		r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, r.config.Id, eventParams)
		return nil, utils.Errorf("failed to create coordinator for plan review: %v", err)
	}

	return &planCoordinatorSession{
		r:                 r,
		cod:               cod,
		uid:               uid,
		planPayload:       planPayload,
		planInput:         input,
		forceManualReview: forceManualReview,
		eventParams:       eventParams,
		planCtx:           planCtx,
		cancel:            cancel,
		unregisterMirror:  unregisterMirror,
		unsubscribeHotpatch: func() {
			r.config.HotPatchBroadcaster.Unsubscribe(hotpatchChan)
		},
	}, nil
}

// AsyncExecuteCod asynchronously executes an approved plan on an existing coordinator.
func (r *ReAct) AsyncExecuteCod(ctx context.Context, coordinatorID string, onFinished func(error)) {
	if strings.TrimSpace(coordinatorID) == "" {
		if onFinished != nil {
			onFinished(utils.Error("coordinator id is empty"))
		}
		return
	}

	task := r.GetCurrentTask()
	reactTaskID := ""
	if task != nil {
		reactTaskID = task.GetId()
	}
	eventParams := map[string]any{
		"re-act_id":      r.config.Id,
		"re-act_task":    reactTaskID,
		"coordinator_id": coordinatorID,
		"mode":           "execute_cod",
	}

	go func() {
		var finalErr error
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("AsyncExecuteCod panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
			if finalErr != nil {
				r.EmitPlanExecFail(finalErr.Error())
			}
			r.EmitJSON(schema.EVENT_TYPE_END_PLAN_AND_EXECUTION, r.config.Id, eventParams)
			if onFinished != nil {
				onFinished(finalErr)
			}
		}()

		if ctx == nil {
			ctx = r.config.Ctx
		}

		execDone := make(chan struct{})
		finalErr = r.invokePlanExecuteOnly(execDone, ctx,
			WithInvokePlanAndExecuteCoordinatorID(coordinatorID),
			WithInvokePlanAndExecuteTask(task),
		)
		<-execDone
	}()
}

func executePlanInputFromPlanResponse(planPayload string, rsp *aid.PlanResponse, fallback *aicommon.ExecutePlanInput) *aicommon.ExecutePlanInput {
	if rsp == nil || rsp.RootTask == nil {
		return nil
	}
	facts := rsp.Facts
	document := rsp.Document
	if facts == "" && fallback != nil {
		facts = fallback.PlanFacts
	}
	if document == "" && fallback != nil {
		document = fallback.PlanDocument
	}
	return &aicommon.ExecutePlanInput{
		PlanPayload:  planPayload,
		PlanData:     aid.SerializeRootTaskToPlanData(rsp.RootTask),
		PlanFacts:    facts,
		PlanDocument: document,
	}
}
