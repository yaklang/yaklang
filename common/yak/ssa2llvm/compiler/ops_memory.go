package compiler

import (
	"fmt"
	"strconv"
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
	case ssa.StringTypeKind:
		// String slicing (s[low:high]) carries the parent string and rune
		// bounds; plain string constants never go through Make.
		return c.compileMakeStringSlice(inst)
	case ssa.MapTypeKind:
		return c.compileMakeGeneric(inst)
	case ssa.ChanTypeKind:
		return c.compileMakeChan(inst)
	default:
		// For unhandled types, create a null/zero placeholder
		c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
		return nil
	}
}

// compileMakeStringSlice lowers s[low:high] where s is a yak string. Bounds
// are rune indices (len(str) == len([]rune(str))), so slicing must operate on
// runes, not bytes.
func (c *Compiler) compileMakeStringSlice(inst *ssa.Make) error {
	i64 := c.LLVMCtx.Int64Type()
	parentVal := llvm.ConstInt(i64, 0, false)
	if parentID := inst.GetParent(); parentID > 0 {
		val, err := c.getValue(inst, parentID)
		if err != nil {
			return err
		}
		parentVal = c.coerceToInt64(val)
	}
	low := llvm.ConstInt(i64, 0, false)
	if lowID := inst.GetLow(); lowID > 0 {
		val, err := c.getValue(inst, lowID)
		if err != nil {
			return err
		}
		low = c.coerceToInt64(val)
	}
	high := llvm.ConstInt(i64, ^uint64(0), false) // -1: slice to end
	if highID := inst.GetHigh(); highID > 0 {
		val, err := c.getValue(inst, highID)
		if err != nil {
			return err
		}
		high = c.coerceToInt64(val)
	}
	fn, fnType := c.getOrInsertRuntimeStringSlice()
	val := c.Builder.CreateCall(fnType, fn, []llvm.Value{parentVal, low, high}, fmt.Sprintf("string_slice_%d", inst.GetId()))
	val = c.coerceToInt64(val)
	c.cacheValue(inst.GetId(), val)
	return nil
}

func (c *Compiler) getOrInsertRuntimeStringSlice() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeStringSliceSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) compileMakeChan(inst *ssa.Make) error {
	i64 := c.LLVMCtx.Int64Type()
	size := llvm.ConstInt(i64, 0, false)
	if inst.Len > 0 {
		val, err := c.getValue(inst, inst.Len)
		if err != nil {
			return err
		}
		size = c.coerceToInt64(val)
	}
	spec := contextCallSpec{
		inst: inst,
		kind: abi.KindDispatch,
		target: llvm.ConstInt(
			i64,
			uint64(abi.IDRuntimeMakeChan),
			false,
		),
		args: []contextCallArg{
			{value: size},
		},
		ctxName:   "yak_make_chan_ctx",
		errPrefix: "emitRuntimeMakeChan",
	}
	result, err := c.emitContextCall(spec)
	if err != nil {
		return err
	}
	c.cacheValue(inst.GetId(), c.coerceToInt64(result))
	return nil
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

func (c *Compiler) getOrInsertRuntimeSliceSlice() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeSliceSliceSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64, i64}, false)
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
	for _, pair := range ssa.GetMemberPairs(inst) {
		key, member := pair.Key, pair.Member
		if key == nil || member == nil || member.GetId() <= 0 || member.GetId() == inst.GetId() {
			continue
		}
		// Reads merged into the Make's pair list (e.g. a[-1] later in the
		// program) carry an Undefined member placeholder; they must not
		// inflate the initial length. Loop-carried member phis are reads too.
		if u, isUndef := member.(*ssa.Undefined); isUndef && u != nil {
			continue
		}
		if u, isUndef := key.(*ssa.Undefined); isUndef && u != nil {
			continue
		}
		if _, isPhi := member.(*ssa.Phi); isPhi {
			continue
		}
		if _, isPhi := key.(*ssa.Phi); isPhi {
			continue
		}
		count++
	}
	return count
}

