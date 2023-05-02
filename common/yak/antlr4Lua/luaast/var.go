package luaast

import (
	lua "yaklang.io/yaklang/common/yak/antlr4Lua/parser"
)

func (l *LuaTranslator) VisitVar(isAssign bool, raw lua.IVarContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.VarContext)
	if i == nil {
		return nil
	}
	varSuffixes := i.AllVarSuffix()
	if name := i.NAME(); name != nil {
		idName := name.GetText()
		var sym, ok = l.currentSymtbl.GetSymbolByVariableName(idName) // check nearest first

		if symExisted, exist := l.constTbl[idName]; ok && exist && isAssign {
			if symExisted == sym {
				panic("attempt to assign to const variable '" + idName + "'")
			}
		}

		if !ok && isAssign {
			var err error
			sym, err = l.rootSymtbl.NewSymbolWithReturn(idName) // without `local` keyword all var will be defined in global scope
			if err != nil {
				l.panicCompilerError(autoCreateSymbolFailed, idName)
			}
		}

		if isAssign && len(varSuffixes) == 0 { // var for assign
			l.pushLeftRef(sym)
		} else if ok { // var for exp use
			l.pushRef(sym)
		} else { // define but not assign
			l.pushIdentifierName(idName)
		}

		cnt := 1
		for _, varSuffix := range varSuffixes {
			if isAssign && ok && len(varSuffixes) == cnt {
				l.VisitVarSuffix(varSuffix, isAssign, ok)
				cnt++
			} else {
				l.VisitVarSuffix(varSuffix, isAssign, false)
				cnt++
			}
		}

		return nil

	} else if exp := i.Exp(); exp != nil && len(varSuffixes) > 0 && i.LParen() != nil { // a[1]
		l.VisitExp(exp)
		for _, varSuffix := range varSuffixes {
			l.VisitVarSuffix(varSuffix, isAssign, true)
		}
	}

	return nil
}

func (l *LuaTranslator) VisitVariadicEllipsis(isAssign bool) interface{} {
	if l == nil {
		return nil
	}

	idName := "..."
	var sym, ok = l.currentSymtbl.GetSymbolByVariableName(idName) // check nearest first
	if symExisted, exist := l.constTbl[idName]; ok && exist && isAssign {
		if symExisted == sym {
			panic("attempt to assign to const variable '" + idName + "'")
		}
	}
	if !ok && isAssign {
		var err error
		sym, err = l.rootSymtbl.NewSymbolWithReturn(idName) // without `local` keyword all var will be defined in global scope
		if err != nil {
			l.panicCompilerError(autoCreateSymbolFailed, idName)
		}
	}
	if isAssign { // var for assign
		l.pushLeftRef(sym)
	} else if ok { // var for exp use
		l.pushRef(sym)
	} else { // define but not assign
		l.pushIdentifierName(idName)
	}

	return nil
}

func (l *LuaTranslator) VisitVariadicEllipsisForTblConstruct(isAssign bool) interface{} {
	if l == nil {
		return nil
	}

	idName := "..."
	var sym, ok = l.currentSymtbl.GetSymbolByVariableName(idName) // check nearest first
	if symExisted, exist := l.constTbl[idName]; ok && exist && isAssign {
		if symExisted == sym {
			panic("attempt to assign to const variable '" + idName + "'")
		}
	}
	if !ok && isAssign {
		var err error
		sym, err = l.rootSymtbl.NewSymbolWithReturn(idName) // without `local` keyword all var will be defined in global scope
		if err != nil {
			l.panicCompilerError(autoCreateSymbolFailed, idName)
		}
	}
	if isAssign { // var for assign
		l.pushLeftRef(sym)
	} else if ok { // var for exp use
		l.pushRef(sym)
	} else { // define but not assign
		l.pushIdentifierName(idName)
	}

	return nil
}

func (l *LuaTranslator) VisitLocalVarWithName(isAssign bool, name string, attrib lua.IAttribContext) interface{} {
	if l == nil {
		return nil
	}

	optionalModifier := attrib.(*lua.AttribContext)
	if optionalModifier == nil {
		return nil
	}

	modifier := ""
	// TODO: close attribute的实现
	if optionalModifier.NAME() != nil {
		modifier = optionalModifier.NAME().GetText()
	}

	if modifier != "" {
		if modifier == "close" { // close还没实现
			panic("close attribute not implemented yet")
		} else if modifier != "const" {
			panic("unknown attribute '" + modifier + "'")
		}
	}
	idName := name
	var sym, ok = l.currentSymtbl.GetLocalSymbolByVariableName(idName)

	if !ok && isAssign {
		var err error
		sym, err = l.currentSymtbl.NewSymbolWithReturn(idName)
		if err != nil {
			l.panicCompilerError(autoCreateSymbolFailed, idName)
		}
		if modifier == "const" {
			l.constTbl[idName] = sym
		}
	}

	if isAssign && !ok { // var for assign
		l.pushLeftRef(sym)
	} else if ok { // var for exp use
		l.pushRef(sym)
	} else { // define but not assign
		l.pushIdentifierName(idName)
	}

	return nil

}

func (l *LuaTranslator) VisitVarSuffix(raw lua.IVarSuffixContext, isAssign bool, isInitialized bool) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.VarSuffixContext)
	if i == nil {
		return nil
	}
	nameAndArgs := i.AllNameAndArgs()
	if nameAndArgs != nil {
		// 链式调用
		for _, nameAndArg := range nameAndArgs {
			l.VisitNameAndArgs(nameAndArg)
		}
	}
	if exp := i.Exp(); exp != nil { // [exp] 数组情况
		l.VisitExp(exp)
		if isAssign && isInitialized {
			l.pushListWithLen(2)
			return nil
		}
		l.pushBool(false)
		l.pushIterableCall(1)
		return nil
	}
	if name := i.NAME(); name != nil {
		l.pushString(name.GetText(), name.GetText())
		if isAssign && isInitialized {
			l.pushListWithLen(2)
			return nil
		}
		l.pushBool(false)
		l.pushIterableCall(1)
		return nil
	}
	return nil
}

func (l *LuaTranslator) VisitVarOrExp(isAssign bool, raw lua.IVarOrExpContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.VarOrExpContext)
	if i == nil {
		return nil
	}
	if var_ := i.Var_(); var_ != nil {
		l.VisitVar(isAssign, var_)
		return nil
	}
	if exp := i.Exp(); exp != nil {
		l.VisitExp(exp)
		return nil
	}
	return nil
}
