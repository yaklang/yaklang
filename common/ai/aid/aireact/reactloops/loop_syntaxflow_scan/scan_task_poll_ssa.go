// Scan-task polling + SSA risk overview hydration on the orchestrator loop (background goroutine during scan/watch).
package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// LoopVarInterpretLog 逐次解读/轮询的追加日志键（与 [sfu.LoopVarSFInterpretLog] 一致）。
const LoopVarInterpretLog = sfu.LoopVarSFInterpretLog

// AppendSfScanInterpretLog appends one interpret/poll line via syntaxflow_utils.
func AppendSfScanInterpretLog(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, taskID, line string) {
	sfu.AppendSfScanInterpretLog(loop, r, taskID, line)
}

// Re-export syntaxflow_utils risk-overview helpers so existing imports of loop_syntaxflow_scan keep compiling.
var (
	PersistEffectiveOverviewFilter   = sfu.PersistEffectiveOverviewFilter
	MergeReloadSSARiskOverviewFilter = sfu.MergeReloadSSARiskOverviewFilter
	ApplySSARiskOverviewDB           = sfu.ApplySSARiskOverviewDB
)

// StartScanTaskStatusPoll starts a light poll until the SyntaxFlow scan task is no longer
// "executing" (or an error reading DB). During执行中：定时或以 risk 条数增长调用
// ApplySSARiskOverviewToInterpret(limit=40)，与终态同源（是否走 ssa_risk_overview 子环仅看 YAK_SSA_RISK_OVERVIEW_SUBLOOP）。
// 终态时写入扫描总结、更大 limit 的同入口总览、sf_scan_final_report_due=1 并强提示终局报告。
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
				parentT := OrchestratorParentTaskID(loop, task.GetId())
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
					// 定时 40s 或 risk 每增加 riskDeltaForOverview 条：与终态相同入口灌入 orchestrator loop（limit 较小）。
					if lastRiskForDelta < 0 {
						lastRiskForDelta = st.RiskCount
					}
					needOvh := time.Since(lastOverviewAt) >= overviewInterval || st.RiskCount >= lastRiskForDelta+riskDeltaForOverview
					if needOvh && time.Since(pollAt) >= 15*time.Second {
						ApplySSARiskOverviewToInterpret(loop, r, db, task, runtimeID, filterRT, 40)
						hint := loop.Get("ssa_risk_total_hint")
						AppendSFPipelineLine(loop, fmt.Sprintf("【3·扫描中·Risk 总览】已刷新 limit=40 approx=%s", hint))
						AppendSfScanInterpretLog(loop, r, runtimeID, "定时/增量: ApplySSARiskOverviewToInterpret(limit=40)")
						EmitSyntaxFlowScanPhase(loop, 3, "tick", "risk_overview_tick",
							"扫描中 risk 总览已灌入 / in-scan overview hydrate", runtimeID, "", map[string]any{
								"approx_count": hint, "list_limit": int64(40),
							})
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
				loop.Set(sfu.LoopVarSFRiskConverged, "1")
				EmitSyntaxFlowUserStageMarkdown(loop, parentT, "p2_scan_finished_user",
					BuildScanStagePhase2ScanFinishedTable(st))
				// 扫描已终态 + 风险读数已稳定
				endText := FormatSyntaxFlowScanEndReport(st)
				loop.Set(sfu.LoopVarSFScanEndSummary, endText)
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
				AppendSfScanInterpretLog(loop, r, runtimeID, "scan 终态: 终局总览已灌入 (ApplySSARiskOverviewToInterpret, limit="+fmt.Sprintf("%d", lim)+")")

				res, err := LoadScanSessionResult(db, runtimeID, DefaultRiskSampleLimit)
				if err != nil {
					log.Warnf("[syntaxflow_scan] poll end LoadScanSessionResult: %v", err)
					r.AddToTimeline("syntaxflow_scan", "扫描已结束，但无法刷新结果摘要: "+err.Error())
					AppendSfScanInterpretLog(loop, r, runtimeID, "poll 结束: 刷新摘要失败 "+err.Error())
					EmitSyntaxFlowScanProgress(loop, "scan_complete_degraded",
						"扫描已结束但摘要刷新失败 / finished, summary refresh failed", runtimeID, err.Error())
				} else {
					loop.Set(sfu.LoopVarSyntaxFlowTaskID, runtimeID)
					loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, "watch_complete")
					loop.Set("sf_scan_review_preface",
						"扫描已终态；终局数据见 loop 变量 sf_scan_pipeline_summary、sf_scan_scan_end_summary、ssa_risk_list_summary、ssa_risk_total_hint 及报告输入物化。\n\n"+
							utils.ShrinkTextBlock(res.Preface, 4000))
					loop.Set(sfu.LoopVarSFFinalReportDue, "1")
					r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock("扫描已结束(终态): "+endText, 4000))
					AppendSfScanInterpretLog(loop, r, runtimeID,
						"【终局】扫描已停。须输出大总结：合并 A/B/C、覆盖 sf_scan_interpret_log 与 ssa_risk 中每个 risk，勿遗漏。总 risk 约 "+loop.Get("ssa_risk_total_hint")+" 条。")
					EmitSyntaxFlowScanProgress(loop, "scan_complete",
						"扫描已结束，终局 data 与 pipeline 已灌入 / scan finished, final context ready", runtimeID, "")
				}
				EmitSyntaxFlowScanProgress(loop, "final_report_required",
					"须输出大总结 / MUST deliver final merged report (all risks covered)", runtimeID, "")
				EmitSyntaxFlowScanPhase(loop, 3, "end", "risk_poll_done",
					"风险轮询阶段结束，进入终局报告物化 / end risk poll", runtimeID, "", nil)
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

