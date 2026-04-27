package loop_syntaxflow_scan

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	sfu "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/syntaxflow_utils"
	"github.com/yaklang/yaklang/common/log"
)

var programNameSanitize = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// inferredSSAProgramNameForPath returns a stable, DB-suitable program name for same-process
// SyntaxFlow / code-scan runs when the user did not set program_name in JSON.
func inferredSSAProgramNameForPath(localPath string) string {
	clean := filepath.Clean(strings.TrimSpace(localPath))
	base := filepath.Base(clean)
	base = programNameSanitize.ReplaceAllString(base, "_")
	if base == "" || base == "." {
		base = "proj"
	}
	sum := sha256.Sum256([]byte(clean))
	return fmt.Sprintf("ai_sf_%s_%x", base, sum[:6])
}

// BuildCodeScanJSONForLocalPath builds a minimal code-scan JSON for a local file or directory
// (in-process compile + SyntaxFlow; no language guessing from full user text).
func BuildCodeScanJSONForLocalPath(localPath string) (string, error) {
	p := strings.TrimSpace(localPath)
	if p == "" {
		return "", errors.New("empty local path")
	}
	if st, err := os.Stat(p); err != nil {
		return "", err
	} else if !st.IsDir() {
		p = filepath.Dir(p)
	}
	raw, err := buildMinimalInProcessCodeScanJSON(p)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func buildMinimalInProcessCodeScanJSON(localPath string) ([]byte, error) {
	localPath = strings.TrimSpace(localPath)
	if localPath == "" {
		return nil, errors.New("empty path")
	}
	cfg, err := ssaconfig.NewCLIScanConfig(
		ssaconfig.WithCodeSourceKind(ssaconfig.CodeSourceLocal),
		ssaconfig.WithCodeSourceLocalFile(localPath),
		ssaconfig.WithCompileMemoryCompile(false),
		ssaconfig.WithSyntaxFlowMemory(false),
		ssaconfig.WithSetProgramName(inferredSSAProgramNameForPath(localPath)),
	)
	if err != nil {
		return nil, err
	}
	return cfg.ToJSONRaw()
}

// LoadProgramsFromCodeScanJSON 解析与 `yak code-scan --config` 同族的 JSON，并加载 SSA Program。
// 使用 ssaapi.ParseProject（**不**经 ssa_compile 的 Yak 插件路径）：内存模式与落库模式均走同进程 SSA 编译。
// 在存在 profile 库时，会按 [github.com/yaklang/yaklang/common/yak/ssa_compile.EnsureSSAProjectRowForCodeScan]
// 与 SSAProject 表对齐（查/建/更新配置并写回 project_id），避免「有 program、无 project」的语义断裂。
//
// 同进程起扫不依赖 yak 插件的 SSA 编译；本包在 profile 存在时仅调用 EnsureSSAProjectRowForCodeScan 对齐工程行。
func LoadProgramsFromCodeScanJSON(ctx context.Context, jsonRaw []byte) (cfg *ssaconfig.Config, progs []*ssaapi.Program, err error) {
	if len(jsonRaw) == 0 {
		return nil, nil, utils.Error("empty code-scan config json")
	}
	cfg, err = ssaconfig.NewCLIScanConfig(ssaconfig.WithJsonRawConfig(jsonRaw))
	if err != nil {
		return nil, nil, err
	}
	if db := consts.GetGormProfileDatabase(); db != nil {
		cfg, _, err = ssa_compile.EnsureSSAProjectRowForCodeScan(ctx, db, cfg)
		if err != nil {
			return nil, nil, err
		}
	}
	progs, err = loadProgramsForCodeScanConfig(ctx, cfg)
	if err != nil {
		return cfg, nil, err
	}
	return cfg, progs, nil
}

func loadProgramsForCodeScanConfig(ctx context.Context, cfg *ssaconfig.Config) ([]*ssaapi.Program, error) {
	if cfg == nil {
		return nil, utils.Error("config is nil")
	}
	targetPath := cfg.GetCodeSourceLocalFileOrURL()
	programName := strings.TrimSpace(cfg.GetProgramName())

	if targetPath != "" {
		// 落库模式：需非空 program_name，ssaapi 才会用 ProgramCacheDBWrite；否则用内存 IR。
		if !cfg.GetCompileMemory() && strings.TrimSpace(cfg.GetProgramName()) == "" {
			cfg.SetProgramName(inferredSSAProgramNameForPath(targetPath))
		}
		configJSON, err := cfg.ToJSONString()
		if err != nil {
			return nil, err
		}
		ps, err := ssaapi.ParseProject(
			ssaconfig.WithConfigJson(configJSON),
			ssaconfig.WithContext(ctx),
		)
		if err != nil {
			return nil, err
		}
		if len(ps) == 0 {
			return nil, utils.Errorf("内存编译未产生任何 Program")
		}
		return []*ssaapi.Program(ps), nil
	}
	if programName != "" {
		ret := ssaapi.LoadProgramRegexp(programName)
		if len(ret) == 0 {
			return nil, utils.Errorf("数据库中未找到 SSA Program: %s", programName)
		}
		return ret, nil
	}
	return nil, utils.Errorf("code-scan JSON 需包含 CodeSource 本地路径，或 BaseInfo.program_names 指向已编译 Program")
}

// CodeScanToSyntaxFlowRuleOptions 与 yak code-scan 在 useConfigMode 下追加到 StartScan 的规则/内存相关选项对齐（子集；不含 WithPrograms / WithContext）。
func CodeScanToSyntaxFlowRuleOptions(cfg *ssaconfig.Config) []ssaconfig.Option {
	if cfg == nil {
		return nil
	}
	out := make([]ssaconfig.Option, 0, 4)
	out = append(out, ssaconfig.WithRuleFilterLibRuleKind("noLib"))
	out = append(out, ssaconfig.WithSyntaxFlowMemory(cfg.GetSyntaxFlowMemory()))
	if rf := cfg.GetRuleFilter(); rf != nil {
		out = append(out, ssaconfig.WithRuleFilter(rf))
	}
	return out
}

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

// runPhaseCompileAndScan runs P2: attach to task_id or compile+StartScan, then wireSession on the interpret sub-loop.
func runPhaseCompileAndScan(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	state *SyntaxFlowState,
	interpretLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
) error {
	state.SetPhase(SyntaxFlowPhaseCompileScan)
	pT := task.GetId()
	parentID := OrchestratorParentTaskID(interpretLoop, pT)

	// Re-resolve task id on the loop (after parent→child var sync) for tool reload_* consistency.
	if id := strings.TrimSpace(interpretLoop.Get(sfu.LoopVarSyntaxFlowTaskID)); id != "" {
		return phaseAttachToTaskID(r, db, state, interpretLoop, task, id)
	}

	j := strings.TrimSpace(state.GetResolvedSFScanConfigJSON())
	if j == "" {
		j = strings.TrimSpace(interpretLoop.Get(sfu.LoopVarSFScanConfigJSON))
	}
	if j == "" {
		abort(interpretLoop, r, "缺少扫描配置：无 task_id 且未解析到 sf_scan_config_json。")
		return fmt.Errorf("missing scan config after intake")
	}

	EmitSyntaxFlowScanProgress(interpretLoop, "resolve_config", "已得到扫描配置，准备编译 / scan config ready", "", "")

	inferred := state.GetConfigInferred()
	if inferred == "" {
		inferred = "0"
	}
	if strings.TrimSpace(interpretLoop.Get(sfu.LoopVarSFScanConfigJSON)) == "" {
		interpretLoop.Set(sfu.LoopVarSFScanConfigJSON, j)
	}
	interpretLoop.Set("sf_scan_config_inferred", inferred)

	proj := strings.TrimSpace(interpretLoop.Get(sfu.LoopVarProjectPath))
	uHint := strings.TrimSpace(task.GetUserInput())
	phase0 := BuildScanStagePhase0Intake(proj, inferred, utils.ShrinkTextBlock(uHint, 2000), pT)
	EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p0_intake", phase0)

	compileT0 := time.Now()
	EmitSyntaxFlowScanPhase(interpretLoop, 1, "start", "compile", "阶段1：开始编译 SSA（落库模式）/ phase1 compile (DB)", "", "", nil)
	EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p1_compile_start", BuildScanStagePhase1CompileStart(j))

	ctx := task.GetContext()
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
					EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, fmt.Sprintf("p1_compile_heartbeat_%d", hbSeq),
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

	cfg, progs, err := LoadProgramsFromCodeScanJSON(ctx, []byte(j))
	if stopHB != nil {
		close(stopHB)
	}
	if err != nil {
		log.Warnf("[syntaxflow_scan] compile/load programs: %v", err)
		EmitSyntaxFlowScanPhase(interpretLoop, 1, "end", "compile_failed", "编译失败 / compile failed", "", err.Error(), nil)
		EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p1_compile_fail", BuildScanStagePhase1CompileFailed(err.Error()))
		abort(interpretLoop, r, fmt.Sprintf("起扫前编译失败：%v", err))
		return err
	}
	pn := ""
	if cfg != nil {
		pn = cfg.GetProgramName()
	}
	compileMs := time.Since(compileT0).Milliseconds()
	// 归约为 []*ssaapi.Program 以复用表构建（ssaconfig 返回的 progs 已是 ssaapi.Program 切片）
	_ = pn
	meta := fmt.Sprintf("program_name=%q program_count=%d duration_ms=%d", pn, len(progs), compileMs)
	interpretLoop.Set(sfu.LoopVarSFCompileMeta, meta)
	AppendSFPipelineLine(interpretLoop, "【1·编译】"+meta)
	EmitSyntaxFlowScanPhase(interpretLoop, 1, "end", "compile_ok", "阶段1：编译完成 / phase1 compile done", "", "", map[string]any{
		"program_name": pn, "program_count": len(progs), "duration_ms": compileMs,
	})
	EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p1_compile_done", BuildScanStagePhase1CompileDone(progs, compileMs))

	scanCtx := task.GetContext()
	opts := make([]ssaconfig.Option, 0, 6)
	if scanCtx != nil {
		opts = append(opts, ssaconfig.WithContext(scanCtx))
	}
	opts = append(opts, syntaxflow_scan.WithPrograms(progs...))
	opts = append(opts, CodeScanToSyntaxFlowRuleOptions(cfg)...)
	EmitSyntaxFlowScanPhase(interpretLoop, 2, "start", "scan", "阶段2：启动 SyntaxFlow 扫描 / phase2 scan start", "", "", nil)
	AppendSFPipelineLine(interpretLoop, "【2·起扫】准备 StartScanInBackground（program 见上）")
	EmitSyntaxFlowScanProgress(interpretLoop, "start_scan", "启动后台 SyntaxFlow 扫描 / starting background scan", "", "")
	tid, err := syntaxflow_scan.StartScanInBackground(scanCtx, opts...)
	if err != nil {
		log.Warnf("[syntaxflow_scan] start: %v", err)
		abort(interpretLoop, r, fmt.Sprintf("起扫失败：%v", err))
		return err
	}
	state.SetTaskID(tid)
	interpretLoop.Set(sfu.LoopVarSyntaxFlowTaskID, tid)
	AppendSFPipelineLine(interpretLoop, fmt.Sprintf("【2·起扫】task_id=%s 后台扫描已启动", tid))
	EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p2_scan_started", BuildScanStagePhase2ScanStart(tid))

	EmitSyntaxFlowScanProgress(interpretLoop, "load_session", "读取任务行与风险摘要 / loading task row and risks", tid, "")
	res, err := LoadScanSessionResult(db, tid, DefaultRiskSampleLimit)
	if err != nil {
		log.Warnf("[syntaxflow_scan] load after start: %v", err)
		EmitSyntaxFlowScanProgress(interpretLoop, "session_degraded",
			"已起扫，任务行暂不可读，请检查数据库迁移或 SSA 工程库 / task row unreadable (DB/migrations)", tid, err.Error())
		EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p2_session_degraded", fmt.Sprintf(
			"# 代码扫描·阶段 2\n\n## 会话读库降级\n\n**task_id**: `%s`\n\n读库错误：\n```\n%s\n```\n已注册 watch+poll，将重试。",
			tid, utils.ShrinkTextBlock(err.Error(), 2000),
		))
		if poll := wireSession(r, db, interpretLoop, task, tid, inferred,
			fmt.Sprintf("已起扫 task_id=%s，详情暂不可读：%v\n\n", tid, err), nil); poll {
			EmitSyntaxFlowScanPhase(interpretLoop, 2, "tick", "scan_running", "阶段2：扫描执行中（任务行待可读）/ phase2 running", tid, "", nil)
			EmitSyntaxFlowScanPhase(interpretLoop, 3, "start", "interpret", "阶段3：风险轮询与解读 / phase3 interpret", tid, "", nil)
			EmitSyntaxFlowScanProgress(interpretLoop, "watch", "扫描未结束，已订阅 SSA 更新并轮询任务行 / scan running, SSA watch + task poll", tid, "")
		}
		return nil
	}
	if res.ScanTask != nil && res.ScanTask.Status != schema.SYNTAXFLOWSCAN_EXECUTING {
		EmitSyntaxFlowScanPhase(interpretLoop, 2, "end", "scan_done", "阶段2：扫描已结束 / phase2 scan done", tid, "", nil)
	} else {
		st := ""
		if res.ScanTask != nil {
			st = res.ScanTask.Status
		}
		EmitSyntaxFlowScanPhase(interpretLoop, 2, "tick", "scan_running", "阶段2：扫描执行中 / phase2 scan running", tid, "", map[string]any{
			"status": st,
		})
	}
	poll := wireSession(r, db, interpretLoop, task, tid, inferred,
		"下列信息来自同进程新起的扫描，可在此基础上解读；不得编造未列出的 risk id。\n\n", res)
	AppendSfScanInterpretLog(interpretLoop, r, tid, "init: 已加载首包 risk 样本；请结合 preface 解读")
	EmitSyntaxFlowScanProgress(interpretLoop, "ready", "扫描会话已就绪 / scan session ready", tid, "")
	EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p2_session_ready", fmt.Sprintf(
		"# 代码扫描·阶段 2/3\n\n## 会话已就绪\n\n- **task_id**: `%s`\n- **将注册轮询**（扫描可能仍在进行）: %v\n\n**preface 头（截断）**:\n```\n%s\n```",
		tid, poll, utils.ShrinkTextBlock(interpretLoop.Get("sf_scan_review_preface"), 6000),
	))
	if poll {
		EmitSyntaxFlowScanPhase(interpretLoop, 3, "start", "interpret", "阶段3：风险轮询、解读与终局报告 / phase3 interpret", tid, "", nil)
		EmitSyntaxFlowScanProgress(interpretLoop, "watch", "扫描未结束，已订阅 SSA 更新并轮询任务行 / scan running, SSA watch + task poll", tid, "")
	} else {
		ApplyFinalReportContextWhenScanAlreadyDone(interpretLoop, r, db, task, tid, res)
		EmitSyntaxFlowScanPhase(interpretLoop, 3, "start", "interpret", "阶段3：风险解读与终局大总结 / phase3 interpret (scan already done)", tid, "", nil)
		EmitSyntaxFlowScanProgress(interpretLoop, "final_report_required", "须输出大总结 / MUST deliver final merged report", tid, "")
		EmitSyntaxFlowScanPhase(interpretLoop, 4, "start", "final_report_mandatory",
			"请输出含各阶段 + 扫描统计 + 全部 risk 的终局大总结 / deliver final report (mandatory)", tid, "", nil)
	}
	return nil
}

