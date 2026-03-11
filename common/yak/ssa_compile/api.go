package ssa_compile

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// ProjectAutoDetective runs the "SSA 项目探测" flow (unless Config is provided)
// and returns the resolved compile config (with Options applied).
func ProjectAutoDetective(ctx context.Context, conf *SSADetectConfig) (*AutoDetectInfo, *ssaconfig.Config, error) {
	if conf == nil {
		conf = &SSADetectConfig{}
	}
	return resolveCompileConfig(ctx, conf)
}

// ParseProjectWithConfig compiles a project using a pre-resolved config, without re-running auto-detect.
func ParseProjectWithConfig(ctx context.Context, config *ssaconfig.Config, options ...ssaconfig.Option) (*SSADetectResult, error) {
	return ParseProjectWithAutoDetective(ctx, &SSADetectConfig{
		Config:             config,
		Options:            options,
		CompileImmediately: true,
	})
}

// ParseProjectWithName compiles a project using a pre-resolved config while forcing a program name.
func ParseProjectWithName(ctx context.Context, config *ssaconfig.Config, programName string, options ...ssaconfig.Option) (*SSADetectResult, error) {
	if config == nil {
		return nil, utils.Errorf("config is nil")
	}

	cfg, err := cloneConfigWithOptions(config, append(options, ssaconfig.WithContext(ctx))...)
	if err != nil {
		return nil, err
	}
	cfg.SetProgramName(programName)

	return ParseProjectWithAutoDetective(ctx, &SSADetectConfig{
		Config:                      cfg,
		CompileImmediately:          true,
		ForceProgramName:            true,
		DisableTimestampProgramName: true,
	})
}