const (
	// envSSARiskOverviewSubLoop 不为 "0"/"false"/"off" 时，扫中 tick 与终态总览均优先走 ssa_risk_overview 子环 + copy（失败则回退 ApplySSARiskOverviewDB）。
	envSSARiskOverviewSubLoop = "YAK_SSA_RISK_OVERVIEW_SUBLOOP"
)

var interpretSSAVarMu sync.Mutex

func envFalsy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off", "no", "":
		return true
	default:
		return false
	}
}

// UseSSARiskOverviewSubLoop returns true unless YAK_SSA_RISK_OVERVIEW_SUBLOOP is 0/false/off.
func UseSSARiskOverviewSubLoop() bool {
	v := os.Getenv(envSSARiskOverviewSubLoop)
	if v == "" {
		return true
	}
	return !envFalsy(v)
}

// WithInterpretSSAVarLock serializes writers that Set ssa_risk_* from the poll goroutine vs. other paths.
func WithInterpretSSAVarLock(fn func()) {
	interpretSSAVarMu.Lock()
	defer interpretSSAVarMu.Unlock()
	fn()
}

// NewSyntaxflowSubTask 与 loop_syntaxflow_scan.newSubTask 同形，供子环独立子任务 id。
func NewSyntaxflowSubTask(parent aicommon.AIStatefulTask, name string) aicommon.AIStatefulTask {
	if parent == nil {
		return nil
	}
	subID := fmt.Sprintf("%s-%s", parent.GetId(), name)
	return aicommon.NewSubTaskBase(parent, subID, parent.GetUserInput(), true)
}

// CopyOverviewOutputsToParent copies overview loop vars from ssa_risk_overview sub-run into the orchestrator loop.
func CopyOverviewOutputsToParent(overview, interpret *reactloops.ReActLoop) {
	if overview == nil || interpret == nil {
		return
	}
	keys := []string{
		"ssa_risk_overview_preface",
		"ssa_risk_list_summary",
		"ssa_risk_total_hint",
		sfu.LoopVarSSAOverviewFilterJSON,
		sfu.LoopVarSSARisksFilterJSON,
	}
	for _, k := range keys {
		interpret.Set(k, overview.Get(k))
	}
}

func capOverviewSubLoopMaxIter(r aicommon.AIInvokeRuntime) int {
	if r == nil {
		return 3
	}
	n := int(r.GetConfig().GetMaxIterationCount())
	if n > 5 {
		n = 5
	}
	if n < 1 {
		n = 1
	}
	return n
}

