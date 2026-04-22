package syntaxflow_utils

import (
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// WithReloadSyntaxFlowScanSessionAction registers reload_syntaxflow_scan_session: reload scan task + SSA risk sample for a task_id from DB.
func WithReloadSyntaxFlowScanSessionAction(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"reload_syntaxflow_scan_session",
		"Load SyntaxFlowScanTask and a sample of SSA risks for the given task_id (SSA runtime id) from the database, then refresh sf_scan_review_preface, sf_scan_task_id, and sf_scan_session_mode=attach. Equivalent to a successful attach path in the syntaxflow_scan init task.",
		[]aitool.ToolOption{
			aitool.WithStringParam("task_id", aitool.WithParam_Description("SyntaxFlow scan task id (UUID), same as SSA Risk runtime_id for that scan."), aitool.WithParam_Required(true)),
		},
		func(_ *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("task_id") == "" {
				return utils.Error("task_id is required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			taskID := action.GetString("task_id")
			db := r.GetConfig().GetDB()
			if db == nil {
				r.AddToTimeline("syntaxflow_scan", "reload_syntaxflow_scan_session: 无数据库连接")
				operator.Feedback("reload_syntaxflow_scan_session failed: database not available")
				operator.Continue()
				return
			}
			res, err := LoadScanSessionResult(db, taskID, DefaultRiskSampleLimit)
			if err != nil {
				log.Warnf("[syntaxflow_scan] reload LoadScanSessionResult: %v", err)
				r.AddToTimeline("syntaxflow_scan", fmt.Sprintf("reload failed task_id=%s: %v", taskID, err))
				operator.Feedback(fmt.Sprintf("reload_syntaxflow_scan_session failed: %v", err))
				operator.Continue()
				return
			}
			loop.Set("sf_scan_task_id", taskID)
			loop.Set("sf_scan_session_mode", "attach")
			preface := "下列信息来自数据库（扫描任务 + 该 runtime 下 SSA Risk 列表），仅可在此基础上解读；不得编造未列出的 risk id。\n\n" + res.Preface
			loop.Set("sf_scan_review_preface", preface)
			r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(preface, 4000))
			operator.Feedback(preface)
			operator.Continue()
		},
	)
}
