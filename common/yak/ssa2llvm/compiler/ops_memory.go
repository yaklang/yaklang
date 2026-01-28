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

	val := c.emitRuntimeGetField(parentVal, keyStr, inst.GetId())
	c.Values[inst.GetId()] = val
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
			return buildZExt(c.Builder, val, c.LLVMCtx.Int64Type(), "val_i64")
		}
		if width > 64 {
			return buildTrunc(c.Builder, val, c.LLVMCtx.Int64Type(), "val_i64")
		}
		return buildSExt(c.Builder, val, c.LLVMCtx.Int64Type(), "val_i64")
	}
	return c.Builder.CreatePtrToInt(val, c.LLVMCtx.Int64Type(), "ptr_i64")
}

func (c *Compiler) emitRuntimeGetField(objVal llvm.Value, keyStr string, id int64) llvm.Value {
	fn, fnType := c.getOrInsertRuntimeGetField()
	keyPtr := buildGlobalStringPtr(c.Builder, keyStr, fmt.Sprintf("member_key_%d", id))
	objPtr := c.coerceToI8Ptr(objVal)
	return c.Builder.CreateCall(fnType, fn, []llvm.Value{objPtr, keyPtr}, "member_get")
}

func (c *Compiler) emitRuntimeSetField(objVal llvm.Value, keyStr string, val llvm.Value, id int64) {
	fn, fnType := c.getOrInsertRuntimeSetField()
	keyPtr := buildGlobalStringPtr(c.Builder, keyStr, fmt.Sprintf("member_key_%d", id))
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

	obj := mc.GetObject()
	key := mc.GetKey()
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