// runSSARiskOverviewSubLoopWithChild creates an overview ReAct loop, runs Execute, then
// ApplySSARiskOverviewDB on the same child to reach listLimit rows, for copy to interpret.
func runSSARiskOverviewSubLoopWithChild(
	r aicommon.AIInvokeRuntime,
	interpret *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
	riskDB *gorm.DB,
	taskID string,
	filter *ypb.SSARisksFilter,
	listLimit int64,
) (overview *reactloops.ReActLoop, err error) {
	if r == nil || interpret == nil || parentTask == nil {
		return nil, fmt.Errorf("nil invoker, interpret or task")
	}
	if riskDB == nil {
		return nil, fmt.Errorf("nil risk db")
	}
	if filter == nil {
		if taskID == "" {
			return nil, fmt.Errorf("empty taskID and filter")
		}
		filter = &ypb.SSARisksFilter{RuntimeID: []string{taskID}}
	}
	overview, err = reactloops.CreateLoopByName(
		schema.AI_REACT_LOOP_NAME_SSA_RISK_OVERVIEW,
		r,
		reactloops.WithMaxIterations(capOverviewSubLoopMaxIter(r)),
	)
	if err != nil {
		return nil, err
	}
	if tid := strings.TrimSpace(interpret.Get(sfu.LoopVarSyntaxFlowTaskID)); tid != "" {
		overview.Set(sfu.LoopVarSyntaxFlowTaskID, tid)
	} else if taskID != "" {
		overview.Set(sfu.LoopVarSyntaxFlowTaskID, taskID)
	}
	for _, k := range []string{sfu.LoopVarSSARisksFilterJSON, sfu.LoopVarSSAOverviewFilterJSON, sfu.LoopVarSyntaxFlowScanSessionMode} {
		if s := interpret.Get(k); s != "" {
			overview.Set(k, s)
		}
	}
	PersistEffectiveOverviewFilter(overview, filter)
	sub := NewSyntaxflowSubTask(parentTask, "ssa_risk_overview_subloop")
	if sub == nil {
		return nil, fmt.Errorf("subtask")
	}
	if err := overview.ExecuteWithExistedTask(sub); err != nil {
		return overview, err
	}
	_ = ApplySSARiskOverviewDB(overview, r, riskDB, sub, filter, listLimit)
	return overview, nil
}

// ApplySSARiskOverviewToInterpret 扫中长扫 tick 与终态共用入口：优先 ssa_risk_overview 子环 + copy，YAK_SSA_RISK_OVERVIEW_SUBLOOP 关则直写 ApplySSARiskOverviewDB。
func ApplySSARiskOverviewToInterpret(
	interpret *reactloops.ReActLoop,
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	parentTask aicommon.AIStatefulTask,
	taskID string,
	filter *ypb.SSARisksFilter,
	listLimit int64,
) {
	if interpret == nil || r == nil {
		return
	}
	riskDB := sfu.GetSSADB()
	if riskDB == nil {
		riskDB = db
	}
	if riskDB == nil {
		_ = ApplySSARiskOverviewDB(interpret, r, nil, parentTask, filter, listLimit)
		return
	}
	if !UseSSARiskOverviewSubLoop() {
		WithInterpretSSAVarLock(func() {
			_ = ApplySSARiskOverviewDB(interpret, r, riskDB, parentTask, filter, listLimit)
		})
		return
	}
	WithInterpretSSAVarLock(func() {
		ov, err := runSSARiskOverviewSubLoopWithChild(r, interpret, parentTask, riskDB, taskID, filter, listLimit)
		if err != nil {
			log.Warnf("[ssa_risk_overview] subloop: %v, fallback Apply", err)
			_ = ApplySSARiskOverviewDB(interpret, r, riskDB, parentTask, filter, listLimit)
			return
		}
		if ov == nil {
			_ = ApplySSARiskOverviewDB(interpret, r, riskDB, parentTask, filter, listLimit)
			return
		}
		CopyOverviewOutputsToParent(ov, interpret)
		hint := interpret.Get("ssa_risk_total_hint")
		AppendSFPipelineLine(interpret, fmt.Sprintf("【3.x·子环 ssa_risk_overview】已完成，approx count=%s", hint))
	})
}