func phaseAttachToTaskID(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	state *SyntaxFlowState,
	interpretLoop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	id string,
) error {
	pT := task.GetId()
	parentID := OrchestratorParentTaskID(interpretLoop, pT)
	AppendSFPipelineLine(interpretLoop, "【0·入参】附着已有 task_id，跳过本地编译与 StartScanInBackground。")
	interpretLoop.Set(sfu.LoopVarSFCompileMeta, "mode=attach (compile skipped)")
	EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p0_attach", BuildScanStagePhase0Attach(id, pT))
	EmitSyntaxFlowScanPhase(interpretLoop, 1, "end", "compile_skipped", "附着 task_id，跳过本地编译 / attach mode, skip compile", id, "", nil)
	EmitSyntaxFlowScanProgress(interpretLoop, "load_session", "按 task_id 加载扫描会话 / loading scan by task_id", id, "")
	res, err := LoadScanSessionResult(db, id, DefaultRiskSampleLimit)
	if err != nil {
		log.Warnf("[syntaxflow_scan] attach: %v", err)
		abort(interpretLoop, r, fmt.Sprintf("按 task_id 加载失败：%v", err))
		return err
	}
	EmitSyntaxFlowScanPhase(interpretLoop, 2, "end", "session_attached", "已加载历史任务与风险摘要 / session loaded", id, "", nil)
	inferred := interpretLoop.Get("sf_scan_config_inferred")
	if inferred == "" {
		inferred = "0"
	}
	poll := wireSession(r, db, interpretLoop, task, id, inferred, "下列信息来自已附着任务（SSA Risk 抽样），仅可在此基础上解读；不得编造未列出的 risk id。\n\n", res)
	EmitSyntaxFlowScanProgress(interpretLoop, "ready", "已附着历史扫描会话 / attached existing session", id, "")
	EmitSyntaxFlowUserStageMarkdown(interpretLoop, parentID, "p2_attach_ready", fmt.Sprintf(
		"# 代码扫描·阶段 2/3\n\n## 附着会话已就绪\n\n- **task_id**: `%s`\n- **仍轮询**（若任务未结束）: %v\n\n**preface 头（截断）**:\n```\n%s\n```",
		id, poll, utils.ShrinkTextBlock(interpretLoop.Get("sf_scan_review_preface"), 6000),
	))
	if poll {
		EmitSyntaxFlowScanPhase(interpretLoop, 3, "start", "interpret", "进入风险轮询与解读阶段 / risk poll & interpret", id, "", nil)
		EmitSyntaxFlowScanProgress(interpretLoop, "watch", "扫描未结束，已订阅 SSA 更新并轮询任务行 / scan running, SSA watch + task poll", id, "")
	} else {
		ApplyFinalReportContextWhenScanAlreadyDone(interpretLoop, r, db, task, id, res)
		EmitSyntaxFlowScanPhase(interpretLoop, 3, "start", "interpret", "进入风险解读与终局大总结 / interpret & final (already done)", id, "", nil)
		EmitSyntaxFlowScanProgress(interpretLoop, "final_report_required", "须输出大总结 / MUST deliver final merged report", id, "")
		EmitSyntaxFlowScanPhase(interpretLoop, 4, "start", "final_report_mandatory",
			"请输出含各阶段 + 扫描统计 + 全部 risk 的终局报告 / deliver final report (mandatory)", id, "", nil)
	}
	return nil
}

func abort(loop *reactloops.ReActLoop, r aicommon.AIInvokeRuntime, msg string) {
	loop.Set("sf_scan_review_preface", msg)
	r.AddToTimeline("syntaxflow_scan", msg)
	EmitSyntaxFlowScanProgress(loop, "failed", "初始化未通过 / init aborted", "", msg)
}

// wireSession 将扫描会话数据写入 loop 与 timeline，并在需要时注册 SSA 风险监听与终态轮询。返回值 poll
// 为 true 时表示扫描可能仍在进行，已注册 watch+poll，调用方应在发完 ready 后按需再发「watch」进度事件。
func wireSession(
	r aicommon.AIInvokeRuntime,
	db *gorm.DB,
	loop *reactloops.ReActLoop,
	task aicommon.AIStatefulTask,
	taskID, inferred, intro string,
	res *ScanSessionResult,
) (poll bool) {
	loop.Set("sf_scan_task_id", taskID)
	loop.Set("sf_scan_session_mode", "attach")
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
