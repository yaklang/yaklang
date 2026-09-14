package syntaxflow_scan

import (
	"context"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/syntaxflow/sfpattern"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileProject, when set, compiles a local code source with struct rules.
// ssa_compile registers this in init to avoid an import cycle.
var CompileProject func(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error)

var (
	sourceModeRe = regexp.MustCompile(`(?m)mode:\s*"?source"?`)
	structModeRe = regexp.MustCompile(`(?m)mode:\s*"?struct"?`)
)

// ScanProjectFromJSON is the script/platform entry: one ssaconfig JSON blob
// plus optional callbacks. CLI and gRPC parse their inputs into the same
// JSON/options and call ScanProject.
func ScanProjectFromJSON(ctx context.Context, raw string, extra ...ssaconfig.Option) error {
	opts := append([]ssaconfig.Option{ssaconfig.WithJsonRawConfig([]byte(raw))}, extra...)
	return ScanProject(ctx, opts...)
}

// ScanProject is the product pipeline for CLI, gRPC, and yak scripts.
// Stages: 收集代码 → 代码检测 → 语义检测 → 深度分析.
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
	// A program name with a code source is the name to compile into, not a DB load.
	namedOnly := !hasLoaded && len(cfg.GetProgramNames()) > 0 && !hasCode
	hasProgram := hasLoaded || namedOnly
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
		if CompileProject == nil {
			return utils.Errorf("ScanProject: compiler is not registered")
		}
		emit(StageReview, 0, nil)
		if !hasProgram && localDir == "" {
			emit(StageCollect, 0.5, nil)
		}
		compileOpts := structCompileOptions(cfg)
		compileOpts = append(compileOpts, ssaapi.WithProcess(func(msg string, process float64) {
			emit(StageReview, process, nil)
		}))
		prog, err := CompileProject(ctx, cfg.Config, compileOpts...)
		if err != nil {
			return err
		}
		if prog == nil {
			return utils.Errorf("compile result is empty")
		}
		cfg.Programs = append(cfg.Programs, prog)
		emitStructResults(cfg, prog)
		emit(StageReview, 1, nil)
		if localDir == "" {
			emit(StageCollect, 1, nil)
		}
		hasProgram = true
	}

	if wantSource && hasProgram && !inspectedLive {
		emit(StageInspect, 0, nil)
		if err := StartScan(ctx, inspectCompiledSourceOptions(cfg, emit)...); err != nil {
			return err
		}
		emit(StageInspect, 1, nil)
	}

	if wantAnalyze && hasProgram {
		emit(StageAnalyze, 0, nil)
		if err := StartScan(ctx, analyzeOptions(cfg, emit)...); err != nil {
			return err
		}
		emit(StageAnalyze, 1, nil)
	}

	if !wantSource && !wantReview && !wantAnalyze {
		return StartScan(ctx, scanOptionsFromProjectConfig(cfg)...)
	}
	return nil
}

func productModes(cfg *Config) (source, review, analyze bool) {
	if cfg == nil {
		return true, true, true
	}
	if modes := cfg.GetRuleFilterMode(); len(modes) > 0 {
		for _, m := range modes {
			switch strings.ToLower(strings.TrimSpace(m)) {
			case string(schema.SFR_MODE_SOURCE):
				source = true
			case string(schema.SFR_MODE_STRUCT):
				review = true
			case string(schema.SFR_MODE_SSA):
				analyze = true
			}
		}
		return source, review, analyze
	}
	if inferred := inferModesFromCustomRules(cfg); inferred != nil {
		return inferred[0], inferred[1], inferred[2]
	}
	return true, true, true
}

func inferModesFromCustomRules(cfg *Config) []bool {
	contents := customRuleContents(cfg)
	if len(contents) == 0 {
		return nil
	}
	var source, review, analyze bool
	for _, raw := range contents {
		switch peekRuleMode(raw) {
		case schema.SFR_MODE_SOURCE:
			source = true
		case schema.SFR_MODE_STRUCT:
			review = true
		default:
			analyze = true
		}
	}
	return []bool{source, review, analyze}
}

func customRuleContents(cfg *Config) []string {
	if cfg == nil {
		return nil
	}
	if cfg.IsTaskLocalRuleInput() {
		rules, _, err := loadTaskLocalSyntaxFlowRules(cfg.SyntaxFlowRule)
		if err != nil {
			return nil
		}
		out := make([]string, 0, len(rules))
		for _, rule := range rules {
			if rule != nil && strings.TrimSpace(rule.Content) != "" {
				out = append(out, rule.Content)
			}
		}
		return out
	}
	inputs := cfg.GetRuleInput()
	if len(inputs) == 0 {
		return nil
	}
	out := make([]string, 0, len(inputs))
	for _, in := range inputs {
		if in == nil || strings.TrimSpace(in.Content) == "" {
			continue
		}
		out = append(out, in.Content)
	}
	return out
}

func peekRuleMode(raw string) schema.SyntaxFlowRuleModeType {
	if sourceModeRe.MatchString(raw) {
		return schema.SFR_MODE_SOURCE
	}
	if structModeRe.MatchString(raw) {
		return schema.SFR_MODE_STRUCT
	}
	return schema.SFR_MODE_SSA
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

func structCompileOptions(cfg *Config) []ssaconfig.Option {
	var opts []ssaconfig.Option
	raws := customStructRuleRaws(cfg)
	if len(raws) > 0 {
		for _, raw := range raws {
			opts = append(opts, ssaapi.WithStructRuleRaw(raw))
		}
		return opts
	}
	if len(customRuleContents(cfg)) > 0 {
		// Exclusive custom pack with no struct rules: do not load builtin struct.
		return opts
	}
	opts = append(opts, ssaapi.WithStructRule(true))
	return opts
}

func customStructRuleRaws(cfg *Config) []string {
	var raws []string
	for _, raw := range customRuleContents(cfg) {
		if peekRuleMode(raw) == schema.SFR_MODE_STRUCT {
			raws = append(raws, raw)
		}
	}
	return raws
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
	opts := programScanOptions(cfg)
	opts = append(opts,
		WithCompiledSource(true),
		ssaconfig.WithRuleFilterMode(string(schema.SFR_MODE_SOURCE)),
		WithProcessCallback(wrapStageProcess(cfg, StageInspect, emit)),
	)
	return opts
}

func analyzeOptions(cfg *Config, emit func(ProductStage, float64, *RuleProcessInfoList)) []ssaconfig.Option {
	opts := programScanOptions(cfg)
	opts = append(opts,
		ssaconfig.WithRuleFilterMode(string(schema.SFR_MODE_SSA)),
		WithProcessCallback(wrapStageProcess(cfg, StageAnalyze, emit)),
	)
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
