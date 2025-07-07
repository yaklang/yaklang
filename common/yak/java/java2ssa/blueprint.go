package java2ssa

import "github.com/yaklang/yaklang/common/yak/ssa"

func (y *singleFileBuilder) GetBluePrint(name string) *ssa.Blueprint {
	if y == nil {
		return nil
	}
	// try to get inner class firstly
	if y.MarkedThisClassBlueprint != nil {
		n := y.MarkedThisClassBlueprint.Name + INNER_CLASS_SPLIT + name
		bp := y.FunctionBuilder.GetBluePrint(n)
		if bp != nil {
			return bp
		}
	}
	return y.FunctionBuilder.GetBluePrint(name)
}
