package java2ssa

import (
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// VisitArrayInitializer 一维数组声明
func (y *builder) VisitArrayInitializer(raw javaparser.IArrayInitializerContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.ArrayInitializerContext)
	if i == nil {
		return nil
	}
	if i.Expression(0) == nil {
		return y.EmitMakeBuildWithType(
			ssa.NewSliceType(ssa.BasicTypes[ssa.AnyTypeKind]),
			y.EmitConstInst(0), y.EmitConstInst(0),
		)
	}
	allExpr := i.AllExpression()
	obj := y.InterfaceAddFieldBuild(len(allExpr),
		func(i int) ssa.Value { return ssa.NewConst(i) },
		func(i int) ssa.Value { return y.VisitExpression(allExpr[i]) },
	)
	obj.GetType().(*ssa.ObjectType).Kind = ssa.SliceTypeKind
	return obj
}

// VisitTwoDimArrayInitializer 二维数组声明
func (y *builder) VisitTwoDimArrayInitializer(raw javaparser.ITwoDimArraryInitializerContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*javaparser.TwoDimArraryInitializerContext)
	if i == nil {
		return nil
	}
	if i.ArrayInitializer(0) == nil {
		return y.EmitMakeBuildWithType(
			ssa.NewSliceType(ssa.BasicTypes[ssa.AnyTypeKind]),
			y.EmitConstInst(0), y.EmitConstInst(0),
		)
	}

	allExpr := i.AllArrayInitializer()
	obj := y.InterfaceAddFieldBuild(len(allExpr),
		func(i int) ssa.Value { return ssa.NewConst(i) },
		func(i int) ssa.Value { return y.VisitArrayInitializer(allExpr[i]) },
	)
	obj.GetType().(*ssa.ObjectType).Kind = ssa.SliceTypeKind
	return obj
}
