package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitTryCatchFinally(raw phpparser.ITryCatchFinallyContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.TryCatchFinallyContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitThrowStatement(raw phpparser.IThrowStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ThrowStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
