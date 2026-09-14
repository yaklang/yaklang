package syntaxflow_scan

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileProject, when set, compiles a local code source with builtin struct
// rules. ssa_compile registers this in init to avoid an import cycle.
var CompileProject func(ctx context.Context, cfg *ssaconfig.Config) (*ssaapi.Program, error)

// ScanProject is the product compile+scan pipeline. Adapters (code-scan, gRPC,
// yak scripts) should call this instead of wiring compile and StartScan
// themselves. Source rules run on the compiled IrSource snapshot.
func ScanProject(ctx context.Context, opts ...ssaconfig.Option) error {
	opts = append([]ssaconfig.Option{WithCompiledSource(true)}, opts...)
	cfg := &Config{ScanTaskCallback: &ScanTaskCallback{}}
	var err error
	cfg.Config, err = ssaconfig.New(ssaconfig.ModeAll, opts...)
	if err != nil {
		return err
	}
	ssaconfig.ApplyExtraOptions(cfg, cfg.Config)

	if len(cfg.Programs) == 0 && len(cfg.QueryTargets) == 0 {
		target := ""
		if cfg.Config != nil {
			target = strings.TrimSpace(cfg.GetCodeSourceLocalFileOrURL())
		}
		if target != "" {
			if CompileProject == nil {
				return utils.Errorf("ScanProject: compiler is not registered")
			}
			prog, err := CompileProject(ctx, cfg.Config)
			if err != nil {
				return err
			}
			if prog == nil {
				return utils.Errorf("compile result is empty")
			}
			cfg.Programs = append(cfg.Programs, prog)
		}
	}
	return StartScan(ctx, scanOptionsFromProjectConfig(cfg)...)
}

func scanOptionsFromProjectConfig(cfg *Config) []ssaconfig.Option {
	opts := []ssaconfig.Option{WithCompiledSource(true)}
	if cfg == nil {
		return opts
	}
	if len(cfg.Programs) > 0 {
		opts = append(opts, WithPrograms(cfg.Programs...))
	}
	if len(cfg.QueryTargets) > 0 {
		opts = append(opts, WithQueryTargets(cfg.QueryTargets...))
	}
	if cfg.resultCallback != nil {
		opts = append(opts, WithScanResultCallback(cfg.resultCallback))
	}
	if cfg.ProcessCallback != nil {
		opts = append(opts, WithProcessCallback(cfg.ProcessCallback))
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
	if modes := cfg.GetRuleFilterMode(); len(modes) > 0 {
		opts = append(opts, ssaconfig.WithRuleFilterMode(modes...))
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
