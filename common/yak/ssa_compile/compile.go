package ssa_compile

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileWithConfig compiles a project in-process via ssaapi.ParseProject.
// ExtraInfo (struct rules, process callbacks) is copied after JSON round-trip.
func CompileWithConfig(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
	if cfg == nil {
		return nil, utils.Errorf("compile config is nil")
	}
	raw, err := cfg.ToJSONString()
	if err != nil {
		return nil, err
	}
	opts := []ssaconfig.Option{
		ssaconfig.WithConfigJson(raw),
		ssaconfig.WithContext(ctx),
		copyExtraInfo(cfg),
	}
	opts = append(opts, extra...)
	progs, err := ssaapi.ParseProject(opts...)
	if err != nil {
		return nil, err
	}
	if len(progs) == 0 || progs[0] == nil {
		return nil, utils.Errorf("compile result is empty")
	}
	return progs[0], nil
}

func extraInfoForcesInProcess(config *ssaconfig.Config) bool {
	return config != nil && len(config.ExtraInfo) > 0
}

func copyExtraInfo(src *ssaconfig.Config) ssaconfig.Option {
	return func(dst *ssaconfig.Config) error {
		if src == nil || dst == nil {
			return nil
		}
		for key, values := range src.ExtraInfo {
			for _, value := range values {
				dst.SetExtraInfo(key, value)
			}
		}
		return nil
	}
}
