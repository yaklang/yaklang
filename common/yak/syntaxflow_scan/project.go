package syntaxflow_scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileProject is registered by ssa_compile init so this package does not
// import ssa_compile (ssa_compile -> yakscript -> yak -> syntaxflow_scan).
var CompileProject func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error)

// ScanProjectFromJSON is the script/platform entry: one ssaconfig JSON blob
// plus optional callbacks. CLI and gRPC parse their inputs into the same
// JSON/options and call ScanProject. It returns the same ProjectResult.
func ScanProjectFromJSON(ctx context.Context, raw string, extra ...ssaconfig.Option) (ProjectResult, error) {
	opts := append([]ssaconfig.Option{ssaconfig.WithJsonRawConfig([]byte(raw))}, extra...)
	return ScanProject(ctx, opts...)
}

// ScanProject is the product pipeline for CLI and yak scripts.
//
//	cli/script → syntaxflow-scan → (ssa-compile → yak 编译脚本 | syntaxflow)
//
// gRPC SyntaxFlowScan stays on Scan: the frontend compiles first, then scans.
//
// code-scan runs all three modes by default:
//   - source: live local FS (-t) or IrSource snapshot (-p)
//   - struct: compile-time unit scan (-t) or intra application/library scan (-p, no compile)
//   - ssa: always on a DB-loaded program (-t reloads after SaveToDatabase)
//
// Stages: 收集代码 → 代码检测 → 语义检测 → 深度分析.
// Mode is selected by WithMode (stacked). An empty mode list is compile-only:
// it persists IR, reports StageCompile, and skips every rule set so platforms
// can reuse the program later. Callers that want rule sets (code-scan, gRPC)
// pass the modes they need explicitly.
//
// It returns either the terminal ProjectResult (what ran, per-stage status,
// metrics, aggregate success) or an error. A run whose useful stages succeeded
// returns a result with Succeeded=true even when a sibling stage failed, so
// callers render the outcome instead of re-deriving success from job state.
func ScanProject(ctx context.Context, opts ...ssaconfig.Option) (result ProjectResult, err error) {
	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	cfg.Config, err = ssaconfig.New(ssaconfig.ModeAll, opts...)
	if err != nil {
		return ProjectResult{}, err
	}
	ssaconfig.ApplyExtraOptions(cfg, cfg.Config)
	// The project owns one report across source, compile-time struct and SSA
	// scans. A nested StartScan must not close it after its individual stage.
	if cfg.Reporter != nil {
		defer func() {
			if saveErr := cfg.Reporter.Save(); saveErr != nil {
				err = errors.Join(err, fmt.Errorf("save project report: %w", saveErr))
			}
		}()
	}
	cfg.SetSyntaxFlowResultSaveMemory()
	if cfg.SyntaxFlow != nil {
		cfg.SyntaxFlow.Memory = true
	}
	// In-process ScanProject compile (process callbacks force ExtraInfo) skips
	// the SSA compile plugin that stamps projectName(timestamp). Without a
	// program name, buildSSARisk drops every hit and the scan reports 0 risks.
	if strings.TrimSpace(cfg.GetProgramName()) == "" {
		base := strings.TrimSpace(cfg.GetProjectName())
		if base == "" {
			base = "scan-project"
		}
		cfg.SetProgramName(fmt.Sprintf("%s(%s)", base, time.Now().Format("2006-01-02 15:04:05")))
	}

	recorder := newStageOutcomeRecorder()
	report := func(stage ProductStage, err error) { recorder.record(stage, err) }
	// programName is published through ProjectResult so compile-only runs can
	// hand the persisted IR name back to the platform.
	programName := strings.TrimSpace(cfg.GetProgramName())
	maxStage := ProductStage("")
	maxOverall := 0.0

	emit := func(stage ProductStage, progress float64, info *RuleProcessInfoList) {
		// Every product signal flows through here, so this is the one place
		// that keeps per-stage timing and counters current.
		if progress <= 0 {
			recorder.enter(stage)
		}
		recorder.observe(stage, info)
		if cfg.stageCallback == nil {
			return
		}
		if progress < 0 {
			progress = 0
		}
		if progress > 1 {
			progress = 1
		}
		if ProductStageRank(stage) < ProductStageRank(maxStage) {
			return
		}
		maxStage = stage
		overall := stage.OverallProgress(progress)
		if overall < maxOverall {
			overall = maxOverall
		} else {
			maxOverall = overall
		}
		cfg.stageCallback(stage, overall, progress, info)
	}

	mode := resolveProductModes(cfg)
	wantSource, wantReview, wantAnalyze := mode.source, mode.review, mode.analyze
	compileOnly := mode.compileOnly

	// Stage risk counts come from the result stream, attributed by the rule's
	// own mode. This keeps per-stage metrics authoritative without Legion
	// re-deriving them from job chains.
	userResultCallback := cfg.resultCallback
	cfg.resultCallback = func(result *ScanResult) {
		if result != nil && result.Result != nil {
			if cfg.Reporter != nil {
				cfg.Reporter.AddSyntaxFlowResult(result.Result)
			}
			stage := resultModeStage(result)
			if rule := result.Result.GetRule(); rule != nil {
				recorder.addRule(stage, rule.RuleName)
			}
			if count := int64(result.Result.RiskCount()); count > 0 {
				recorder.addRisk(stage, count)
			}
		}
		if userResultCallback != nil {
			userResultCallback(result)
		}
	}
	// The reporter is one object shared by every stage scan below: results
	// stream into it as each stage runs (see notifyResult), and it keeps its
	// document current so a stage can save a valid snapshot at any time.
	hasLoaded := len(cfg.Programs) > 0
	hasCode := hasCodeSource(cfg)
	localDir := localSourceDir(cfg)
	// Program names without a code source load an already-compiled IR.
	namedOnly := !hasLoaded && len(cfg.GetProgramNames()) > 0 && !hasCode
	if namedOnly {
		if err := loadNamedPrograms(cfg); err != nil {
			return finishScanProject(cfg, recorder, programName, err)
		}
		hasLoaded = len(cfg.Programs) > 0
	}
	hasProgram := hasLoaded
	needCompile := !hasLoaded && hasCode &&
		(compileOnly || wantReview || wantAnalyze || (wantSource && localDir == ""))

	// Collect is "the project tree is here". A local directory or loaded IR is
	// already collected; a remote URL is not collected until clone/extract
	// succeeds inside compile. Reporting collect success before that made a
	// clone failure look like a 语义检测 failure.
	sourceReady := hasProgram || localDir != ""
	emit(StageCollect, 0, nil)
	if sourceReady {
		emit(StageCollect, 1, nil)
		report(StageCollect, nil)
	}

	inspectedLive := false
	if wantSource && localDir != "" && !hasProgram {
		if err := attachLiveSourceTarget(cfg, localDir); err != nil {
			return finishScanProject(cfg, recorder, programName, err)
		}
		emit(StageInspect, 0, nil)
		err := StartScan(ctx, inspectLiveSourceOptions(cfg, emit)...)
		report(StageInspect, err)
		if err != nil {
			return finishScanProject(cfg, recorder, programName, err)
		}
		emit(StageInspect, 1, nil)
		inspectedLive = true
	}

	if needCompile {
		// A compile serves whichever product stage asked for it. Compile-only
		// runs report StageCompile; review runs own it because struct rules run
		// during compile; analyze-only runs fold it into the analyze stage and
		// must NOT report review, which would claim 语义检测 ran.
		compileStage := ProductStage("")
		switch {
		case compileOnly:
			compileStage = StageCompile
		case wantReview:
			compileStage = StageReview
		case wantAnalyze:
			recorder.enter(StageAnalyze)
		}
		if compileOnly {
			emit(StageCompile, 0, nil)
		} else if wantReview && sourceReady {
			emit(StageReview, 0, nil)
		} else if !sourceReady {
			emit(StageCollect, 0.5, nil)
		}
		collectClosed := sourceReady
		var compileOpts []ssaconfig.Option
		if wantReview {
			compileOpts = structCompileOptions(cfg)
		}
		compileOpts = append(compileOpts, ssaapi.WithProcess(func(msg string, process float64) {
			scaleInfo := parseCompileScale(msg)
			if scaleInfo != nil {
				recorder.observeScale(scaleInfo)
			}
			info := scaleInfo
			if !sourceReady && !collectClosed {
				if scaleInfo != nil {
					// Filesystem scan after clone/extract: the tree is here, so
					// collect ends and the remaining compile belongs to review.
					emit(StageCollect, 1, scaleInfo)
					report(StageCollect, nil)
					collectClosed = true
					if wantReview {
						emit(StageReview, 0, scaleInfo)
					} else if compileOnly {
						emit(StageCompile, process, scaleInfo)
					}
					return
				}
				emit(StageCollect, 0.5+process*0.5, info)
				return
			}
			if compileOnly {
				emit(StageCompile, process, info)
			} else if wantReview {
				emit(StageReview, process, info)
			}
		}))
		prog, err := compileProductProject(ctx, cfg.Config, compileOpts...)
		if !sourceReady {
			if isSourceCollectError(err) {
				if !collectClosed {
					report(StageCollect, err)
				}
				return finishScanProject(cfg, recorder, programName, err)
			}
			if !collectClosed {
				report(StageCollect, nil)
				emit(StageCollect, 1, nil)
				collectClosed = true
			}
		}
		captureProgramEvidence(recorder, prog)
		if prog != nil {
			diagnostic := prog.Program.CompileDiagnostics()
			if diagnostic.Incomplete() {
				recorder.mu.Lock()
				if recorder.compileDiagnostics == nil {
					recorder.compileDiagnostics = make(map[ProductStage]ssa.CompileDiagnostics)
				}
				recorder.compileDiagnostics[compileStage] = diagnostic
				if wantAnalyze {
					recorder.compileDiagnostics[StageAnalyze] = diagnostic
				}
				recorder.mu.Unlock()
			}
		}
		if wantReview && err == nil {
			emitStructResults(cfg, prog)
			recorder.observeStruct(prog)
			emit(StageReview, 1, mergeStageInfo(scaleInfoFromRecorder(recorder), structRuleProcessInfo(prog)))
		}
		if compileStage != "" {
			report(compileStage, err)
		}
		if err != nil {
			return finishScanProject(cfg, recorder, programName, err)
		}
		if prog != nil {
			if name := strings.TrimSpace(prog.GetProgramName()); name != "" {
				programName = name
			}
		}
		// SaveToDatabase closes the compile cache. SSA must load a DBRead
		// program; scanning the closed DBWrite program skips every SSA rule.
		cfg.Programs = append(cfg.Programs, reloadCompiledProgram(prog))
		if compileOnly {
			emit(StageCompile, 1, scaleInfoFromRecorder(recorder))
		}
		hasProgram = true
	} else if wantReview && hasLoaded {
		emit(StageReview, 0, nil)
		var reviewErr error
		for _, prog := range cfg.Programs {
			if err := scanLoadedProgramStruct(cfg, prog); err != nil {
				reviewErr = err
				break
			}
			emitStructResults(cfg, prog)
			recorder.observeStruct(prog)
		}
		report(StageReview, reviewErr)
		if reviewErr != nil {
			return finishScanProject(cfg, recorder, programName, reviewErr)
		}
		emit(StageReview, 1, mergeStageInfo(scaleInfoFromRecorder(recorder), structRuleProcessInfoAll(cfg.Programs)))
	}

	// After a live inspect, only SSA remains. Inspect and analyze run as
	// separate StartScan calls so each stage keeps its own rule/risk counts.
	if hasProgram && (wantAnalyze || (wantSource && !inspectedLive)) {
		runSource := wantSource && !inspectedLive
		if runSource {
			emit(StageInspect, 0, nil)
			err := StartScan(ctx, inspectCompiledSourceOptions(cfg, emit)...)
			report(StageInspect, err)
			if err == nil {
				emit(StageInspect, 1, nil)
			}
		}
		if wantAnalyze {
			emit(StageAnalyze, 0, nil)
			err := StartScan(ctx, analyzeOptions(cfg, emit)...)
			report(StageAnalyze, err)
			if err != nil {
				return finishScanProject(cfg, recorder, programName, err)
			}
			emit(StageAnalyze, 1, nil)
		}
	}

	// No product stage selected and not a compile-only run: fall back to a
	// plain scan over whatever targets the caller configured.
	if !compileOnly && !wantSource && !wantReview && !wantAnalyze {
		err := StartScan(ctx, scanOptionsFromProjectConfig(cfg)...)
		if err != nil {
			return finishScanProject(cfg, recorder, programName, err)
		}
	}
	return finishScanProject(cfg, recorder, programName, nil)
}

