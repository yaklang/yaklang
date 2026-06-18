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
	if inst != nil {
		if _, ok := c.getCachedValue(inst, inst.GetId()); ok {
			return nil
		}
	}
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
		c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
		return nil
	}
}

func (c *Compiler) getOrInsertRuntimeMakeSlice() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.MakeSliceSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeMakeObject() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.MakeObjectSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, nil, false)
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

func makeInitialMemberCount(inst *ssa.Make) int64 {
	if inst == nil {
		return 0
	}
	count := int64(0)
	inst.ForEachMember(func(key, member ssa.Value) bool {
		if key == nil || member == nil || member.GetId() <= 0 || member.GetId() == inst.GetId() {
			return true
		}
		count++
		return true
	})
	return count
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
	} else if memberCount := makeInitialMemberCount(inst); memberCount > 0 {
		length = llvm.ConstInt(i64, uint64(memberCount), false)
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
	c.cacheValue(inst.GetId(), val)
	return c.emitInitialMakeMemberAssignments(inst, val)
}

// compileMakeGeneric allocates a generic object (i8*)
func (c *Compiler) compileMakeGeneric(inst *ssa.Make) error {
	makeFn, makeType := c.getOrInsertRuntimeMakeObject()
	objVal := c.Builder.CreateCall(makeType, makeFn, nil, "make_object")
	c.cacheValue(inst.GetId(), objVal)
	return c.emitInitialMakeMemberAssignments(inst, objVal)
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
	c.cacheValue(inst.GetId(), rawVal)
	return nil
}

func (c *Compiler) getOrInsertMalloc() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.InternalMallocSymbol)
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
	if inst != nil {
		if _, ok := c.getCachedValue(inst, inst.GetId()); ok {
			return nil
		}
	}
	fn := inst.GetFunc()
	if fn == nil {
		return fmt.Errorf("ParameterMember %s has no function", inst.GetName())
	}

	parentID, err := c.resolveParameterMemberParentID(fn, inst)
	if err != nil {
		return err
	}

	keyID := inst.MemberCallKey
	keyVal, ok := fn.GetValueById(keyID)
	if !ok {
		return fmt.Errorf("key value %d not found", keyID)
	}
	keyStr := c.resolveMemberKeyString(keyVal)

	parentVal, err := c.getValue(inst, parentID)
	if err != nil {
		return fmt.Errorf("parent value %d for ParameterMember %s: %w", parentID, inst.GetName(), err)
	}

	val := c.emitRuntimeGetField(parentVal, keyStr, inst.GetId())
	c.cacheValue(inst.GetId(), val)
	return nil
}

func (c *Compiler) resolveParameterMemberParentID(fn *ssa.Function, inst *ssa.ParameterMember) (int64, error) {
	if fn == nil || inst == nil {
		return 0, fmt.Errorf("resolveParameterMemberParentID: missing function or parameter member")
	}

	switch inst.MemberCallKind {
	case ssa.ParameterMemberCall:
		if inst.MemberCallObjectIndex >= len(fn.Params) {
			return 0, fmt.Errorf("ParameterMember index %d out of bounds (params len %d)", inst.MemberCallObjectIndex, len(fn.Params))
		}
		return fn.Params[inst.MemberCallObjectIndex], nil
	case ssa.MoreParameterMember:
		if inst.MemberCallObjectIndex >= len(fn.ParameterMembers) {
			return 0, fmt.Errorf("MoreParameterMember index %d out of bounds", inst.MemberCallObjectIndex)
		}
		return fn.ParameterMembers[inst.MemberCallObjectIndex], nil
	case ssa.FreeValueMemberCall:
		for variable, id := range fn.FreeValues {
			if variable != nil && variable.GetName() == inst.MemberCallObjectName {
				return id, nil
			}
		}
		return 0, fmt.Errorf("free value %q not found for ParameterMember %s", inst.MemberCallObjectName, inst.GetName())
	default:
		return 0, fmt.Errorf("unsupported ParameterMember kind: %v", inst.MemberCallKind)
	}
}

