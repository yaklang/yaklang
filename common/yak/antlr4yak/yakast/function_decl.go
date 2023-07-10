package yakast

import (
	"fmt"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"

	uuid "github.com/satori/go.uuid"
)

func (y *YakCompiler) VisitAnonymousFunctionDecl(raw yak.IAnonymousFunctionDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.AnonymousFunctionDeclContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	var funcName string
	var funcSymbolId int
	if i.FunctionNameDecl() != nil {
		funcName = i.FunctionNameDecl().GetText()
		id, err := y.currentSymtbl.NewSymbolWithReturn(funcName)
		if err != nil {
			y.panicCompilerError(compileError, "cannot create new symbol for function name: "+funcName)
		}
		funcSymbolId = id
	}

	//函数分为闭包函数（包括箭头函数）和全局函数。
	//闭包函数可以在任何地方定义，并且继承父作用域。全局函数只能在根作用域定义。
	//闭包函数必须使用变量来接收或者立即调用，全局函数作用域是全局,且可以在任何位置调用。
	//闭包函数存于栈中，全局函数存于全局变量中。

	// 切换符号表和代码栈

	recoverCodeStack := y.SwitchCodes()
	recoverSymbolTable := y.SwitchSymbolTable("function", uuid.NewV4().String())
	defer recoverSymbolTable()
	var paramsSymbol []int
	var fun *yakvm.Function
	var isVariable bool
	if i.EqGt() != nil {
		// 处理参数：为参数设置函数内定义域的符号表
		if i.LParen() != nil && i.RParen() != nil {
			y.writeString("(")
			paramsSymbol, isVariable = y.VisitFunctionParamDecl(i.FunctionParamDecl())
			y.writeString(")")
			y.writeStringWithWhitespace("=>")
		} else {
			symbolText := i.Identifier().GetText()
			y.writeString(symbolText)
			y.writeStringWithWhitespace("=>")
			symbolId, err := y.currentSymtbl.NewSymbolWithReturn(symbolText)
			if err != nil {
				y.panicCompilerError(compileError, "cannot create identifier["+i.Identifier().GetText()+"] for params (arrow function): "+err.Error())
			}
			paramsSymbol = append(paramsSymbol, symbolId)
		}

		// 箭头函数模式
		// Expression 和 Block 需要分别支持
		if i.Block() == nil && i.Expression() == nil {
			y.panicCompilerError(compileError, "BUG: arrow function need expression or block at least")
		}
		if i.Block() != nil {
			y.VisitBlock(i.Block(), true)
			y.pushOperator(yakvm.OpReturn)
		} else {
			// 一般来说，这儿的栈是不平的，但是因为这是函数调用内部，最后一个栈数据应该作为函数返回值，这儿所以不需要处理，其他的情况
			// 隐式来说，这个相当于是 () => {return 123;} 也就是说 ()=>123 和 ()=>{return 123}等价，栈不平，在函数结束的时候
			// 应该 pop 一次栈数据做返回，如果没有，就返回 undefined
			y.VisitExpression(i.Expression())
			y.pushOperator(yakvm.OpReturn)
		}

		// 编译好的 FuncCode 配合符号表，一般来说就可以供执行和调用了
		fun = yakvm.NewFunction(y.codes, y.currentSymtbl)
		if y.sourceCodePointer != nil {
			fun.SetSourceCode(*y.sourceCodePointer)
		}
	} else {
		// 创建符号
		if fn := i.Func(); fn != nil {
			y.writeString(fn.GetText())
		}
		if funcName != "" {
			y.writeString(" ")
			y.writeString(funcName)
		}
		y.writeString("(")
		paramsSymbol, isVariable = y.VisitFunctionParamDecl(i.FunctionParamDecl())
		y.writeString(") ")
		// visit代码块
		y.VisitBlock(i.Block(), true)
		y.pushOperator(yakvm.OpReturn)
		funcCode := y.codes
		// 编译好的 FuncCode 配合符号表，一般来说就可以供执行和调用了
		fun = yakvm.NewFunction(funcCode, y.currentSymtbl)
		if y.sourceCodePointer != nil {
			fun.SetSourceCode(*y.sourceCodePointer)
		}
	}
	fun.GetSymbolId()
	if funcName != "" {
		// 如果函数名存在的话，设置函数名，创建新符号，并且把新符号告诉函数，以便后续处理
		fun.SetName(funcName)
		fun.SetSymbol(funcSymbolId)
	}

	//恢复现场
	recoverCodeStack()

	if fun == nil {
		y.panicCompilerError(compileError, "cannot create yak function from compiler")
	}
	fun.SetParamSymbols(paramsSymbol)
	fun.SetIsVariableParameter(isVariable)
	funcVal := &yakvm.Value{
		TypeVerbose: "anonymous-function",
		Value:       fun,
	}
	// 闭包函数，直接push到栈中
	if funcName != "" {
		y.pushLeftRef(fun.GetSymbolId())
	}
	y.pushValue(funcVal)
	// 如果有函数名的话，进行快速赋值
	if funcName != "" {
		funcVal.TypeVerbose = "named-function"
		y.pushOperator(yakvm.OpFastAssign)
	}

	return nil
}

