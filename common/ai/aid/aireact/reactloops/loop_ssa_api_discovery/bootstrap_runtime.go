package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// Markers for idempotent merge into ToolCallIntervalReviewExtraPrompt (progress audit during tool runs).
const (
	markerIntervalReviewSsaDiscovery = "<!-- ssa_api_discovery_interval_review:code_tools -->"
)

// Guidance for interval-review AI: avoid spurious cancel on slow I/O, reducing context-canceled tool failures.
const intervalReviewBlockSsaDiscovery = `### SSA API 发现 — 代码浏览类工具（read_file / tree / grep / glob 等）
- 多数调用在 **数秒～一两分钟** 内结束；大目录 tree、大文件 read_file 可能更久。**应等待本轮工具彻底结束**（进程已返回、反馈已落到时间线）再判定成败，**不要**仅因「暂时没有新 stdout」「间隔审计又弹了一次」就 **cancel**。
- **除非** stderr/反馈中出现 **明确错误**（如路径不存在、权限拒绝、panic、非零退出说明失败）或**已确认卡死无进度**，否则 interval review 必须 **continue**，**不要**提前终止工具。被误 cancel 常表现为工具侧 **context canceled**，并浪费剩余的任务级超时预算。
- 本 loop 在**一次成功工具调用后**还可能触发 **自动满意度验证（额外一轮 LLM）**，墙钟会叠加。随意 cancel 工具会放大「父任务 context 先结束 → 后续 read_file 报 canceled」的风险。
- **call_expectations** 应写**合理区间**（如 "~2s, single small yaml" 或 "~30s, tree under src/main/java"），避免写成「必须 <1s」却在大盘上必然超时，导致审计模型误判。`

func appendIntervalReviewExtraBlock(base, marker, body string) string {
	block := strings.TrimSpace(marker) + "\n" + strings.TrimSpace(body)
	if strings.TrimSpace(base) == "" {
		return block
	}
	return strings.TrimSpace(base) + "\n\n" + block
}

// mergeSsaDiscoveryIntervalReviewExtraPrompt appends loop-specific interval-review guidance when missing.
func mergeSsaDiscoveryIntervalReviewExtraPrompt(cfg *aicommon.Config) {
	if cfg == nil {
		return
	}
	current := strings.TrimSpace(cfg.GetToolCallIntervalReviewExtraPrompt())
	if current == "" {
		current = strings.TrimSpace(cfg.GetConfigString(aicommon.ConfigKeyToolCallIntervalReviewExtraPrompt))
	}
	if strings.Contains(current, markerIntervalReviewSsaDiscovery) {
		return
	}
	out := appendIntervalReviewExtraBlock(current, markerIntervalReviewSsaDiscovery, intervalReviewBlockSsaDiscovery)
	if err := aicommon.WithToolCallIntervalReviewExtraPrompt(out)(cfg); err != nil {
		log.Warnf("ssa_api_discovery: apply ToolCallIntervalReviewExtraPrompt: %v", err)
	}
}

func failBootstrap(db *gorm.DB, err error) (*Runtime, error) {
	_ = closeGorm(db)
	return nil, err
}

// discoveryTaskWorkDir 与 Init 中一致的工作目录解析（未配置时返回空字符串，由调用方决定是否落临时目录）。
func discoveryTaskWorkDir(r aicommon.AIInvokeRuntime) string {
	if r == nil {
		return ""
	}
	cfg := r.GetConfig()
	if cfg == nil {
		return ""
	}
	if w, ok := cfg.(interface{ GetOrCreateWorkDir() string }); ok {
		return w.GetOrCreateWorkDir()
	}
	return ""
}

// BootstrapDiscoveryRuntime 打开 SQLite、创建/恢复会话、探活、SSA 编译；不在任意 loop 上设置 runtime。
func BootstrapDiscoveryRuntime(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask) (*Runtime, error) {
	parsed, err := ParseUserInputLenient(task.GetUserInput())
	if err != nil {
		return nil, err
	}
	return BootstrapDiscoveryRuntimeFromParsed(r, task, parsed)
}

