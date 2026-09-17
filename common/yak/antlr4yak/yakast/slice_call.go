package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func (y *YakCompiler) VisitSliceCall(raw yak.ISliceCallContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.SliceCallContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	//检查参数数量
	exps := i.AllExpression()
	if len(exps) == 0 && len(i.AllColon()) == 0 {
		y.panicCompilerError(sliceCallNoParamError)
	}
	if len(exps) > 3 {
		y.panicCompilerError(sliceCallTooManyParamError)
	}
	y.writeString("[")
	defer y.writeString("]")
	//解决:一侧为空的情况
	childrens := i.GetChildren()
	expect := true // 记录状态，如果期望是数字，得到的是:，则push一个默认数，不切换状态。
	t := 0         // 记录参数个数
	idEnd := false
	omitted := 0
	visitChildrens := childrens[1:]
	lenOfVisitChildrens := len(visitChildrens)
	for index, children := range visitChildrens {
		if expect {
			expression, isExpression := children.(*yak.ExpressionContext)

			if isExpression {
				// 表达式值类型必须为int，step值不能为0，否则报错
				//if t == 2 &&  != nil {
				//	panic(" step cannot be zero")
				//}
				y.VisitExpression(expression)
				expect = !expect
			} else {
				omitted |= 1 << t
				if t == 1 {
					idEnd = true
				}

				if index != lenOfVisitChildrens-1 {
					y.writeString(":")
				}
				if t == 2 {
					y.pushInteger(1, "1")
				} else {
					y.pushInteger(0, "0")
				}
			}

			t += 1
		} else { // 如果期望是:, 则直接切换状态
			if index != lenOfVisitChildrens-1 {
				y.writeString(":")
			}
			expect = !expect
		}
	}
	y.pushBool(idEnd)
	y.pushIterableCall(t)
	// Keep the legacy end flag on the stack for cached bytecode compatibility;
	// new code records each omitted bound separately on the instruction.
	y.codes[len(y.codes)-1].Op1 = yakvm.NewAutoValue(omitted)
	return nil
}