// compileMemberCall handles generic member access (MemberCall interface)
func (c *Compiler) compileMemberCall(contextInst ssa.Instruction, val ssa.Value, mc ssa.MemberCall) error {
	_ = mc
	obj := ssa.GetLatestObject(val)
	key := ssa.GetLatestKey(val)
	keyStr := c.resolveMemberKeyString(key)

	if obj != nil {
		if extern, ok := ssa.ToExternLib(obj); ok && extern != nil {
			if err := c.compileExternLibMember(contextInst, val, extern, key, keyStr); err != nil {
				return err
			}
			return nil
		}

		var fn *ssa.Function
		if contextInst != nil {
			fn = contextInst.GetFunc()
		}
		if pkg := c.resolveMemberObjectName(fn, obj); pkg != "" && keyStr != "" {
			if err := c.compileYaklibExportMember(contextInst, val, pkg, keyStr); err != nil {
				return err
			}
			if _, ok := c.getCachedValue(contextInst, val.GetId()); ok {
				return nil
			}
		}
	}

	if obj == nil {
		if _, ok := val.(*ssa.Undefined); ok {
			zero := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
			c.cacheValue(val.GetId(), zero)
			return nil
		}
		return fmt.Errorf("compileMemberCall: object is nil for value %d", val.GetId())
	}

	memberID := val.GetId()
	var loadCtx ssa.Instruction
	if inst, ok := val.(ssa.Instruction); ok {
		loadCtx = inst
	} else {
		loadCtx = contextInst
	}
	var emitErr error
	c.withSSADefInsertPoint(memberID, func() {
		if !c.isSSAValueStored(obj.GetId()) && !c.hasValueSlot(obj.GetId()) {
			if _, err := c.getValue(loadCtx, obj.GetId()); err != nil {
				emitErr = fmt.Errorf("compileMemberCall: failed to get object value: %w", err)
				return
			}
		}
		c.reanchorSSADefInsertPoint(memberID)
		slot := c.ensureValueSlot(memberID)
		if slot.IsNil() {
			emitErr = fmt.Errorf("compileMemberCall: slot for value %d unavailable", memberID)
			return
		}
		c.reanchorSSADefInsertPoint(memberID)
		parentVal := c.loadSSAValue(obj.GetId())
		valResult := c.emitRuntimeGetField(parentVal, keyStr, memberID)
		c.Builder.CreateStore(c.coerceToInt64(valResult), slot)
		c.markSSAValueStored(memberID)
	})
	if emitErr != nil {
		return emitErr
	}
	if err := c.maybeEmitMemberSet(contextInst, val, memberID); err != nil {
		return err
	}
	return emitErr
}

func (c *Compiler) compileDynamicMemberValue(contextInst ssa.Instruction, val ssa.Value) error {
	valResult, err := c.dynamicMemberReadValue(contextInst, val, val.GetId())
	if err != nil {
		return err
	}
	c.cacheValue(val.GetId(), valResult)
	return c.maybeEmitMemberSet(contextInst, val, val.GetId())
}

func (c *Compiler) dynamicMemberReadValue(contextInst ssa.Instruction, val ssa.Value, memberID int64) (llvm.Value, error) {
	obj := val.GetObject()
	key := val.GetKey()
	keyStr := c.resolveMemberKeyString(key)
	if obj == nil || keyStr == "" {
		return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false), nil
	}

	parentVal, err := c.valueForMemberObject(contextInst, obj)
	if err != nil {
		return llvm.Value{}, fmt.Errorf("dynamicMemberReadValue: failed to get object value: %w", err)
	}
	return c.emitRuntimeGetField(parentVal, keyStr, memberID), nil
}

func (c *Compiler) valueForMemberObject(contextInst ssa.Instruction, obj ssa.Value) (llvm.Value, error) {
	if obj != nil && c.hasValueSlot(obj.GetId()) {
		return c.loadSSAValue(obj.GetId()), nil
	}
	return c.getValue(contextInst, obj.GetId())
}

func (c *Compiler) compileUndefined(inst *ssa.Undefined) error {
	if inst == nil || !inst.IsMember() || !c.hasAssignedMemberCallVariable(inst) {
		return nil
	}
	if c.shouldReadMemberValueDynamically(inst, inst.GetId()) {
		return c.compileDynamicMemberValue(inst, inst)
	}
	return c.compileMemberCall(inst, inst, inst)
}

