package ssa_compile

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/syntaxflow_scan"
)

func init() {
	syntaxflow_scan.CompileProject = compileForProductScan
}

func compileForProductScan(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
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
