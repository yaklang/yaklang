package decompiler

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/rewriter"
)

func ParseBytesCode(decompiler *core.Decompiler) (res []statements.Statement, err error) {
	//defer func() {
	//	if e := recover(); e != nil {
	//		err = utils.Error(e)
	//	}
	//}()
	err = decompiler.ParseSourceCode()
	if err != nil {
		return nil, err
	}
	//println(utils.DumpNodesToDotExp(decompiler.RootNode))
	err = rewriter.CheckNodesIsValid(decompiler.RootNode)
	if err != nil {
		return nil, err
	}

	//core.GenerateDominatorTree(decompiler.RootNode)
	statementManager := rewriter.NewRootStatementManager(decompiler.RootNode)
	statementManager.SetId(decompiler.CurrentId)

	err = statementManager.Rewrite()
	if err != nil {
		return nil, err
	}
	//println(utils.DumpNodesToDotExp(decompiler.RootNode))
	sts, err := statementManager.ToStatements(func(node *core.Node) bool {
		return true
	})
	//println("to statement end")
	sts = funk.Filter(sts, func(item *core.Node) bool {
		_, ok := item.Statement.(*statements.StackAssignStatement)
		return !ok
	}).([]*core.Node)
	if err != nil {
		return nil, err
	}
	return core.NodesToStatements(sts), nil
}
