package luaast

import (
	lua "yaklang.io/yaklang/common/yak/antlr4Lua/parser"
)

func (l *LuaTranslator) VisitFunctionCall(raw lua.IFunctioncallContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.FunctioncallContext)
	if i == nil {
		return nil
	}

	// 函数调用需要先把参数压栈
	// 调用的时候，call n 表示要取多少数出来
	varOrExp := i.VarOrExp()
	args := i.AllNameAndArgs()

	l.VisitVarOrExp(false, varOrExp)

	for _, arg := range args { // at least one since it include LParen and RParen
		l.VisitNameAndArgs(arg)
	}

	return nil
}

func (l *LuaTranslator) VisitNameAndArgs(raw lua.INameAndArgsContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.NameAndArgsContext)
	if i == nil {
		return nil
	}

	if name := i.NAME(); name != nil { // member call
		l.pushString(name.GetText(), name.GetText())
		if i.Colon() != nil {
			l.pushLuaObjectMemberCall()
			if args := i.Args(); args != nil {
				l.VisitArgsWhenMemberCall(args)
				return nil
			}
		}
		l.pushLuaStaticMemberCall()
	}
	if args := i.Args(); args != nil {
		l.VisitArgs(args)
		return nil
	}
	return nil
}

func (l *LuaTranslator) VisitArgs(raw lua.IArgsContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.ArgsContext)
	if i == nil {
		return nil
	}
	if lParen := i.LParen(); lParen != nil {
		if expList := i.Explist(); expList != nil {
			ctx := expList.(*lua.ExplistContext)
			l.VisitExpListWithoutLen(expList)
			l.pushCall(len(ctx.AllExp()))
			return nil
		}
		l.pushCall(0) // function call with empty paren
		return nil
	}
	if tableConstructor := i.Tableconstructor(); tableConstructor != nil {
		l.VisitTableConstructor(tableConstructor)
		l.pushCall(1)
		return nil
	}
	if str := i.String_(); str != nil {
		l.VisitString(str)
		l.pushCall(1)
		return nil
	}
	return nil
}

func (l *LuaTranslator) VisitArgsWhenMemberCall(raw lua.IArgsContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.ArgsContext)
	if i == nil {
		return nil
	}
	if lParen := i.LParen(); lParen != nil {
		if expList := i.Explist(); expList != nil {
			ctx := expList.(*lua.ExplistContext)
			l.VisitExpListWithoutLen(expList)
			l.pushCall(len(ctx.AllExp()) + 1)
			return nil
		}
		l.pushCall(1) // function call with empty paren
	}
	if tableConstructor := i.Tableconstructor(); tableConstructor != nil {
		l.VisitTableConstructor(tableConstructor)
		l.pushCall(2)
	}

	if str := i.String_(); str != nil {
		l.VisitString(str)
		l.pushCall(2)
		return nil
	}
	return nil
}
