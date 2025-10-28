//go:build !no_language
// +build !no_language

package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitActualArguments(raw phpparser.IActualArgumentsContext) ([]ssa.Value, bool) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, false
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.ActualArgumentsContext)
	if i == nil {
		return nil, false
	}

	// PHP8 annotation
	argStmt := i.AllArguments()
	var args []ssa.Value
	ellipsis := false
	for _, a := range argStmt {
		vals, ellipsisCurrent := y.VisitArguments(a)
		args = append(args, vals...)
		if ellipsisCurrent {
			ellipsis = true
		}
	}

	for _, a := range i.AllSquareCurlyExpression() {
		y.VisitSquareCurlyExpression(a)
	}

	return args, ellipsis
}
func (y *builder) VisitArguments(raw phpparser.IArgumentsContext) ([]ssa.Value, bool) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, false
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.ArgumentsContext)
	if i == nil {
		return nil, false
	}
	tmp := y.isFunction
	y.isFunction = false
	defer func() {
		y.isFunction = tmp
	}()
	var ret []ssa.Value

	var ellipsis bool
	for _, arg := range i.AllActualArgument() {
		value, b := y.VisitActualArgument(arg)
		if b {
			ellipsis = true
		}
		ret = append(ret, value)
	}

	return ret, ellipsis
}

func (y *builder) VisitActualArgument(raw phpparser.IActualArgumentContext) (ssa.Value, bool) {
	if y == nil || raw == nil || y.IsStop() {
		return nil, false
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*phpparser.ActualArgumentContext)
	if i == nil {
		return nil, false
	}

	if i.Expression() != nil {
		val := y.VisitExpression(i.Expression())
		return val, i.Ellipsis() != nil
	} else if i.Ampersand() != nil {
		return y.VisitChain(i.Chain()), false
	}
	return nil, false
}
