package luaast

import lua "yaklang/common/yak/antlr4Lua/parser"

// VisitLabel save label to a dedicated label table just like symbolTable with no op
func (l *LuaTranslator) VisitLabel(raw lua.ILabelContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.LabelContext)
	if i == nil {
		return nil
	}
	label := i.NAME()
	labelName := label.GetText()

	l.pushNOP()

	var _, ok = l.currentLabeltbl.GetSymbolByVariableName(labelName)
	if !ok {
		var err error
		_, err = l.currentLabeltbl.NewSymbolWithReturn(labelName, l.GetCodeIndex())
		if err != nil {
			l.panicCompilerError(autoCreateLabelFailed, labelName)
		}
	} else {
		l.panicCompilerError(labelAlreadyDefined, labelName)
	}

	return nil

}