func (c *Compiler) compileMakeSlice(inst *ssa.Make, typ ssa.Type) error {
	i64 := c.LLVMCtx.Int64Type()
	// Slice-of-slice (a[low:high] on a yak slice): copy the selected window
	// through the runtime, mirroring compileMakeStringSlice for strings.
	// Unspecified bounds are -1 so the runtime can apply the correct defaults
	// for positive and negative (reverse) steps.
	if parentID := inst.GetParent(); parentID > 0 {
		parentVal := llvm.ConstInt(i64, 0, false)
		if val, err := c.getValue(inst, parentID); err == nil {
			parentVal = c.coerceToInt64(val)
		}
		low := llvm.ConstInt(i64, ^uint64(0), false) // -1: unspecified
		if lowID := inst.GetLow(); lowID > 0 {
			if val, err := c.getValue(inst, lowID); err == nil {
				low = c.coerceToInt64(val)
			}
		}
		high := llvm.ConstInt(i64, ^uint64(0), false) // -1: slice to end
		if highID := inst.GetHigh(); highID > 0 {
			if val, err := c.getValue(inst, highID); err == nil {
				high = c.coerceToInt64(val)
			}
		}
		step := llvm.ConstInt(i64, 1, false)
		if maxID := inst.GetStep(); maxID > 0 {
			if val, err := c.getValue(inst, maxID); err == nil {
				step = c.coerceToInt64(val)
			}
		}
		fn, fnType := c.getOrInsertRuntimeSliceSlice()
		val := c.Builder.CreateCall(fnType, fn, []llvm.Value{parentVal, low, high, step}, fmt.Sprintf("slice_slice_%d", inst.GetId()))
		c.cacheValue(inst.GetId(), c.coerceToInt64(val))
		return nil
	}
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

	parentVal, err := c.getValue(inst, parentID)
	if err != nil {
		return fmt.Errorf("parent value %d for ParameterMember %s: %w", parentID, inst.GetName(), err)
	}

	val := c.emitRuntimeGetFieldByKey(parentVal, keyVal, inst, inst.GetId())
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
	if val != nil && val.GetFunc() != nil && c.currentFunction() != nil && val.GetFunc() != c.currentFunction() {
		if !c.valueHasLocalDependency(val.GetFunc(), val) {
			return nil
		}
	}
	obj := ssa.GetLatestObject(val)
	key := ssa.GetLatestKey(val)
	if nameObj := c.memberObjectFromValueName(val); nameObj != nil && (obj == nil || nameObj.GetId() != obj.GetId()) {
		// The front end can re-parent a member value (e.g. result["kind"] =
		// params.info.kind) without renaming it; trust the name's object id so
		// params.info.kind still reads through params.info instead of result.
		obj = nameObj
	}
	keyStr := c.resolveMemberKeyString(key)

	if obj != nil {
		// A closure free-value parameter whose default is an extern lib (e.g.
		// tcp captured by a go func) must be resolved to the extern lib at
		// compile time; the runtime represents extern libs as 0 and cannot
		// dispatch methods on them.
		if param, ok := ssa.ToParameter(obj); ok && param != nil && param.GetDefault() != nil {
			if extern, ok := ssa.ToExternLib(param.GetDefault()); ok && extern != nil {
				obj = extern
			}
		}
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
	emitMemberRead := func() error {
		if !c.isSSAValueStored(obj.GetId()) && !c.hasValueSlot(obj.GetId()) {
			if _, err := c.getValue(loadCtx, obj.GetId()); err != nil {
				return fmt.Errorf("compileMemberCall: failed to get object value: %w", err)
			}
		}
		slot := c.ensureValueSlot(memberID)
		if slot.IsNil() {
			return fmt.Errorf("compileMemberCall: slot for value %d unavailable", memberID)
		}
		parentVal := c.loadSSAValue(obj.GetId())
		valResult := c.emitRuntimeGetFieldByKey(parentVal, key, loadCtx, memberID)
		c.Builder.CreateStore(c.coerceToInt64(valResult), slot)
		c.markSSAValueStored(memberID)
		return nil
	}
	var emitErr error
	// Member reads are emitted at the use site rather than anchored to the
	// value's SSA def block. Objects shared across functions (e.g. a global
	// params map mutated by callees) must be re-read where they are used;
	// anchoring to the def block would cache a stale (often entry-time) value.
	emitErr = emitMemberRead()
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
	c.storeSSAValue(val.GetId(), valResult)
	return c.maybeEmitMemberSet(contextInst, val, val.GetId())
}

func (c *Compiler) dynamicMemberReadValue(contextInst ssa.Instruction, val ssa.Value, memberID int64) (llvm.Value, error) {
	obj, key := c.firstOwnerObjectKey(val)
	if nameObj := c.memberObjectFromValueName(val); nameObj != nil && (obj == nil || nameObj.GetId() != obj.GetId()) {
		obj = nameObj
	}
	keyStr := c.resolveRuntimeMemberKeyString(key)
	if obj == nil || keyStr == "" {
		return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false), nil
	}

	parentVal, err := c.valueForMemberObject(contextInst, obj)
	if err != nil {
		return llvm.Value{}, fmt.Errorf("dynamicMemberReadValue: failed to get object value: %w", err)
	}
	return c.emitRuntimeGetFieldByKey(parentVal, key, contextInst, memberID), nil
}