// finishScanProject decides the aggregate result and publishes the terminal
// ProjectResult. A project scan counts as successful when the source was
// collected and at least one detection stage succeeded; later stage failures
// stay visible through stage outcomes instead of turning an already-useful
// result into a failed job.
func finishScanProject(cfg *Config, recorder *stageOutcomeRecorder, programName string, err error) (ProjectResult, error) {
	// Each stage already saved a snapshot on its way out; this final save is the
	// authoritative one and also covers a run whose stages were skipped or
	// failed before saving anything.
	saveProjectReport(cfg)

	result := ProjectResult{
		Stages:      recorder.Outcomes(),
		ProgramName: programName,
		Succeeded:   recorder.Succeeded(),
	}
	if recorder != nil {
		scale := recorder.Scale()
		result.TotalFiles = scale.TotalFiles
		result.HandlerFiles = scale.HandlerFiles
		result.PrehandlerFiles = scale.PrehandlerFiles
		result.TotalBytes = scale.TotalBytes
		result.TotalLines = scale.TotalLines
		result.SourceStatistics = recorder.SourceStatistics()
	}
	result.SkippedStages = skippedRequestedStages(resolveProductModes(cfg), result.Stages)
	result.IncompleteStages = len(result.SkippedStages) > 0
	for _, stage := range result.Stages {
		if stage.Status == StageStatusPartial || stage.Status == StageStatusFailed {
			result.IncompleteStages = true
		}
	}
	if err != nil {
		result.Error = err.Error()
	}
	if cfg != nil && cfg.projectResultCallback != nil {
		cfg.projectResultCallback(result)
	}
	if err == nil {
		return result, nil
	}
	if result.Succeeded {
		return result, nil
	}
	return result, err
}

