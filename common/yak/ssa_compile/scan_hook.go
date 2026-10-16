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

// compileForProductScan is the production compiler for a project scan.
//
// syntaxflow_scan cannot import this package: ssa_compile -> yakscript -> yak
// -> syntaxflow_scan. The init registration is what keeps auto-detect
// (ParseProjectWithConfig) on the product path. When this package is not
// linked, CompileProject stays nil and the scan falls back to ParseProject.
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
