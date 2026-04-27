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
	"google.golang.org/protobuf/encoding/protojson"
)

const defaultOverviewPageLimit int64 = 40

// PersistEffectiveOverviewFilter stores the filter used for overview queries on the loop (protojson).
// It mirrors the same JSON into sfu.LoopVarSSARisksFilterJSON so BuildSSARisksFilterFromLoop stays aligned.
func PersistEffectiveOverviewFilter(loop *reactloops.ReActLoop, filter *ypb.SSARisksFilter) {
	if loop == nil || filter == nil {
		return
	}
	b, err := protojson.Marshal(filter)
	if err != nil {
		return
	}
	s := string(b)
	loop.Set(sfu.LoopVarSSAOverviewFilterJSON, s)
	loop.Set(sfu.LoopVarSSARisksFilterJSON, s)
}

func appendUniqueStr(slice []string, v string) []string {
	for _, x := range slice {
		if x == v {
			return slice
		}
	}
	return append(slice, v)
}

// MergeReloadSSARiskOverviewFilter builds the filter for reload_ssa_risk_overview from action params,
// stored sfu.LoopVarSSAOverviewFilterJSON, or from BuildSSARisksFilterFromLoop after sfu.SyncSSARisksFilterFromIrifyToLoop.
func MergeReloadSSARiskOverviewFilter(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, action *aicommon.Action) *ypb.SSARisksFilter {
	rawFj := strings.TrimSpace(action.GetString("filter_json"))
	var base *ypb.SSARisksFilter
	if rawFj != "" {
		base = &ypb.SSARisksFilter{}
		if err := protojson.Unmarshal([]byte(rawFj), base); err != nil {
			sfu.SyncSSARisksFilterFromIrifyToLoop(loop, task)
			base = sfu.BuildSSARisksFilterFromLoop(loop, "")
		}
	} else if loop != nil {
		if s := strings.TrimSpace(loop.Get(sfu.LoopVarSSAOverviewFilterJSON)); s != "" {
			base = &ypb.SSARisksFilter{}
			if err := protojson.Unmarshal([]byte(s), base); err != nil {
				base = nil
			}
		}
	}
	if base == nil {
		if loop != nil && task != nil && strings.TrimSpace(loop.Get(sfu.LoopVarSSAOverviewFilterJSON)) == "" {
			sfu.SyncSSARisksFilterFromIrifyToLoop(loop, task)
		}
		base = sfu.BuildSSARisksFilterFromLoop(loop, "")
	}
	if s := strings.TrimSpace(action.GetString("search")); s != "" {
		base.Search = s
	}
	for _, part := range strings.Split(action.GetString("runtime_id"), ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			base.RuntimeID = appendUniqueStr(base.RuntimeID, part)
		}
	}
	if p := strings.TrimSpace(action.GetString("program_name")); p != "" {
		base.ProgramName = appendUniqueStr(base.ProgramName, p)
	}
	return base
}

