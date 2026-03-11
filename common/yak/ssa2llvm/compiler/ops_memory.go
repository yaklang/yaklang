package compiler

import (
	"fmt"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

// compileMake handles SSA Make instruction (allocates arrays, slices, maps, etc.)
func (c *Compiler) compileMake(inst *ssa.Make) error {
	typ := inst.GetType()
	switch typ.GetTypeKind() {
	case ssa.StructTypeKind:
		return c.compileMakeStruct(inst, typ)
	case ssa.AnyTypeKind, ssa.ObjectTypeKind:
		// For Any/Object types, allocate generic memory. We represent addresses as `i64`
		// (uintptr) and only cast to pointers at FFI/runtime boundaries.
		return c.compileMakeGeneric(inst)
	case ssa.SliceTypeKind, ssa.BytesTypeKind:
		return c.compileMakeSlice(inst, typ)
	case ssa.MapTypeKind:
		return c.compileMakeGeneric(inst)
	default:
		// For unhandled types, create a null/zero placeholder
		c.Values[inst.GetId()] = llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		return nil
	}
}

func (c *Compiler) getOrInsertRuntimeMakeSlice() (llvm.Value, llvm.Type) {
	name := abi.MakeSliceSymbol
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func sliceElementKind(typ ssa.Type) abi.SliceElemKind {
	if typ == nil {
		return abi.SliceElemAny
	}
	if typ.GetTypeKind() == ssa.BytesTypeKind {
		return abi.SliceElemByte
	}

	objectType, ok := typ.(*ssa.ObjectType)
	if !ok || objectType == nil || objectType.FieldType == nil {
		return abi.SliceElemAny
	}

	switch objectType.FieldType.GetTypeKind() {
	case ssa.NumberTypeKind:
		return abi.SliceElemInt64
	case ssa.StringTypeKind:
		return abi.SliceElemString
	case ssa.ByteTypeKind, ssa.BytesTypeKind:
		return abi.SliceElemByte
	case ssa.BooleanTypeKind:
		return abi.SliceElemBool
	default:
		return abi.SliceElemAny
	}
}

func (c *Compiler) compileMakeSlice(inst *ssa.Make, typ ssa.Type) error {
	i64 := c.LLVMCtx.Int64Type()
	length := llvm.ConstInt(i64, 0, false)
	if inst.Len > 0 {
		val, err := c.getValue(inst, inst.Len)
		if err != nil {
			return err
		}
		length = c.coerceToInt64(val)
	}

	capacity := length
	if inst.Cap > 0 {
		val, err := c.getValue(inst, inst.Cap)
		if err != nil {
			return err
		}
		capacity = c.coerceToInt64(val)
	}

	makeFn, makeType := c.getOrInsertRuntimeMakeSlice()
	elemKind := llvm.ConstInt(i64, uint64(sliceElementKind(typ)), false)
	val := c.Builder.CreateCall(makeType, makeFn, []llvm.Value{elemKind, length, capacity}, "make_slice")
	c.Values[inst.GetId()] = val
	return nil
}

// compileMakeGeneric allocates a generic object (i8*)
func (c *Compiler) compileMakeGeneric(inst *ssa.Make) error {
	// Allocate 8 bytes (one i64) as placeholder
	size := llvm.ConstInt(c.LLVMCtx.Int64Type(), 8, false)
	mallocFn, mallocType := c.getOrInsertMalloc()
	rawVal := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{size}, "generic_alloc")
	// Keep as i64 (uintptr)
	c.Values[inst.GetId()] = rawVal
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
	// Keep as i64 (uintptr)
	c.Values[inst.GetId()] = rawVal
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

	keyID := inst.MemberCallKey
	keyVal, ok := fn.GetValueById(keyID)
	if !ok {
		return fmt.Errorf("key value %d not found", keyID)
	}

	keyStr := c.resolveMemberKeyString(keyVal)

	val := c.emitRuntimeGetField(parentVal, keyStr, inst.GetId())
	c.Values[inst.GetId()] = val
	return nil
}

// compileMemberCall handles generic member access (MemberCall interface)
func (c *Compiler) compileMemberCall(val ssa.Value, mc ssa.MemberCall) error {
	obj := ssa.GetLatestObject(val)
	key := ssa.GetLatestKey(val)

	if obj == nil {
		return fmt.Errorf("compileMemberCall: object is nil for value %d", val.GetId())
	}

	parentVal, err := c.getValue(nil, obj.GetId())
	if err != nil {
		return fmt.Errorf("compileMemberCall: failed to get object value: %w", err)
	}

	keyStr := c.resolveMemberKeyString(key)

	valResult := c.emitRuntimeGetField(parentVal, keyStr, val.GetId())
	c.Values[val.GetId()] = valResult
	return nil
}

func (c *Compiler) getOrInsertRuntimeGetField() (llvm.Value, llvm.Type) {
	name := "yak_runtime_get_field"
	fn := c.Mod.NamedFunction(name)

	retType := c.LLVMCtx.Int64Type()
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(retType, []llvm.Type{i8Ptr, i8Ptr}, false)

	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeSetField() (llvm.Value, llvm.Type) {
	name := "yak_runtime_set_field"
	fn := c.Mod.NamedFunction(name)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr, i8Ptr, c.LLVMCtx.Int64Type()}, false)

	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeToCString() (llvm.Value, llvm.Type) {
	name := "yak_runtime_to_cstring"
	fn := c.Mod.NamedFunction(name)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(i8Ptr, []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) resolveMemberKeyString(key ssa.Value) string {
	if cinst, ok := ssa.ToConstInst(key); ok {
		return strings.Trim(cinst.String(), "\"")
	}
	return strings.Trim(key.GetName(), "\"")
}

func (c *Compiler) coerceToI8Ptr(val llvm.Value) llvm.Value {
	i8PtrType := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	if val.Type().IntTypeWidth() > 0 {
		return c.Builder.CreateIntToPtr(val, i8PtrType, "obj_ptr")
	}
	if val.Type() != i8PtrType {
		return c.Builder.CreateBitCast(val, i8PtrType, "obj_ptr_cast")
	}
	return val
}

func (c *Compiler) coerceToInt64(val llvm.Value) llvm.Value {
	if val.Type().IntTypeWidth() == 64 {
		return val
	}
	if val.Type().IntTypeWidth() > 0 {
		width := val.Type().IntTypeWidth()
		if width == 1 {
			return c.Builder.CreateZExt(val, c.LLVMCtx.Int64Type(), "val_i64")
		}
		if width > 64 {
			return c.Builder.CreateTrunc(val, c.LLVMCtx.Int64Type(), "val_i64")
		}
		return c.Builder.CreateSExt(val, c.LLVMCtx.Int64Type(), "val_i64")
	}
	return c.Builder.CreatePtrToInt(val, c.LLVMCtx.Int64Type(), "ptr_i64")
}

func (c *Compiler) emitRuntimeGetField(objVal llvm.Value, keyStr string, id int64) llvm.Value {
	fn, fnType := c.getOrInsertRuntimeGetField()
	keyPtr := c.Builder.CreateGlobalStringPtr(keyStr, fmt.Sprintf("member_key_%d", id))
	objPtr := c.coerceToI8Ptr(objVal)
	return c.Builder.CreateCall(fnType, fn, []llvm.Value{objPtr, keyPtr}, "member_get")
}

func (c *Compiler) emitRuntimeSetField(objVal llvm.Value, keyStr string, val llvm.Value, id int64) {
	fn, fnType := c.getOrInsertRuntimeSetField()
	keyPtr := c.Builder.CreateGlobalStringPtr(keyStr, fmt.Sprintf("member_key_%d", id))
	objPtr := c.coerceToI8Ptr(objVal)
	intVal := c.coerceToInt64(val)
	c.Builder.CreateCall(fnType, fn, []llvm.Value{objPtr, keyPtr, intVal}, "")
}

func (c *Compiler) maybeEmitMemberSet(contextInst ssa.Instruction, val ssa.Value, llvmVal llvm.Value) error {
	mc, ok := val.(ssa.MemberCall)
	if !ok || !mc.IsMember() {
		return nil
	}
	switch val.(type) {
	case *ssa.ParameterMember, *ssa.Undefined:
		return nil
	}

	obj := ssa.GetLatestObject(val)
	key := ssa.GetLatestKey(val)
	if obj == nil || key == nil {
		return nil
	}

	objVal, err := c.getValue(contextInst, obj.GetId())
	if err != nil {
		return err
	}
	keyStr := c.resolveMemberKeyString(key)
	if keyStr == "" {
		return nil
	}

	c.emitRuntimeSetField(objVal, keyStr, llvmVal, val.GetId())
	return nil
}
