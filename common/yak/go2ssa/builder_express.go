package go2ssa

import (
	"fmt"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type getSingleExpr interface {
	Expression(i int) gol.IExpressionContext
}

func (b *astbuilder) buildExpression(exp *gol.ExpressionContext,IslValue bool) (ssa.Value, *ssa.Variable) {
	if exp == nil {
		return nil, nil
	}

	getValue := func(single getSingleExpr, i int) ssa.Value {
		if s := single.Expression(i); s != nil {
			rightv, _ := b.buildExpression(s.(*gol.ExpressionContext),IslValue)
			return rightv
		} else {
			return nil
		}
	}

	fmt.Printf("exp = %v\n", exp.GetText())

	if ret := exp.PrimaryExpr();ret != nil{
		return b.buildPrimaryExpression(ret.(*gol.PrimaryExprContext), IslValue)
	}

	if !IslValue { // right
		if op := exp.GetUnary_op(); op != nil {
			var ssaop ssa.UnaryOpcode
	
			switch op.GetText() {
			case "+":
				ssaop = ssa.OpPlus
			case "-":
				ssaop = ssa.OpNeg
			case "!":
				ssaop = ssa.OpNot
			case "^":
				ssaop = ssa.OpBitwiseNot
			case "<-":
				ssaop = ssa.OpChan
			case "*":
				// TODO
			case "&":
			default:
				b.NewError(ssa.Error, TAG, UnaryOperatorNotSupport(op.GetText()))
			}
	
			op1 := getValue(exp, 0)
			if op1 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitUnOp(ssaop, op1),nil
		}
	
		if op := exp.GetAdd_op(); op != nil {
			var ssaop ssa.BinaryOpcode
	
			switch op.GetText() {
			case "+":
				ssaop = ssa.OpAdd
			case "-":
				ssaop = ssa.OpSub
			case "|":
				ssaop = ssa.OpOr
			case "^":
				ssaop = ssa.OpXor
			default:
			}
	
			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil || op2 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitBinOp(ssaop, op1, op2),nil
		}
	
		if op := exp.GetMul_op(); op != nil {
			var ssaop ssa.BinaryOpcode
	
			switch op.GetText() {
			case "*":
				ssaop = ssa.OpMul
			case "/":
				ssaop = ssa.OpDiv
			case "%":
				ssaop = ssa.OpMod
			case "&":
				ssaop = ssa.OpAnd
			case "<<":
				ssaop = ssa.OpShl
			case ">>":
				ssaop = ssa.OpShr
			case "&^":
				ssaop = ssa.OpAndNot
			default:
			}
	
			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil || op2 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitBinOp(ssaop, op1, op2),nil
		}
	
		if op := exp.GetRel_op(); op != nil {
			var ssaop ssa.BinaryOpcode

			switch op.GetText() {
			case "==":
				ssaop = ssa.OpEq
			case "!=":
				ssaop = ssa.OpNotEq
			case "<":
				ssaop = ssa.OpLt
			case ">":
				ssaop = ssa.OpGt
			case "<=":
				ssaop = ssa.OpLtEq
			case ">=":
				ssaop = ssa.OpGtEq
			default:
			}
			
			op1 := getValue(exp, 0)
			op2 := getValue(exp, 1)
			if op1 == nil || op2 == nil {
				b.NewError(ssa.Error, TAG, "in operator need two expression")
				return nil, nil
			}
			return b.EmitBinOp(ssaop, op1, op2),nil
		}
	}else{ // left

	}

	return nil, nil
}

func (b *astbuilder) buildPrimaryExpression(exp *gol.PrimaryExprContext,IslValue bool) (ssa.Value, *ssa.Variable) {
	if ret := exp.Operand(); ret != nil {
		return b.buildOperandExpression(ret.(*gol.OperandContext), IslValue)
	}

	if IslValue {
		rv,_ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext),false)
		
		if ret := exp.Index(); ret != nil {
		    index := b.buildIndexExpression(ret.(*gol.IndexContext))
			return nil, b.CreateMemberCallVariable(rv, index)
		}

		if ret := exp.DOT(); ret != nil {
			if id := exp.IDENTIFIER(); id != nil {
				return nil, b.CreateMemberCallVariable(rv, b.EmitConstInst(id.GetText()))
			}
		}
	}

	if !IslValue {
		rv,_ := b.buildPrimaryExpression(exp.PrimaryExpr().(*gol.PrimaryExprContext),false)
		if ret := exp.Arguments(); ret != nil {
			args := b.buildArgumentsExpression(ret.(*gol.ArgumentsContext))
			return b.EmitCall(b.NewCall(rv, args)),nil
		}

		if ret := exp.Index(); ret != nil {
		    index := b.buildIndexExpression(ret.(*gol.IndexContext))
			return b.ReadMemberCallVariable(rv, index), nil
		}

		if ret := exp.Slice_(); ret != nil {
		    values := b.buildSliceExpression(ret.(*gol.Slice_Context))
			return b.EmitMakeSlice(rv, values[0], values[1], values[2]), nil
		}

		if ret := exp.DOT(); ret != nil {
			if id := exp.IDENTIFIER(); id != nil {
				test := id.GetText()
				member :=  b.ReadMemberCallVariable(rv, b.EmitConstInst(test))
				return member, nil
			}
		}
	}

	return nil, nil
}

