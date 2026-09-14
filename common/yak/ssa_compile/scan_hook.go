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

// compileForProductScan is syntaxflow_scan -> ssa_compile -> yak 编译脚本
// (ParseProjectWithConfig). Extra options (struct rules, process) stay on the
// config and force in-process compile so ExtraInfo is not dropped by JSON.
func compileForProductScan(ctx context.Context, cfg *ssaconfig.Config, extra ...ssaconfig.Option) (*ssaapi.Program, error) {
	res, err := ParseProjectWithConfig(ctx, cfg, extra...)
	if err != nil {
		return nil, err
	}
	if res == nil || res.Program == nil {
		return nil, utils.Errorf("compile result is empty")
	}
	return res.Program, nil
}
