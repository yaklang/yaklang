package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitFunctionDeclaration(raw phpparser.IFunctionDeclarationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.FunctionDeclarationContext)
	if i == nil {
		return nil
	}

	var attr string
	if ret := i.Attributes(); ret != nil {
		y.VisitAttributes(ret)
		_ = attr
	}

	isRef := i.Ampersand() != nil
	_ = isRef

	funcName := i.Identifier().GetText()
	funcDec, symbolTable := y.ir.NewFunc(funcName)
	current := y.ir.CurrentBlock
	y.ir.AddSubFunction(func() {
		y.ir = y.ir.PushFunction(funcDec, symbolTable, current)
		defer func() {
			y.ir.Finish()
			y.ir = y.ir.PopFunction()
		}()

		y.VisitFormalParameterList(i.FormalParameterList())
		y.VisitBlockStatement(i.BlockStatement())
	})
	y.ir.WriteVariable(funcName, funcDec)
	return nil
}
