package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitPhpBlock(raw phpparser.IPhpBlockContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.PhpBlockContext)
	if i == nil {
		return nil
	}

	return nil
}