// productModeSelection is the resolved product intent for one ScanProject run.
// resultModeStage attributes one streamed result to its product stage by the
// rule's own mode, so per-stage risk counts stay traceable to that rule set.
func resultModeStage(result *ScanResult) ProductStage {
	if result == nil || result.Result == nil {
		return StageAnalyze
	}
	rule := result.Result.GetRule()
	if rule == nil {
		return StageAnalyze
	}
	switch rule.Mode {
	case schema.SFR_MODE_SOURCE:
		return StageInspect
	case schema.SFR_MODE_STRUCT:
		return StageReview
	default:
		return StageAnalyze
	}
}

type productModeSelection struct {
	source  bool
	review  bool
	analyze bool
	// compileOnly persists IR without running any rule set. It is selected by
	// passing no mode at all, so platforms can compile once and scan later.
	compileOnly bool
}

// resolveProductModes maps the stacked mode list onto product stages. An empty
// list means compile-only: collect source and persist IR, run no rules.
func resolveProductModes(cfg *Config) productModeSelection {
	if cfg == nil || cfg.ScanTaskCallback == nil || len(cfg.scanModes) == 0 {
		return productModeSelection{compileOnly: true}
	}
	var selection productModeSelection
	for _, m := range cfg.scanModes {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case SourceMode:
			selection.source = true
		case StructMode:
			selection.review = true
		case SSAMode:
			selection.analyze = true
		}
	}
	if !selection.source && !selection.review && !selection.analyze {
		// Unrecognized values must not silently become compile-only.
		return productModeSelection{source: true, review: true, analyze: true}
	}
	return selection
}

