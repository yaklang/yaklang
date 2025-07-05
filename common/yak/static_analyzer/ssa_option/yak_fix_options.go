package ssa_option

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

func fixFunctionOption() []ssaapi.Option {
	callableBuilder := func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
		// callable(anyFunc) bool
		f := ssa.NewFunctionWithType(id,
			ssa.NewFunctionTypeDefine(
				id,
				[]ssa.Type{ssa.NewAnyFunctionType()},
				[]ssa.Type{ssa.CreateBooleanType()},
				false))
		f.SetRange(b.CurrentRange)
		return f
	}
	return []ssaapi.Option{
		ssaapi.WithExternBuildValueHandler("callable", callableBuilder),
		ssaapi.WithExternBuildValueHandler("dyn.IsYakFunc", callableBuilder),
	}
}
