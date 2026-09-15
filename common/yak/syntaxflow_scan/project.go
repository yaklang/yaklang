package syntaxflow_scan

import (
	"context"
	"strings"

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
func ScanProject(ctx context.Context, opts ...ssaconfig.Option) (ProjectResult, error) {
	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	var err error
	cfg.Config, err = ssaconfig.New(ssaconfig.ModeAll, opts...)
	if err != nil {
		return ProjectResult{}, err
	}
	ssaconfig.ApplyExtraOptions(cfg, cfg.Config)

	recorder := newStageOutcomeRecorder()
	report := func(stage ProductStage, err error) { recorder.record(stage, err) }
	// programName is published through ProjectResult so compile-only runs can
	// hand the persisted IR name back to the platform.
	programName := strings.TrimSpace(cfg.GetProgramName())

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
		cfg.stageCallback(stage, stage.OverallProgress(progress), progress, info)
	}

	mode := resolveProductModes(cfg)
	wantSource, wantReview, wantAnalyze := mode.source, mode.review, mode.analyze
	compileOnly := mode.compileOnly

	// Stage risk counts come from the result stream, attributed by the rule's
	// own mode. This keeps per-stage metrics authoritative without Legion
	// re-deriving them from job chains.
	if cfg.resultCallback != nil {
		userResultCallback := cfg.resultCallback
		cfg.resultCallback = func(result *ScanResult) {
			if result != nil && result.Result != nil {
				if count := int64(result.Result.RiskCount()); count > 0 {
					switch resultModeStage(result) {
					case StageInspect:
						recorder.addRisk(StageInspect, count)
					case StageReview:
						recorder.addRisk(StageReview, count)
					default:
						recorder.addRisk(StageAnalyze, count)
					}
				}
			}
			userResultCallback(result)
		}
	}
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

	emit(StageCollect, 0, nil)
	if hasProgram || localDir != "" {
		emit(StageCollect, 1, nil)
	}
	report(StageCollect, nil)

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
		compileStage := StageReview
		if compileOnly {
			compileStage = StageCompile
		}
		if compileOnly {
			emit(StageCompile, 0, nil)
		} else if wantReview {
			emit(StageReview, 0, nil)
		} else if localDir == "" {
			emit(StageCollect, 0.5, nil)
		}
		var compileOpts []ssaconfig.Option
		if wantReview {
			compileOpts = structCompileOptions(cfg)
		}
		compileOpts = append(compileOpts, ssaapi.WithProcess(func(msg string, process float64) {
			if compileOnly {
				emit(StageCompile, process, nil)
			} else if wantReview {
				emit(StageReview, process, nil)
			} else if localDir == "" {
				emit(StageCollect, 0.5+process*0.5, nil)
			}
		}))
		prog, err := compileProductProject(ctx, cfg.Config, compileOpts...)
		report(compileStage, err)
		if err != nil {
			return finishScanProject(cfg, recorder, programName, err)
		}
		if wantReview {
			emitStructResults(cfg, prog)
			emit(StageReview, 1, nil)
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
			emit(StageCompile, 1, nil)
		}
		if localDir == "" {
			emit(StageCollect, 1, nil)
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
		}
		report(StageReview, reviewErr)
		if reviewErr != nil {
			return finishScanProject(cfg, recorder, programName, reviewErr)
		}
		emit(StageReview, 1, nil)
	}

	// -p / reloaded -t: one StartScan. Source attaches via IrSource; SSA runs
	// on the program. After a live inspect, only SSA remains.
	if hasProgram && (wantAnalyze || (wantSource && !inspectedLive)) {
		runSource := wantSource && !inspectedLive
		if runSource {
			emit(StageInspect, 0, nil)
		}
		if wantAnalyze {
			emit(StageAnalyze, 0, nil)
		}
		// One StartScan serves both remaining stages; a failure marks them all
		// failed because the engine cannot attribute it to a single stage.
		err := StartScan(ctx, compiledProgramScanOptions(cfg, emit, runSource, wantAnalyze)...)
		if runSource {
			report(StageInspect, err)
		}
		if wantAnalyze {
			report(StageAnalyze, err)
		}
		if err != nil {
			return finishScanProject(cfg, recorder, programName, err)
		}
		if runSource {
			emit(StageInspect, 1, nil)
		}
		if wantAnalyze {
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
	result := ProjectResult{
		Stages:      recorder.Outcomes(),
		ProgramName: programName,
		Succeeded:   recorder.Succeeded(),
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
	return strings.TrimSpace(cfg.GetCodeSourceLocalFile())
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
	if cfg.resultCallback != nil {
		opts = append(opts, WithScanResultCallback(cfg.resultCallback))
	}
	if cfg.errorCallback != nil {
		opts = append(opts, WithErrorCallback(cfg.errorCallback))
	}
	if cfg.pauseCheck != nil {
		opts = append(opts, WithPauseFunc(cfg.pauseCheck))
	}
	if cfg.Reporter != nil {
		opts = append(opts, WithReporter(cfg.Reporter))
	}
	if cfg.GetScanIgnoreLanguage() {
		opts = append(opts, ssaconfig.WithScanIgnoreLanguage(true))
	}
	if filter := cfg.GetRuleFilter(); filter != nil {
		opts = append(opts, ssaconfig.WithRuleFilter(filter))
	}
	for _, input := range cfg.GetRuleInput() {
		if input != nil {
			opts = append(opts, ssaconfig.WithRuleInput(input))
		}
	}
	if cfg.GetScanConcurrency() > 0 {
		opts = append(opts, ssaconfig.WithScanConcurrency(cfg.GetScanConcurrency()))
	}
	return opts
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