// firstOwnerObjectKey returns the first (definition) owner pair of a member
// value. GetObject()/GetKey() return the latest owner, which for a value
// reused by a later assignment is the assignment target, not the object the
// member was originally read from.
func (c *Compiler) firstOwnerObjectKey(val ssa.Value) (ssa.Value, ssa.Value) {
	if val == nil {
		return nil, nil
	}
	pairs := val.GetObjectKeyPairs()
	if len(pairs) == 0 {
		return nil, nil
	}
	return pairs[0].Object, pairs[0].Key
}

func (c *Compiler) valueForMemberObject(contextInst ssa.Instruction, obj ssa.Value) (llvm.Value, error) {
	if obj != nil && c.hasValueSlot(obj.GetId()) {
		return c.loadSSAValue(obj.GetId()), nil
	}
	return c.getValue(contextInst, obj.GetId())
}

func (c *Compiler) compileUndefined(inst *ssa.Undefined) error {
	if inst == nil {
		return nil
	}
	// Extern value placeholders (e.g. VULINBOX injected from the compile
	// environment via WithExternValue) carry their real value in
	// Program.ExternInstance; compile them as constants instead of zero.
	if inst.IsExtern() && inst.GetFunc() != nil {
		if prog := inst.GetFunc().GetProgram(); prog != nil && prog.ExternInstance != nil {
			if v, ok := prog.ExternInstance[inst.GetName()]; ok {
				if s, ok := v.(string); ok {
					ptr := c.Builder.CreateGlobalStringPtr(s, fmt.Sprintf("extern_str_%d", inst.GetId()))
					c.cacheValue(inst.GetId(), llvm.ConstPtrToInt(ptr, c.LLVMCtx.Int64Type()))
					return nil
				}
			}
		}
	}
	if inst.GetFunc() != nil && c.currentFunction() != nil && inst.GetFunc() != c.currentFunction() {
		if !c.valueHasLocalDependency(inst.GetFunc(), inst) {
			return nil
		}
	}
	if !inst.IsMember() || !c.hasAssignedMemberCallVariable(inst) {
		if c.shouldReadMemberValueDynamically(inst, inst.GetId()) {
			return nil
		}
		// A range-loop variable is an Undefined placeholder whose body
		// references never see the condition block's binding. Resolve the
		// variable's current value through the scope chain (e.g. a
		// "#<id>.k" member variable whose name part matches "k" in the loop
		// condition scope) so m[k] uses the per-iteration field value.
		if resolved := c.resolveUndefinedScopeValue(inst); resolved != nil {
			val, err := c.getValue(inst, resolved.GetId())
			if err == nil {
				c.cacheValue(inst.GetId(), val)
				return c.maybeEmitMemberSet(inst, inst, inst.GetId())
			}
		}
		return nil
	}
	if c.shouldReadMemberValueDynamically(inst, inst.GetId()) {
		return c.compileDynamicMemberValue(inst, inst)
	}
	return c.compileMemberCall(inst, inst, inst)
}

