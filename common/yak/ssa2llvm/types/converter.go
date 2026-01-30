package types

import (
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type TypeConverter struct {
	Ctx       llvm.Context
	TypeCache map[string]llvm.Type
}

func NewTypeConverter(ctx llvm.Context) *TypeConverter {
	return &TypeConverter{
		Ctx:       ctx,
		TypeCache: make(map[string]llvm.Type),
	}
}

func (tc *TypeConverter) ConvertType(t ssa.Type) llvm.Type {
	if t == nil {
		return tc.Ctx.VoidType()
	}

	typeName := t.String()
	if cached, ok := tc.TypeCache[typeName]; ok {
		return cached
	}

	var llvmType llvm.Type

	switch t.GetTypeKind() {
	case ssa.NumberTypeKind:
		// MVP: Assume int64 for all numbers
		llvmType = tc.Ctx.Int64Type()
	case ssa.BooleanTypeKind:
		llvmType = tc.Ctx.Int1Type()
	case ssa.StringTypeKind:
		// String as i8*
		llvmType = llvm.PointerType(tc.Ctx.Int8Type(), 0)
	case ssa.AnyTypeKind, ssa.ObjectTypeKind, ssa.SliceTypeKind, ssa.MapTypeKind:
		// Opaque runtime-managed types use pointer semantics
		llvmType = llvm.PointerType(tc.Ctx.Int8Type(), 0)
	case ssa.PointerKind:
		// Default to i8* for opaque pointers or if we can't determine pointee
		llvmType = llvm.PointerType(tc.Ctx.Int8Type(), 0)
	case ssa.StructTypeKind:
		if st, ok := t.(*ssa.ObjectType); ok {
			llvmType = tc.createStructType(st)
		} else {
			llvmType = llvm.PointerType(tc.Ctx.Int8Type(), 0)
		}
	case ssa.NullTypeKind, ssa.UndefinedTypeKind:
		llvmType = tc.Ctx.VoidType()
	default:
		// Fallback for Phase 1/5 compatibility
		llvmType = tc.Ctx.Int64Type()
	}

	tc.TypeCache[typeName] = llvmType
	return llvmType
}

func (tc *TypeConverter) createStructType(t *ssa.ObjectType) llvm.Type {
	name := t.Name
	if name == "" {
		name = "struct_anon"
	}

	// Use named struct to handle recursion
	structType := tc.Ctx.StructCreateNamed(name)
	// Cache it immediately to handle recursive references
	tc.TypeCache[t.String()] = structType

	fieldTypes := make([]llvm.Type, len(t.FieldTypes))
	for i, ft := range t.FieldTypes {
		fieldTypes[i] = tc.ConvertType(ft)
	}

	structType.StructSetBody(fieldTypes, false)
	return structType
}
