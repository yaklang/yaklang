package decompiler

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/rewriter"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
)

func ParseBytesCode(decompiler *core.Decompiler) ([]core.Statement, error) {
	err := decompiler.ParseSourceCode()
	if err != nil {
		return nil, err
	}
	println(utils.DumpOpcodesToDotExp(decompiler.OpCodeRoot))
	println(utils.DumpNodesToDotExp(decompiler.RootNode))
	err = rewriter.CheckNodesIsValid(decompiler.RootNode)
	if err != nil {
		return nil, err
	}
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
	if err != nil {
		return nil, err
	}
	return utils.NodesToStatements(sts), nil
}
