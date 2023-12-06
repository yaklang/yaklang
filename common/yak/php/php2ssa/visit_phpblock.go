package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitPhpBlock(raw phpparser.IPhpBlockContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.PhpBlockContext)
	if i == nil {
		return nil
	}

	// import? what the fuck?
	if len(i.AllImportStatement()) > 0 {
		// handle ImportStmt
	}

	for _, stmt := range i.AllTopStatement() {
		y.VisitTopStatement(stmt)
	}

	return nil
}
