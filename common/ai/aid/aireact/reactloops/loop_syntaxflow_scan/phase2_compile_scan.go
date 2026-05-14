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

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
)

const (
	introAttach = "下列信息来自已附着任务（SSA Risk 抽样），仅可在此基础上解读；不得编造未列出的 risk id。\n\n"
	introStart  = "下列信息来自同进程新起的扫描，可在此基础上解读；不得编造未列出的 risk id。\n\n"
)

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

func runPhase2(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	state *SyntaxFlowState,
	scanLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
) (*riskDispatcher, error) {
	state.SetPhase(SyntaxFlowPhaseCompileScan)
	pT := task.GetId()
	parentID := OrchestratorParentTaskID(scanLoop, pT)

	switch state.GetSessionMode() {
	case SyntaxFlowSessionModeAttach:
		AppendSFPipelineLine(scanLoop, "【0·入参】附着已有 task_id，跳过编译与起扫。")
		scanLoop.Set(sfu.LoopVarSFCompileMeta, "mode=attach (compile skipped)")
		EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p0_attach", BuildScanStagePhase0Attach(state.GetTaskID(), pT))
		return runAttach(r, db, state, scanLoop, task, state.GetTaskID(), parentID, introAttach)

	case SyntaxFlowSessionModeNone:
		abort(scanLoop, r, "session mode 仍为 none：P1 未提交 attach/start（不应进入 P2）")
		return nil, fmt.Errorf("syntaxflow scan: session mode none after intake")

	case SyntaxFlowSessionModeStart:
		j := strings.TrimSpace(state.GetSFScanConfigJSON())
		if j == "" {
			j = strings.TrimSpace(scanLoop.Get(sfu.LoopVarSFScanConfigJSON))
		}

		proj := strings.TrimSpace(state.GetProjectPath())
		if proj == "" {
			proj = strings.TrimSpace(scanLoop.Get(sfu.LoopVarProjectPath))
		}
		if proj == "" && j == "" {
			abort(scanLoop, r, "缺少扫描配置：start 模式需要项目路径或 sf_scan_config_json。")
			return nil, fmt.Errorf("missing scan config after intake")
		}
		if proj == "" {
			proj = strings.TrimSpace(scanLoop.Get(sfu.LoopVarProjectPath))
		}
		uHint := strings.TrimSpace(task.GetUserInput())
		EmitSyntaxFlowScanPhase(scanLoop, 0, "", "resolve_config", "已得到扫描配置，准备编译 / scan config ready", "", "", nil)
		EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p0_intake", BuildScanStagePhase0Intake(proj, j, utils.ShrinkTextBlock(uHint, 2000), pT))

		var cfg *ssaconfig.Config
		var progs []*ssaapi.Program
		var err error
		var p1 string
		var resolveCfg func() (*ssaconfig.Config, error)
		if proj != "" {
			p1 = BuildScanStagePhase1CompileStart(fmt.Sprintf(`{"_intake":"local_path_only","path":%q}`, strings.TrimSpace(proj)))
			resolveCfg = func() (*ssaconfig.Config, error) {
				return sfu.ResolveCodeScanConfigFromProjectPath(task.GetContext(), proj)
			}
		} else {
			resolveCfg = func() (*ssaconfig.Config, error) {
				return sfu.ResolveCodeScanConfigFromJSON(task.GetContext(), []byte(j))
			}
		}
		cfg, progs, _, err = syntaxFlowCompileFromResolved(task.GetContext(), r, scanLoop, parentID, task, p1, resolveCfg)
		if err != nil {
			return nil, err
		}
		tid, err := syntaxFlowStartScanInBackground(r, task.GetContext(), state, scanLoop, cfg, progs, parentID)
		if err != nil {
			return nil, err
		}
		return runAttach(r, db, state, scanLoop, task, tid, parentID, introStart)

	default:
		abort(scanLoop, r, fmt.Sprintf("不支持的 session mode: %v", state.GetSessionMode()))
		return nil, fmt.Errorf("unsupported session mode: %v", state.GetSessionMode())
	}
}

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

func runAttach(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	state *SyntaxFlowState,
	scanLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	taskID, parentID, intro string,
) (*riskDispatcher, error) {
	disp := newRiskDispatcher(r, scanLoop, task, db, taskID)

	res, err := LoadScanSessionResult(db, taskID, DefaultRiskSampleLimit)
	if err != nil {
		log.Warnf("[syntaxflow_scan] load session: %v", err)
		EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p2_session_degraded", fmt.Sprintf(
			"# 代码扫描\n\n## 会话读库降级\n\n**task_id**: `%s`\n\n读库错误：\n```\n%s\n```\n已注册 watch+poll，将重试。",
			taskID, utils.ShrinkTextBlock(err.Error(), 2000),
		))
		loopSetPreface(scanLoop, fmt.Sprintf("task_id=%s，详情暂不可读：%v\n\n", taskID, err), nil)
		disp.Start(task.GetContext())
		StartScanTaskStatusPoll(scanLoop, r, task, taskID, disp)
		return disp, nil
	}

	scanAlreadyDone := res.ScanTask != nil && res.ScanTask.Status != schema.SYNTAXFLOWSCAN_EXECUTING

	loopSetPreface(scanLoop, intro, res)
	AppendSfScanInterpretLog(scanLoop, r, taskID, "init: 已加载首包 risk 样本")
	EmitSyntaxFlowUserStageMarkdown(scanLoop, parentID, "p2_session_ready", fmt.Sprintf(
		"# 代码扫描·阶段 2\n\n## 会话已就绪\n\n- **task_id**: `%s`\n- **扫描已结束**: %v\n\n**preface 头（截断）**:\n```\n%s\n```",
		taskID, scanAlreadyDone, utils.ShrinkTextBlock(scanLoop.Get("sf_scan_review_preface"), 6000),
	))

	if scanAlreadyDone {
		if res.ScanTask != nil {
			endText := FormatSyntaxFlowScanEndReport(res.ScanTask)
			scanLoop.Set(sfu.LoopVarSFScanEndSummary, endText)
			AppendSFPipelineLine(scanLoop, "【2·结束】"+endText)
		}
		disp.SeedExistingRisks(task.GetContext())
		disp.NotifyScanTerminal()
	} else {
		disp.Start(task.GetContext())
		StartScanTaskStatusPoll(scanLoop, r, task, taskID, disp)
	}
	return disp, nil
}

func loopSetPreface(loop *reactloops.ReActLoop, intro string, res *sfu.ScanSessionResult) {
	preface := intro
	if res != nil {
		preface += res.Preface
	}
	loop.Set("sf_scan_review_preface", preface)
	loop.Set(sfu.LoopVarSyntaxFlowScanSessionMode, sfu.SessionModeAttach)
}

func abort(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, msg string) {
	loop.Set("sf_scan_review_preface", msg)
	if r != nil {
		r.AddToTimeline("syntaxflow_scan", msg)
	}
}
