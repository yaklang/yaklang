//go:build !no_language
// +build !no_language

package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitGlobalStatement(raw phpparser.IGlobalStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.GlobalStatementContext)
	if i == nil {
		return nil
	}
	currentMode := y.SupportClosure
	y.SupportClosure = true
	defer func() {
		y.SupportClosure = currentMode
	}()
	for _, globalVarContext := range i.AllChain() {
		defaultValue := y.VisitChain(globalVarContext)
		left := y.VisitChainLeft(globalVarContext)
		value := y.BuildFreeValue(left.GetName())
		p, ok := ssa.ToParameter(defaultValue)
		if ok && p.GetDefault() != nil {
			value.SetDefault(p.GetDefault())
		}
		value.SetType(defaultValue.GetType())
	}
	return nil
}
