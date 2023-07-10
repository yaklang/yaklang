package luaast

import (
	uuid "github.com/satori/go.uuid"
	lua "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"strings"
)

// VisitFuncNameAndBody is used to visit vanilla named function declaration
func (l *LuaTranslator) VisitFuncNameAndBody(name lua.IFuncnameContext, body lua.IFuncbodyContext) interface{} {
	if l == nil || name == nil || body == nil {
		return nil
	}

	i, _ := name.(*lua.FuncnameContext)
	t, _ := body.(*lua.FuncbodyContext)
	if i == nil || t == nil {
		return nil
	}

	allName := i.AllNAME()

	var funcName string
	var funcSymbolId int
	var paramsSymbol []int
	var isVariable bool

	if len(allName) > 1 { // member func
		funcName = ""
		id := l.rootSymtbl.NewSymbolWithoutName() // if none `local` keyword specified, function def always be in global
		funcSymbolId = id
	} else if len(allName) == 1 { // normal func
		funcName = allName[0].GetText()
		sym, ok := l.currentSymtbl.GetSymbolByVariableName(funcName)
		if !ok {
			id, err := l.rootSymtbl.NewSymbolWithReturn(funcName) // if none `local` keyword specified, function def always be in global
			if err != nil {
				panic("BUG: cannot create new symbol for function name: " + funcName)
			}
			funcSymbolId = id
		} else {
			funcSymbolId = sym
		}
	}

	recoverCodeStack := l.SwitchCodes()
	recoverSymbolTable := l.SwitchSymbolTable("function", uuid.NewV4().String())

	if parList := t.Parlist(); parList != nil { // have arg
		paramsSymbol, isVariable = l.VisitParList(parList)
	} else { // no arg
		paramsSymbol = make([]int, 0)
		isVariable = false
	}

	if i.Colon() != nil {
		selfID, err := l.currentSymtbl.NewSymbolWithReturn("self")
		if err != nil {
			l.panicCompilerError("cannot create symbol for function params decl")
		}
		paramsSymbol = append([]int{selfID}, paramsSymbol...)
	}

	l.VisitBlock(t.Block())
	if l.codes[len(l.codes)-1].Opcode != yakvm.OpReturn {
		l.pushOperator(yakvm.OpReturn)
	}

	function := yakvm.NewFunction(l.codes, l.currentSymtbl)
	function.GetSymbolId()
	// 设置函数名，创建新符号，并且把新符号告诉函数，以便后续处理
	function.SetName(funcName)
	function.SetSymbol(funcSymbolId)
	function.SetSourceCode(l.sourceCode)

	//恢复code stack
	recoverCodeStack()

	if function == nil {
		panic("BUG: cannot create lua function from compiler")
	}
	function.SetParamSymbols(paramsSymbol)
	// TODO: 这里看一下yak的可变函数行为模式和lua的区别看看要不要更改
	function.SetIsVariableParameter(isVariable)
	funcVal := &yakvm.Value{
		TypeVerbose: "anonymous-function",
		Value:       function,
	}

	// 闭包函数，直接push到栈中
	if funcName != "" {
		l.pushLeftRef(function.GetSymbolId())
	}

	l.pushValue(funcVal)

	// 如果有函数名的话，进行快速赋值
	if funcName != "" {
		funcVal.TypeVerbose = "named-function"
		l.pushGlobalFastAssign()
		l.pushOpPop()
	}

	recoverSymbolTable()

	if len(allName) > 1 {
		objectName := allName[0].GetText()
		objectID, ok := l.currentSymtbl.GetSymbolByVariableName(objectName)
		if !ok {
			panic("attempt to index a nil value/ cannot find object")
		}

		l.pushListWithLen(1)
		l.pushLeftRef(function.GetSymbolId())
		l.pushListWithLen(1)
		l.pushGlobalAssign()

		l.pushRef(function.GetSymbolId())
		l.pushListWithLen(1)

		cnt := 1
		l.pushRef(objectID)
		for index := 1; index < len(allName); index++ {
			if cnt == len(allName)-1 {
				propertyName := allName[index].GetText()
				l.pushString(propertyName, propertyName)
				l.pushListWithLen(2)
			} else {
				cnt++
				propertyName := allName[index].GetText()
				l.pushString(propertyName, propertyName)
				l.pushBool(false)
				l.pushIterableCall(1)
			}
		}
		l.pushListWithLen(1)
		l.pushGlobalAssign()
	}
	return nil
}

