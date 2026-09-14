package syntaxflow_scan

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa_compile"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ScanProjectFromJSON is the script/platform entry: one ssaconfig JSON blob
// plus optional callbacks. CLI and gRPC parse their inputs into the same
// JSON/options and call ScanProject.
func ScanProjectFromJSON(ctx context.Context, raw string, extra ...ssaconfig.Option) error {
	opts := append([]ssaconfig.Option{ssaconfig.WithJsonRawConfig([]byte(raw))}, extra...)
	return ScanProject(ctx, opts...)
}

// ScanProject is the product pipeline for CLI, gRPC, and yak scripts.
//
//	cli/grpc/script → syntaxflow-scan → (ssa-compile → ssaapi | syntaxflow)
//
// Project input: live source inspect → compile+struct review → SSA analyze.
// Program input: IrSource inspect → stored/program struct review → SSA analyze.
// Stages: 收集代码 → 代码检测 → 语义检测 → 深度分析.
// Mode is selected by WithMode (stacked); default is source+struct+ssa.
func ScanProject(ctx context.Context, opts ...ssaconfig.Option) error {
	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	var err error
	cfg.Config, err = ssaconfig.New(ssaconfig.ModeAll, opts...)
	if err != nil {
		return err
	}
	ssaconfig.ApplyExtraOptions(cfg, cfg.Config)

	emit := func(stage ProductStage, progress float64, info *RuleProcessInfoList) {
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

	wantSource, wantReview, wantAnalyze := productModes(cfg)
	hasLoaded := len(cfg.Programs) > 0
	hasCode := hasCodeSource(cfg)
	localDir := localSourceDir(cfg)
	// Program names without a code source load an already-compiled IR.
	namedOnly := !hasLoaded && len(cfg.GetProgramNames()) > 0 && !hasCode
	if namedOnly {
		if err := loadNamedPrograms(cfg); err != nil {
			return err
		}
		hasLoaded = len(cfg.Programs) > 0
	}
	hasProgram := hasLoaded
	needCompile := !hasLoaded && hasCode && (wantReview || wantAnalyze || (wantSource && localDir == ""))

	emit(StageCollect, 0, nil)
	if hasProgram || localDir != "" {
		emit(StageCollect, 1, nil)
	}

	inspectedLive := false
	if wantSource && localDir != "" && !hasProgram {
		if err := attachLiveSourceTarget(cfg, localDir); err != nil {
			return err
		}
		emit(StageInspect, 0, nil)
		if err := StartScan(ctx, inspectLiveSourceOptions(cfg, emit)...); err != nil {
			return err
		}
		emit(StageInspect, 1, nil)
		inspectedLive = true
	}

	if needCompile {
		if wantReview {
			emit(StageReview, 0, nil)
		} else if localDir == "" {
			emit(StageCollect, 0.5, nil)
		}
		var compileOpts []ssaconfig.Option
		if wantReview {
			compileOpts = structCompileOptions(cfg)
		}
		compileOpts = append(compileOpts, ssaapi.WithProcess(func(msg string, process float64) {
			if wantReview {
				emit(StageReview, process, nil)
			} else if localDir == "" {
				emit(StageCollect, 0.5+process*0.5, nil)
			}
		}))
		prog, err := compileProductProject(ctx, cfg.Config, compileOpts...)
		if err != nil {
			return err
		}
		cfg.Programs = append(cfg.Programs, prog)
		if wantReview {
			emitStructResults(cfg, prog)
			emit(StageReview, 1, nil)
		}
		if localDir == "" {
			emit(StageCollect, 1, nil)
		}
		hasProgram = true
	} else if wantReview && hasLoaded {
		emit(StageReview, 0, nil)
		for _, prog := range cfg.Programs {
			emitStructResults(cfg, prog)
		}
		emit(StageReview, 1, nil)
	}

	// Already-compiled programs (gRPC/program name) must use one StartScan so
	// clients keep a single task ID. Source rules attach via IrSource; SSA
	// rules run on the program. After a live inspect, only SSA remains.
	if hasProgram && (wantAnalyze || (wantSource && !inspectedLive)) {
		runSource := wantSource && !inspectedLive
		if runSource {
			emit(StageInspect, 0, nil)
		}
		if wantAnalyze {
			emit(StageAnalyze, 0, nil)
		}
		if err := StartScan(ctx, compiledProgramScanOptions(cfg, emit, runSource, wantAnalyze)...); err != nil {
			return err
		}
		if runSource {
			emit(StageInspect, 1, nil)
		}
		if wantAnalyze {
			emit(StageAnalyze, 1, nil)
		}
	}

	if !wantSource && !wantReview && !wantAnalyze {
		return StartScan(ctx, scanOptionsFromProjectConfig(cfg)...)
	}
	return nil
}

func productModes(cfg *Config) (source, review, analyze bool) {
	if cfg == nil || len(cfg.scanModes) == 0 {
		return true, true, true
	}
	for _, m := range cfg.scanModes {
		switch strings.ToLower(strings.TrimSpace(m)) {
		case SourceMode:
			source = true
		case StructMode:
			review = true
		case SSAMode:
			analyze = true
		}
	}
	return source, review, analyze
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
	return ssa_compile.CompileWithConfig(ctx, cfg, extra...)
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
	opts := programScanOptions(cfg)
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
	opts := programScanOptions(cfg)
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