// ApplySSARiskOverviewDB loads counts and a page of risks into loop vars (same fields as loop_ssa_risk_overview init_task).
func ApplySSARiskOverviewDB(loop *reactloops.ReActLoop, invoker aicommon.AIInvokeRuntime, db *gorm.DB, task aicommon.AIStatefulTask, filter *ypb.SSARisksFilter, listLimit int64) string {
	if loop == nil || invoker == nil {
		return ""
	}
	if db == nil {
		invoker.AddToTimeline("ssa_risk_overview", "当前环境无数据库连接，无法列出 SSA Risk。请检查项目数据库与 SSA 工程库配置后重试。")
		loop.Set("ssa_risk_overview_preface", "无 DB：仅可根据用户文字做一般性建议，勿编造 risk_id。")
		loop.Set("ssa_risk_total_hint", "")
		return loop.Get("ssa_risk_overview_preface")
	}
	if listLimit <= 0 {
		listLimit = defaultOverviewPageLimit
	}
	PersistEffectiveOverviewFilter(loop, filter)

	count, err := yakit.QuerySSARiskCount(db, filter)
	if err != nil {
		log.Warnf("[ssa_risk_overview] QuerySSARiskCount: %v", err)
		msg := fmt.Sprintf("统计 SSA Risk 失败: %v", err)
		invoker.AddToTimeline("ssa_risk_overview", msg)
		loop.Set("ssa_risk_overview_preface", "无法完成数据库统计。\n\n"+msg)
		loop.Set("ssa_risk_total_hint", "")
		return loop.Get("ssa_risk_overview_preface")
	}

	paging := &ypb.Paging{Page: 1, Limit: listLimit, OrderBy: "id", Order: "desc"}
	_, risks, err := yakit.QuerySSARisk(db, filter, paging)
	if err != nil {
		log.Warnf("[ssa_risk_overview] QuerySSARisk: %v", err)
		msg := fmt.Sprintf("查询 SSA Risk 失败: %v", err)
		invoker.AddToTimeline("ssa_risk_overview", msg)
		loop.Set("ssa_risk_overview_preface", "无法拉取风险列表。\n\n"+msg)
		loop.Set("ssa_risk_total_hint", fmt.Sprintf("%d", count))
		return loop.Get("ssa_risk_overview_preface")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("匹配条件 approximate count: %d；本页抽样 %d 条。\n\n", count, len(risks)))
	for i, rk := range risks {
		sb.WriteString(fmt.Sprintf("%d. id=%d | sev=%s | program=%s | rule=%s | title=%s\n",
			i+1, rk.ID, rk.Severity, utils.ShrinkTextBlock(rk.ProgramName, 80),
			utils.ShrinkTextBlock(rk.FromRule, 60), utils.ShrinkTextBlock(rk.Title, 120)))
	}
	summary := sb.String()
	loop.Set("ssa_risk_list_summary", summary)
	loop.Set("ssa_risk_total_hint", fmt.Sprintf("%d", count))
	invoker.AddToTimeline("ssa_risk_overview", utils.ShrinkTextBlock(summary, 4000))

	preface := "下列摘要来自数据库查询，仅可在此基础上归纳、聚类、搜索建议；不得编造未列出的 risk_id。\n\n" + summary
	loop.Set("ssa_risk_overview_preface", preface)
	return preface
}

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
					pipe := loop.Get(sfu.LoopVarSFPipelineSummary)
					full := "【==== 大总结用数据：以下须全部纳入终局报告 ====】\n\n"
					full += "【A·各阶段摘要 sf_scan_pipeline_summary】\n" + pipe + "\n\n"
					full += "【B·扫描行终态】\n" + endText + "\n\n"
					full += "【C·风险列表与抽样】优先阅读 reactive 中 ssa_risk_list_summary / ssa_risk_total_hint；与 preface 中条目不冲突。\n\n"
					full += "下列信息来自数据库（扫描已结束，任务行 + SSA Risk 列表）：\n" + res.Preface
					loop.Set("sf_scan_review_preface", full)
					loop.Set(sfu.LoopVarSFFinalReportDue, "1")
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

const (
	// envSSARiskOverviewSubLoop 不为 "0"/"false"/"off" 时，终局/轮询总览走 ssa_risk_overview 子环 + copy（失败则回退 Apply）。
	envSSARiskOverviewSubLoop = "YAK_SSA_RISK_OVERVIEW_SUBLOOP"
	// envSSARiskOverviewInScanSubLoop 为 "1"/"true"/"on" 时，长扫中周期总览也跑短子环（默认关，仅 emit）。
	envSSARiskOverviewInScanSubLoop = "YAK_SSA_RISK_OVERVIEW_IN_SCAN_SUBLOOP"
)

var interpretSSAVarMu sync.Mutex

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

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

// UseInScanSSARiskOverviewSubLoop 长扫中周期子环（成本高；默认关）。
func UseInScanSSARiskOverviewSubLoop() bool {
	return envTruthy(os.Getenv(envSSARiskOverviewInScanSubLoop))
}

// WithInterpretSSAVarLock serializes writers that Set ssa_risk_* on the interpret loop from
// the poll goroutine vs. other code paths. reload_ssa_risk_overview 仍走模型同线程，不在此包一层。
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

// CopyOverviewOutputsToParent copies overview loop vars needed by syntaxflow_scan_interpret reactive.
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
	for _, k := range []string{sfu.LoopVarSSARisksFilterJSON, sfu.LoopVarSSAOverviewFilterJSON, sfu.LoopVarSyntaxFlowScanSessionMode, "sf_scan_task_id"} {
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

// ApplySSARiskOverviewToInterpret 终局/轮询入口：优先子环 + copy，失败或 env 关则 Apply 直写 interpret。
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
