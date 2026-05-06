package loop_syntaxflow_scan

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func newSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

func syncSyntaxflowVarsToChild(parent, child *reactloops.ReActLoop) {
	keys := []string{
		sfu.LoopVarSyntaxFlowTaskID,
		sfu.LoopVarSFScanConfigJSON,
		sfu.LoopVarSyntaxFlowScanSessionMode,
		"sf_scan_config_inferred",
	}
	for _, k := range keys {
		if v := parent.Get(k); v != "" {
			child.Set(k, v)
		}
	}
}

// buildSyntaxflowOrchestratorInit runs P1 intake → P2 compile/scan (or attach) → P3 interpret sub-loop, then op.Done().
func buildSyntaxflowOrchestratorInit(r aicommon.AIInvokeRuntime, state *SyntaxFlowState) func(*reactloops.ReActLoop, aicommon.AIStatefulTask, *reactloops.InitTaskOperator) {
	return func(parentLoop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		userInput := task.GetUserInput()
		r.AddToTimeline("syntaxflow_scan", "SyntaxFlow 扫描编排开始 / orchestrator: "+utils.ShrinkTextBlock(userInput, 300))

		if err := runPhase1Intake(r, state, parentLoop, task); err != nil {
			op.Failed(err)
			return
		}

		db := sfu.GetSSADB()
		if db == nil {
			op.Failed(fmt.Errorf("无 SSA 工程库连接，无法使用 SyntaxFlow 扫描（请确认运行环境已初始化 SSA 数据库）"))
			return
		}
		// 附着路径：尽早校验 task_id 在 SSA 工程库是否存在，避免进入解读后才发现会话无效
		if strings.TrimSpace(state.GetResolvedSFScanConfigJSON()) == "" && strings.TrimSpace(state.GetTaskID()) != "" {
			if err := EnsureSyntaxFlowScanTaskExists(db, state.GetTaskID()); err != nil {
				op.Failed(err)
				return
			}
		}

		interpretLoop, err := buildPhaseInterpretLoop(r)
		if err != nil {
			op.Failed(err)
			return
		}
		syncSyntaxflowVarsToChild(parentLoop, interpretLoop)
		// 供解读 Init / 阶段 Markdown 去重与父任务对齐（子任务 Id 与编排任务 Id 不同）
		interpretLoop.Set("sf_orchestrator_parent_task_id", task.GetId())

		if err := runPhaseCompileAndScan(r, db, state, interpretLoop, task); err != nil {
			op.Failed(err)
			return
		}

		state.SetPhase(SyntaxFlowPhaseInterpret)
		if err := interpretLoop.ExecuteWithExistedTask(newSubTask(task, "syntaxflow_interpret")); err != nil {
			log.Warnf("[syntaxflow_scan] interpret sub-loop: %v", err)
		}
		state.SetPhase(SyntaxFlowPhaseReport)
		WaitForSyntaxFlowReportGate(task.GetContext(), interpretLoop)
		if strings.TrimSpace(interpretLoop.Get(sfu.LoopVarSFRiskConverged)) != "1" {
			r.AddToTimeline("syntaxflow_scan", "P4 物化前未在预期窗口内看到风险行数与任务行已联合收敛，仍将尝试成稿，建议核对 SSA risk 表是否仍写入。")
		}
		runPhaseReportGenerating(r, interpretLoop, task)
		state.SetPhase(SyntaxFlowPhaseDone)
		op.Done()
	}
}
