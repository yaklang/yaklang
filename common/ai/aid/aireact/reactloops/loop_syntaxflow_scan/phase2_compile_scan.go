package loop_syntaxflow_scan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
)

// AppendSFPipelineLine appends one line to sf_scan_pipeline_summary (compile / scan / overview ticks / 终态).
func AppendSFPipelineLine(loop *reactloops.ReActLoop, line string) {
	if loop == nil {
		return
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	prev := strings.TrimSpace(loop.Get(sfu.LoopVarSFPipelineSummary))
	if prev == "" {
		loop.Set(sfu.LoopVarSFPipelineSummary, line)
		return
	}
	loop.Set(sfu.LoopVarSFPipelineSummary, prev+"\n"+line)
}

// runPhaseCompileAndScan is P2 orchestrator:
//
//	switch state.GetSessionMode():
//	  Attach → attachAndWireSession（跳过 compile/startScan）
//	  Start → compile → startScan → attachAndWireSession
//	  None → 错误（P1 应已写入 attach 或 start）
func runPhaseCompileAndScan(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	state *SyntaxFlowState,
	scanLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
) error {
	state.SetPhase(SyntaxFlowPhaseCompileScan)
	pT := task.GetId()
	parentID := OrchestratorParentTaskID(scanLoop, pT)

	switch state.GetSessionMode() {
	case SyntaxFlowSessionModeAttach:
		AppendSFPipelineLine(scanLoop, "【0·入参】附着已有 task_id，跳过编译与起扫。")
		scanLoop.Set(sfu.LoopVarSFCompileMeta, "mode=attach (compile skipped)")
		EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p0_attach", BuildScanStagePhase0Attach(state.GetTaskID(), pT))
		EmitSyntaxFlowScanPhase(scanLoop, 1, "end", "compile_skipped", "附着 task_id，跳过本地编译 / attach mode, skip compile", state.GetTaskID(), "", nil)
		return attachAndWireSession(r, db, state, scanLoop, task, state.GetTaskID(), parentID, pT,
			"下列信息来自已附着任务（SSA Risk 抽样），仅可在此基础上解读；不得编造未列出的 risk id。\n\n")

	case SyntaxFlowSessionModeNone:
		abort(scanLoop, r, "session mode 仍为 none：P1 未提交 attach/start（不应进入 P2）")
		return fmt.Errorf("syntaxflow scan: session mode none after intake")

	case SyntaxFlowSessionModeStart:
		j := strings.TrimSpace(state.GetSFScanConfigJSON())
		if j == "" {
			j = strings.TrimSpace(scanLoop.Get(sfu.LoopVarSFScanConfigJSON))
		}

		inferred := state.GetConfigInferred()
		if inferred == "" {
			inferred = "0"
		}
		if strings.TrimSpace(scanLoop.Get(sfu.LoopVarSFScanConfigJSON)) == "" && j != "" {
			scanLoop.Set(sfu.LoopVarSFScanConfigJSON, j)
		}
		scanLoop.Set("sf_scan_config_inferred", inferred)

		proj := strings.TrimSpace(state.GetProjectPath())
		if proj == "" {
			proj = strings.TrimSpace(scanLoop.Get(sfu.LoopVarProjectPath))
		}
		uHint := strings.TrimSpace(task.GetUserInput())
		EmitSyntaxFlowScanProgress(scanLoop, "resolve_config", "已得到扫描配置，准备编译 / scan config ready", "", "")
		EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p0_intake", BuildScanStagePhase0Intake(proj, inferred, utils.ShrinkTextBlock(uHint, 2000), pT))

		usePath := proj != "" && (j == "" || inferred == "1")
		if !usePath && j == "" {
			abort(scanLoop, r, "缺少扫描配置：start 模式需要项目路径或 sf_scan_config_json。")
			return fmt.Errorf("missing scan config after intake")
		}
		var cfg *ssaconfig.Config
		var progs []*ssaapi.Program
		var err error
		var p1 string
		var resolveCfg func() (*ssaconfig.Config, error)
		if usePath {
			p1 = BuildScanStagePhase1CompileStart(fmt.Sprintf(`{"_intake":"local_path_only","path":%q}`, strings.TrimSpace(proj)))
			pathArg := proj
			resolveCfg = func() (*ssaconfig.Config, error) {
				return sfu.ResolveCodeScanConfigFromProjectPath(task.GetContext(), pathArg)
			}
		} else {
			p1 = BuildScanStagePhase1CompileStart(j)
			jsonArg := j
			resolveCfg = func() (*ssaconfig.Config, error) {
				return sfu.ResolveCodeScanConfigFromJSON(task.GetContext(), []byte(jsonArg))
			}
		}
		cfg, progs, _, err = syntaxFlowCompileFromResolved(task.GetContext(), r, scanLoop, parentID, task, p1, resolveCfg)
		if err != nil {
			return err
		}
		tid, err := syntaxFlowStartScanInBackground(r, task.GetContext(), state, scanLoop, cfg, progs, parentID)
		if err != nil {
			return err
		}
		return attachAndWireSession(r, db, state, scanLoop, task, tid, parentID, pT,
			"下列信息来自同进程新起的扫描，可在此基础上解读；不得编造未列出的 risk id。\n\n")

	default:
		abort(scanLoop, r, fmt.Sprintf("不支持的 session mode: %v", state.GetSessionMode()))
		return fmt.Errorf("unsupported session mode: %v", state.GetSessionMode())
	}
}

// runSyntaxFlowCompilePhase 执行阶段 1 编译：心跳、加载（由 load 提供）、emit。
func runSyntaxFlowCompilePhase(
	ctx context.Context,
	r aicommon.AIInvokeRuntime,
	scanLoop *reactloops.ReActLoop,
	parentID string,
	task aicommon.AIStatefulTask,
	p1CompileStartMarkdown string,
	load func() (*ssaconfig.Config, []*ssaapi.Program, error),
) (cfg *ssaconfig.Config, progs []*ssaapi.Program, compileMs int64, err error) {
	_ = task
	compileT0 := time.Now()
	EmitSyntaxFlowScanPhase(scanLoop, 1, "start", "compile", "阶段1：开始编译 SSA（落库模式）/ phase1 compile (DB)", "", "", nil)
	EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p1_compile_start", p1CompileStartMarkdown)

	var stopHB chan struct{}
	if ctx != nil {
		stopHB = make(chan struct{})
		var hbSeq int
		go func() {
			timer := time.NewTimer(3 * time.Minute)
			defer timer.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-stopHB:
					return
				case <-timer.C:
					hbSeq++
					elapsed := time.Since(compileT0)
					EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, fmt.Sprintf("p1_compile_heartbeat_%d", hbSeq),
						BuildScanStagePhase1CompileHeartbeat(elapsed, hbSeq))
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					timer.Reset(3 * time.Minute)
				}
			}
		}()
	}

	cfg, progs, err = load()
	if stopHB != nil {
		close(stopHB)
	}
	if err != nil {
		log.Warnf("[syntaxflow_scan] compile/load programs: %v", err)
		EmitSyntaxFlowScanPhase(scanLoop, 1, "end", "compile_failed", "编译失败 / compile failed", "", err.Error(), nil)
		EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p1_compile_fail", BuildScanStagePhase1CompileFailed(err.Error()))
		abort(scanLoop, r, fmt.Sprintf("起扫前编译失败：%v", err))
		return nil, nil, 0, err
	}
	pn := ""
	if cfg != nil {
		pn = cfg.GetProgramName()
	}
	compileMs = time.Since(compileT0).Milliseconds()
	meta := fmt.Sprintf("program_name=%q program_count=%d duration_ms=%d", pn, len(progs), compileMs)
	scanLoop.Set(sfu.LoopVarSFCompileMeta, meta)
	AppendSFPipelineLine(scanLoop, "【1·编译】"+meta)
	EmitSyntaxFlowScanPhase(scanLoop, 1, "end", "compile_ok", "阶段1：编译完成 / phase1 compile done", "", "", map[string]any{
		"program_name": pn, "program_count": len(progs), "duration_ms": compileMs,
	})
	EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p1_compile_done", BuildScanStagePhase1CompileDone(progs, compileMs))
	return cfg, progs, compileMs, nil
}

