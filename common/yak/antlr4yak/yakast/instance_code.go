package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	"github.com/google/uuid"
)

func (y *YakCompiler) VisitInstanceCode(raw yak.IInstanceCodeContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.InstanceCodeContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	if op := i.Func(); op != nil {
		y.writeString(op.GetText() + " ")
	}
	//var funcName = fmt.Sprintf("anonymous-%v", uuid.New().String())

	var yakFn *yakvm.Function
	tableRecover := y.SwitchSymbolTable("instanceCode", uuid.New().String())
	defer tableRecover()
	recoverFunc := y.SwitchCodes()

	y.VisitBlock(i.Block(), true)
	y.pushOperator(yakvm.OpReturn)

	funcCode := make([]*yakvm.Code, len(y.codes))
	copy(funcCode, y.codes)
	freeValues := y.FreeValues

	recoverFunc()

	yakFn = yakvm.NewFunction(funcCode, y.currentSymtbl)
	yakFn.FreeValue = freeValues

	if y.sourceCodePointer != nil {
		yakFn.SetSourceCode(*y.sourceCodePointer)
	}
	if yakFn == nil {
		y.panicCompilerError(compileError, "cannot create yak function from compiler")
	}

	value := &yakvm.Value{
		TypeVerbose: "anonymous-function",
		Value:       yakFn,
	}
	// 配置函数
	//y.pushScope(yakvm.GetCurrentTableCount())
	if len(yakFn.FreeValue) == 0 {
		y.pushValue(value)
	} else {
		y.pushValueWithCopy(value)
	}
	y.pushCall(0)
	return nil
}