// skippedRequestedStages lists detection stages the caller asked for that never
// started. A failed or canceled stage ran, so it is not skipped.
func skippedRequestedStages(mode productModeSelection, outcomes []StageOutcome) []string {
	if mode.compileOnly {
		return nil
	}
	present := map[ProductStage]struct{}{}
	for _, outcome := range outcomes {
		present[outcome.Stage] = struct{}{}
	}
	var skipped []string
	if mode.source {
		if _, ok := present[StageInspect]; !ok {
			skipped = append(skipped, string(StageInspect))
		}
	}
	if mode.review {
		if _, ok := present[StageReview]; !ok {
			skipped = append(skipped, string(StageReview))
		}
	}
	if mode.analyze {
		if _, ok := present[StageAnalyze]; !ok {
			skipped = append(skipped, string(StageAnalyze))
		}
	}
	return skipped
}

func compileRuleContent(raw string) (*schema.SyntaxFlowRule, error) {
	frame, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(raw)
	if err != nil {
		return nil, err
	}
	rule := frame.GetRule()
	if rule == nil {
		return nil, utils.Error("compiled rule has no schema")
	}
	rule.Content = raw
	rule.NormalizeMode()
	return rule, nil
}

func (c *Config) customRules() []*schema.SyntaxFlowRule {
	if c == nil {
		return nil
	}
	if c.parsedCustomRulesDone {
		return c.parsedCustomRules
	}
	c.parsedCustomRulesDone = true
	var raws []string
	if c.IsTaskLocalRuleInput() {
		rules, _, err := loadTaskLocalSyntaxFlowRules(c.SyntaxFlowRule)
		if err != nil {
			return nil
		}
		c.parsedCustomRules = rules
		return rules
	}
	for _, in := range c.GetRuleInput() {
		if in == nil || strings.TrimSpace(in.Content) == "" {
			continue
		}
		raws = append(raws, in.Content)
	}
	for _, raw := range raws {
		rule, err := compileRuleContent(raw)
		if err != nil {
			continue
		}
		c.parsedCustomRules = append(c.parsedCustomRules, rule)
	}
	return c.parsedCustomRules
}

