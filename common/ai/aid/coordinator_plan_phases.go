package aid

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const Phase_PlanReady = "plan_ready"

// runPlanPhaseThroughReview runs plan loop, user review, and persists plan_ready state.
// It does not execute subtasks.
func (c *Coordinator) runPlanPhaseThroughReview() error {
	c.planUserStatus("正在梳理任务目标", "Understanding the task goals", aicommon.WithStatusCode("plan.understanding"))
	c.EmitInfo("start to create plan request")
	planReq, err := c.createPlanRequest(c.userInput)
	if err != nil {
		c.planUserStatus("暂时没能整理出执行方案", "Unable to prepare an execution plan", aicommon.WithStatusCode("plan.failed"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("create planRequest failed: %v", err)
		return utils.Errorf("coordinator: create planRequest failed: %v", err)
	}

	c.planUserStatus("正在整理执行方案", "Preparing the execution plan", aicommon.WithStatusCode("plan.generating"))
	c.EmitInfo("start to invoke plan request")
	rsp, err := planReq.Invoke()
	if err != nil {
		c.planUserStatus("暂时没能整理出执行方案", "Unable to prepare an execution plan", aicommon.WithStatusCode("plan.failed"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("invoke planRequest failed(first): %v", err)
		return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
	}
	rsp, err = planReq.ensurePlanExecutableDAG(rsp)
	if err != nil {
		c.planUserStatus("步骤之间存在冲突，正在重新整理", "Some steps conflict and need to be reorganized", aicommon.WithStatusCode("plan.order_failed"), aicommon.WithStatusState(aicommon.StatusStateRecovering))
		c.EmitError("validate generated plan executable DAG failed: %v", err)
		return err
	}

	c.planUserStatus("执行方案已经准备好，等你确认", "The execution plan is ready for your review", aicommon.WithStatusCode("plan.awaiting_review"), aicommon.WithStatusState(aicommon.StatusStateWaiting))
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	c.EmitRequireReviewForPlan(rsp, ep.GetId())
	c.DoWaitAgree(c.GetContext(), ep)
	params := ep.GetParams()
	c.ReleaseInteractiveEvent(ep.GetId(), params)
	if params == nil {
		c.planUserStatus("没有收到有效的确认结果", "No valid review response was received", aicommon.WithStatusCode("plan.review_failed"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("user review params is nil, plan failed")
		return utils.Errorf("coordinator: user review params is nil")
	}

	c.planUserStatus("正在根据你的意见调整方案", "Updating the plan based on your feedback", aicommon.WithStatusCode("plan.revising"))
	c.EmitInfo("start to handle review plan response")
	rsp, err = planReq.handleReviewPlanResponse(rsp, params)
	if err != nil {
		c.planUserStatus("暂时没能应用这次调整", "Unable to apply the requested changes", aicommon.WithStatusCode("plan.revision_failed"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("handle review plan response failed: %v", err)
		return utils.Errorf("coordinator: handle review plan response failed: %v", err)
	}
	rsp, err = planReq.ensurePlanExecutableDAG(rsp)
	if err != nil {
		c.planUserStatus("正在重新校对步骤顺序", "Rechecking the order of the steps", aicommon.WithStatusCode("plan.reordering"), aicommon.WithStatusState(aicommon.StatusStateRecovering))
		c.EmitError("validate reviewed plan executable DAG failed: %v", err)
		return err
	}

	if rsp.RootTask == nil {
		c.planUserStatus("执行方案还不够完整", "The execution plan is incomplete", aicommon.WithStatusCode("plan.invalid"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("root aiTask is nil, plan failed")
		return utils.Errorf("coordinator: root aiTask is nil")
	}

	c.planUserStatus("正在准备执行计划", "Preparing to execute the plan", aicommon.WithStatusCode("plan.preparing"))
	root := rsp.RootTask
	c.rootTask = root
	c.ContextProvider.StoreRootTask(root)
	c.savePlanAndExecState(Phase_PlanReady, nil)
	if len(root.Subtasks) <= 0 {
		c.planUserStatus("执行方案中没有可推进的步骤", "The plan has no actionable steps", aicommon.WithStatusCode("plan.empty"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("no subtasks found, this task is not a valid task")
		return utils.Errorf("coordinator: no subtasks found")
	}
	log.Infof("create aiTask pipeline: %v", root.Name)
	for stepIdx, taskIns := range root.Subtasks {
		log.Infof("step %d: %v", stepIdx, taskIns.Name)
	}
	alltools, err := c.AiToolManager.GetEnableTools()
	if err != nil {
		log.Warnf("coordinator: get all tools failed: %v", err)
	}
	if len(alltools) <= 0 {
		log.Warnf("coordinator: no tools enable")
	}
	return nil
}

func (c *Coordinator) runExecuteRoot(startTaskID string) error {
	c.planUserStatus("正在推进任务", "Working through the task", aicommon.WithStatusCode("plan.executing"))
	c.EmitInfo("start to create runtime")
	c.ensureSessionSnapshotEmitHandler()
	if c.rootTask != nil {
		c.Config.SetHotpatchCurrentTaskIdResolver(func() string {
			return c.rootTask.GetId()
		})
		aicommon.BeginSessionSnapshotExecutionForTask(c.Config, c.rootTask, time.Now())
	}
	defer func() {
		if c.rootTask != nil {
			aicommon.FinalizeSessionSnapshotExecutionForTask(c.Config, c.rootTask, time.Now())
			reactloops.EmitSessionSnapshot(c.Config, nil, c.rootTask)
		}
	}()
	rt := c.createRuntime()
	c.runtime = rt
	if err := rt.Invoke(c.rootTask, startTaskID); err != nil {
		c.planUserStatus("任务推进时遇到问题", "A problem occurred while executing the task", aicommon.WithStatusCode("plan.execution_failed"), aicommon.WithStatusState(aicommon.StatusStateError))
		return err
	}
	return nil
}

func (c *Coordinator) ensureSessionSnapshotEmitHandler() {
	if c == nil || c.Config == nil {
		return
	}
	c.Config.SetSessionSnapshotEmitHandler(func() {
		reactloops.EmitSessionSnapshot(c.Config, nil, c.rootTask)
	})
}

func (c *Coordinator) tryRecoverAndExecute(startTaskID string) (bool, error) {
	recoveryStartTaskID := c.getRecoveryStartTaskID()
	if recoveryStartTaskID == "" {
		recoveryStartTaskID = startTaskID
	}
	recoveredRoot, _, ok, err := c.tryRecoverPlanAndExec(recoveryStartTaskID)
	if !ok {
		return false, nil
	}
	if err != nil {
		c.planUserStatus("暂时没能恢复之前的进度", "Unable to restore the previous progress", aicommon.WithStatusCode("plan.recovery_failed"), aicommon.WithStatusState(aicommon.StatusStateError))
		if isPlanExecutableDAGValidationError(err) {
			c.recordPlanDAGValidationFailure(err, 0)
		}
		c.EmitError("recover plan-and-exec failed: %v", err)
		return false, utils.Errorf("coordinator: recover plan-and-exec failed: %v", err)
	}
	c.planUserStatus("正在恢复之前的进度", "Restoring the previous progress", aicommon.WithStatusCode("plan.recovering"), aicommon.WithStatusState(aicommon.StatusStateRecovering))
	c.rootTask = recoveredRoot
	c.ContextProvider.StoreRootTask(recoveredRoot)
	if len(recoveredRoot.Subtasks) <= 0 {
		c.planUserStatus("没有找到可以继续的步骤", "No actionable steps were found", aicommon.WithStatusCode("plan.empty"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("no subtasks found in recovered task tree")
		return false, utils.Errorf("coordinator: no subtasks found in recovered task tree")
	}
	if err := c.runExecuteRoot(recoveryStartTaskID); err != nil {
		return true, err
	}
	return true, nil
}

func (c *Coordinator) runReportAndFinishPhases() error {
	c.planUserStatus("正在汇总任务成果", "Summarizing the task results", aicommon.WithStatusCode("plan.summarizing"))
	if c.ResultHandler != nil {
		c.ResultHandler(c)
	} else if c.GenerateReport {
		c.planUserStatus("正在整理最终报告", "Preparing the final report", aicommon.WithStatusCode("plan.report"))
		c.EmitInfo("start report generation via focus mode loop")
		if err := c.generateReportViaFocusMode(); err != nil {
			c.planUserStatus("暂时没能完成最终报告", "Unable to complete the final report", aicommon.WithStatusCode("plan.report_failed"), aicommon.WithStatusState(aicommon.StatusStateError))
			c.EmitError("report generation via focus mode failed: %v", err)
			return utils.Errorf("coordinator: report generation failed: %v", err)
		}
	}

	c.planUserStatus("任务已经处理完成", "The task has been completed", aicommon.WithStatusCode("plan.completed"), aicommon.WithStatusState(aicommon.StatusStateSuccess))
	c.EmitInfo("coordinator run finished")
	c.Wait()
	return nil
}

// RunPlanOnly executes the plan loop and user review, then persists plan_ready without running subtasks.
func (c *Coordinator) RunPlanOnly() error {
	c.planUserStatus("正在准备任务规划", "Preparing task planning", aicommon.WithStatusCode("plan.preparing"))

	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()
	c.emitBaseCapabilityInventory()

	if err := c.runPlanPhaseThroughReview(); err != nil {
		return err
	}

	c.planUserStatus("执行方案已经准备好，等你开始", "The execution plan is ready to start", aicommon.WithStatusCode("plan.ready"), aicommon.WithStatusState(aicommon.StatusStateWaiting))
	c.EmitInfo("plan phase completed, awaiting execution")
	c.Wait()
	return nil
}

// RunExecuteApprovedPlan executes an in-memory approved plan without running the plan loop.
func (c *Coordinator) RunExecuteApprovedPlan() error {
	c.planUserStatus("正在准备执行计划", "Preparing to execute the plan", aicommon.WithStatusCode("plan.preparing"))

	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()
	c.emitBaseCapabilityInventory()

	if c.rootTask == nil {
		c.planUserStatus("没有找到已确认的执行方案", "No approved execution plan was found", aicommon.WithStatusCode("plan.not_found"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("no approved plan found for execution")
		return utils.Errorf("coordinator: no approved plan to execute for %s", c.GetRuntimeId())
	}
	if len(c.rootTask.Subtasks) <= 0 {
		c.planUserStatus("执行方案中没有可推进的步骤", "The plan has no actionable steps", aicommon.WithStatusCode("plan.empty"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("no subtasks found, this task is not a valid task")
		return utils.Errorf("coordinator: no subtasks found")
	}
	if err := c.validatePlanExecutableDAG(c.rootTask); err != nil {
		c.planUserStatus("步骤之间存在冲突，暂时无法执行", "Some plan steps conflict and cannot be executed", aicommon.WithStatusCode("plan.order_failed"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.recordPlanDAGValidationFailure(err, 0)
		return utils.Errorf("coordinator: approved plan executable DAG validation failed: %v", err)
	}

	if err := c.runExecuteRoot(""); err != nil {
		return err
	}
	return c.runReportAndFinishPhases()
}

// RunExecuteOnly executes a previously approved plan (plan_ready in DB) without re-running plan loop.
func (c *Coordinator) RunExecuteOnly() error {
	c.planUserStatus("正在恢复执行方案", "Restoring the execution plan", aicommon.WithStatusCode("plan.recovering"), aicommon.WithStatusState(aicommon.StatusStateRecovering))

	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()
	c.emitBaseCapabilityInventory()

	recovered, err := c.tryRecoverAndExecute("")
	if err != nil {
		return err
	}
	if !recovered {
		c.planUserStatus("没有找到可以继续的执行方案", "No execution plan is available to continue", aicommon.WithStatusCode("plan.not_found"), aicommon.WithStatusState(aicommon.StatusStateError))
		c.EmitError("no approved plan found for execution")
		return utils.Errorf("coordinator: no approved plan to execute for %s", c.GetRuntimeId())
	}

	return c.runReportAndFinishPhases()
}
