//go:build !no_language
// +build !no_language

package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitDeclareStatement(raw phpparser.IDeclareStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.DeclareStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