// BootstrapDiscoveryRuntimeFromParsed 使用已合并的结构化输入（如路由后在 Init 中经 AI 补全的 Code path / Target），其余逻辑同 BootstrapDiscoveryRuntime。
func BootstrapDiscoveryRuntimeFromParsed(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, parsed *ParsedUserInput) (*Runtime, error) {
	if parsed == nil {
		return nil, utils.Error("nil parsed input")
	}
	workDir := discoveryTaskWorkDir(r)
	if workDir == "" {
		workDir = filepath.Join(os.TempDir(), "ssa_discovery_"+uuid.NewString())
		_ = os.MkdirAll(workDir, 0o755)
		log.Warnf("ssa_api_discovery: workdir fallback to %s", workDir)
	}
	execLogger := NewExecutionLogger(workDir)
	execLogger.StepStart("bootstrap", "programmatic")
	bootstrapStart := time.Now()

	var err error
	parsed, err = ResolveRemoteCodePath(task.GetContext(), r, workDir, parsed)
	if err != nil {
		execLogger.StepError("bootstrap", "programmatic", bootstrapStart, err, nil)
		return nil, err
	}

	if c, ok := r.GetConfig().(*aicommon.Config); ok {
		mergeSsaDiscoveryIntervalReviewExtraPrompt(c)
	}

	sessionDBCfg := store.SessionDBConfig{WorkDir: workDir}
	if dsn := strings.TrimSpace(parsed.SessionDBDSN); dsn != "" {
		sessionDBCfg.Dialect = "postgres"
		sessionDBCfg.DSN = dsn
	}
	db, err := store.OpenSessionDBFromConfig(sessionDBCfg)
	if err != nil {
		return nil, utils.Wrapf(err, "open discovery session db")
	}
	repo := store.NewRepository(db)

	reuseUUID := ""
	if strings.TrimSpace(parsed.CodePath) == "" {
		prev, perr := repo.GetLatestSession()
		if perr == nil && prev != nil && prev.CodePathOK && strings.TrimSpace(prev.CodeRootPath) != "" {
			parsed.CodePath = prev.CodeRootPath
			if strings.TrimSpace(parsed.TargetRaw) == "" {
				parsed.TargetRaw = prev.TargetRaw
			}
			reuseUUID = prev.UUID
			r.AddToTimeline("[ssa_discovery]", fmt.Sprintf(
				"Follow-up: restored Code path + Target from latest SQLite session uuid=%s (same work dir).",
				reuseUUID,
			))
		}
	}
	if strings.TrimSpace(parsed.CodePath) == "" {
		return failBootstrap(db, utils.Error(
			"code path not found: first message must include \"Code path: /abs/dir\", or SSH remote source (ssh_host + remote_code_path + ssh_password/ssh_key); "+
				"follow-up messages can omit it if this Yakit task work dir already has ssa_discovery/session.sqlite3 from an earlier successful run",
		))
	}

	absCode, errPath := AbsCodeDir(parsed.CodePath)

	var sess *store.DiscoverySession
	var prevRootForSSA string

	if reuseUUID != "" {
		sess, err = repo.GetSessionByUUID(reuseUUID)
		if err != nil || sess == nil {
			return failBootstrap(db, utils.Wrapf(err, "reload discovery session %s", reuseUUID))
		}
		prevRootForSSA = sess.CodeRootPath
		if errPath != nil {
			sess.CodePathOK = false
			sess.CodePathError = errPath.Error()
		} else {
			sess.CodeRootPath = absCode
			sess.CodePathOK = true
			sess.CodePathError = ""
		}
		if uerr := repo.UpdateSession(sess); uerr != nil {
			return failBootstrap(db, utils.Wrapf(uerr, "update session"))
		}
		_ = repo.AppendEvent(sess.ID, "info", "follow_up_init", string(utils.Jsonify(map[string]any{
			"user_input_excerpt": utils.ShrinkString(task.GetUserInput(), 240),
		})))
	} else {
		sessUUID := uuid.NewString()
		if su := strings.TrimSpace(parsed.SessionUUID); su != "" {
			if existing, e := repo.GetSessionByUUID(su); e == nil && existing != nil {
				reuseUUID = su
			} else {
				sessUUID = su
			}
		}
		if reuseUUID != "" {
			sess, err = repo.GetSessionByUUID(reuseUUID)
			if err != nil || sess == nil {
				return failBootstrap(db, utils.Wrapf(err, "reload discovery session %s", reuseUUID))
			}
			prevRootForSSA = sess.CodeRootPath
			if errPath != nil {
				sess.CodePathOK = false
				sess.CodePathError = errPath.Error()
			} else {
				sess.CodeRootPath = absCode
				sess.CodePathOK = true
				sess.CodePathError = ""
			}
			if uerr := repo.UpdateSession(sess); uerr != nil {
				return failBootstrap(db, utils.Wrapf(uerr, "update session"))
			}
			_ = repo.AppendEvent(sess.ID, "info", "follow_up_init", string(utils.Jsonify(map[string]any{
				"user_input_excerpt": utils.ShrinkString(task.GetUserInput(), 240),
			})))
		} else {
		sess = &store.DiscoverySession{
			UUID:         sessUUID,
			TargetRaw:    parsed.TargetRaw,
			Phase:        PhaseInitialized,
			CodeRootPath: parsed.CodePath,
		}
		if errPath != nil {
			sess.CodePathOK = false
			sess.CodePathError = errPath.Error()
		} else {
			sess.CodeRootPath = absCode
			sess.CodePathOK = true
		}
		if err := repo.CreateSession(sess); err != nil {
			return failBootstrap(db, utils.Wrapf(err, "create session"))
		}
		}
	}

	authUser, authPass := ResolveUserCredentials(parsed)
	rt := &Runtime{
		DB:                       db,
		Repo:                     repo,
		WorkDir:                  workDir,
		SQLitePath:               store.DBPath(workDir),
		SessionDBDSN:             strings.TrimSpace(parsed.SessionDBDSN),
		Session:                  sess,
		UserAuthUsername:         authUser,
		UserAuthPassword:         authPass,
		UserAuthCredentialGroups: append([]UserCredentialGroup(nil), parsed.AuthCredentialGroups...),
		Phase4ModeRaw:            NormalizePhase4Mode(parsed.Phase4Mode),
		SkipDirectoryAnalysis:       parsed.SkipDirectoryAnalysis || os.Getenv("YAK_SSA_SKIP_DIR_ANALYSIS") == "1",
		AllowPartialAuth:            parsed.AllowPartialAuth || os.Getenv("YAK_SSA_AUTH_PARTIAL_OK") == "1",
		FrameworkToolkitEnabled:     parsed.FrameworkToolkitEnabled || os.Getenv("YAK_SSA_FRAMEWORK_TOOLKIT") == "1",
		ExecutionLogger:             execLogger,
	}
	if summary := credentialGroupsTimelineSummary(parsed.AuthCredentialGroups); summary != "" {
		r.AddToTimeline("[ssa_discovery]", "user auth "+summary)
	} else if authUser != "" {
		r.AddToTimeline("[ssa_discovery]", fmt.Sprintf("user auth configured username=%s (password provided=%v)", authUser, authPass != ""))
	}
	if rt.AllowPartialAuth {
		r.AddToTimeline("[ssa_discovery]", "partial_auth enabled: continue with verified realms only; unverified realms skipped programmatically")
	}
	if rt.FrameworkToolkitEnabled {
		r.AddToTimeline("[ssa_discovery]", "framework_toolkit enabled: router will select framework or fallback to full AI pipeline")
	}

	dbRef := store.SessionDBRef(workDir, rt.SessionDBDSN)
	r.AddToTimeline("[ssa_discovery]", fmt.Sprintf("session=%s db=%s", sess.UUID, dbRef))

	if !sess.CodePathOK {
		_ = repo.AppendEvent(sess.ID, "error", "invalid code path", string(utils.Jsonify(map[string]string{"err": sess.CodePathError})))
		_ = repo.UpdateSessionFields(sess.UUID, map[string]interface{}{
			"notes": sess.CodePathError,
			"phase": PhaseInitialized,
		})
		return failBootstrap(db, utils.Errorf("invalid code path: %s", sess.CodePathError))
	}

	skipSSACompile := reuseUUID != "" && errPath == nil &&
		filepath.Clean(prevRootForSSA) == filepath.Clean(absCode) &&
		sess.SSACompileOK && strings.TrimSpace(sess.Language) != "" && parsed.LanguageHint == ""

	if !skipSSACompile {
		lang, lerr := ResolveLanguage(sess.CodeRootPath, parsed.LanguageHint)
		if lerr != nil {
			_ = repo.AppendEvent(sess.ID, "error", "language resolution failed", string(utils.Jsonify(map[string]string{"err": lerr.Error()})))
			sess.Language = ""
			_ = repo.UpdateSession(sess)
			log.Warnf("ssa_api_discovery: language error: %v", lerr)
		} else {
			sess.Language = string(lang)
			_ = repo.UpdateSession(sess)
		}
	}

	if parsed.TargetRaw != "" {
		parsed.TargetRaw = NormalizeTargetString(parsed.TargetRaw)
		pr := ProbeTarget(task.GetContext(), parsed.TargetRaw)
		now := time.Now()
		sess.TargetReachable = pr.Reachable
		sess.TargetProbeMethod = pr.ProbeMethod
		sess.TargetProbeDetail = pr.Detail
		sess.TargetHost = pr.Host
		sess.TargetPort = pr.Port
		sess.TargetScheme = pr.Scheme
		sess.TargetProbedAt = &now
		sess.TargetRaw = parsed.TargetRaw
		if !pr.Reachable {
			sess.Notes = fmt.Sprintf("Target unreachable (%s): %s — continuing with SSA.", pr.ProbeMethod, pr.Detail)
			log.Infof("ssa_api_discovery: target not reachable, detail=%s", pr.Detail)
		} else {
			log.Infof("ssa_api_discovery: target reachable via %s", pr.ProbeMethod)
		}
		_ = repo.UpdateSession(sess)
		_ = repo.AppendEvent(sess.ID, "info", "target_probe", string(utils.Jsonify(map[string]any{
			"reachable": pr.Reachable, "method": pr.ProbeMethod, "detail": pr.Detail,
		})))
		rt.Session = sess
	}

	if skipSSACompile {
		sess2, _ := repo.GetSessionByUUID(sess.UUID)
		if sess2 != nil {
			rt.Session = sess2
		}
		log.Infof("ssa_api_discovery: skip SSA recompile session=%s phase=%s", sess.UUID, rt.Session.Phase)
		return rt, nil
	}

	if sess.Language == "" {
		sess.SSACompileOK = false
		sess.SSACompileError = "language not resolved"
		sess.Phase = PhaseSSADone
		_ = repo.UpdateSession(sess)
		rt.Session = sess
		log.Warnf("ssa_api_discovery: skip SSA, no language")
		return rt, nil
	}

	langEnum := ssaconfig.Language(sess.Language)
	progName := fmt.Sprintf("disc_%s", sess.UUID[:8])
	progs, perr := ssaapi.ParseProjectFromPath(sess.CodeRootPath,
		ssaapi.WithLanguage(langEnum),
		ssaapi.WithProgramName(progName),
		ssaapi.WithContext(task.GetContext()),
	)
	if perr != nil || len(progs) == 0 {
		sess.SSACompileOK = false
		if perr != nil {
			sess.SSACompileError = perr.Error()
		} else {
			sess.SSACompileError = "compilation produced no programs"
		}
		log.Warnf("ssa_api_discovery: SSA compile failed: %v", perr)
	} else {
		prog := progs[0]
		sess.SSACompileOK = true
		sess.SSAProgramName = prog.GetProgramName()
		sess.SSAFileCount = len(prog.Program.FileList)
		sess.SSACompileError = ""
		log.Infof("ssa_api_discovery: SSA ok program=%s files=%d", sess.SSAProgramName, sess.SSAFileCount)
	}
	sess.Phase = PhaseSSADone
	_ = repo.UpdateSession(sess)
	_ = repo.AppendEvent(sess.ID, "info", "ssa_compile", string(utils.Jsonify(map[string]any{
		"ok": sess.SSACompileOK, "program": sess.SSAProgramName, "files": sess.SSAFileCount, "err": sess.SSACompileError,
	})))
	rt.Session = sess
	execLogger.StepEnd("bootstrap", "programmatic", bootstrapStart, []string{store.DBPath(workDir)})
	return rt, nil
}
