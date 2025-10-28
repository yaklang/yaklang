//go:build !no_language
// +build !no_language

package java2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *singleFileBuilder) VisitArrayInitializer(raw javaparser.IArrayInitializerContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.ArrayInitializerContext)
	if i == nil {
		return nil
	}

	allVariableInitializer := i.AllVariableInitializer()
	if len(allVariableInitializer) == 0 {
		return y.EmitMakeBuildWithType(
			ssa.NewSliceType(ssa.CreateAnyType()),
			y.EmitConstInstPlaceholder(0), y.EmitConstInstPlaceholder(0),
		)
	}
	obj := y.InterfaceAddFieldBuild(len(allVariableInitializer),
		func(i int) ssa.Value { return y.EmitConstInstPlaceholder(i) },
		func(i int) ssa.Value {
			return y.VisitVariableInitializer(allVariableInitializer[i])
		})
	if utils.IsNil(obj) {
		return y.EmitUndefined(raw.GetText())
	}
	obj.GetType().(*ssa.ObjectType).Kind = ssa.SliceTypeKind
	return obj

}
