package luaast

import lua "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"

func (l *LuaTranslator) VisitExpList(raw lua.IExplistContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.ExplistContext)
	if i == nil {
		return nil
	}

	allExps := i.AllExp()

	for _, exp := range allExps {
		l.VisitExp(exp)
	}

	if allExps != nil {
		l.pushListWithLen(len(allExps))
	}

	return nil
}

func (l *LuaTranslator) VisitExpListWithoutLen(raw lua.IExplistContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.ExplistContext)
	if i == nil {
		return nil
	}

	allExps := i.AllExp()

	for _, exp := range allExps {
		l.VisitExp(exp)
	}

	return nil
}