func hasCodeSource(cfg *Config) bool {
	if cfg == nil || cfg.Config == nil {
		return false
	}
	return strings.TrimSpace(cfg.GetCodeSourceLocalFileOrURL()) != ""
}

func localSourceDir(cfg *Config) string {
	if cfg == nil || cfg.Config == nil || cfg.GetCodeSource() == nil {
		return ""
	}
	if cfg.GetCodeSourceKind() != ssaconfig.CodeSourceLocal {
		return ""
	}
	dir := strings.TrimSpace(cfg.GetCodeSourceLocalFile())
	if dir == "" {
		return ""
	}
	// Live source inspection walks the path as a directory. Archives and plain
	// files must fall through to the compile pipeline, which knows how to open
	// zip/jar code sources; walking them here fails with
	// "root path is not a directory".
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

func emitStructResults(cfg *Config, prog *ssaapi.Program) {
	if cfg == nil || cfg.resultCallback == nil || prog == nil {
		return
	}
	for _, res := range prog.StructScanResults() {
		if res == nil {
			continue
		}
		cfg.resultCallback(&ScanResult{Status: "executing", Result: res})
	}
}

func wrapStageProcess(cfg *Config, stage ProductStage, emit func(ProductStage, float64, *RuleProcessInfoList)) ProcessCallback {
	var user ProcessCallback
	if cfg != nil {
		user = cfg.ProcessCallback
	}
	return func(taskID, status string, progress float64, info *RuleProcessInfoList) {
		emit(stage, progress, info)
		if user != nil {
			user(taskID, status, stage.OverallProgress(progress), info)
		}
	}
}

// isSourceCollectError is a clone/extract failure that happened before the
// project tree existed. Those belong to collect, not to the detection stage
// that compile would have served after the tree was in place.
func isSourceCollectError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SSA Git clone failed") ||
		strings.Contains(msg, "git clone:")
}

const compileScalePrefix = "ssa-compile-scale:"

func parseCompileScale(msg string) *RuleProcessInfoList {
	msg = strings.TrimSpace(msg)
	if !strings.HasPrefix(msg, compileScalePrefix) {
		return nil
	}
	var scale struct {
		TotalFiles      int64 `json:"total_files"`
		HandlerFiles    int64 `json:"handler_files"`
		PrehandlerFiles int64 `json:"prehandler_files"`
		TotalBytes      int64 `json:"total_bytes"`
	}
	raw := strings.TrimPrefix(msg, compileScalePrefix)
	if json.Unmarshal([]byte(raw), &scale) != nil {
		return nil
	}
	if scale.TotalFiles <= 0 && scale.HandlerFiles <= 0 && scale.TotalBytes <= 0 {
		return nil
	}
	return &RuleProcessInfoList{
		TotalFiles:      scale.TotalFiles,
		HandlerFiles:    scale.HandlerFiles,
		PrehandlerFiles: scale.PrehandlerFiles,
		TotalBytes:      scale.TotalBytes,
	}
}

func mergeStageInfo(scale, rules *RuleProcessInfoList) *RuleProcessInfoList {
	if scale == nil {
		return rules
	}
	if rules == nil {
		return scale
	}
	out := *scale
	out.Rules = rules.Rules
	if rules.TotalQuery > 0 {
		out.TotalQuery = rules.TotalQuery
		out.FinishedQuery = rules.FinishedQuery
		out.FailedQuery = rules.FailedQuery
		out.SuccessQuery = rules.SuccessQuery
		out.SkippedQuery = rules.SkippedQuery
	}
	if rules.RiskCount > 0 {
		out.RiskCount = rules.RiskCount
	}
	return &out
}

