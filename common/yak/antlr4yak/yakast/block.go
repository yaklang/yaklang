package yakast

import (
	"github.com/google/uuid"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

// Legacy inline options are accepted for source compatibility; the compiler
// only emits bytecode and has no layout state.
func (y *YakCompiler) VisitBlockWithCallback(raw yak.IBlockContext, callback func(*YakCompiler), _ ...bool) interface{} {
	return y.VisitBlockWithCallbacks(raw, callback, nil)
}

// VisitBlockWithCallbacks injects bytecode immediately after entering a block
// scope and immediately before leaving it, in the same order as its statements.
func (y *YakCompiler) VisitBlockWithCallbacks(raw yak.IBlockContext, preCallback, postCallback func(*YakCompiler), _ ...bool) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*yak.BlockContext)
	if i == nil {
		return nil
	}
	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()
	recoverScope := y.SwitchSymbolTableInNewScope("block", uuid.New().String())
	if preCallback != nil {
		preCallback(y)
	}
	y.VisitStatementList(i.StatementList())
	if postCallback != nil {
		postCallback(y)
	}
	recoverScope()
	return nil
}

func (y *YakCompiler) VisitBlock(raw yak.IBlockContext, _ ...bool) interface{} {
	return y.VisitBlockWithCallback(raw, nil)
}
