package luaast

// VisitGoto jmp to label which is nop
func (l *LuaTranslator) VisitGoto(labelName string) interface{} {
	if l == nil || labelName == "" {
		return nil
	}

	sym, ok := l.currentLabeltbl.GetSymbolByVariableName(labelName)
	if !ok {
		l.panicCompilerError(labelNotDefined, labelName)
	}
	codeIndex, _ := l.currentLabeltbl.GetJmpIndexByVariableId(sym)

	l.pushJmpWithIndex(codeIndex)
	return nil
}