func (b *astbuilder) buildSliceExpression(exp *gol.Slice_Context) ([3]ssa.Value) {
	var values [3]ssa.Value
	
	if low := exp.GetLow(); low != nil {
		rightv,_ := b.buildExpression(low.(*gol.ExpressionContext), false)
	    values[0] = rightv
	}
	if high := exp.GetHigh(); high != nil {
		rightv,_ := b.buildExpression(high.(*gol.ExpressionContext), false)
	    values[1] = rightv
	}
	if max := exp.GetMax(); max != nil {
		rightv,_ := b.buildExpression(max.(*gol.ExpressionContext), false)
	    values[2] = rightv
	}

    return values
}

func (b *astbuilder) buildIndexExpression(arg *gol.IndexContext) (ssa.Value) {
	if exp := arg.Expression(); exp != nil {
		rv, _ := b.buildExpression(exp.(*gol.ExpressionContext), false)
		return rv
	}
	return nil
}

func (b *astbuilder) buildArgumentsExpression(arg *gol.ArgumentsContext) ([]ssa.Value) {
	var args []ssa.Value

	if typ := arg.Type_(); typ != nil {
	    ssatyp := b.buildType(typ.(*gol.Type_Context))
		args = append(args, b.EmitTypeValue(ssatyp))
	}

	if expl := arg.ExpressionList(); expl != nil {
		for _, exp := range expl.(*gol.ExpressionListContext).AllExpression(){
			rv, _ := b.buildExpression(exp.(*gol.ExpressionContext), false)
			args = append(args, rv)
		}
	}

	return args
}

func (b *astbuilder) buildExpressionStmt(stmt *gol.ExpressionStmtContext) []ssa.Value {
    if exp := stmt.Expression(); exp != nil {
        rightv,_ := b.buildExpression(exp.(*gol.ExpressionContext),false)
		return []ssa.Value{rightv}
    }
	return nil
}

func (b *astbuilder) buildOperandExpression(exp *gol.OperandContext, IslValue bool) (ssa.Value, *ssa.Variable) {
	recoverRange := b.SetRange(exp.BaseParserRuleContext)
	defer recoverRange()

	if !IslValue { // right
		if literal := exp.Literal(); literal != nil {
			return b.buildLiteral(literal.(*gol.LiteralContext)), nil
		}
		if id := exp.OperandName(); id != nil {
			return b.buildOperandNameR(id.(*gol.OperandNameContext)), nil
		}
		if e := exp.Expression(); e != nil {
			return b.buildExpression(e.(*gol.ExpressionContext), false)
		}
	} else { // left
		if id := exp.OperandName(); id != nil {
			return nil, b.buildOperandNameL(id.(*gol.OperandNameContext), false)
		}
	}
	return nil, nil
}

func (b *astbuilder) buildOperandNameL(name *gol.OperandNameContext, isLocal bool) *ssa.Variable {
	recoverRange := b.SetRange(name.BaseParserRuleContext)
	defer recoverRange()

	if id := name.IDENTIFIER(); id != nil {
		text := id.GetText()
		if text == "_" {
			b.NewError(ssa.Warn, TAG, "cannot use _ as value")
		}
		if b.GetFromCmap(text) {
			b.NewError(ssa.Warn, TAG, "cannot assign to const value")
		}
		if isLocal {
			return b.CreateLocalVariable(text)
		} else {
			return b.CreateVariable(text)
		}
	}
	return nil
}

func (b *astbuilder) buildOperandNameR(name *gol.OperandNameContext) ssa.Value {
	recoverRange := b.SetRange(name.BaseParserRuleContext)
	defer recoverRange()

	if id := name.IDENTIFIER(); id != nil {
		text := id.GetText()
		if text == "_" {
			b.NewError(ssa.Warn, TAG, "cannot use _ as value")
		}
		if text == "true" || text == "false" {
			return b.buildBoolLiteral(text)
		}
		v := b.PeekValue(text)
		if v != nil {
			return v
		}

		funcs := b.GetProgram().Funcs
		v = funcs[text]

		if v.(*ssa.Function) == nil {
			v = b.GetGlobalVariable(text)
		}
		if v == nil {
			b.NewError(ssa.Warn, TAG, fmt.Sprintf("not find variable %s in current scope", text))
			return b.EmitUndefined(text)
		}
		return v

	}
	return nil
}