func (y *YakCompiler) VisitFunctionParamDecl(raw yak.IFunctionParamDeclContext) ([]int, bool) {
	if y == nil || raw == nil {
		return nil, false
	}

	i, _ := raw.(*yak.FunctionParamDeclContext)
	if i == nil {
		return nil, false
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	ellipsis := i.Ellipsis()
	ids := i.AllIdentifier()
	lenOfIds := len(ids)
	symbols := make([]int, lenOfIds)

	tokenStart := i.BaseParserRuleContext.GetStart().GetColumn()
	lineLength := tokenStart
	eachParamOneLine := false
	identifierTokenLengths := make([]int, lenOfIds)
	for index, id := range ids {
		identifierTokenLengths[index] = len(id.GetText())
		if !eachParamOneLine && identifierTokenLengths[index] > FORMATTER_RECOMMEND_PARAM_LENGTH {
			eachParamOneLine = true
		}
	}

	if lenOfIds == 1 && eachParamOneLine {
		eachParamOneLine = false
	}

	hadIncIndent := false
	comments := getIdentifersSurroundComments(i.GetParser().GetTokenStream(), i.GetStart(), i.GetStop(), lenOfIds)

	for index, id := range ids {
		idText := id.GetText()
		lineLength += identifierTokenLengths[index]

		if lenOfIds > 1 { // 如果不是只有一个参数，超出单行最长长度或任意一个参数过长，就换行
			if eachParamOneLine {
				y.writeNewLine()
				if !hadIncIndent {
					y.incIndent()
					hadIncIndent = true
				}
				y.writeIndent()
				lineLength = y.indent*4 + identifierTokenLengths[index]
			} else if lineLength > FORMATTER_MAXWIDTH {
				y.writeNewLine()
				y.writeWhiteSpace(tokenStart)
				lineLength = tokenStart + identifierTokenLengths[index]
			}
		}

		symbolId, err := y.currentSymtbl.NewSymbolWithReturn(idText)
		if err != nil {
			y.panicCompilerError(compileError, "cannot create symbol for function params decl")
		}
		symbols[index] = symbolId

		y.writeString(idText)

		if comments[index] != "" {
			y.writeString(fmt.Sprintf(" /* %s */", comments[index]))
		}

		// 如果是最后一个参数且有...，就要加...
		if index == lenOfIds-1 {
			if ellipsis != nil {
				y.writeString("...")
			}
		}
		// 如果不是最后一个参数或者每个参数一行就要加,
		if index != lenOfIds-1 || eachParamOneLine {
			y.writeString(", ")
			lineLength += 2
		}
		// 如果是最后一个参数且每个参数一行，就要换行
		if index == lenOfIds-1 && eachParamOneLine {
			y.writeNewLine()
			if hadIncIndent {
				y.decIndent()
			}
			y.writeIndent()
		}
	}

	return symbols, ellipsis != nil
}