// syntaxFlowCompileFromResolved：先 resolveCfg 得 cfg，再 [sfu.CompileProgramsFromCodeScanConfig]（与 JSON 或 path 无关）。
func syntaxFlowCompileFromResolved(
	ctx context.Context,
	r aicommon.AIInvokeRuntime,
	scanLoop *reactloops.ReActLoop,
	parentID string,
	task aicommon.AIStatefulTask,
	p1CompileStartMarkdown string,
	resolveCfg func() (*ssaconfig.Config, error),
) (cfg *ssaconfig.Config, progs []*ssaapi.Program, compileMs int64, err error) {
	return runSyntaxFlowCompilePhase(ctx, r, scanLoop, parentID, task, p1CompileStartMarkdown, func() (*ssaconfig.Config, []*ssaapi.Program, error) {
		cfg, err := resolveCfg()
		if err != nil {
			return nil, nil, err
		}
		progs, err := sfu.CompileProgramsFromCodeScanConfig(ctx, cfg)
		return cfg, progs, err
	})
}

// syntaxFlowStartScanInBackground: call engine StartScanInBackground, write back task_id to state and loop.
func syntaxFlowStartScanInBackground(
	r aicommon.AIInvokeRuntime,
	scanCtx context.Context,
	state *SyntaxFlowState,
	scanLoop *reactloops.ReActLoop,
	cfg *ssaconfig.Config,
	progs []*ssaapi.Program,
	parentID string,
) (tid string, err error) {
	opts := make([]ssaconfig.Option, 0, 6)
	if scanCtx != nil {
		opts = append(opts, ssaconfig.WithContext(scanCtx))
	}
	opts = append(opts, syntaxflow_scan.WithPrograms(progs...))
	opts = append(opts, CodeScanToSyntaxFlowRuleOptions(cfg)...)
	EmitSyntaxFlowScanPhase(scanLoop, 2, "start", "scan", "阶段2：启动 SyntaxFlow 扫描 / phase2 scan start", "", "", nil)
	AppendSFPipelineLine(scanLoop, "【2·起扫】准备 StartScanInBackground（program 见上）")
	EmitSyntaxFlowScanProgress(scanLoop, "start_scan", "启动后台 SyntaxFlow 扫描 / starting background scan", "", "")
	tid, err = syntaxflow_scan.StartScanInBackground(scanCtx, opts...)
	if err != nil {
		log.Warnf("[syntaxflow_scan] start: %v", err)
		abort(scanLoop, r, fmt.Sprintf("起扫失败：%v", err))
		return "", err
	}
	state.SetTaskID(tid)
	scanLoop.Set(sfu.LoopVarSyntaxFlowTaskID, tid)
	AppendSFPipelineLine(scanLoop, fmt.Sprintf("【2·起扫】task_id=%s 后台扫描已启动", tid))
	EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p2_scan_started", BuildScanStagePhase2ScanStart(tid))
	return tid, nil
}

