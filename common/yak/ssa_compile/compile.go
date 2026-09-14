package ssa_compile

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

// CompileWithConfig compiles a project in-process via ssaapi.ParseProject.
// Extra options (struct rules, process callbacks) are applied after JSON
// round-trip so ExtraInfo is preserved. This is the compile entry used by
// syntaxflow_scan.ScanProject: syntaxflow-scan -> ssa-compile -> ssaapi.
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
