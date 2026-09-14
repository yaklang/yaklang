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

func compileForProductScan(ctx context.Context, cfg *ssaconfig.Config) (*ssaapi.Program, error) {
	res, err := ParseProjectWithAutoDetective(ctx, &SSADetectConfig{
		Config:                      cfg,
		CompileImmediately:          true,
		DisableTimestampProgramName: true,
		Options:                     []ssaconfig.Option{ssaapi.WithStructRule(true)},
	})
	if err != nil {
		return nil, err
	}
	if res == nil || res.Program == nil {
		return nil, utils.Errorf("compile result is empty")
	}
	return res.Program, nil
}
