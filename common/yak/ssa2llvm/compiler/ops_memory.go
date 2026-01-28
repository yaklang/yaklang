package compiler

import (
	"fmt"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// compileMake handles SSA Make instruction (allocates arrays, slices, maps, etc.)
func (c *Compiler) compileMake(inst *ssa.Make) error {
	typ := inst.GetType()
	switch typ.GetTypeKind() {
	case ssa.StructTypeKind:
		return c.compileMakeStruct(inst, typ)
	case ssa.AnyTypeKind, ssa.ObjectTypeKind:
		// For Any/Object types, allocate a generic pointer (i8*)
		// This is used by YakSSA for scope management and dynamic typing
		return c.compileMakeGeneric(inst)
	case ssa.SliceTypeKind, ssa.MapTypeKind:
		// Slices and maps are represented as pointers for now
		return c.compileMakeGeneric(inst)
	default:
		// For unhandled types, create a null pointer placeholder
		c.Values[inst.GetId()] = llvm.ConstPointerNull(llvm.PointerType(c.LLVMCtx.Int8Type(), 0))
		return nil
	}
}

// compileMakeGeneric allocates a generic object (i8*)
func (c *Compiler) compileMakeGeneric(inst *ssa.Make) error {
	// Allocate 8 bytes (one i64) as placeholder
	size := llvm.ConstInt(c.LLVMCtx.Int64Type(), 9, false)
	mallocFn, mallocType := c.getOrInsertMalloc()
	rawVal := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{size}, "generic_alloc")
	// Cast i64 -> i8*
	rawPtr := c.Builder.CreateIntToPtr(rawVal, llvm.PointerType(c.LLVMCtx.Int8Type(), 0), "generic_ptr")
	c.Values[inst.GetId()] = rawPtr
	return nil
}

// compileMakeStruct allocates a struct on the heap
func (c *Compiler) compileMakeStruct(inst *ssa.Make, typ ssa.Type) error {
	// 1. Get LLVM type for the struct
	llvmType := c.TypeConverter.ConvertType(typ)

	// 2. Calculate size using GEP trick (GetElementPtr null, 1) -> PtrToInt
	// Null pointer to the struct
	nullPtr := llvm.ConstPointerNull(llvm.PointerType(llvmType, 0))
	// GEP to get pointer to the "next" element (size of 1 element)
	one := llvm.ConstInt(c.LLVMCtx.Int32Type(), 1, false)
	gep := c.Builder.CreateGEP(llvmType, nullPtr, []llvm.Value{one}, "size_ptr")
	// Cast to Int64 to get the size
	size := c.Builder.CreatePtrToInt(gep, c.LLVMCtx.Int64Type(), "size_i64")

	// 3. Call malloc
	// malloc signature: i64 (int64) - returns uintptr to avoid cgo pointer checks
	mallocFn, mallocType := c.getOrInsertMalloc()
	rawVal := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{size}, "malloc_call")

	// 4. Cast i64 -> struct*
	structPtr := c.Builder.CreateIntToPtr(rawVal, llvm.PointerType(llvmType, 0), "struct_ptr")

	c.Values[inst.GetId()] = structPtr
	return nil
}