// attachAndWireSession loads the scan session for taskID (may degrade to nil), calls wireSession, emits ready, starts poll if needed.
// Shared tail for both the attach path (skip compile+start) and the fresh-scan path (after startScan).
func attachAndWireSession(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	state *SyntaxFlowState,
	scanLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	taskID, parentID, pT, intro string,
) error {
	_ = pT
	inferred := state.GetConfigInferred()
	if inferred == "" {
		inferred = "0"
	}

	EmitSyntaxFlowScanProgress(scanLoop, "load_session", "加载扫描会话 / loading scan session", taskID, "")
	res, err := LoadScanSessionResult(db, taskID, DefaultRiskSampleLimit)
	if err != nil {
		log.Warnf("[syntaxflow_scan] load session: %v", err)
		EmitSyntaxFlowScanProgress(scanLoop, "session_degraded",
			"任务行暂不可读 / task row unreadable (DB/migrations)", taskID, err.Error())
		EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p2_session_degraded", fmt.Sprintf(
			"# 代码扫描\n\n## 会话读库降级\n\n**task_id**: `%s`\n\n读库错误：\n```\n%s\n```\n已注册 watch+poll，将重试。",
			taskID, utils.ShrinkTextBlock(err.Error(), 2000),
		))
		poll := wireSession(r, db, scanLoop, task, taskID, inferred,
			fmt.Sprintf("task_id=%s，详情暂不可读：%v\n\n", taskID, err), nil)
		if poll {
			EmitSyntaxFlowScanPhase(scanLoop, 2, "tick", "scan_running", "扫描执行中（任务行待可读）/ running", taskID, "", nil)
			emitPostWireHandoff(scanLoop, r, db, task, taskID, poll, nil,
				"阶段3：后台风险轮询与 SSA 总览灌入 / phase3 risk poll",
				"阶段3：扫描已结束，准备终局物化 / phase3 scan terminal")
			EmitSyntaxFlowScanProgress(scanLoop, "watch", "扫描未结束，已订阅轮询 / scan running, poll registered", taskID, "")
		}
		return nil
	}

	if res.ScanTask != nil && res.ScanTask.Status != schema.SYNTAXFLOWSCAN_EXECUTING {
		EmitSyntaxFlowScanPhase(scanLoop, 2, "end", "scan_done", "扫描已结束 / scan done", taskID, "", nil)
	} else {
		st := ""
		if res.ScanTask != nil {
			st = res.ScanTask.Status
		}
		EmitSyntaxFlowScanPhase(scanLoop, 2, "tick", "scan_running", "扫描执行中 / scan running", taskID, "", map[string]any{
			"status": st,
		})
	}

	poll := wireSession(r, db, scanLoop, task, taskID, inferred, intro, res)
	AppendSfScanInterpretLog(scanLoop, r, taskID, "init: 已加载首包 risk 样本")
	EmitSyntaxFlowScanProgress(scanLoop, "ready", "扫描会话已就绪 / scan session ready", taskID, "")
	EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p2_session_ready", fmt.Sprintf(
		"# 代码扫描·阶段 2\n\n## 会话已就绪\n\n- **task_id**: `%s`\n- **轮询**: %v\n\n**preface 头（截断）**:\n```\n%s\n```",
		taskID, poll, utils.ShrinkTextBlock(scanLoop.Get("sf_scan_review_preface"), 6000),
	))
	emitPostWireHandoff(scanLoop, r, db, task, taskID, poll, res,
		"阶段3：后台风险轮询、SSA 总览与终局物化 / phase3 risk poll",
		"阶段3：扫描已结束，准备终局物化 / phase3 scan terminal")
	if poll {
		EmitSyntaxFlowScanProgress(scanLoop, "watch", "扫描未结束，已订阅轮询 / scan running, poll registered", taskID, "")
	}
	return nil
}

