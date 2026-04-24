package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"time"

	sfutil "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// StartScanTaskStatusPoll starts a light poll until the SyntaxFlow scan task is no longer
// "executing" (or an error reading DB). During执行中：定时或以 risk 条数增长触发与 reload_ssa_risk_overview
// 等价的 DB 总览；终态时写入扫描总结、大页 risk 列表、sf_scan_final_report_due=1 并强提示终局报告。
// 不注册 schema 侧第二路广播：与 main 的 [schema.SetBroadCast_Data] 单 handler 一致；SSA Risk / ScanTask
// 的 GORM 仍通过 [schema] 内 broadcast 推前端，本处仅依赖本 goroutine 的定时轮询读 DB。
func StartScanTaskStatusPoll(
	db *gorm.DB,
	loop *reactloops.ReActLoop,
	r aicommon.AIInvokeRuntime,
	task aicommon.AIStatefulTask,
	runtimeID string,
	ctx context.Context,
) {
	if db == nil || runtimeID == "" || ctx == nil {
		return
	}
	ticker := time.NewTicker(45 * time.Second)
	const (
		overviewInterval     = 40 * time.Second
		riskDeltaForOverview = int64(30)
		finalRiskPageCap     = int64(500)
	)
	go func() {
		defer ticker.Stop()
		pollAt := time.Now()
		lastOverviewAt := time.Now()
		lastUserStageAt := time.Now()
		var lastRiskForDelta int64 = -1
		var lastUserEmitRisk int64 = -1
		userStageIdx := 0
		var riskGateLast int64 = -1
		var riskGateSame int
		filterRT := &ypb.SSARisksFilter{RuntimeID: []string{runtimeID}}
		const userStageInterval = 3 * time.Minute
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				st, err := schema.GetSyntaxFlowScanTaskById(db, runtimeID)
				if err != nil {
					log.Debugf("[syntaxflow_scan] poll task: %v", err)
					continue
				}
				if st == nil {
					continue
				}
				s := st.Status
				parentT := OrchestratorParentTaskID(loop, "")
				if s == schema.SYNTAXFLOWSCAN_EXECUTING {
					riskGateLast = -1
					riskGateSame = 0
					now := time.Now()
					line := FormatScanTaskProgressLine(st) + fmtRiskLine(st)
					AppendSfScanInterpretLog(loop, r, runtimeID, "poll: 扫描进行中 "+line)
					EmitSyntaxFlowScanPhase(loop, 3, "tick", "risk_poll",
						"扫描进行中，已读任务行 / scan running, task row", runtimeID, "", map[string]any{
							"total_query": st.TotalQuery, "success_query": st.SuccessQuery,
							"failed_query": st.FailedQuery, "risk_count": st.RiskCount, "status": st.Status,
						})
					// 用户向：~3 分钟 或 风险 +riskDeltaForOverview
					needUser := time.Since(lastUserStageAt) >= userStageInterval
					if lastUserEmitRisk >= 0 && st.RiskCount >= lastUserEmitRisk+riskDeltaForOverview {
						needUser = true
					}
					if needUser && time.Since(pollAt) >= 10*time.Second {
						res, _ := LoadScanSessionResult(db, runtimeID, DefaultRiskSampleLimit)
						EmitSyntaxFlowUserStageMarkdown(loop, parentT, fmt.Sprintf("p2_scan_user_%d", userStageIdx),
							BuildScanStagePhase2Progress(st, res, userStageIdx, false))
						userStageIdx++
						lastUserStageAt = time.Now()
						lastUserEmitRisk = st.RiskCount
					} else if lastUserEmitRisk < 0 {
						lastUserEmitRisk = st.RiskCount
					}
					// 定时 40s 或 risk 每增加 riskDeltaForOverview 条，刷新与 loop_ssa_risk_overview 同口径的摘要
					if lastRiskForDelta < 0 {
						lastRiskForDelta = st.RiskCount
					}
					needOvh := time.Since(lastOverviewAt) >= overviewInterval || st.RiskCount >= lastRiskForDelta+riskDeltaForOverview
					if needOvh && time.Since(pollAt) >= 15*time.Second {
						if UseInScanSSARiskOverviewSubLoop() {
							ApplySSARiskOverviewToInterpret(loop, r, db, task, runtimeID, filterRT, 40)
							hint := loop.Get("ssa_risk_total_hint")
							AppendSFPipelineLine(loop, fmt.Sprintf("【3·扫描中·Risk 总览】子环 ssa_risk_overview 已刷新，approx count=%s", hint))
							AppendSfScanInterpretLog(loop, r, runtimeID, "定时/增量: 子环 ssa_risk_overview 已跑（YAK_SSA_RISK_OVERVIEW_IN_SCAN_SUBLOOP，limit=40）")
							EmitSyntaxFlowScanPhase(loop, 3, "tick", "risk_overview_tick",
								"扫描中 risk 总览子环 / in-scan overview subloop", runtimeID, "", map[string]any{
									"approx_count": hint, "mode": "in_scan_subloop",
								})
						} else {
							AppendSfScanInterpretLog(loop, r, runtimeID, "定时 tick：扫中不自动灌 DB 总览（见终态 ssa_risk_overview 子环；或设 YAK_SSA_RISK_OVERVIEW_IN_SCAN_SUBLOOP=1）")
							EmitSyntaxFlowScanPhase(loop, 3, "tick", "risk_overview_deferred",
								"扫中不刷新全库总览，终态由子环灌入 / in-scan overview deferred (mode A)", runtimeID, "", map[string]any{
									"mode": "deferred",
								})
						}
						lastOverviewAt = now
						lastRiskForDelta = st.RiskCount
					}
					continue
				}
				// 扫描任务行已非 executing：等 risk 条数在 DB 侧连续 2 次读数相同后再跑终态灌入
				if riskGateLast < 0 {
					riskGateLast = st.RiskCount
					riskGateSame = 1
				} else if st.RiskCount == riskGateLast {
					riskGateSame++
				} else {
					riskGateLast = st.RiskCount
					riskGateSame = 1
				}
				if riskGateSame < 2 {
					if time.Since(lastUserStageAt) >= userStageInterval {
						res, _ := LoadScanSessionResult(db, runtimeID, DefaultRiskSampleLimit)
						EmitSyntaxFlowUserStageMarkdown(loop, parentT, fmt.Sprintf("p2_stabilize_%d", userStageIdx),
							BuildScanStagePhase2Progress(st, res, userStageIdx, true))
						userStageIdx++
						lastUserStageAt = time.Now()
					}
					continue
				}
				loop.Set(sfutil.LoopVarSFRiskConverged, "1")
				EmitSyntaxFlowUserStageMarkdown(loop, parentT, "p2_scan_finished_user",
					BuildScanStagePhase2ScanFinishedTable(st))
				// 扫描已终态 + 风险读数已稳定
				endText := FormatSyntaxFlowScanEndReport(st)
				loop.Set(sfutil.LoopVarSFScanEndSummary, endText)
				AppendSFPipelineLine(loop, "【2·结束】"+endText)
				EmitSyntaxFlowScanPhase(loop, 2, "end", "scan_finished",
					"SyntaxFlow 扫描任务已终态 / scan task terminal", runtimeID, "", map[string]any{
						"task_status": st.Status, "total_query": st.TotalQuery, "success_query": st.SuccessQuery,
						"failed_query": st.FailedQuery, "skip_query": st.SkipQuery, "risk_count": st.RiskCount,
					})
				// 大页拉取 risk 供终局「每条都讲到」
				lim := finalRiskPageCap
				if c, e := yakit.QuerySSARiskCount(db, filterRT); e == nil && c > 0 && int64(c) < lim {
					lim = int64(c)
				}
				if lim < 1 {
					lim = 100
				}
				ApplySSARiskOverviewToInterpret(loop, r, db, task, runtimeID, filterRT, lim)
				AppendSFPipelineLine(loop, fmt.Sprintf("【4·全量风险列表】已按 runtime 灌入终局总览（最多 %d 条，ssa_risk_list_summary）", lim))
				AppendSfScanInterpretLog(loop, r, runtimeID, "scan 终态: 终局总览已灌入 (ssa_risk_overview 子环或回退, limit="+fmt.Sprintf("%d", lim)+")")

				res, err := LoadScanSessionResult(db, runtimeID, DefaultRiskSampleLimit)
				if err != nil {
					log.Warnf("[syntaxflow_scan] poll end LoadScanSessionResult: %v", err)
					r.AddToTimeline("syntaxflow_scan", "扫描已结束，但无法刷新结果摘要: "+err.Error())
					AppendSfScanInterpretLog(loop, r, runtimeID, "poll 结束: 刷新摘要失败 "+err.Error())
					EmitSyntaxFlowScanProgress(loop, "scan_complete_degraded",
						"扫描已结束但摘要刷新失败 / finished, summary refresh failed", runtimeID, err.Error())
				} else {
					loop.Set("sf_scan_task_id", runtimeID)
					loop.Set("sf_scan_session_mode", "watch_complete")
					pipe := loop.Get(sfutil.LoopVarSFPipelineSummary)
					full := "【==== 大总结用数据：以下须全部纳入终局报告 ====】\n\n"
					full += "【A·各阶段摘要 sf_scan_pipeline_summary】\n" + pipe + "\n\n"
					full += "【B·扫描行终态】\n" + endText + "\n\n"
					full += "【C·风险列表与抽样】优先阅读 reactive 中 ssa_risk_list_summary / ssa_risk_total_hint；与 preface 中条目不冲突。\n\n"
					full += "下列信息来自数据库（扫描已结束，任务行 + SSA Risk 列表）：\n" + res.Preface
					loop.Set("sf_scan_review_preface", full)
					loop.Set(sfutil.LoopVarSFFinalReportDue, "1")
					r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock("扫描已结束(终态): "+endText, 4000))
					AppendSfScanInterpretLog(loop, r, runtimeID,
						"【终局】扫描已停。须输出大总结：合并 A/B/C、覆盖 sf_scan_interpret_log 与 ssa_risk 中每个 risk，勿遗漏。总 risk 约 "+loop.Get("ssa_risk_total_hint")+" 条。")
					EmitSyntaxFlowScanProgress(loop, "scan_complete",
						"扫描已结束，终局 data 与 pipeline 已灌入 / scan finished, final context ready", runtimeID, "")
				}
				EmitSyntaxFlowScanProgress(loop, "final_report_required",
					"须输出大总结 / MUST deliver final merged report (all risks covered)", runtimeID, "")
				EmitSyntaxFlowScanPhase(loop, 3, "end", "interpret_round_done",
					"风险轮询阶段结束，进入终局大总结 / end risk poll", runtimeID, "", nil)
				EmitSyntaxFlowScanPhase(loop, 4, "start", "final_report_mandatory",
					"请输出含各阶段摘要 + 扫描统计 + 全部 risk 的终局大总结 / deliver final merged report (mandatory)", runtimeID, "", nil)
				EmitSyntaxFlowScanPhase(loop, 4, "tick", "final_report",
					"见 sf_scan_final_report_due=1 与 ssa_risk_list_summary", runtimeID, "", nil)
				return
			}
		}
	}()
}

func fmtRiskLine(st *schema.SyntaxFlowScanTask) string {
	if st == nil {
		return ""
	}
	return fmt.Sprintf(" risk_count=%d crit=%d high=%d warn=%d low=%d info=%d",
		st.RiskCount, st.CriticalCount, st.HighCount, st.WarningCount, st.LowCount, st.InfoCount)
}
