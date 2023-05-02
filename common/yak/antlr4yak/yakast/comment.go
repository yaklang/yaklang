package yakast

import (
	"strings"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitLineCommentStmt(i *yak.LineCommentStmtContext) interface{} {
	if y == nil || i == nil {
		return nil
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	y.writeString(strings.TrimSpace(i.GetText()))
	return nil
}