func structRuleProcessInfo(prog *ssaapi.Program) *RuleProcessInfoList {
	if prog == nil {
		return nil
	}
	stats := prog.StructScanRuleStats()
	if len(stats) == 0 {
		return nil
	}
	info := &RuleProcessInfoList{TotalQuery: int64(len(stats))}
	for _, stat := range stats {
		info.FinishedQuery++
		if stat.Failed {
			info.FailedQuery++
		} else {
			info.SuccessQuery++
		}
		if stat.RiskCount > 0 {
			info.RiskCount += stat.RiskCount
		}
		item := &RuleProcessInfo{
			RuleName:    stat.RuleName,
			ProgramName: stat.ProgramName,
			StartTime:   stat.StartTime,
			EndTime:     stat.EndTime,
			Progress:    1,
			Finished:    true,
			RiskCount:   stat.RiskCount,
		}
		if stat.Failed && stat.Error != "" {
			item.Error = fmt.Errorf("%s", stat.Error)
		}
		info.Rules = append(info.Rules, item)
	}
	return info
}

func structRuleProcessInfoAll(progs []*ssaapi.Program) *RuleProcessInfoList {
	var info *RuleProcessInfoList
	for _, prog := range progs {
		part := structRuleProcessInfo(prog)
		if part == nil {
			continue
		}
		if info == nil {
			copied := *part
			copied.Rules = append([]*RuleProcessInfo(nil), part.Rules...)
			info = &copied
			continue
		}
		info.Rules = append(info.Rules, part.Rules...)
		info.TotalQuery += part.TotalQuery
		info.FinishedQuery += part.FinishedQuery
		info.FailedQuery += part.FailedQuery
		info.SuccessQuery += part.SuccessQuery
		info.SkippedQuery += part.SkippedQuery
		info.RiskCount += part.RiskCount
	}
	return info
}

// saveProjectReport writes the report the stages of this project scan have
// streamed into. Every stage saves its own snapshot as it ends; this call
// supersedes those with the finished document.
func saveProjectReport(cfg *Config) {
	if cfg == nil || cfg.Reporter == nil {
		return
	}
	if err := cfg.Reporter.Save(); err != nil {
		log.Errorf("save report failed: %v", err)
	}
}

func scaleInfoFromRecorder(recorder *stageOutcomeRecorder) *RuleProcessInfoList {
	if recorder == nil {
		return nil
	}
	scale := recorder.Scale()
	if scale.TotalFiles <= 0 && scale.TotalBytes <= 0 && scale.TotalLines <= 0 {
		return nil
	}
	return &RuleProcessInfoList{
		TotalFiles:      scale.TotalFiles,
		HandlerFiles:    scale.HandlerFiles,
		PrehandlerFiles: scale.PrehandlerFiles,
		TotalBytes:      scale.TotalBytes,
		TotalLines:      scale.TotalLines,
	}
}

func captureProgramEvidence(recorder *stageOutcomeRecorder, prog *ssaapi.Program) {
	if recorder == nil || prog == nil {
		return
	}
	if stats, err := prog.GetSourceStatistics(); err == nil && stats != nil {
		recorder.observeAnalyzedSource(stats)
	}
}

func compileProductProject(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
	if CompileProject == nil {
		return nil, utils.Errorf("ScanProject: compiler is not registered")
	}
	return CompileProject(ctx, cfg, extra...)
}

func reloadCompiledProgram(prog *ssaapi.Program) *ssaapi.Program {
	if prog == nil || prog.Program == nil {
		return prog
	}
	if prog.Program.DatabaseKind == ssa.ProgramCacheMemory {
		return prog
	}
	if strings.TrimSpace(prog.GetProgramName()) == "" {
		return prog
	}
	return ssaapi.ReloadProgramFromDatabase(prog)
}

func scanLoadedProgramStruct(cfg *Config, prog *ssaapi.Program) error {
	if prog == nil {
		return nil
	}
	opts := structCompileOptions(cfg)
	if len(opts) == 0 {
		return nil
	}
	return prog.ScanProgramStruct(opts...)
}