func (c *Compiler) getOrInsertRuntimeGetField() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeGetFieldSymbol)
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
	name := c.runtimeSymName(abi.RuntimeSetFieldSymbol)
	fn := c.Mod.NamedFunction(name)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr, i8Ptr, c.LLVMCtx.Int64Type(), c.LLVMCtx.Int64Type()}, false)

	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeToCString() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeToCStringSymbol)
	fn := c.Mod.NamedFunction(name)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(i8Ptr, []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) resolveMemberKeyString(key ssa.Value) string {
	if key == nil {
		return ""
	}
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

func (c *Compiler) emitRuntimeSetField(objVal llvm.Value, keyStr string, val llvm.Value, ssaVal ssa.Value, id int64) {
	fn, fnType := c.getOrInsertRuntimeSetField()
	keyPtr := c.Builder.CreateGlobalStringPtr(keyStr, fmt.Sprintf("member_key_%d", id))
	objPtr := c.coerceToI8Ptr(objVal)
	intVal := c.coerceToInt64(val)
	flags := uint64(0)
	ssaVal = c.effectiveRuntimeFieldValue(ssaVal)
	if ssaVal != nil && c.ssaValueIsPointer(ssaVal, ssaVal.GetFunc()) {
		tag := llvm.ConstInt(c.LLVMCtx.Int64Type(), yakTaggedPointerMask, false)
		intVal = c.Builder.CreateOr(intVal, tag, "yak_set_field_arg_tag")
	}
	if ssaVal != nil {
		if typ := ssaVal.GetType(); typ != nil {
			switch typ.GetTypeKind() {
			case ssa.BooleanTypeKind:
				flags |= abi.FlagFieldBool
			case ssa.StringTypeKind:
				flags |= abi.FlagFieldString
			}
		}
		if flags&abi.FlagFieldString == 0 && c.memberTargetFieldTypeKind(ssaVal) == ssa.StringTypeKind {
			flags |= abi.FlagFieldString
		}
	}
	flagVal := llvm.ConstInt(c.LLVMCtx.Int64Type(), flags, false)
	c.Builder.CreateCall(fnType, fn, []llvm.Value{objPtr, keyPtr, intVal, flagVal}, "")
}

func (c *Compiler) markInitialMemberValue(id int64) {
	if c == nil || id <= 0 {
		return
	}
	if c.initialMemberValueIDs == nil {
		c.initialMemberValueIDs = make(map[int64]struct{})
	}
	c.initialMemberValueIDs[id] = struct{}{}
}

func (c *Compiler) isInitialMemberValue(id int64) bool {
	if c == nil || id <= 0 || c.initialMemberValueIDs == nil {
		return false
	}
	_, ok := c.initialMemberValueIDs[id]
	return ok
}

func (c *Compiler) isInitializingMemberValue(id int64) bool {
	if c == nil || id <= 0 || c.initializingMemberValueIDs == nil {
		return false
	}
	return c.initializingMemberValueIDs[id] > 0
}

func (c *Compiler) withInitializingMemberValue(id int64, fn func() error) error {
	if fn == nil {
		return nil
	}
	if c == nil || id <= 0 {
		return fn()
	}
	if c.initializingMemberValueIDs == nil {
		c.initializingMemberValueIDs = make(map[int64]int)
	}
	c.initializingMemberValueIDs[id]++
	defer func() {
		c.initializingMemberValueIDs[id]--
		if c.initializingMemberValueIDs[id] <= 0 {
			delete(c.initializingMemberValueIDs, id)
		}
	}()
	return fn()
}

func (c *Compiler) effectiveRuntimeFieldValue(ssaVal ssa.Value) ssa.Value {
	if sideEffect, ok := ssaVal.(*ssa.SideEffect); ok && sideEffect != nil {
		if actual := c.resolveSideEffectActualValue(sideEffect); actual != nil {
			return actual
		}
	}
	if phi, ok := ssaVal.(*ssa.Phi); ok && phi != nil {
		if hint := c.phiRuntimeFieldValueHint(phi); hint != nil {
			return hint
		}
	}
	return ssaVal
}

func (c *Compiler) phiRuntimeFieldValueHint(phi *ssa.Phi) ssa.Value {
	if phi == nil || phi.GetFunc() == nil {
		return nil
	}
	for _, edgeID := range phi.Edge {
		if edgeID <= 0 {
			continue
		}
		edge, ok := phi.GetFunc().GetValueById(edgeID)
		if !ok || edge == nil {
			continue
		}
		if inst, ok := edge.(ssa.Instruction); ok && inst.IsLazy() {
			if self, ok := inst.Self().(ssa.Value); ok && self != nil {
				edge = self
			}
		}
		if sideEffect, ok := edge.(*ssa.SideEffect); ok && sideEffect != nil {
			if actual := c.resolveSideEffectActualValue(sideEffect); actual != nil {
				edge = actual
			}
		}
		if typ := edge.GetType(); typ != nil {
			switch typ.GetTypeKind() {
			case ssa.StringTypeKind, ssa.BooleanTypeKind:
				return edge
			}
		}
		if constInst, ok := edge.(*ssa.ConstInst); ok && constInst != nil {
			if constInst.IsString() || constInst.IsBoolean() {
				return edge
			}
		}
	}
	return nil
}

func (c *Compiler) memberTargetFieldTypeKind(ssaVal ssa.Value) ssa.TypeKind {
	if ssaVal == nil || !ssaVal.IsMember() || ssaVal.GetObject() == nil {
		return ssa.AnyTypeKind
	}
	objType, ok := ssaVal.GetObject().GetType().(*ssa.ObjectType)
	if !ok || objType == nil || objType.FieldType == nil {
		return ssa.AnyTypeKind
	}
	return objType.FieldType.GetTypeKind()
}

func (c *Compiler) maybeEmitMemberSet(contextInst ssa.Instruction, val ssa.Value, resultID int64) error {
	if val == nil {
		return nil
	}
	initialMemberValue := c.isInitialMemberValue(resultID)
	if initialMemberValue && c.isInitializingMemberValue(resultID) {
		return nil
	}
	if val.IsMember() && !initialMemberValue {
		switch val.(type) {
		case *ssa.ParameterMember, *ssa.Undefined:
		default:
			obj := val.GetObject()
			key := val.GetKey()
			if obj != nil && key != nil {
				if !c.memberAssignmentObjectAvailable(contextInst, obj) {
					c.queuePendingMemberSet(val, resultID, obj, key, true)
					return c.emitAssignedMemberVariableSets(contextInst, val, resultID)
				}
				objVal, err := c.getValue(contextInst, obj.GetId())
				if err != nil {
					return err
				}
				keyStr := c.resolveMemberKeyString(key)
				if keyStr != "" && !c.shouldSkipOutdatedMemberSet(val, keyStr) {
					llvmVal, err := c.valueForMemberSet(contextInst, val, resultID, false)
					if err != nil {
						return err
					}
					c.emitRuntimeSetField(objVal, keyStr, llvmVal, c.assignedSSAValue(contextInst, resultID), val.GetId())
					c.markMemberVariableSetEmitted(resultID, obj, keyStr)
				}
			}
		}
	}
	return c.emitAssignedMemberVariableSets(contextInst, val, resultID)
}

func (c *Compiler) shouldSkipOutdatedMemberSet(val ssa.Value, keyStr string) bool {
	if val == nil || keyStr == "" || val.GetObject() == nil {
		return false
	}
	switch val.(type) {
	case *ssa.ConstInst, *ssa.Make:
	default:
		return false
	}
	currentID := val.GetId()
	if currentID <= 0 {
		return false
	}
	skip := false
	val.GetObject().ForEachMember(func(key, member ssa.Value) bool {
		if key == nil || member == nil || member.GetId() == currentID {
			return true
		}
		if c.resolveMemberKeyString(key) == keyStr && member.GetId() > currentID {
			if c.memberValueOverridesInSameBlock(val, member) {
				skip = true
				return false
			}
		}
		return true
	})
	return skip
}

func (c *Compiler) memberValueOverridesInSameBlock(current, candidate ssa.Value) bool {
	currentInst, ok := current.(ssa.Instruction)
	if !ok || currentInst == nil || currentInst.GetBlock() == nil {
		return false
	}
	candidateInst, ok := candidate.(ssa.Instruction)
	if !ok || candidateInst == nil || candidateInst.GetBlock() == nil {
		return false
	}
	currentBlock := currentInst.GetBlock()
	candidateBlock := candidateInst.GetBlock()
	if currentBlock.GetId() != candidateBlock.GetId() {
		return false
	}
	return instructionIndex(candidateBlock, candidate.GetId()) > instructionIndex(currentBlock, current.GetId())
}

func (c *Compiler) shouldReadMemberValueDynamically(val ssa.Value, id int64) bool {
	if val == nil || id <= 0 || c.isInitializingMemberValue(id) || !val.IsMember() {
		return false
	}
	if current := c.currentFunction(); current != nil && val.GetFunc() != nil && val.GetFunc() != current {
		return false
	}
	switch val.(type) {
	case *ssa.Parameter, *ssa.ParameterMember, *ssa.SideEffect:
		return false
	case *ssa.Undefined:
		return !val.IsExtern() && val.GetObject() != nil && val.GetKey() != nil
	}
	return val.GetObject() != nil && val.GetKey() != nil
}

func (c *Compiler) initialMemberValueOverridden(val ssa.Value) bool {
	if val == nil || !c.isInitialMemberValue(val.GetId()) || val.GetObject() == nil || val.GetKey() == nil {
		return false
	}
	keyStr := c.resolveMemberKeyString(val.GetKey())
	if keyStr == "" {
		return false
	}
	overridden := false
	val.GetObject().ForEachMember(func(key, member ssa.Value) bool {
		if key == nil || member == nil || member.GetId() == val.GetId() {
			return true
		}
		if member.GetId() > val.GetId() && c.resolveMemberKeyString(key) == keyStr {
			overridden = true
			return false
		}
		return true
	})
	return overridden
}

func (c *Compiler) assignedSSAValue(contextInst ssa.Instruction, resultID int64) ssa.Value {
	var fn *ssa.Function
	if contextInst != nil {
		fn = contextInst.GetFunc()
	} else {
		fn = c.currentFunction()
	}
	if fn == nil {
		return nil
	}
	got, ok := fn.GetValueById(resultID)
	if !ok || got == nil {
		return nil
	}
	assigned, _ := got.(ssa.Value)
	return assigned
}

func (c *Compiler) hasAssignedMemberCallVariable(val ssa.Value) bool {
	if val == nil {
		return false
	}
	for _, variable := range val.GetAllVariables() {
		if variable != nil && variable.IsMemberCall() {
			return true
		}
	}
	return false
}

func (c *Compiler) emitAssignedMemberVariableSets(contextInst ssa.Instruction, val ssa.Value, resultID int64) error {
	if val == nil || resultID <= 0 {
		return nil
	}
	vars := val.GetAllVariables()
	if len(vars) == 0 {
		return nil
	}

	for _, variable := range vars {
		if variable == nil || !variable.IsMemberCall() {
			continue
		}
		obj := ssa.GetLatestObject(variable)
		key := ssa.GetLatestKey(variable)
		if obj == nil || key == nil {
			continue
		}
		if !c.memberAssignmentValueExists(contextInst, obj) {
			continue
		}
		keyStr := c.resolveMemberKeyString(key)
		if keyStr == "" || c.sameMemberTarget(val, obj, keyStr) {
			continue
		}
		if !c.memberAssignmentObjectAvailable(contextInst, obj) {
			c.queuePendingMemberSet(val, resultID, obj, key, false)
			continue
		}
		if c.memberVariableSetEmitted(resultID, obj, keyStr) {
			continue
		}
		objectWasStored := c.isSSAValueStored(obj.GetId())
		objVal, err := c.getValue(contextInst, obj.GetId())
		if err != nil {
			return err
		}
		emitSet := func() error {
			currentObjVal := objVal
			if c.isSSAValueStored(obj.GetId()) {
				currentObjVal = c.loadSSAValue(obj.GetId())
			}
			llvmVal, err := c.valueForMemberSet(contextInst, val, resultID, true)
			if err != nil {
				return err
			}
			c.emitRuntimeSetField(currentObjVal, keyStr, llvmVal, c.assignedSSAValue(contextInst, resultID), resultID)
			c.markMemberVariableSetEmitted(resultID, obj, keyStr)
			return nil
		}
		if !objectWasStored && c.isSSAValueStored(obj.GetId()) {
			if objInst, ok := obj.(ssa.Instruction); ok && objInst != nil {
				if err := c.withInstructionInsertPoint(objInst, emitSet); err != nil {
					return err
				}
				continue
			}
		}
		if err := emitSet(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) finishConstValue(inst *ssa.ConstInst, resultID int64) error {
	if inst == nil {
		return nil
	}
	if err := c.maybeEmitMemberSet(inst, inst, resultID); err != nil {
		return err
	}
	return c.emitMemberVariableSetsForCompiledKey(inst, inst)
}

func (c *Compiler) emitMemberVariableSetsForCompiledKey(contextInst ssa.Instruction, key ssa.Value) error {
	if c == nil || key == nil || key.GetId() <= 0 {
		return nil
	}
	return c.flushPendingMemberSets(contextInst, nil, key)
}

func (c *Compiler) emitMemberVariableSetsForCompiledObject(contextInst ssa.Instruction, obj ssa.Value) error {
	if c == nil || obj == nil || obj.GetId() <= 0 {
		return nil
	}
	return c.flushPendingMemberSets(contextInst, obj, nil)
}

func (c *Compiler) emitDirectMemberValueSetIfReady(contextInst ssa.Instruction, source ssa.Value, resultID int64) bool {
	if source == nil || resultID <= 0 || !source.IsMember() || source.GetObject() == nil || source.GetKey() == nil {
		return false
	}
	obj := source.GetObject()
	keyStr := c.resolveMemberKeyString(source.GetKey())
	if keyStr == "" || c.memberVariableSetEmitted(resultID, obj, keyStr) {
		return false
	}
	if !c.memberAssignmentObjectAvailable(contextInst, obj) {
		return false
	}
	objVal, ok := c.getCachedValue(contextInst, obj.GetId())
	if !ok || objVal.IsNil() {
		if c.isSSAValueStored(obj.GetId()) || c.hasValueSlot(obj.GetId()) {
			objVal = c.loadSSAValue(obj.GetId())
		}
	}
	if objVal.IsNil() {
		return false
	}
	llvmVal, err := c.valueForMemberSet(contextInst, source, resultID, false)
	if err != nil || llvmVal.IsNil() {
		return false
	}
	c.emitRuntimeSetField(objVal, keyStr, llvmVal, source, resultID)
	c.markMemberVariableSetEmitted(resultID, obj, keyStr)
	return true
}

func (c *Compiler) valueAvailableAtInstruction(value ssa.Value, contextInst ssa.Instruction) bool {
	if value == nil {
		return false
	}
	if _, ok := c.getCachedValue(contextInst, value.GetId()); ok {
		return true
	}
	if contextInst == nil && c.hasValueSlot(value.GetId()) {
		return true
	}
	if phi, ok := value.(*ssa.Phi); ok && phi != nil && contextInst != nil && contextInst.GetBlock() != nil {
		if c.hasValueSlot(value.GetId()) {
			if phi.GetBlock() == nil {
				return true
			}
			return c.blockDominates(contextInst.GetFunc(), phi.GetBlock().GetId(), contextInst.GetBlock().GetId())
		}
	}
	valueInst, ok := value.(ssa.Instruction)
	if !ok || valueInst == nil {
		return true
	}
	if contextInst == nil || valueInst.GetBlock() == nil || contextInst.GetBlock() == nil {
		return false
	}
	if valueInst.GetBlock().GetId() != contextInst.GetBlock().GetId() {
		return c.blockDominates(contextInst.GetFunc(), valueInst.GetBlock().GetId(), contextInst.GetBlock().GetId())
	}
	return instructionIndex(valueInst.GetBlock(), valueInst.GetId()) <= instructionIndex(contextInst.GetBlock(), contextInst.GetId())
}

func (c *Compiler) queuePendingMemberSet(source ssa.Value, resultID int64, obj, key ssa.Value, direct bool) {
	if c == nil || c.function == nil || source == nil || resultID <= 0 || obj == nil || key == nil {
		return
	}
	keyStr := c.resolveMemberKeyString(key)
	if keyStr == "" {
		return
	}
	pendingKey := c.memberVariableSetKey(resultID, obj, keyStr)
	if _, ok := c.function.pendingMemberSets[pendingKey]; !ok {
		c.function.pendingMemberSetKeys = append(c.function.pendingMemberSetKeys, pendingKey)
	}
	c.function.pendingMemberSets[pendingKey] = pendingMemberSet{
		source:   source,
		resultID: resultID,
		obj:      obj,
		key:      key,
		direct:   direct,
	}
}

func (c *Compiler) flushPendingMemberSets(contextInst ssa.Instruction, obj, key ssa.Value) error {
	if c == nil || c.function == nil || len(c.function.pendingMemberSets) == 0 {
		return nil
	}
	for _, pendingKey := range append([]string{}, c.function.pendingMemberSetKeys...) {
		pending, ok := c.function.pendingMemberSets[pendingKey]
		if !ok {
			continue
		}
		if obj != nil && (pending.obj == nil || pending.obj.GetId() != obj.GetId()) {
			continue
		}
		pendingKeyStr := c.resolveMemberKeyString(pending.key)
		if key != nil && (pending.key == nil || pendingKeyStr != c.resolveMemberKeyString(key)) {
			continue
		}
		if !c.pendingMemberSetInContext(pending, contextInst) {
			continue
		}
		if !c.valueAvailableAtInstruction(pending.source, contextInst) {
			continue
		}
		var emitted bool
		if pending.direct {
			emitted = c.emitDirectMemberValueSetIfReady(contextInst, pending.source, pending.resultID)
		} else {
			emitted = c.emitMemberVariableSetIfReady(contextInst, pending.source, pending.resultID, pending.obj, pending.key)
		}
		if emitted {
			delete(c.function.pendingMemberSets, pendingKey)
		}
	}
	return nil
}

func (c *Compiler) pendingMemberSetInContext(pending pendingMemberSet, contextInst ssa.Instruction) bool {
	if contextInst == nil {
		return true
	}
	fn := contextInst.GetFunc()
	if fn == nil {
		return true
	}
	for _, value := range []ssa.Value{pending.source, pending.obj, pending.key} {
		if value == nil {
			continue
		}
		if valueFn := value.GetFunc(); valueFn != nil && valueFn != fn {
			return false
		}
	}
	return true
}

func instructionIndex(block *ssa.BasicBlock, id int64) int {
	if block == nil {
		return -1
	}
	for index, instID := range block.Insts {
		if instID == id {
			return index
		}
	}
	return -1
}

func (c *Compiler) emitMemberVariableSetIfReady(contextInst ssa.Instruction, source ssa.Value, resultID int64, obj, key ssa.Value) bool {
	if source == nil || resultID <= 0 || obj == nil || key == nil {
		return false
	}
	if !c.memberAssignmentObjectAvailable(contextInst, obj) {
		return false
	}
	keyStr := c.resolveMemberKeyString(key)
	if keyStr == "" || c.sameMemberTarget(source, obj, keyStr) {
		return false
	}
	if c.memberVariableSetEmitted(resultID, obj, keyStr) {
		return false
	}

	objVal, ok := c.getCachedValue(contextInst, obj.GetId())
	if !ok || objVal.IsNil() {
		if c.isSSAValueStored(obj.GetId()) || c.hasValueSlot(obj.GetId()) {
			objVal = c.loadSSAValue(obj.GetId())
		}
	}
	if objVal.IsNil() {
		return false
	}
	llvmVal, err := c.valueForMemberSet(contextInst, source, resultID, true)
	if err != nil || llvmVal.IsNil() {
		return false
	}
	if c.memberVariableSetEmitted(resultID, obj, keyStr) {
		return true
	}
	c.emitRuntimeSetField(objVal, keyStr, llvmVal, c.assignedSSAValue(contextInst, resultID), resultID)
	c.markMemberVariableSetEmitted(resultID, obj, keyStr)
	return true
}

func (c *Compiler) valueForMemberSet(contextInst ssa.Instruction, source ssa.Value, resultID int64, dynamicMemberRead bool) (llvm.Value, error) {
	if source != nil {
		if _, ok := source.(*ssa.Phi); ok && c.hasValueSlot(resultID) {
			return c.loadSSAValue(resultID), nil
		}
		if dynamicMemberRead {
			if memberSource := c.memberSourceForMemberSetDynamicRead(source); memberSource != nil {
				return c.dynamicMemberReadValue(contextInst, memberSource, resultID)
			}
		}
		if c.shouldReadInitialMemberValueForMemberSet(source) {
			return c.dynamicMemberReadValue(contextInst, source, resultID)
		}
	}
	return c.finishGetValue(contextInst, resultID)
}

func (c *Compiler) shouldReadInitialMemberValueForMemberSet(source ssa.Value) bool {
	return source != nil &&
		source.IsMember() &&
		source.GetObject() != nil &&
		source.GetKey() != nil &&
		c.isInitialMemberValue(source.GetId()) &&
		!c.isInitializingMemberValue(source.GetId())
}

func (c *Compiler) memberSourceForMemberSetDynamicRead(source ssa.Value) ssa.Value {
	if source == nil {
		return nil
	}
	memberSource := c.effectiveRuntimeFieldValue(source)
	if memberSource == nil || memberSource.GetId() <= 0 || c.isInitializingMemberValue(memberSource.GetId()) {
		return nil
	}
	if !c.shouldReadMemberValueDynamically(memberSource, memberSource.GetId()) {
		return nil
	}
	return memberSource
}

func (c *Compiler) memberVariableSetKey(resultID int64, obj ssa.Value, keyStr string) string {
	objID := int64(0)
	if obj != nil {
		objID = obj.GetId()
	}
	return fmt.Sprintf("%d:%d:%s", resultID, objID, keyStr)
}

func (c *Compiler) memberVariableSetEmitted(resultID int64, obj ssa.Value, keyStr string) bool {
	if c == nil || resultID <= 0 || obj == nil || keyStr == "" {
		return false
	}
	if c.emittedMemberVariableSets == nil {
		return false
	}
	_, ok := c.emittedMemberVariableSets[c.memberVariableSetKey(resultID, obj, keyStr)]
	return ok
}

func (c *Compiler) markMemberVariableSetEmitted(resultID int64, obj ssa.Value, keyStr string) {
	if c == nil || resultID <= 0 || obj == nil || keyStr == "" {
		return
	}
	if c.emittedMemberVariableSets == nil {
		c.emittedMemberVariableSets = make(map[string]struct{})
	}
	c.emittedMemberVariableSets[c.memberVariableSetKey(resultID, obj, keyStr)] = struct{}{}
}

func (c *Compiler) memberAssignmentObjectAvailable(contextInst ssa.Instruction, obj ssa.Value) bool {
	if obj == nil {
		return false
	}
	if !c.memberAssignmentValueExists(contextInst, obj) {
		return false
	}
	var fn *ssa.Function
	if contextInst != nil {
		fn = contextInst.GetFunc()
	} else {
		fn = c.currentFunction()
	}
	if fn == nil {
		return true
	}
	switch obj.(type) {
	case *ssa.Parameter, *ssa.ParameterMember:
		return obj.GetFunc() == fn
	}
	objInst, ok := obj.(ssa.Instruction)
	if !ok || objInst == nil || contextInst == nil || objInst.GetBlock() == nil || contextInst.GetBlock() == nil {
		return false
	}
	if objInst.GetBlock().GetId() != contextInst.GetBlock().GetId() {
		return c.blockDominates(fn, objInst.GetBlock().GetId(), contextInst.GetBlock().GetId())
	}
	return instructionIndex(objInst.GetBlock(), objInst.GetId()) <= instructionIndex(contextInst.GetBlock(), contextInst.GetId())
}

func (c *Compiler) memberAssignmentValueExists(contextInst ssa.Instruction, val ssa.Value) bool {
	if val == nil || val.GetId() <= 0 {
		return false
	}
	var fn *ssa.Function
	if contextInst != nil {
		fn = contextInst.GetFunc()
	} else {
		fn = c.currentFunction()
	}
	if fn == nil {
		return true
	}
	_, ok := fn.GetValueById(val.GetId())
	return ok
}

func (c *Compiler) blockDominates(fn *ssa.Function, dominatorID, blockID int64) bool {
	if fn == nil || dominatorID <= 0 || blockID <= 0 {
		return false
	}
	if dominatorID == blockID || dominatorID == fn.EnterBlock {
		return true
	}

	blockIDs := collectFunctionBlockIDs(fn)
	if len(blockIDs) == 0 {
		return false
	}
	all := make(map[int64]struct{}, len(blockIDs))
	for _, id := range blockIDs {
		all[id] = struct{}{}
	}
	if _, ok := all[dominatorID]; !ok {
		return false
	}
	if _, ok := all[blockID]; !ok {
		return false
	}

	doms := make(map[int64]map[int64]struct{}, len(blockIDs))
	for _, id := range blockIDs {
		if id == fn.EnterBlock {
			doms[id] = map[int64]struct{}{id: {}}
			continue
		}
		doms[id] = cloneIDSet(all)
	}

	changed := true
	for changed {
		changed = false
		for _, id := range blockIDs {
			if id == fn.EnterBlock {
				continue
			}
			preds := predecessorBlockIDs(fn, id)
			if len(preds) == 0 {
				preds = blockPreds(fn, id)
			}
			next := cloneIDSet(all)
			seenPred := false
			for _, predID := range preds {
				predDom, ok := doms[predID]
				if !ok {
					continue
				}
				if !seenPred {
					next = cloneIDSet(predDom)
					seenPred = true
				} else {
					next = intersectIDSets(next, predDom)
				}
			}
			if !seenPred {
				next = map[int64]struct{}{}
			}
			next[id] = struct{}{}
			if !sameIDSet(doms[id], next) {
				doms[id] = next
				changed = true
			}
		}
	}
	_, ok := doms[blockID][dominatorID]
	return ok
}

func blockPreds(fn *ssa.Function, blockID int64) []int64 {
	if fn == nil || blockID <= 0 {
		return nil
	}
	blockVal, ok := fn.GetValueById(blockID)
	if !ok || blockVal == nil {
		return nil
	}
	block, ok := ssa.ToBasicBlock(blockVal)
	if !ok || block == nil {
		return nil
	}
	return append([]int64{}, block.Preds...)
}

func cloneIDSet(in map[int64]struct{}) map[int64]struct{} {
	out := make(map[int64]struct{}, len(in))
	for id := range in {
		out[id] = struct{}{}
	}
	return out
}

func intersectIDSets(left, right map[int64]struct{}) map[int64]struct{} {
	out := make(map[int64]struct{}, len(left))
	for id := range left {
		if _, ok := right[id]; ok {
			out[id] = struct{}{}
		}
	}
	return out
}

func sameIDSet(left, right map[int64]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for id := range left {
		if _, ok := right[id]; !ok {
			return false
		}
	}
	return true
}

func (c *Compiler) withInstructionInsertPoint(inst ssa.Instruction, fn func() error) error {
	if c == nil || inst == nil || inst.GetBlock() == nil || fn == nil {
		return nil
	}
	targetBB, ok := c.Blocks[inst.GetBlock().GetId()]
	if !ok || targetBB.IsNil() {
		return fn()
	}
	restoreBB := c.restoreInsertBlock(nil)
	prevActive := int64(0)
	if c.function != nil {
		prevActive = c.function.activeBlockID
		c.function.activeBlockID = inst.GetBlock().GetId()
	}
	c.setInsertPointBeforeTerminator(targetBB)
	err := fn()
	if !restoreBB.IsNil() {
		c.restoreInsertPoint(restoreBB)
	}
	if c.function != nil {
		c.function.activeBlockID = prevActive
	}
	return err
}

func (c *Compiler) sameMemberTarget(val ssa.Value, obj ssa.Value, keyStr string) bool {
	if val == nil || obj == nil || keyStr == "" || !val.IsMember() || val.GetObject() == nil {
		return false
	}
	return val.GetObject().GetId() == obj.GetId() && c.resolveMemberKeyString(val.GetKey()) == keyStr
}

func (c *Compiler) valueForObjectMemberAssignment(contextInst ssa.Instruction, member ssa.Value) (llvm.Value, error) {
	if member == nil || !member.IsMember() || member.GetObject() == nil || member.GetKey() == nil {
		return c.getValue(contextInst, member.GetId())
	}
	objVal, err := c.getValue(contextInst, member.GetObject().GetId())
	if err != nil {
		return llvm.Value{}, err
	}
	keyStr := c.resolveMemberKeyString(member.GetKey())
	if keyStr == "" {
		return c.getValue(contextInst, member.GetId())
	}
	return c.emitRuntimeGetField(objVal, keyStr, member.GetId()), nil
}

func (c *Compiler) emitInitialMakeMemberAssignments(inst *ssa.Make, objVal llvm.Value) error {
	if inst == nil || objVal.IsNil() {
		return nil
	}

	seen := make(map[string]struct{})
	var emitErr error
	inst.ForEachMember(func(key, member ssa.Value) bool {
		if key == nil || member == nil || member.GetId() <= 0 || member.GetId() == inst.GetId() {
			return true
		}
		keyStr := c.resolveMemberKeyString(key)
		if keyStr == "" {
			return true
		}
		if _, ok := seen[keyStr]; ok {
			return true
		}
		seen[keyStr] = struct{}{}
		c.markInitialMemberValue(member.GetId())
		var llvmVal llvm.Value
		err := c.withInitializingMemberValue(member.GetId(), func() error {
			var err error
			llvmVal, err = c.valueForInitialMakeMemberAssignment(inst, member, inst, keyStr)
			return err
		})
		if err != nil {
			emitErr = fmt.Errorf("emitInitialMakeMemberAssignments: field %q: %w", keyStr, err)
			return false
		}
		c.emitRuntimeSetField(objVal, keyStr, llvmVal, member, member.GetId())
		if err := c.maybeEmitMemberSet(inst, member, member.GetId()); err != nil {
			emitErr = fmt.Errorf("emitInitialMakeMemberAssignments: field %q member variables: %w", keyStr, err)
			return false
		}
		return true
	})
	return emitErr
}

func (c *Compiler) valueForInitialMakeMemberAssignment(contextInst ssa.Instruction, member ssa.Value, owner ssa.Value, keyStr string) (llvm.Value, error) {
	if c.shouldReadMemberValueForInitialMakeMember(member, owner, keyStr) {
		return c.dynamicMemberReadValue(contextInst, member, member.GetId())
	}
	return c.getValue(contextInst, member.GetId())
}

func (c *Compiler) shouldReadMemberValueForInitialMakeMember(member ssa.Value, owner ssa.Value, keyStr string) bool {
	if member == nil || !member.IsMember() || member.GetObject() == nil || member.GetKey() == nil {
		return false
	}
	if c.sameMemberTarget(member, owner, keyStr) {
		return false
	}
	if current := c.currentFunction(); current != nil && member.GetFunc() != nil && member.GetFunc() != current {
		return false
	}
	switch v := member.(type) {
	case *ssa.Parameter, *ssa.ParameterMember, *ssa.SideEffect:
		return false
	case *ssa.Undefined:
		return !v.IsExtern()
	}
	return true
}

func (c *Compiler) emitObjectMemberAssignments(contextInst ssa.Instruction, obj ssa.Value, objVal llvm.Value) error {
	if obj == nil || objVal.IsNil() {
		return nil
	}

	var emitErr error
	obj.ForEachMember(func(key, member ssa.Value) bool {
		if key == nil || member == nil {
			return true
		}
		keyStr := c.resolveMemberKeyString(key)
		if keyStr == "" {
			return true
		}
		llvmVal, err := c.valueForObjectMemberAssignment(contextInst, member)
		if err != nil {
			emitErr = fmt.Errorf("emitObjectMemberAssignments: field %q: %w", keyStr, err)
			return false
		}
		c.emitRuntimeSetField(objVal, keyStr, llvmVal, member, obj.GetId())
		return true
	})
	return emitErr
}
