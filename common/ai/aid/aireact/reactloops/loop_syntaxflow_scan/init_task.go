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

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
		db := r.GetConfig().GetDB()
		if db == nil {
			r.AddToTimeline("syntaxflow_scan", "无数据库连接，无法加载 SyntaxFlow 扫描任务。请在 Yakit 环境重试。")
			loop.Set("sf_scan_review_preface", "无 DB：请通过附件或子 Loop 变量提供 task_id，并在连接数据库后重试。")
			loop.Set("sf_scan_session_mode", "no_db")
			op.Continue()
			return
		}

		mode := sfu.SyntaxFlowScanSessionMode(task, loop)
		taskID, haveID := sfu.SyntaxFlowTaskID(task, loop)

		if haveID && taskID != "" {
			res, err := sfu.LoadScanSessionResult(db, taskID, sfu.DefaultRiskSampleLimit)
			if err != nil {
				log.Warnf("[syntaxflow_scan] LoadScanSessionResult: %v", err)
				r.AddToTimeline("syntaxflow_scan", fmt.Sprintf("未找到 task_id=%s: %v", taskID, err))
				loop.Set("sf_scan_review_preface", fmt.Sprintf("未找到或无法加载任务 %s: %v", taskID, err))
				loop.Set("sf_scan_session_mode", "attach_failed")
				op.Continue()
				return
			}
			loop.Set("sf_scan_task_id", taskID)
			loop.Set("sf_scan_session_mode", "attach")
			preface := "下列信息来自数据库（扫描任务 + 该 runtime 下 SSA Risk 列表），仅可在此基础上解读；不得编造未列出的 risk id。\n\n" + res.Preface
			loop.Set("sf_scan_review_preface", preface)
			r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(preface, 4000))
			op.Continue()
			return
		}

		if mode == sfu.SessionModeStart {
			var b strings.Builder
			b.WriteString("已选择「发起扫描」会话模式（附件 session_mode=start 或 Loop 变量 syntaxflow_scan_session_mode）。\n")
			b.WriteString("在 AI 会话内完整启动 SyntaxFlow 扫描依赖 IRify/Yakit 侧项目与规则配置。\n")
			b.WriteString("推荐：在 Yakit/IRify 中发起扫描；完成后通过附件 **irify_syntaxflow / task_id**（与 SSA Risk 的 runtime_id 一致）附着任务并解读结果。\n")
			if progs := sfu.ProgramNamesHint(task); len(progs) > 0 {
				b.WriteString(fmt.Sprintf("附件 programs 提示：%v。\n", progs))
			}
			loop.Set("sf_scan_session_mode", "start_intent")
			loop.Set("sf_scan_review_preface", b.String())
			r.AddToTimeline("syntaxflow_scan", b.String())
			op.Continue()
			return
		}

		r.AddToTimeline("syntaxflow_scan", "未提供 task_id：请在 Yakit 中为该对话附加 irify_syntaxflow/task_id，或由编排方设置 Loop 变量 syntaxflow_task_id；发起扫描指引请附加 session_mode=start。")
		loop.Set("sf_scan_review_preface", "缺少结构化输入：需要附件 Type=irify_syntaxflow Key=task_id（UUID），或子 Loop WithVar(syntaxflow_task_id)；发起扫描请设 session_mode=start。")
		loop.Set("sf_scan_session_mode", "unresolved")
		op.Continue()
	}
}