// VisitLocalFuncNameAndBody is used to visit vanilla named local function declaration
func (l *LuaTranslator) VisitLocalFuncNameAndBody(name string, body lua.IFuncbodyContext) interface{} {
	if l == nil || name == "" || body == nil {
		return nil
	}

	t, _ := body.(*lua.FuncbodyContext)
	if t == nil {
		return nil
	}

	var funcName string
	var funcSymbolId int
	var paramsSymbol []int
	var isVariable bool

	funcName = name
	sym, ok := l.currentSymtbl.GetLocalSymbolByVariableName(funcName)
	if !ok {
		id, err := l.currentSymtbl.NewSymbolWithReturn(funcName) // if none `local` keyword specified, function def always be in global
		if err != nil {
			panic("BUG: cannot create new symbol for function name: " + funcName)
		}
		funcSymbolId = id
	} else {
		funcSymbolId = sym
	}

	recoverCodeStack := l.SwitchCodes()
	recoverSymbolTable := l.SwitchSymbolTable("function", uuid.NewV4().String())
	defer recoverSymbolTable()

	if parList := t.Parlist(); parList != nil { // have arg
		paramsSymbol, isVariable = l.VisitParList(parList)
	} else { // no arg
		paramsSymbol = make([]int, 0)
		isVariable = false
	}

	l.VisitBlock(t.Block())
	if l.codes[len(l.codes)-1].Opcode != yakvm.OpReturn {
		l.pushOperator(yakvm.OpReturn)
	}

	function := yakvm.NewFunction(l.codes, l.currentSymtbl)
	function.SetSourceCode(l.sourceCode)

	function.GetSymbolId()
	// 设置函数名，创建新符号，并且把新符号告诉函数，以便后续处理
	function.SetName(funcName)
	function.SetSymbol(funcSymbolId)

	//恢复code stack
	recoverCodeStack()

	if function == nil {
		panic("BUG: cannot create lua function from compiler")
	}
	function.SetParamSymbols(paramsSymbol)
	function.SetIsVariableParameter(isVariable)
	funcVal := &yakvm.Value{
		TypeVerbose: "anonymous-function",
		Value:       function,
	}
	// 闭包函数，直接push到栈中
	if funcName != "" {
		l.pushLeftRef(function.GetSymbolId())
	}
	l.pushValue(funcVal)
	// 如果有函数名的话，进行快速赋值
	if funcName != "" {
		funcVal.TypeVerbose = "named-function"
		l.pushLocalFastAssign()
		l.pushOpPop()
	}

	return nil
}

// VisitFunctionDef is used to visit closure function declaration
func (l *LuaTranslator) VisitFunctionDef(def lua.IFunctiondefContext) interface{} {
	if l == nil || def == nil {
		return nil
	}

	i, _ := def.(*lua.FunctiondefContext)

	if i == nil {
		return nil
	}

	t := i.Funcbody().(*lua.FuncbodyContext)

	if t == nil {
		return nil
	}

	var funcName string
	var funcSymbolId int
	var paramsSymbol []int
	var isVariable bool

	recoverCodeStack := l.SwitchCodes()
	recoverSymbolTable := l.SwitchSymbolTable("function", uuid.NewV4().String())
	defer recoverSymbolTable()

	if parList := t.Parlist(); parList != nil { // have arg
		paramsSymbol, isVariable = l.VisitParList(parList)
	} else { // no arg
		paramsSymbol = make([]int, 0)
		isVariable = false
	}

	l.VisitBlock(t.Block())
	if l.codes[len(l.codes)-1].Opcode != yakvm.OpReturn {
		l.pushOperator(yakvm.OpReturn)
	}

	function := yakvm.NewFunction(l.codes, l.currentSymtbl)
	function.SetSourceCode(l.sourceCode)

	function.GetSymbolId()
	// 设置函数名，创建新符号，并且把新符号告诉函数，以便后续处理
	function.SetName(funcName)
	function.SetSymbol(funcSymbolId)

	//恢复code stack
	recoverCodeStack()

	if function == nil {
		panic("BUG: cannot create yak function from compiler")
	}
	function.SetParamSymbols(paramsSymbol)
	function.SetIsVariableParameter(isVariable)
	funcVal := &yakvm.Value{
		TypeVerbose: "anonymous-function",
		Value:       function,
	}

	// 闭包函数，直接push到栈中
	if funcName != "" {
		l.pushLeftRef(function.GetSymbolId())
	}

	l.pushValue(funcVal)

	// 如果有函数名的话，进行快速赋值
	if funcName != "" {
		funcVal.TypeVerbose = "named-function"
		l.pushGlobalFastAssign()
	}

	return nil
}

func (l *LuaTranslator) VisitParList(parlist lua.IParlistContext) ([]int, bool) {
	if l == nil || parlist == nil {
		return nil, false
	}

	parList := parlist.(*lua.ParlistContext)

	if parList == nil {
		return nil, false
	}

	var argName []string
	var symbols []int
	var lenOfIds int
	if parList.GetText() != "..." {
		nameList := parList.Namelist()
		argName = strings.Split(nameList.GetText(), ",")
		if parList.Ellipsis() != nil {
			argName = append(argName, "...")
		}
		lenOfIds = len(argName)
		symbols = make([]int, lenOfIds)

	} else if parList.GetText() == "..." { // only ... as func param
		argName = []string{"..."}
		symbols = make([]int, 1)
	} else { // no param
		symbols = make([]int, 0)
	}

	for index, name := range argName {
		symbolId, err := l.currentSymtbl.NewSymbolWithReturn(name)
		if err != nil {
			l.panicCompilerError("cannot create symbol for function params decl")
		}
		symbols[index] = symbolId
	}

	return symbols, parList.Ellipsis() != nil
}
