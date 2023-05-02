package luaast

import lua "yaklang.io/yaklang/common/yak/antlr4Lua/parser"

func (l *LuaTranslator) VisitVarList(isAssign bool, raw lua.IVarlistContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.VarlistContext)
	if i == nil {
		return nil
	}

	allVars := i.AllVar_()

	for _, _var := range allVars {
		l.VisitVar(isAssign, _var)
	}
	if allVars != nil {
		l.pushListWithLen(len(allVars))
	}
	return nil
}
