package ssa_option

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

var (
	sliceT        = ssa.NewSliceType(ssa.TypeT)
	mapUT         = ssa.NewMapType(ssa.TypeU, ssa.TypeT)
	sliceTOrMapUT = ssa.NewOrType(
		sliceT,
		mapUT,
	)
	sliceTOrString = ssa.NewOrType(
		sliceT,
		ssa.CreateStringType(),
	)
	sliceTOrMapUTOrString = ssa.NewOrType(
		sliceT,
		mapUT,
		ssa.CreateStringType(),
	)
)

func genericFuncHandler(typ *ssa.FunctionType, b *ssa.FunctionBuilder, id string, v any) ssa.Value {
	// GenericFunc(T) T
	f := ssa.NewFunctionWithType(id, typ)
	f.SetGeneric(true)
	f.SetRange(b.CurrentRange)
	return f
}

func foreachHandler(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
	// Foreach/ForeachRight([]T, func(T))
	return genericFuncHandler(
		ssa.NewFunctionTypeDefine(id, []ssa.Type{
			sliceT,
			ssa.NewFunctionTypeDefine("",
				[]ssa.Type{ssa.TypeT},
				[]ssa.Type{},
				false),
		}, []ssa.Type{}, false),
		b, id, v)
}

func setCalculateHandler(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
	return genericFuncHandler(
		ssa.NewFunctionTypeDefine(id, []ssa.Type{
			sliceT,
			sliceT,
		}, []ssa.Type{sliceT}, false),
		b, id, v)
}

func genericGlobalFunctionOption() []ssaapi.Option {
	return []ssaapi.Option{
		ssaapi.WithExternBuildValueHandler("append", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// append([]T, T...) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{sliceT, ssa.TypeT}, []ssa.Type{sliceT}, true),
				b, id, v)
		}),
	}
}

func genericXLibraryFunctionOption() []ssaapi.Option {
	return []ssaapi.Option{
		ssaapi.WithExternBuildValueHandler("x.Map", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Map(Or([]T | Map[U]T), func(T) -> V) []V
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					ssa.NewOrType(
						sliceT,
						ssa.NewMapType(ssa.TypeU, ssa.TypeT),
					),
					ssa.NewFunctionTypeDefine("",
						[]ssa.Type{ssa.TypeT},
						[]ssa.Type{ssa.TypeV},
						false),
				}, []ssa.Type{ssa.NewSliceType(ssa.TypeV)}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Reduce", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Reduce([]T|Map[U]T, func(U, T) -> U, U) U
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceTOrMapUT,
					ssa.NewFunctionTypeDefine("",
						[]ssa.Type{ssa.TypeU, ssa.TypeT},
						[]ssa.Type{ssa.TypeU},
						false),
					ssa.TypeU,
				}, []ssa.Type{ssa.TypeU}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Filter", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Filter([]T|Map[U]T, func(T) -> bool) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceTOrMapUT,
					ssa.NewFunctionTypeDefine("",
						[]ssa.Type{ssa.TypeT},
						[]ssa.Type{ssa.CreateBooleanType()},
						false),
				}, []ssa.Type{sliceT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Find", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Find([]T|Map[U]T, func(T) -> bool) T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceTOrMapUT,
					ssa.NewFunctionTypeDefine("",
						[]ssa.Type{ssa.TypeT},
						[]ssa.Type{ssa.CreateBooleanType()},
						false),
				}, []ssa.Type{ssa.TypeT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Foreach", foreachHandler),
		ssaapi.WithExternBuildValueHandler("x.ForeachRight", foreachHandler),
		ssaapi.WithExternBuildValueHandler("x.Contains", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Contains([]T|Map[U]T|String, T) bool
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceTOrMapUTOrString,
					ssa.TypeT,
				}, []ssa.Type{ssa.CreateBooleanType()}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.IndexOf", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// IndexOf([]T|String, T) int
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceTOrString,
					ssa.TypeT,
				}, []ssa.Type{ssa.CreateBooleanType()}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Difference", setCalculateHandler),
		ssaapi.WithExternBuildValueHandler("x.Subtract", setCalculateHandler),
		ssaapi.WithExternBuildValueHandler("x.Intersect", setCalculateHandler),
		ssaapi.WithExternBuildValueHandler("x.IsSubset", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// IsSubset([]T, []T) bool
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
					sliceT,
				}, []ssa.Type{ssa.CreateBooleanType()}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.IsEqual", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// IsEqual(T, T) bool
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					ssa.TypeT,
					ssa.TypeT,
				}, []ssa.Type{ssa.CreateBooleanType()}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Chunk", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Chunk([]T, int) [][]T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
					ssa.CreateNumberType(),
				}, []ssa.Type{ssa.NewSliceType(sliceT)}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.RemoveRepeat", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// RemoveRepeat([]T) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{sliceT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Tail", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Tail([]T) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{sliceT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Head", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Head([]T) T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{ssa.TypeT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Drop", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Drop([]T) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{sliceT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Shift", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Shift([]T) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{sliceT}, false),
				b, id, v)
		}),
		// Values, Keys, Zip
		ssaapi.WithExternBuildValueHandler("x.ToFloat64", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// ToFloat64(x number) (number, bool)
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					ssa.CreateNumberType(),
				}, []ssa.Type{ssa.CreateNumberType(), ssa.CreateBooleanType()}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Shuffle", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Shuffle([]T) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{sliceT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Reverse", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Reverse([]T) []T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{sliceT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Sum", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Sum([]T) number
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{ssa.CreateNumberType()}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.All", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// All(T...) bool
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					ssa.TypeT,
				}, []ssa.Type{ssa.CreateBooleanType()}, true),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Max", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Max([]T) T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{ssa.TypeT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Min", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Min([]T) T
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
				}, []ssa.Type{ssa.TypeT}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Some", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Some([]T, T...) bool
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
					ssa.TypeT,
				}, []ssa.Type{ssa.CreateNumberType()}, true),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Every", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Every([]T, T...) bool
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
					ssa.TypeT,
				}, []ssa.Type{ssa.CreateNumberType()}, true),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Any", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Any(T...) bool
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					ssa.TypeT,
				}, []ssa.Type{ssa.CreateNumberType()}, true),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.Sort", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// Sort([]T, func(number, number) -> bool)
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					sliceT,
					ssa.NewFunctionTypeDefine("", []ssa.Type{
						ssa.CreateNumberType(), ssa.CreateNumberType(),
					},
						[]ssa.Type{ssa.CreateBooleanType()}, false),
				}, []ssa.Type{}, false),
				b, id, v)
		}),
		ssaapi.WithExternBuildValueHandler("x.If", func(b *ssa.FunctionBuilder, id string, v any) ssa.Value {
			// If(bool,T,U) T|U
			return genericFuncHandler(
				ssa.NewFunctionTypeDefine(id, []ssa.Type{
					ssa.CreateBooleanType(),
					ssa.TypeT,
					ssa.TypeU,
				}, []ssa.Type{
					ssa.NewOrType(ssa.TypeT, ssa.TypeU),
				}, false),
				b, id, v)
		}),
	}
}