func (c *Compiler) resolveUndefinedScopeValue(inst *ssa.Undefined) ssa.Value {
	if inst == nil || inst.GetBlock() == nil || inst.GetBlock().ScopeTable == nil {
		return nil
	}
	scope := inst.GetBlock().ScopeTable
	for name := range inst.GetAllVariables() {
		base := name
		if idx := strings.LastIndex(base, "."); idx >= 0 {
			base = base[idx+1:]
		}
		if base == "" {
			continue
		}
		if variable := ssa.ReadVariableFromScopeAndParent(scope, base); variable != nil {
			if value := variable.GetValue(); value != nil && value.GetId() != inst.GetId() {
				return value
			}
		}
	}
	return nil
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
	i64 := c.LLVMCtx.Int64Type()
	// The runtime export takes uintptr (not unsafe.Pointer): integer member
	// keys (m[0], m[i]) must be able to reach the conversion without cgo's
	// unpinned-pointer check rejecting small raw words.
	fnType := llvm.FunctionType(i8Ptr, []llvm.Type{i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

// resolveRuntimeMemberKeyString resolves a member key for a runtime field
// read. Next-tuple members are placeholder constants whose SSA name is
// "#<id>.key/field/ok"; the runtime tuple's actual keys are the suffix.
func (c *Compiler) resolveRuntimeMemberKeyString(key ssa.Value) string {
	if key == nil {
		return ""
	}
	name := strings.Trim(key.GetName(), "\"")
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		suffix := name[idx+1:]
		switch suffix {
		case "key", "field", "ok":
			return suffix
		}
	}
	return c.resolveMemberKeyString(key)
}

func (c *Compiler) resolveMemberKeyString(key ssa.Value) string {
	if key == nil {
		return ""
	}
	if cinst, ok := ssa.ToConstInst(key); ok {
		return strings.Trim(cinst.String(), "\"")
	}
	if name := strings.Trim(key.GetName(), "\""); name != "" {
		return name
	}
	if key.GetId() > 0 {
		return fmt.Sprintf("#%d", key.GetId())
	}
	return ""
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

func (c *Compiler) emitRuntimeDropError(ret llvm.Value) llvm.Value {
	fn, fnType := c.getOrInsertRuntimeDropError()
	objPtr := c.coerceToI8Ptr(ret)
	return c.Builder.CreateCall(fnType, fn, []llvm.Value{objPtr}, "drop_error")
}

func (c *Compiler) getOrInsertRuntimeDropError() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeDropErrorSymbol)
	fn := c.Mod.NamedFunction(name)
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.Int64Type(), []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) emitRuntimeGetField(objVal llvm.Value, keyStr string, id int64) llvm.Value {
	fn, fnType := c.getOrInsertRuntimeGetField()
	keyPtr := c.Builder.CreateGlobalStringPtr(keyStr, fmt.Sprintf("member_key_%d", id))
	objPtr := c.coerceToI8Ptr(objVal)
	return c.Builder.CreateCall(fnType, fn, []llvm.Value{objPtr, keyPtr}, "member_get")
}

// memberKeyIsStaticString reports whether an SSA member key is a string
// constant (so it can be lowered to a static member_key global).
func (c *Compiler) memberKeyIsStaticString(key ssa.Value) bool {
	if key == nil {
		return false
	}
	_, ok := ssa.ToConstInst(key)
	return ok
}

// memberKeyIsStringConst reports whether the member key is a string constant
// (a method name), as opposed to a numeric index or a dynamic key.
func (c *Compiler) memberKeyIsStringConst(key ssa.Value) bool {
	if key == nil {
		return false
	}
	ci, ok := ssa.ToConstInst(key)
	if !ok {
		return false
	}
	return ci.IsString()
}

// emitRuntimeGetFieldByKey lowers a member read for a possibly dynamic key
// (e.g. m[k] where k is a loop variable). Static string keys use the existing
// global-string path; dynamic keys are converted to a C string at runtime.
func (c *Compiler) emitRuntimeGetFieldByKey(objVal llvm.Value, key ssa.Value, contextInst ssa.Instruction, id int64) llvm.Value {
	if c.memberKeyIsStaticString(key) {
		return c.emitRuntimeGetField(objVal, c.resolveMemberKeyString(key), id)
	}
	if keyVal, err := c.getValue(contextInst, key.GetId()); err == nil && !keyVal.IsNil() && !c.isZeroRuntimeKey(keyVal) {
		toCStrFn, toCStrType := c.getOrInsertRuntimeToCString()
		keyPtr := c.Builder.CreateCall(toCStrType, toCStrFn, []llvm.Value{c.coerceToInt64(keyVal)}, fmt.Sprintf("member_key_ptr_%d", id))
		fn, fnType := c.getOrInsertRuntimeGetField()
		return c.Builder.CreateCall(fnType, fn, []llvm.Value{c.coerceToI8Ptr(objVal), keyPtr}, "member_get")
	}
	// A dynamic key that cannot be resolved at the current use site (e.g. a
	// cross-function front-end artifact) yields zero instead of reading the
	// placeholder SSA name as a real key.
	return llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
}

func (c *Compiler) emitRuntimeSetField(objVal llvm.Value, keyStr string, val llvm.Value, ssaVal ssa.Value, id int64) {

	fn, fnType := c.getOrInsertRuntimeSetField()
	keyPtr := c.Builder.CreateGlobalStringPtr(keyStr, fmt.Sprintf("member_key_%d", id))
	objPtr := c.coerceToI8Ptr(objVal)
	intVal := c.coerceToInt64(val)
	flags := uint64(0)
	ssaVal = c.effectiveRuntimeFieldValue(ssaVal)
	if ssaVal != nil && c.ssaValueIsFunction(ssaVal) {
		// A function value stored in a container must be a materialized
		// closure object (shadow handle), not a raw function pointer: the
		// runtime decodes untagged addresses above 1 MiB as C strings, and a
		// later call through the container would either crash or call the
		// wrong target. Materialize the closure so free values travel with it.
		if fn := ssaVal.GetFunc(); fn != nil {
			if ssaFn, ok := c.functionValueForArg(fn, ssaVal.GetId()); ok && ssaFn != nil && !ssaFn.IsExtern() {
				if closure, err := c.materializeCallableClosure(nil, ssaFn); err == nil {
					intVal = c.coerceToInt64(closure)
				}
			}
		}
	} else if ssaVal != nil && c.ssaValueIsPointer(ssaVal, ssaVal.GetFunc()) {
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

func (c *Compiler) getOrInsertRuntimeConcat() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeConcatSymbol)
	fn := c.Mod.NamedFunction(name)
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(i8Ptr, []llvm.Type{i8Ptr, i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) emitRuntimeConcat(lhs, rhs llvm.Value, name string) llvm.Value {
	fn, fnType := c.getOrInsertRuntimeConcat()
	res := c.Builder.CreateCall(fnType, fn, []llvm.Value{c.coerceToI8Ptr(lhs), c.coerceToI8Ptr(rhs)}, name)
	return c.coerceToInt64(res)
}

// emitRuntimeSetFieldByKey lowers a member write for a possibly dynamic key
// (e.g. extToLanguage[ext] where ext is a loop variable). Static keys use the
// global-string path; dynamic keys are converted to a C string at runtime so
// the placeholder SSA name (e.g. "#55.field") is never written as the key.
func (c *Compiler) emitRuntimeSetFieldByKey(contextInst ssa.Instruction, objVal llvm.Value, key ssa.Value, keyStr string, val llvm.Value, ssaVal ssa.Value, id int64) {
	if key != nil && !c.memberKeyIsStaticString(key) {
		if keyVal, err := c.getValue(contextInst, key.GetId()); err == nil && !keyVal.IsNil() && !c.isZeroRuntimeKey(keyVal) {
			toCStrFn, toCStrType := c.getOrInsertRuntimeToCString()
			keyPtr := c.Builder.CreateCall(toCStrType, toCStrFn, []llvm.Value{c.coerceToInt64(keyVal)}, fmt.Sprintf("member_key_ptr_set_%d", id))
			fn, fnType := c.getOrInsertRuntimeSetField()
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
			c.Builder.CreateCall(fnType, fn, []llvm.Value{c.coerceToI8Ptr(objVal), keyPtr, intVal, flagVal}, "")
			return
		}
	}
	if !c.memberKeyIsStaticString(key) {
		// Dynamic key that cannot be resolved: skip the write rather than
		// storing under the placeholder SSA name.
		return
	}
	c.emitRuntimeSetField(objVal, keyStr, val, ssaVal, id)
}

// isZeroRuntimeKey reports whether an SSA member key lowered to a runtime
// value is a compile-time zero. Cross-function references (front-end artifacts)
// resolve to zero constants; passing those to yak_runtime_to_cstring would
// produce a nil C string and a misleading runtime failure.
func (c *Compiler) isZeroRuntimeKey(keyVal llvm.Value) bool {
	if keyVal.IsNil() {
		return true
	}
	return llvmIsZeroValue(keyVal)
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
	c.initializingMemberDepth++
	defer func() {
		c.initializingMemberValueIDs[id]--
		if c.initializingMemberValueIDs[id] <= 0 {
			delete(c.initializingMemberValueIDs, id)
		}
		c.initializingMemberDepth--
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
	if val.IsMember() {
		switch val.(type) {
		case *ssa.ParameterMember, *ssa.Undefined, *ssa.Phi:
			// Phis are loop-carried member *reads* merged from side effects;
			// writing the phi back would overwrite the freshly assigned value
			// with the pre-loop value.
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
					c.emitRuntimeSetFieldByKey(contextInst, objVal, key, keyStr, llvmVal, c.assignedSSAValue(contextInst, resultID), val.GetId())
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
	for _, pair := range ssa.GetMemberPairs(val.GetObject()) {
		key, member := pair.Key, pair.Member
		if key == nil || member == nil || member.GetId() == currentID {
			continue
		}
		if c.resolveMemberKeyString(key) == keyStr && member.GetId() > currentID {
			if c.memberValueOverridesInSameBlock(val, member) {
				skip = true
				break
			}
		}
		continue
	}
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
	if _, isConst := val.(*ssa.ConstInst); isConst {
		return false
	}
	if current := c.currentFunction(); current != nil && val.GetFunc() != nil && val.GetFunc() != current {
		return false
	}

	// A function value stored as an object member is a closure/function pointer,
	// not a dynamic field read: materialize it directly instead of reading the
	// member back through the object.
	if ssaFn, ok := ssa.ToFunction(val); ok && ssaFn != nil {
		return false
	}
	// A value whose latest owner object lives in another function is a shared
	// definition (e.g. a constant reused by two object literals), not a member
	// read of the current function's object. Reading it through that foreign
	// object would compile a call into the other function (infinite recursion
	// for self-referential factories).
	if current := c.currentFunction(); current != nil {
		if obj := val.GetObject(); obj != nil && obj.GetFunc() != nil && obj.GetFunc() != current {
			return false
		}
	}

	if current := c.currentFunction(); current != nil {
		if obj, _ := c.firstOwnerObjectKey(val); obj != nil && obj.GetFunc() != nil && obj.GetFunc() != current {
			return false
		}
	}
	switch val.(type) {
	case *ssa.Parameter, *ssa.ParameterMember, *ssa.SideEffect, *ssa.Phi:
		return false
	case *ssa.Undefined:
		obj, key := c.firstOwnerObjectKey(val)
		return !val.IsExtern() && obj != nil && key != nil
	}
	obj, key := c.firstOwnerObjectKey(val)
	return obj != nil && key != nil
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
		// Loop-carried phis are member *reads* merged from side effects; their
		// member-call variables must not be re-written at every merge point.
		if _, isPhi := val.(*ssa.Phi); isPhi {
			continue
		}
		obj, key := variable.GetMemberCall()
		if obj == nil || key == nil {
			continue
		}
		if !c.memberAssignmentValueExists(contextInst, obj) {
			continue
		}
		keyStr := c.resolveMemberKeyString(key)
		if keyStr == "" {
			continue
		}
		// An Undefined value with member-call variables is a write placeholder
		// (e.g. a computed value assigned to captured members): its variables
		// are the writes, and the direct maybeEmitMemberSet path skips Undefined,
		// so sameMemberTarget must not suppress them.
		if _, isUndef := val.(*ssa.Undefined); !isUndef && c.sameMemberTarget(val, obj, keyStr) {
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
			c.emitRuntimeSetFieldByKey(contextInst, currentObjVal, key, keyStr, llvmVal, c.assignedSSAValue(contextInst, resultID), resultID)
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
	c.emitRuntimeSetFieldByKey(contextInst, objVal, source.GetKey(), keyStr, llvmVal, source, resultID)
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
	// Loop-carried member phis are reads merged by the front end, not writes
	// to queue: flushing them writes stale/uninitialized keys into the map.
	if phi, ok := source.(*ssa.Phi); ok && phi != nil && c.phiMemberIsLoopCarriedMapRead(phi) {
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
	c.emitRuntimeSetFieldByKey(contextInst, objVal, source.GetKey(), keyStr, llvmVal, c.assignedSSAValue(contextInst, resultID), resultID)
	c.markMemberVariableSetEmitted(resultID, obj, keyStr)
	return true
}

func (c *Compiler) valueForMemberSet(contextInst ssa.Instruction, source ssa.Value, resultID int64, dynamicMemberRead bool) (llvm.Value, error) {
	if source != nil {
		if _, isConst := source.(*ssa.ConstInst); isConst {
			return c.finishGetValue(contextInst, resultID)
		}
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
		!c.isInitializingMemberValue(source.GetId()) &&
		c.initialMemberValueObjectInCurrentFunction(source) &&
		!c.initialMemberValueIsFunction(source)
}

// A function value stored as an object member is a closure/function pointer;
// syncing its member variables must not re-read it through the object.
func (c *Compiler) initialMemberValueIsFunction(source ssa.Value) bool {
	if source == nil {
		return false
	}
	_, ok := ssa.ToFunction(source)
	return ok
}

// A shared initial member value whose latest owner object lives in another
// function must not be re-read through that foreign object: the value was
// already materialized at its definition site (e.g. a constant reused by two
// object literals), and reading it back would compile a call into the other
// function.
func (c *Compiler) initialMemberValueObjectInCurrentFunction(source ssa.Value) bool {
	if source == nil || source.GetObject() == nil {
		return true
	}
	current := c.currentFunction()
	if current == nil {
		return true
	}
	objFn := source.GetObject().GetFunc()
	return objFn == nil || objFn == current
}

func (c *Compiler) memberSourceForMemberSetDynamicRead(source ssa.Value) ssa.Value {
	if source == nil {
		return nil
	}
	if _, isConst := source.(*ssa.ConstInst); isConst {
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

func (c *Compiler) emitInitialMakeMemberAssignments(inst *ssa.Make, objVal llvm.Value) error {
	if inst == nil || objVal.IsNil() {
		return nil
	}

	seen := make(map[string]struct{})
	makeBB := c.restoreInsertBlock(inst)
	makeIndex := -1
	if inst.GetBlock() != nil {
		for i, id := range inst.GetBlock().Insts {
			if id == inst.GetId() {
				makeIndex = i
				break
			}
		}
	}
	var emitErr error
	for _, pair := range ssa.GetMemberPairs(inst) {
		key, member := pair.Key, pair.Member
		if key == nil || member == nil || member.GetId() <= 0 || member.GetId() == inst.GetId() {
			continue
		}
		// Undefined member placeholders (e.g. m[i] or m[0] later in the
		// program) are resolved at their real use site, not during the Make.
		if u, isUndef := member.(*ssa.Undefined); isUndef && u != nil {
			continue
		}
		if u, isUndef := key.(*ssa.Undefined); isUndef && u != nil {
			continue
		}
		// Loop-carried member phis are reads merged by the front end, not
		// literal members to initialize here.
		if _, isPhi := member.(*ssa.Phi); isPhi {
			continue
		}
		if _, isPhi := key.(*ssa.Phi); isPhi {
			continue
		}
		memberInst, memberIsInst := member.(ssa.Instruction)
		if memberIsInst && memberInst.GetBlock() != nil && inst.GetBlock() != nil {
			if memberInst.GetBlock().GetId() != inst.GetBlock().GetId() {
				continue
			}
			if makeIndex >= 0 {
				if idx := instructionIndex(memberInst.GetBlock(), memberInst.GetId()); idx > makeIndex {
					continue
				}
			}
		}
		if keyInst, keyIsInst := key.(ssa.Instruction); keyIsInst && keyInst.GetBlock() != nil && inst.GetBlock() != nil {
			if keyInst.GetBlock().GetId() != inst.GetBlock().GetId() {
				continue
			}
			// Constant literal keys (slice/array indices) are always valid
			// initial members even when they appear after the Make.
			if _, isConst := key.(*ssa.ConstInst); !isConst && makeIndex >= 0 {
				if idx := instructionIndex(keyInst.GetBlock(), keyInst.GetId()); idx > makeIndex {
					continue
				}
			}
		}
		keyStr := c.resolveMemberKeyString(key)
		if keyStr == "" {
			continue
		}
		if _, ok := seen[keyStr]; ok {
			continue
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
			break
		}
		// Member value compilation can leave the Builder in another block (a
		// lazily compiled call or a nested read anchored at its own def point).
		// Force the set_field back to the Make's block so the value dominates
		// the write.
		if !makeBB.IsNil() {
			c.restoreInsertPoint(makeBB)
		}
		c.emitRuntimeSetFieldByKey(inst, objVal, key, keyStr, llvmVal, member, member.GetId())
		if err := c.maybeEmitMemberSet(inst, member, member.GetId()); err != nil {
			emitErr = fmt.Errorf("emitInitialMakeMemberAssignments: field %q member variables: %w", keyStr, err)
			break
		}
	}
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
	// Only skip the dynamic read when the member's *definition* (first owner
	// pair) is the object being created; a shared value whose first owner is a
	// different object must still be read dynamically at the Make's position
	// (and re-emitted there even if an earlier use cached it in another block).
	if obj, key := c.firstOwnerObjectKey(member); obj != nil && key != nil {
		if obj.GetId() == owner.GetId() && c.resolveMemberKeyString(key) == keyStr {
			return false
		}
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

// memberObjectFromValueName resolves the object id embedded in a member
// value's SSA name (e.g. "#1981.kind" -> value 1981). The front end can
// re-parent a member value to a different object/key without updating its
// name; when the name and the latest owner disagree, the name is the more
// reliable record of which object the read should go through.
func (c *Compiler) memberObjectFromValueName(val ssa.Value) ssa.Value {
	if val == nil || val.GetFunc() == nil {
		return nil
	}
	name := val.GetName()
	if len(name) < 3 || name[0] != '#' {
		return nil
	}
	dot := strings.IndexByte(name, '.')
	if dot <= 1 {
		return nil
	}
	id, err := strconv.ParseInt(name[1:dot], 10, 64)
	if err != nil {
		return nil
	}
	obj, ok := val.GetFunc().GetValueById(id)
	if !ok || obj == nil {
		return nil
	}
	return obj
}
