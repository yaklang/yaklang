package php2ssa

import phpparser "github.com/yaklang/yaklang/common/yak/php/parser"

func (y *builder) VisitArguments(raw phpparser.IArgumentsContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ArgumentsContext)
	if i == nil {
		return nil
	}

	//

	return nil
}