func abort(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, msg string) {
	loop.Set("sf_scan_review_preface", msg)
	if r != nil {
		r.AddToTimeline("syntaxflow_scan", msg)
	}
	EmitSyntaxFlowScanProgress(loop, "failed", "初始化未通过 / init aborted", "", msg)
}

// emitPostWireHandoff: after wireSession, either register risk poll (step 3) or apply final context + P4 hints (scan already done).
func emitPostWireHandoff(
	scanLoop *reactloops.ReActLoop,
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	task aicommon.AIStatefulTask,
	taskID string,
	poll bool,
	res *ScanSessionResult,
	msgWhenPolling, msgWhenAlreadyDone string,
) {
	if poll {
		EmitSyntaxFlowScanPhase(scanLoop, 3, "start", "risk_watch", msgWhenPolling, taskID, "", nil)
		return
	}
	ApplyFinalReportContextWhenScanAlreadyDone(scanLoop, r, db, task, taskID, res)
	EmitSyntaxFlowScanPhase(scanLoop, 3, "start", "risk_watch", msgWhenAlreadyDone, taskID, "", nil)
	EmitSyntaxFlowScanProgress(scanLoop, "final_report_required", "须输出大总结 / MUST deliver final merged report", taskID, "")
	EmitSyntaxFlowScanPhase(scanLoop, 4, "start", "final_report_mandatory",
		"请输出含各阶段 + 扫描统计 + 全部 risk 的终局大总结 / deliver final report (mandatory)", taskID, "", nil)
}

// wireSession writes the scan session into loop vars + timeline, registers poll if still executing.
func wireSession(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	taskID, inferred, intro string,
	res *ScanSessionResult,
) (poll bool) {
	loop.Set(sfu.LoopVarSyntaxFlowTaskID, taskID)
	loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, sfu.SessionModeAttach)
	loop.Set("sf_scan_config_inferred", inferred)
	preface := intro
	if res != nil {
		preface += res.Preface
	}
	loop.Set("sf_scan_review_preface", preface)
	r.AddToTimeline("syntaxflow_scan", utils.ShrinkTextBlock(preface, 4000))
	PersistEffectiveOverviewFilter(loop, &ypb.SSARisksFilter{RuntimeID: []string{taskID}})
	poll = res == nil || (res.ScanTask != nil && res.ScanTask.Status == schema.SYNTAXFLOWSCAN_EXECUTING)
	if poll {
		StartScanTaskStatusPoll(db, loop, r, task, taskID, task.GetContext())
	}
	return poll
}