func loadNamedPrograms(cfg *Config) error {
	if cfg == nil {
		return utils.Errorf("scan config is nil")
	}
	for _, name := range cfg.GetProgramNames() {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		prog, err := ssaapi.FromDatabase(name)
		if err != nil {
			return utils.Wrapf(err, "load program %s", name)
		}
		if prog != nil {
			cfg.Programs = append(cfg.Programs, prog)
		}
	}
	if len(cfg.Programs) == 0 {
		return utils.Errorf("no compiled program loaded")
	}
	return nil
}

func structCompileOptions(cfg *Config) []ssaconfig.Option {
	var opts []ssaconfig.Option
	// Struct rules use the same final SyntaxFlow result-save guard as SSA rules.
	if cfg != nil && cfg.IsNoSaveRisk() {
		opts = append(opts, ssaconfig.WithNoSaveRisk(true))
	}
	rules := cfg.customRules()
	var structRaws []string
	for _, rule := range rules {
		if rule != nil && rule.IsStructMode() && strings.TrimSpace(rule.Content) != "" {
			structRaws = append(structRaws, rule.Content)
		}
	}
	if len(structRaws) > 0 {
		for _, raw := range structRaws {
			opts = append(opts, ssaapi.WithStructRuleRaw(raw))
		}
		return opts
	}
	if len(rules) > 0 {
		return opts
	}
	opts = append(opts, ssaapi.WithStructRule(true))
	return opts
}

func attachLiveSourceTarget(cfg *Config, dir string) error {
	if cfg == nil {
		return utils.Errorf("scan config is nil")
	}
	files, err := sfpattern.LoadFilesFromDir(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		files, err = sfpattern.LoadFilesFromFS(filesys.NewRelLocalFs(dir))
		if err != nil {
			return err
		}
	}
	name := "source-target"
	if n := strings.TrimSpace(cfg.GetProjectName()); n != "" {
		name = n
	} else if n := strings.TrimSpace(cfg.GetProgramName()); n != "" {
		name = n
	}
	cfg.QueryTargets = append(cfg.QueryTargets, ssaapi.NewSourceQueryTarget(name, files))
	return nil
}

func inspectLiveSourceOptions(cfg *Config, emit func(ProductStage, float64, *RuleProcessInfoList)) []ssaconfig.Option {
	opts := sharedScanCallbackOptions(cfg)
	if cfg != nil && len(cfg.QueryTargets) > 0 {
		opts = append(opts, WithQueryTargets(cfg.QueryTargets...))
	}
	opts = append(opts,
		ssaconfig.WithRuleFilterMode(string(schema.SFR_MODE_SOURCE)),
		WithProcessCallback(wrapStageProcess(cfg, StageInspect, emit)),
	)
	return opts
}

func inspectCompiledSourceOptions(cfg *Config, emit func(ProductStage, float64, *RuleProcessInfoList)) []ssaconfig.Option {
	return compiledProgramScanOptions(cfg, emit, true, false)
}

func analyzeOptions(cfg *Config, emit func(ProductStage, float64, *RuleProcessInfoList)) []ssaconfig.Option {
	return compiledProgramScanOptions(cfg, emit, false, true)
}

func compiledProgramScanOptions(cfg *Config, emit func(ProductStage, float64, *RuleProcessInfoList), wantSource, wantAnalyze bool) []ssaconfig.Option {
	opts := sharedScanCallbackOptions(cfg)
	if cfg != nil {
		if len(cfg.Programs) > 0 {
			opts = append(opts, WithPrograms(cfg.Programs...))
		}
		if names := cfg.GetProgramNames(); len(names) > 0 {
			opts = append(opts, ssaconfig.WithProgramNames(names...))
		}
	}
	if wantSource {
		opts = append(opts, WithCompiledSource(true))
	}
	switch {
	case wantSource && !wantAnalyze:
		opts = append(opts, ssaconfig.WithRuleFilterMode(string(schema.SFR_MODE_SOURCE)))
	case wantAnalyze && !wantSource:
		opts = append(opts, ssaconfig.WithRuleFilterMode(string(schema.SFR_MODE_SSA)))
	}
	opts = append(opts, WithProcessCallback(func(taskID, status string, progress float64, info *RuleProcessInfoList) {
		switch {
		case wantSource && wantAnalyze:
			if progress < 0.5 {
				emit(StageInspect, progress*2, info)
			} else {
				emit(StageAnalyze, (progress-0.5)*2, info)
			}
		case wantSource:
			emit(StageInspect, progress, info)
		case wantAnalyze:
			emit(StageAnalyze, progress, info)
		}
		if cfg != nil && cfg.ProcessCallback != nil {
			cfg.ProcessCallback(taskID, status, progress, info)
		}
	}))
	return opts
}

