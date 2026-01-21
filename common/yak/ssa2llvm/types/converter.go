package types

import (
	"github.com/yaklang/yaklang/common/yak/ssa"
	"tinygo.org/x/go-llvm"
)

// TODO: Phase 2 - Type System Expansion
// This file will handle conversion from YakSSA types to LLVM types

type TypeConverter struct {
	ctx llvm.Context
}

func NewTypeConverter(ctx llvm.Context) *TypeConverter {
	return &TypeConverter{ctx: ctx}
}

// ConvertType converts a YakSSA type to its LLVM equivalent
// Currently only supports i64 (integers)
// TODO: Add support for:
//   - float64 (f64)
//   - string (pointer to char array)
//   - bool (i1)
//   - arrays
//   - structs
//   - maps
//   - function types
func (tc *TypeConverter) ConvertType(ssaType ssa.Type) llvm.Type {
	// Phase 1: Everything is i64
	return tc.ctx.Int64Type()

	// TODO Phase 2: Implement proper type conversion
	// switch ssaType.Kind() {
	// case ssa.NumberTypeKind:
	//     return tc.ctx.Int64Type()
	// case ssa.FloatTypeKind:
	//     return tc.ctx.DoubleType()
	// case ssa.StringTypeKind:
	//     return llvm.PointerType(tc.ctx.Int8Type(), 0)
	// case ssa.BooleanTypeKind:
	//     return tc.ctx.Int1Type()
	// case ssa.SliceTypeKind:
	//     return tc.convertSliceType(ssaType)
	// case ssa.StructTypeKind:
	//     return tc.convertStructType(ssaType)
	// case ssa.MapTypeKind:
	//     return tc.convertMapType(ssaType)
	// default:
	//     return tc.ctx.Int64Type()
	// }
}

// convertSliceType converts YakSSA slice type to LLVM struct
// TODO: Slice layout should be: {ptr, len, cap}
func (tc *TypeConverter) convertSliceType(ssaType ssa.Type) llvm.Type {
	panic("slice types not yet implemented")
}

// convertStructType converts YakSSA struct type to LLVM struct
// TODO: Need to handle field alignment and padding
func (tc *TypeConverter) convertStructType(ssaType ssa.Type) llvm.Type {
	panic("struct types not yet implemented")
}

// convertMapType converts YakSSA map type to LLVM runtime map handle
// TODO: Maps will be runtime-allocated, return opaque pointer
func (tc *TypeConverter) convertMapType(ssaType ssa.Type) llvm.Type {
	panic("map types not yet implemented")
}
