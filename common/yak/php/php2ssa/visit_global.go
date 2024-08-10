package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitGlobalStatement(raw phpparser.IGlobalStatementContext) interface{} {
	if y == nil || raw == nil {
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
		value.SetDefault(defaultValue)
		value.SetType(defaultValue.GetType())
	}
	return nil
}
