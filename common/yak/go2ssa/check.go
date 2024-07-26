package go2ssa

import (
	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (ass *astbuilder) CheckParameters(check []ssa.Value, target *gol.ParametersContext) bool {
	// TODO
	return true
}

func (ass *astbuilder) CheckResult(check []ssa.Value, target *gol.ResultContext) bool {
	// TODO
	return true
}