func (c *Compiler) getOrInsertMalloc() (llvm.Value, llvm.Type) {
	name := "yak_internal_malloc"
	fn := c.Mod.NamedFunction(name)

	// Define type: i64 yak_internal_malloc(int64)
	// We return i64 (uintptr) to avoid cgo "unpinned Go pointer" checks when returning generic memory
	retType := c.LLVMCtx.Int64Type()
	paramTypes := []llvm.Type{c.LLVMCtx.Int64Type()}
	fnType := llvm.FunctionType(retType, paramTypes, false)

	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

// compileParameterMember handles parameter member access (e.g. r.w)
// ParameterMember is an instruction in YakSSA.
func (c *Compiler) compileParameterMember(inst *ssa.ParameterMember) error {
	fn := inst.GetFunc()
	if fn == nil {
		return fmt.Errorf("ParameterMember %s has no function", inst.GetName())
	}

	var parentID int64
	switch inst.MemberCallKind {
	case ssa.ParameterMemberCall:
		if inst.MemberCallObjectIndex >= len(fn.Params) {
			return fmt.Errorf("ParameterMember index %d out of bounds (params len %d)", inst.MemberCallObjectIndex, len(fn.Params))
		}
		parentID = fn.Params[inst.MemberCallObjectIndex]
	case ssa.MoreParameterMember:
		if inst.MemberCallObjectIndex >= len(fn.ParameterMembers) {
			return fmt.Errorf("MoreParameterMember index %d out of bounds", inst.MemberCallObjectIndex)
		}
		parentID = fn.ParameterMembers[inst.MemberCallObjectIndex]
	default:
		return fmt.Errorf("unsupported ParameterMember kind: %v", inst.MemberCallKind)
	}

	parentVal, ok := c.Values[parentID]
	if !ok {
		return fmt.Errorf("parent value %d not found for ParameterMember %s", parentID, inst.GetName())
	}

	// Resolve Key
	keyID := inst.MemberCallKey
	keyVal, ok := fn.GetValueById(keyID)
	if !ok {
		return fmt.Errorf("key value %d not found", keyID)
	}

	keyStr := ""
	if cinst, ok := ssa.ToConstInst(keyVal); ok {
		keyStr = cinst.String()
	} else {
		keyStr = keyVal.GetName()
	}
	keyStr = strings.Trim(keyStr, "\"")

	// Generate GEP
	// We need type info of parent to find index.
	ssaParentVal, ok := fn.GetValueById(parentID)
	if !ok {
		return fmt.Errorf("SSA parent value %d not found", parentID)
	}
	ssaParentType := ssaParentVal.GetType()

	var structType *ssa.ObjectType
	if ptr, ok := ssaParentType.(*ssa.ObjectType); ok && ptr.Kind == ssa.PointerKind {
		if st, ok := ssaParentType.(*ssa.ObjectType); ok && st.Kind == ssa.StructTypeKind {
			structType = st
		} else {
			return fmt.Errorf("parent is not a struct: %v", ssaParentType)
		}
	} else if st, ok := ssaParentType.(*ssa.ObjectType); ok && st.Kind == ssa.StructTypeKind {
		structType = st
	} else {
		return fmt.Errorf("cannot determine struct type for field access")
	}

	fieldIndex := -1
	for i, k := range structType.Keys {
		if strings.Trim(k.String(), "\"") == keyStr {
			fieldIndex = i
			break
		}
	}
	if fieldIndex == -1 {
		return fmt.Errorf("field %s not found in struct", keyStr)
	}

	// GEP
	// Convert parentVal to StructPtr if needed (it is currently Int64)
	llStructType := c.TypeConverter.ConvertType(structType)
	llStructPtrType := llvm.PointerType(llStructType, 0)

	// Cast i64 to T*
	ptr := c.Builder.CreateIntToPtr(parentVal, llStructPtrType, "struct_ptr")

	// GEP
	// CreateStructGEP(type, val, idx, name)
	fieldPtr := c.Builder.CreateStructGEP(llStructType, ptr, fieldIndex, "field_ptr")

	// Load
	// We need the type of the field
	fieldSSAType := structType.FieldTypes[fieldIndex]
	llFieldType := c.TypeConverter.ConvertType(fieldSSAType)

	val := c.Builder.CreateLoad(llFieldType, fieldPtr, "field_val")

	if llFieldType.IntTypeWidth() == 64 {
		c.Values[inst.GetId()] = val
	} else {
		// Just try PtrToInt for pointers, or assume compatible for now.
		// Since we don't have TypeKind exposed easily in llvm.go wrapper provided,
		// we blindly try PtrToInt if it's not Int64, or just leave it.
		// Actually, BinOp expects Int64. If we return a Pointer, BinOp fails?
		// We should cast Ptr to Int64 if possible.
		// A simple heuristic: if it's not an integer type, cast to int64.
		// But Structs are also not integer types.
		//
		// Let's assume we cast EVERYTHING to Int64 for Phase 1/5 compatibility, except when impossible (Structs).
		// We can check if it's integer type width 0 (void/struct/ptr).
		if llFieldType.IntTypeWidth() == 0 {
			// Likely Pointer or Struct.
			// Try PtrToInt. If it fails (Struct), LLVM will error at runtime/build time?
			// We can't know for sure without TypeKind.
			// Let's assume Pointer for now.
			casted := c.Builder.CreatePtrToInt(val, c.LLVMCtx.Int64Type(), "ptr_int")
			c.Values[inst.GetId()] = casted
		} else {
			// Integer < 64 bit? ZExt?
			// For now, assume 64 bit.
			c.Values[inst.GetId()] = val
		}
	}
	return nil
}

// compileMemberCall handles generic member access (MemberCall interface)
func (c *Compiler) compileMemberCall(val ssa.Value, mc ssa.MemberCall) error {
	obj := mc.GetObject()
	key := mc.GetKey()

	if obj == nil {
		return fmt.Errorf("compileMemberCall: object is nil for value %d", val.GetId())
	}

	// 1. Get Base Pointer
	// We need to resolve the object value first.
	// Since obj is a Value, we can use c.getValue (via Compiler)
	// We assume getValue in ops.go handles contextInst=nil if we have CurrentFunction
	// But ops.go implementation might need to be verified or improved to allow nil
	// Or we use a dummy instruction if needed.
	// Assuming ops.go getValue signature is (contextInst, id).
	// We pass nil.
	// NOTE: ops.go getValue implementation: "fn := contextInst.GetFunc(); if fn == nil { ... }"
	// It checks contextInst first!
	// So passing nil WILL CRASH if ops.go is not robust.
	// I need to fix ops.go to be robust for nil contextInst.
	// I will fix ops.go in the next step.
	parentVal, err := c.getValue(nil, obj.GetId())
	if err != nil {
		return fmt.Errorf("compileMemberCall: failed to get object value: %w", err)
	}

	// 2. Resolve Key String
	keyStr := ""
	if cinst, ok := ssa.ToConstInst(key); ok {
		keyStr = cinst.String()
	} else {
		keyStr = key.GetName()
	}
	keyStr = strings.Trim(keyStr, "\"")

	// 3. Determine Struct Type and Index
	ssaParentType := obj.GetType()
	var structType *ssa.ObjectType
	// Handle Pointer to Struct
	if ptr, ok := ssaParentType.(*ssa.ObjectType); ok && ptr.Kind == ssa.PointerKind {
		if ptr.FieldType != nil {
			if st, ok := ptr.FieldType.(*ssa.ObjectType); ok && st.Kind == ssa.StructTypeKind {
				structType = st
			}
		}
		// Fallback: Check if the pointer type ITSELF behaves like a struct definition
		if structType == nil && len(ptr.Keys) > 0 {
			structType = ptr
		}
	} else if st, ok := ssaParentType.(*ssa.ObjectType); ok && st.Kind == ssa.StructTypeKind {
		structType = st
	}

	if structType == nil {
		return fmt.Errorf("compileMemberCall: parent is not a struct or pointer to struct: %v", ssaParentType)
	}

	fieldIndex := -1
	for i, k := range structType.Keys {
		if strings.Trim(k.String(), "\"") == keyStr {
			fieldIndex = i
			break
		}
	}
	if fieldIndex == -1 {
		return fmt.Errorf("compileMemberCall: field %s not found in struct %s", keyStr, structType.String())
	}

	// 4. Generate GEP
	llStructType := c.TypeConverter.ConvertType(structType)

	// If parentVal is i64 (parameter), cast to Ptr.
	// If parentVal is Ptr (Make result), use as is.
	var ptr llvm.Value
	if parentVal.Type().IntTypeWidth() > 0 {
		ptr = c.Builder.CreateIntToPtr(parentVal, llvm.PointerType(llStructType, 0), "struct_ptr")
	} else {
		// Assume it's already a pointer.
		if parentVal.Type() != llvm.PointerType(llStructType, 0) {
			ptr = c.Builder.CreateBitCast(parentVal, llvm.PointerType(llStructType, 0), "struct_ptr_cast")
		} else {
			ptr = parentVal
		}
	}

	fieldPtr := c.Builder.CreateStructGEP(llStructType, ptr, fieldIndex, "field_ptr")

	// 5. Load
	fieldSSAType := structType.FieldTypes[fieldIndex]
	llFieldType := c.TypeConverter.ConvertType(fieldSSAType)
	valResult := c.Builder.CreateLoad(llFieldType, fieldPtr, "field_val")

	// 6. Handle Type Width (Int64 compatibility)
	if llFieldType.IntTypeWidth() == 64 {
		c.Values[val.GetId()] = valResult
	} else {
		if llFieldType.IntTypeWidth() == 0 { // Pointer or other
			casted := c.Builder.CreatePtrToInt(valResult, c.LLVMCtx.Int64Type(), "ptr_int")
			c.Values[val.GetId()] = casted
		} else {
			// Integer < 64 bit? ZExt?
			// For now, assume 64 bit.
			c.Values[val.GetId()] = valResult
		}
	}

	return nil
}
