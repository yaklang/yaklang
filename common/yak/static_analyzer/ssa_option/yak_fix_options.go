package ssa_option

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func fixFunctionOption() []ssaconfig.Option {
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
	return []ssaconfig.Option{
		ssaapi.WithExternBuildValueHandler("callable", callableBuilder),
		ssaapi.WithExternBuildValueHandler("dyn.IsYakFunc", callableBuilder),
	}
}
