package yak2ssa

import (
	"reflect"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *astbuilder) buildSelectStmt(stmt *yak.SelectStmtContext) {
	restoreRange := b.SetRange(stmt)
	defer restoreRange()
	cases, err := yak.SelectCommunications(stmt)
	if err != nil {
		b.NewError(ssa.Error, TAG, err.Error())
		return
	}
	if len(cases) > 65535 {
		b.NewError(ssa.Error, TAG, "select has too many cases")
		return
	}
	// Evaluate all communication operands before constructing branch scopes.
	// The intrinsic call models one atomic selection; sends/receives are not
	// emitted as unconditional operations while visiting case expressions.
	var args []ssa.Value
	channels := make([]ssa.Value, len(cases))
	for i, c := range cases {
		dir := reflect.SelectDefault
		if c.Channel != nil {
			dir = reflect.SelectRecv
			if c.Send != nil {
				dir = reflect.SelectSend
			}
		}
		args = append(args, b.EmitConstInst(int(dir)))
		if c.Channel != nil {
			channels[i] = b.buildExpression(c.Channel.(*yak.ExpressionContext))
			if channels[i] == nil {
				return
			}
			kind := channels[i].GetType().GetTypeKind()
			if kind != ssa.ChanTypeKind && kind != ssa.AnyTypeKind {
				b.NewError(ssa.Error, TAG, "select case requires a channel")
			}
			args = append(args, channels[i])
			if c.Send != nil {
				value := b.buildExpression(c.Send.(*yak.ExpressionContext))
				if value == nil {
					return
				}
				args = append(args, value)
			}
		}
	}
	method := b.EmitUndefined(yakvm.SelectBuiltinName)
	method.Kind = ssa.UndefinedValueValid
	method.SetExtern(true)
	params := make([]ssa.Type, len(args))
	for i := range params {
		params[i] = ssa.CreateAnyType()
	}
	method.SetType(ssa.NewFunctionTypeDefine(yakvm.SelectBuiltinName, params,
		[]ssa.Type{ssa.CreateNumberType(), ssa.CreateAnyType(), ssa.CreateBooleanType()}, false))
	selected := b.EmitCall(b.NewCall(method, args))
	if len(cases) == 0 {
		// Empty select cannot reach following statements: only cancellation ends it.
		b.EmitReturn(nil)
		return
	}
	sw := b.BuildSwitch()
	sw.AutoBreak = true
	sw.BuildCondition(func() ssa.Value { return b.ReadMemberCallValue(selected, b.EmitConstInst(0)) })
	// On normal execution exactly one case is selected. Use the explicit default
	// (or the final communication when none exists) as the exhaustive fallback,
	// avoiding an impossible unchanged-variable edge in the SSA join.
	fallback := len(cases) - 1
	for i, c := range cases {
		if c.Channel == nil {
			fallback = i
			break
		}
	}
	branches := make([]int, 0, len(cases)-1)
	for i := range cases {
		if i != fallback {
			branches = append(branches, i)
		}
	}
	sw.BuildCaseSize(len(branches))
	sw.SetCase(func(i int) []ssa.Value { return []ssa.Value{b.EmitConstInst(branches[i])} })
	clauses := stmt.AllSelectClause()
	buildBody := func(i int) {
		clause := clauses[i].(*yak.SelectClauseContext)
		restoreRange := b.SetRange(clause)
		defer restoreRange()
		c := cases[i]
		if c.Left != nil {
			lefts := b.buildLeftExpressionList(c.Declare, c.Left.(*yak.LeftExpressionListContext))
			for j, left := range lefts {
				value := b.ReadMemberCallValue(selected, b.EmitConstInst(j+1))
				if j == 0 {
					if typ, ok := channels[i].GetType().(*ssa.ChanType); ok {
						value = b.EmitTypeCast(value, typ.Elem)
					}
				}
				b.AssignVariable(left, value)
			}
		}
		if body, ok := clause.StatementList().(*yak.StatementListContext); ok {
			b.buildStatementList(body)
		}
	}
	sw.BuildBody(func(i int) { buildBody(branches[i]) })
	sw.BuildDefault(func() { buildBody(fallback) })
	sw.Finish()
}
