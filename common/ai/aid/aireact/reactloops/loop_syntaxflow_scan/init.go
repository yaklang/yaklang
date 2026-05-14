package loop_syntaxflow_scan

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			state := NewSyntaxFlowState()
			preset := []reactloops.ReActLoopOption{
				reactloops.WithInitTask(buildSyntaxflowOrchestratorInit(r, state)),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN, r, preset...)
		},
		reactloops.WithVerboseName("IRify · SyntaxFlow Scan"),
		reactloops.WithVerboseNameZh("IRify · SyntaxFlow 扫描"),
		reactloops.WithLoopDescription("Multi-stage SyntaxFlow scan: P1 resolves params from irify_syntaxflow attachments and/or LiteForge on user input (no loop-var reads); attaches validate task id in SSA."),
		reactloops.WithLoopDescriptionZh("SyntaxFlow 扫描：编排入参仅从 irify_syntaxflow 附件 + 用户话术 LiteForge 解析（不在 P1 Read loop）；附着校验 task_id。Subtask 需带齐附件或使用自然语言兜底。"),
		reactloops.WithLoopUsagePrompt("Set irify_syntaxflow attachments (session_mode, and task_id or sf_scan_config_json) on the task, or rely on LiteForge from the user message."),
		reactloops.WithLoopOutputExample(`
* SyntaxFlow 扫描会话：
  {"@action": "syntaxflow_scan", "human_readable_thought": "需要解读已注入的 SyntaxFlow 扫描任务（task_id 由引擎或变量提供）"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN, err)
	}
}

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

		disp, err := runPhase2(r, db, state, parentLoop, task)
		if err != nil {
			op.Failed(err)
			return
		}

		state.SetPhase(SyntaxFlowPhaseReport)
		disp.WaitDrained(task.GetContext())

		runPhaseReportGenerating(r, parentLoop, task)
		state.SetPhase(SyntaxFlowPhaseDone)
		op.Done()
	}
}