func sharedScanCallbackOptions(cfg *Config) []ssaconfig.Option {
	opts := []ssaconfig.Option{}
	if cfg == nil {
		return opts
	}
	// ScanProject starts nested scans via StartScan, which rebuilds the
	// config from the forwarded options only. Rule-detail reporting must be
	// forwarded explicitly or every per-rule snapshot inside a product scan
	// degrades to counter-only events.
	if cfg.ScanTaskCallback != nil && cfg.ProcessWithRule {
		opts = append(opts, WithProcessRuleDetail(true))
	}
	if dir := strings.TrimSpace(cfg.GetDebugDir()); dir != "" {
		opts = append(opts, ssaconfig.WithDebugDir(dir))
	}
	if cfg.resultCallback != nil {
		opts = append(opts, WithScanResultCallback(cfg.resultCallback))
	}
	if cfg.errorCallback != nil {
		opts = append(opts, WithErrorCallback(cfg.errorCallback))
	}
	if cfg.pauseCheck != nil {
		opts = append(opts, WithPauseFunc(cfg.pauseCheck))
	}
	if cfg.GetScanIgnoreLanguage() {
		opts = append(opts, ssaconfig.WithScanIgnoreLanguage(true))
	}
	opts = append(opts, copySyntaxFlowRuleOptions(cfg)...)
	if cfg.GetScanConcurrency() > 0 {
		opts = append(opts, ssaconfig.WithScanConcurrency(cfg.GetScanConcurrency()))
	}
	// Nested stage scans rebuild their config. Preserve both budgets, including
	// explicit zero (disabled), instead of silently reverting to unlimited work.
	opts = append(opts,
		ssaconfig.WithScanRuleTimeout(cfg.GetScanRuleTimeout()),
		ssaconfig.WithScanRuleWorkLimit(cfg.GetScanRuleWorkLimit()),
	)
	// Propagate the risk-persistence setting to nested scan stages.
	if cfg.IsNoSaveRisk() {
		opts = append(opts, ssaconfig.WithNoSaveRisk(true))
	}
	return opts
}

// copySyntaxFlowRuleOptions keeps the dispatch-scoped snapshot (task_local)
// and any inline rule_input / rule_filter when ScanProject starts a nested
// StartScan. Dropping them made product scans query the empty node sfdb and
// finish with Total Rules = 0.
func copySyntaxFlowRuleOptions(cfg *Config) []ssaconfig.Option {
	if cfg == nil || cfg.Config == nil || cfg.SyntaxFlowRule == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]any{
		"SyntaxFlowRule": cfg.SyntaxFlowRule,
		"SyntaxFlow": map[string]any{
			"result_save_kind": string(ssaconfig.SFResultSaveMemory),
			"memory":           true,
		},
	})
	if err != nil {
		return nil
	}
	return []ssaconfig.Option{ssaconfig.WithJsonRawConfig(raw)}
}

func programScanOptions(cfg *Config) []ssaconfig.Option {
	opts := sharedScanCallbackOptions(cfg)
	if cfg == nil {
		return opts
	}
	if len(cfg.Programs) > 0 {
		opts = append(opts, WithPrograms(cfg.Programs...))
	}
	if names := cfg.GetProgramNames(); len(names) > 0 {
		opts = append(opts, ssaconfig.WithProgramNames(names...))
	}
	if len(cfg.QueryTargets) > 0 {
		opts = append(opts, WithQueryTargets(cfg.QueryTargets...))
	}
	return opts
}

func scanOptionsFromProjectConfig(cfg *Config) []ssaconfig.Option {
	opts := programScanOptions(cfg)
	if cfg != nil && cfg.ProcessCallback != nil {
		opts = append(opts, WithProcessCallback(cfg.ProcessCallback))
	}
	if cfg != nil && len(cfg.GetRuleFilterMode()) > 0 {
		opts = append(opts, ssaconfig.WithRuleFilterMode(cfg.GetRuleFilterMode()...))
	}
	return opts
}
