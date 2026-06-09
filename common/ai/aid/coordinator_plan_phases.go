package aid

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const Phase_PlanReady = "plan_ready"

// runPlanPhaseThroughReview runs plan loop, user review, and persists plan_ready state.
// It does not execute subtasks.
func (c *Coordinator) runPlanPhaseThroughReview() error {
	c.planLoadingStatus("创建任务规划 / Creating Plan...")
	c.EmitInfo("start to create plan request")
	planReq, err := c.createPlanRequest(c.userInput)
	if err != nil {
		c.planLoadingStatus("任务规划创建失败 / Plan Creation Failed")
		c.EmitError("create planRequest failed: %v", err)
		return utils.Errorf("coordinator: create planRequest failed: %v", err)
	}

	c.planLoadingStatus("任务规划中... / Waiting AI to Generate Plan...")
	c.EmitInfo("start to invoke plan request")
	rsp, err := planReq.Invoke()
	if err != nil {
		c.planLoadingStatus("任务规划失败 / Plan Generation Failed")
		c.EmitError("invoke planRequest failed(first): %v", err)
		return utils.Errorf("coordinator: invoke planRequest failed: %v", err)
	}

	c.planLoadingStatus("任务规划等待用户审查 / Waiting User to Review Plan...")
	ep := c.Epm.CreateEndpointWithEventType(schema.EVENT_TYPE_PLAN_REVIEW_REQUIRE)
	ep.SetDefaultSuggestionContinue()

	c.EmitRequireReviewForPlan(rsp, ep.GetId())
	c.DoWaitAgree(c.GetContext(), ep)
	params := ep.GetParams()
	c.ReleaseInteractiveEvent(ep.GetId(), params)
	if params == nil {
		c.planLoadingStatus("用户审查失败 / User Review Failed")
		c.EmitError("user review params is nil, plan failed")
		return utils.Errorf("coordinator: user review params is nil")
	}

	c.planLoadingStatus("处理用户审查结果 / Processing User Review...")
	c.EmitInfo("start to handle review plan response")
	rsp, err = planReq.handleReviewPlanResponse(rsp, params)
	if err != nil {
		c.planLoadingStatus("处理审查结果失败 / Review Processing Failed")
		c.EmitError("handle review plan response failed: %v", err)
		return utils.Errorf("coordinator: handle review plan response failed: %v", err)
	}

	if rsp.RootTask == nil {
		c.planLoadingStatus("任务计划无效 / Invalid Task Plan")
		c.EmitError("root aiTask is nil, plan failed")
		return utils.Errorf("coordinator: root aiTask is nil")
	}

	c.planLoadingStatus("初始化任务队列 / Initializing Task Queue...")
	root := rsp.RootTask
	c.rootTask = root
	c.ContextProvider.StoreRootTask(root)
	c.savePlanAndExecState(Phase_PlanReady, nil)
	if len(root.Subtasks) <= 0 {
		c.planLoadingStatus("无有效子任务 / No Valid Subtasks")
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

func (c *Coordinator) runExecuteRoot(startTaskIndex string) error {
	c.planLoadingStatus("执行任务中 / Executing Tasks...")
	c.EmitInfo("start to create runtime")
	rt := c.createRuntime()
	c.runtime = rt
	if err := rt.Invoke(c.rootTask, startTaskIndex); err != nil {
		c.planLoadingStatus("任务执行失败 / Task Execution Failed")
		return err
	}
	return nil
}

func (c *Coordinator) tryRecoverAndExecute(startTaskIndex string) (bool, error) {
	recoveryStartTaskIndex := c.getRecoveryStartTaskIndex()
	if recoveryStartTaskIndex == "" {
		recoveryStartTaskIndex = startTaskIndex
	}
	recoveredRoot, _, ok, err := c.tryRecoverPlanAndExec(recoveryStartTaskIndex)
	if !ok {
		return false, nil
	}
	if err != nil {
		c.planLoadingStatus("恢复执行失败 / Recovery Failed")
		c.EmitError("recover plan-and-exec failed: %v", err)
		return false, utils.Errorf("coordinator: recover plan-and-exec failed: %v", err)
	}
	c.planLoadingStatus("恢复执行 / Recovering Execution...")
	c.rootTask = recoveredRoot
	c.ContextProvider.StoreRootTask(recoveredRoot)
	if len(recoveredRoot.Subtasks) <= 0 {
		c.planLoadingStatus("无有效子任务 / No Valid Subtasks")
		c.EmitError("no subtasks found in recovered task tree")
		return false, utils.Errorf("coordinator: no subtasks found in recovered task tree")
	}
	if err := c.runExecuteRoot(recoveryStartTaskIndex); err != nil {
		return true, err
	}
	return true, nil
}

func (c *Coordinator) runReportAndFinishPhases() error {
	c.planLoadingStatus("生成执行结果 / Generating Results...")
	if c.ResultHandler != nil {
		c.ResultHandler(c)
	} else if c.GenerateReport {
		c.planLoadingStatus("进入报告生成专注模式 / Entering Report Generation Focus Mode...")
		c.EmitInfo("start report generation via focus mode loop")
		if err := c.generateReportViaFocusMode(); err != nil {
			c.planLoadingStatus("报告生成失败 / Report Generation Failed")
			c.EmitError("report generation via focus mode failed: %v", err)
			return utils.Errorf("coordinator: report generation failed: %v", err)
		}
	}

	c.planLoadingStatus("执行完成 / Execution Completed")
	c.EmitInfo("coordinator run finished")
	c.Wait()
	return nil
}

// RunPlanOnly executes the plan loop and user review, then persists plan_ready without running subtasks.
func (c *Coordinator) RunPlanOnly() error {
	c.planLoadingStatus("初始化 / Initializing...")
	defer c.planLoadingStatus("任务规划阶段结束 / Plan Phase Finished")

	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()
	c.emitBaseCapabilityInventory()

	if err := c.runPlanPhaseThroughReview(); err != nil {
		return err
	}

	c.planLoadingStatus("计划已就绪 / Plan Ready")
	c.EmitInfo("plan phase completed, awaiting execution")
	c.Wait()
	return nil
}

// RunExecuteApprovedPlan executes an in-memory approved plan without running the plan loop.
func (c *Coordinator) RunExecuteApprovedPlan() error {
	c.planLoadingStatus("初始化 / Initializing...")
	defer c.planLoadingStatus("任务规划执行结束 / Plan Execution Finished")

	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()
	c.emitBaseCapabilityInventory()

	if c.rootTask == nil {
		c.planLoadingStatus("无已批准计划 / No Approved Plan")
		c.EmitError("no approved plan found for execution")
		return utils.Errorf("coordinator: no approved plan to execute for %s", c.GetRuntimeId())
	}
	if len(c.rootTask.Subtasks) <= 0 {
		c.planLoadingStatus("无有效子任务 / No Valid Subtasks")
		c.EmitError("no subtasks found, this task is not a valid task")
		return utils.Errorf("coordinator: no subtasks found")
	}

	if err := c.runExecuteRoot(""); err != nil {
		return err
	}
	return c.runReportAndFinishPhases()
}

// RunExecuteOnly executes a previously approved plan (plan_ready in DB) without re-running plan loop.
func (c *Coordinator) RunExecuteOnly() error {
	c.planLoadingStatus("初始化 / Initializing...")
	defer c.planLoadingStatus("任务规划执行结束 / Plan Execution Finished")

	c.registerPEModeInputEventCallback()
	c.EmitCurrentConfigInfo()
	c.emitBaseCapabilityInventory()

	recovered, err := c.tryRecoverAndExecute("")
	if err != nil {
		return err
	}
	if !recovered {
		c.planLoadingStatus("无已批准计划 / No Approved Plan")
		c.EmitError("no approved plan found for execution")
		return utils.Errorf("coordinator: no approved plan to execute for %s", c.GetRuntimeId())
	}

	return c.runReportAndFinishPhases()
}
