package yakast

import (
	"fmt"
	yak "yaklang.io/yaklang/common/yak/antlr4yak/parser"
	"yaklang.io/yaklang/common/yak/antlr4yak/yakvm"

	uuid "github.com/satori/go.uuid"
)

func (y *YakCompiler) VisitGoStmt(raw yak.IGoStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.GoStmtContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()
	y.writeString("go ")

	// go expr call;
	// 首先，go 自带一个 pop，这是为了平栈
	// 因为 go 后的表达式用的执行栈不应该和其他一样，所以应该全给他开个新的虚拟机，并且设置好起始的符号表
	// 后面的内容应该是
	//  ...
	//  ...
	// 	...
	//  ...
	//  call n 改成 async-call

	id := fmt.Sprintf("go/%v", uuid.NewV4().String())
	_ = id

	if code := i.InstanceCode(); code != nil {
		y.VisitInstanceCode(i.InstanceCode())
	} else {
		y.VisitExpression(i.Expression())
		y.VisitFunctionCall(i.FunctionCall())
	}

	/*
		新建 Go 指令
	*/
	if lastCode := y.codes[y.GetCodeIndex()]; lastCode.Opcode == yakvm.OpCall {
		// 函数指令
		lastCode.Opcode = yakvm.OpAsyncCall
	}
	return nil
}
