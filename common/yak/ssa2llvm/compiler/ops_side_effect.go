package compiler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

// closureMemberReadValue recovers a member read the front end folded to the
// literal written before a closure ran. `m = {"n": 0}; run(func(){ m.n++ });
// println(m.n)` lowers the read to the constant 0, while the closure's update
// is a side effect on the captured object. When such a side effect exists in
// this function, the read is that side effect.
func (c *Compiler) closureMemberReadValue(contextInst ssa.Instruction, valObj ssa.Value, id int64) llvm.Value {
	ci, ok := valObj.(*ssa.ConstInst)
	if !ok || ci == nil || c == nil || c.function == nil {
		return llvm.Value{}
	}
	fn := ci.GetFunc()
	if fn == nil {
		return llvm.Value{}
	}
	keyText := strings.Trim(ci.String(), "\"")
	if keyText == "" {
		return llvm.Value{}
	}
	ownerID := c.memberReadOwnerID(ci, keyText)
	if ownerID <= 0 {
		return llvm.Value{}
	}
	var matched *ssa.SideEffect
	for _, blockID := range fn.Blocks {
		blockVal, ok := fn.GetValueById(blockID)
		if !ok {
			continue
		}
		block, ok := ssa.ToBasicBlock(blockVal)
		if !ok || block == nil {
			continue
		}
		for _, instID := range block.Insts {
			instVal, _ := fn.GetValueById(instID)
			se, isSE := instVal.(*ssa.SideEffect)
			if !isSE || se == nil {
				continue
			}
			if !c.sideEffectMatchesMemberOwner(fn, se, ownerID) {
				continue
			}
			// The field record is named `#<free>.file_count`. Its verbose name is
			// `params.file_count`, so either suffix identifies this field without
			// accepting another field of the same object.
			if !sideEffectNamesField(se, keyText) {
				continue
			}
			matched = se
		}
	}
	if matched == nil {
		// No side effect was recorded: the closure was handed to a Go function
		// (filesys.onFileStat) that the front end cannot see call it. The update
		// still happened in the closure's cell; read the field there.
		return c.opaqueClosureMemberRead(contextInst, fn, keyText, ownerID)
	}
	if err := c.compileSideEffectValue(matched); err != nil {
		return llvm.Value{}
	}
	if val, ok := c.getCachedValue(contextInst, matched.GetId()); ok && !val.IsNil() {
		return val
	}
	if c.hasValueSlot(matched.GetId()) {
		return c.loadSSAValue(matched.GetId())
	}
	return llvm.Value{}
}

// opaqueClosureMemberRead reads a field from an object a nested closure
// captured and mutated, when the front end recorded no side effect because
// the closure was passed into a Go function. The closure's cell holds the
// object, so the field read sees the closure's writes.
func (c *Compiler) opaqueClosureMemberRead(contextInst ssa.Instruction, fn *ssa.Function, keyText string, ownerID int64) llvm.Value {
	if c == nil || fn == nil || keyText == "" {
		return llvm.Value{}
	}
	var closure *ssa.Function
	var index int
	fn.GetProgram().EachFunction(func(cand *ssa.Function) {
		if closure != nil || cand == nil || cand.GetParent() == nil || cand.GetParent().GetId() != fn.GetId() {
			return
		}
		for i, binding := range callframe.OrderedFreeValueBindings(cand) {
			hit := c.closureWritesMember(cand, binding, keyText, ownerID)
			if hit {
				closure = cand
				index = i
				return
			}
		}
	})
	if closure == nil {
		return llvm.Value{}
	}
	closureVal, err := c.materializeCallableClosure(contextInst, closure)
	if err != nil || closureVal.IsNil() {
		return llvm.Value{}
	}
	obj := c.readClosureFreeValueWord(closureVal, index, true, closure.GetId())
	return c.emitRuntimeGetField(obj, keyText, closure.GetId())
}

// readClosureFreeValueWord reads one captured word of a closure object. byRef
// dereferences a by-reference capture so the result is the captured value.
func (c *Compiler) readClosureFreeValueWord(closureVal llvm.Value, index int, byRef bool, id int64) llvm.Value {
	readFn, readType := c.getOrInsertRuntimeReadClosureFreeValue()
	idx := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(index), false)
	byRefWord := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	if byRef {
		byRefWord = llvm.ConstInt(c.LLVMCtx.Int64Type(), 1, false)
	}
	val := c.Builder.CreateCall(readType, readFn, []llvm.Value{
		c.coerceToInt64(closureVal), idx, byRefWord,
	}, fmt.Sprintf("yak_opaque_fv_%d", id))
	return c.coerceToInt64(val)
}

// closureWritesMember reports whether the closure writes keyText on the
// object bound by the free value (a `params.file_count++` against `params`).
func (c *Compiler) closureWritesMember(fn *ssa.Function, binding callframe.FreeValueBinding, keyText string, ownerID int64) bool {
	if fn == nil || binding.ValueID <= 0 || keyText == "" || binding.ValueID != ownerID {
		return false
	}
	want := fmt.Sprintf("#%d.%s", binding.ValueID, keyText)
	// The member the closure writes is a parameter member of the captured
	// object (`#20.file_count`). SideEffects stays empty when the closure is
	// only handed to a Go function.
	for _, id := range fn.ParameterMembers {
		pm, ok := fn.GetValueById(id)
		if !ok || pm == nil {
			continue
		}
		if pm.GetName() == want {
			return true
		}
	}
	return false
}

// memberSideEffectKey is the field a member side effect writes (`#18.n` ->
// `n`), or empty when the side effect is not a member write.
func (c *Compiler) memberReadOwnerID(ci *ssa.ConstInst, keyText string) int64 {
	if ci == nil || keyText == "" {
		return 0
	}
	for _, user := range ci.GetUsers() {
		member, ok := ssa.ToValue(user)
		if !ok || member == nil || !member.IsMember() || member.GetKey() == nil || member.GetKey().GetId() != ci.GetId() || member.GetObject() == nil {
			continue
		}
		if ssa.GetKeyString(member.GetObject()) != keyText {
			return member.GetObject().GetId()
		}
	}
	return 0
}

func (c *Compiler) sideEffectMatchesMemberOwner(fn *ssa.Function, se *ssa.SideEffect, ownerID int64) bool {
	if fn == nil || se == nil || ownerID <= 0 {
		return false
	}
	name := se.GetName()
	dot := strings.LastIndex(name, ".")
	if dot <= 1 || name[0] != '#' {
		return false
	}
	freeValueID, err := strconv.ParseInt(name[1:dot], 10, 64)
	if err != nil {
		return false
	}
	freeValue, ok := fn.GetValueById(freeValueID)
	if !ok || freeValue == nil {
		return false
	}
	parameter, ok := ssa.ToParameter(freeValue)
	return ok && parameter != nil && parameter.GetDefault() != nil && parameter.GetDefault().GetId() == ownerID
}

func sideEffectNamesField(se *ssa.SideEffect, keyText string) bool {
	if se == nil || keyText == "" {
		return false
	}
	for _, name := range []string{se.GetName(), se.GetVerboseName()} {
		if name == keyText || strings.HasSuffix(name, "."+keyText) {
			return true
		}
	}
	return false
}

func (c *Compiler) memberSideEffectKey(fn *ssa.Function, se *ssa.SideEffect) string {
	if se == nil {
		return ""
	}
	name := se.GetName()
	dot := strings.LastIndex(name, ".")
	if dot < 0 || dot+1 >= len(name) {
		return ""
	}
	return name[dot+1:]
}

// sideEffectForForeignValue finds the side effect in this function whose
// modified value is the given id, and returns that side effect's word. A
// closure's result (`m.n++`) is owned by the closure, so reading it from the
// caller has to go through the side effect the front end recorded.
func (c *Compiler) sideEffectForForeignValue(contextInst ssa.Instruction, fn *ssa.Function, id int64) llvm.Value {
	if c == nil || fn == nil || id <= 0 {
		return llvm.Value{}
	}
	var matched *ssa.SideEffect
	for _, blockID := range fn.Blocks {
		blockVal, ok := fn.GetValueById(blockID)
		if !ok {
			continue
		}
		block, ok := ssa.ToBasicBlock(blockVal)
		if !ok || block == nil {
			continue
		}
		for _, instID := range block.Insts {
			instVal, _ := fn.GetValueById(instID)
			se, isSE := instVal.(*ssa.SideEffect)
			if !isSE || se == nil || se.Value != id {
				continue
			}
			matched = se
		}
	}
	if matched == nil {
		return llvm.Value{}
	}
	if err := c.compileSideEffectValue(matched); err != nil {
		return llvm.Value{}
	}
	var word llvm.Value
	if val, ok := c.getCachedValue(contextInst, matched.GetId()); ok && !val.IsNil() {
		word = val
	} else if c.hasValueSlot(matched.GetId()) {
		word = c.loadSSAValue(matched.GetId())
	}
	if word.IsNil() {
		return llvm.Value{}
	}
	// A member side effect's cell holds the captured object (`m`); the closure
	// updated one field of it (`m.n`). Read that field back.
	if key := c.memberSideEffectKey(fn, matched); key != "" {
		return c.emitRuntimeGetField(word, key, matched.GetId())
	}
	return word
}

// memberCellFromSideEffectName parses a side-effect name of the form
// `#<objectID>.<field>` and resolves the object in the current function. It is
// the fallback for member writes the front end published as a captured side
// effect without owner pairs.
func (c *Compiler) memberCellFromSideEffectName(inst *ssa.SideEffect) (ssa.Value, string, bool) {
	if inst == nil || c == nil {
		return nil, "", false
	}
	fn := c.currentFunction()
	if fn == nil {
		return nil, "", false
	}
	name := inst.GetName()
	if len(name) < 4 || name[0] != '#' {
		return nil, "", false
	}
	dot := strings.LastIndex(name, ".")
	if dot <= 1 || dot+1 >= len(name) {
		return nil, "", false
	}
	field := name[dot+1:]
	objID, err := strconv.ParseInt(name[1:dot], 10, 64)
	if err != nil || objID <= 0 || objID == inst.GetId() {
		return nil, "", false
	}
	obj, ok := fn.GetValueById(objID)
	if !ok || obj == nil {
		return nil, "", false
	}
	// Only a field of an object this function owns can be materialized here.
	// A foreign object sends getValue through the cross-function side-effect
	// lookup and back into this function.
	if objFn := obj.GetFunc(); objFn == nil || objFn.GetId() != fn.GetId() {
		return nil, "", false
	}
	if !c.valueHasLocalDependency(fn, obj) {
		return nil, "", false
	}
	if _, isEffect := obj.(*ssa.SideEffect); isEffect {
		return nil, "", false
	}
	return obj, field, true
}

func (c *Compiler) compileSideEffectInstruction(inst *ssa.SideEffect) error {
	if c.isAsyncSideEffect(inst) {
		return nil
	}
	return c.compileSideEffectValue(inst)
}

func (c *Compiler) compileSideEffectValue(inst *ssa.SideEffect) error {
	if inst == nil {
		return nil
	}
	if c.sideEffectInProgress == nil {
		c.sideEffectInProgress = make(map[int64]struct{})
	}
	if _, busy := c.sideEffectInProgress[inst.GetId()]; busy {
		// Re-entered from getValue -> sideEffectForForeignValue while this
		// side effect is still being lowered. Its word is not available yet,
		// so stop the cycle the same way an unresolvable member key does.
		c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
		return nil
	}
	c.sideEffectInProgress[inst.GetId()] = struct{}{}
	defer delete(c.sideEffectInProgress, inst.GetId())
	// The closure object is stored once, immediately after make_callable.
	// Every later read resolves the free-value slot from that alloca.
	if c.function != nil && c.function.redirectedSlots != nil {
		if _, ok := c.function.redirectedSlots[inst.GetId()]; ok {
			return c.maybeEmitMemberSet(inst, inst, inst.GetId())
		}
	}
	if inst.IsMember() && inst.GetObject() != nil && inst.GetKey() != nil {
		// The side-effect writes at its definition site, which is the FIRST
		// owner pair. GetObject() returns the latest owner: when the written
		// value is later reused (e.g. params.language also feeds
		// result["language"]), that latest owner is a forward reference and
		// pulling it here would emit later calls before the write.
		obj, key := c.firstOwnerObjectKey(inst)
		if obj == nil || key == nil {
			obj, key = inst.GetObject(), inst.GetKey()
		}
		objVal, err := c.getValue(inst, obj.GetId())
		if err != nil {
			return err
		}
		keyStr := c.resolveMemberKeyString(key)
		if keyStr == "" {
			c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
			return nil
		}
		if actualID, err := c.resolveSideEffectActualID(inst); err == nil && actualID > 0 && actualID != inst.GetId() && valueBelongsToFunction(inst.GetFunc(), actualID) {
			actualVal, err := c.getValue(inst, actualID)
			if err == nil && !actualVal.IsNil() {
				c.cacheValue(inst.GetId(), actualVal)
				c.emitRuntimeSetFieldByKey(inst, objVal, key, keyStr, actualVal, c.assignedSSAValue(inst, actualID), inst.GetId())
				return c.maybeEmitMemberSet(inst, inst, inst.GetId())
			}
		}
		c.cacheValue(inst.GetId(), c.emitRuntimeGetFieldByKey(objVal, key, inst, inst.GetId()))
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	// A member owned by the closure (runtime = s.runtimeId) is not a field of
	// this function. Reading it here folds the foreign object to 0 and skips
	// the by-ref slot the callback actually writes.
	if actual := c.resolveSideEffectActualValue(inst); actual != nil && actual.IsMember() && actual.GetObject() != nil && actual.GetKey() != nil && valueBelongsToFunction(inst.GetFunc(), actual.GetId()) {
		objVal, err := c.getValue(inst, actual.GetObject().GetId())
		if err != nil {
			return err
		}
		keyStr := c.resolveMemberKeyString(actual.GetKey())
		if keyStr == "" {
			c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
			return nil
		}
		c.cacheValue(inst.GetId(), c.emitRuntimeGetFieldByKey(objVal, actual.GetKey(), inst, inst.GetId()))
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	// A side effect bound to an outer scope keeps the member cell in its name
	// (`#<objectID>.<field>`) but carries no owner pair. The written value lives
	// in the runtime object, so read the field from that object instead of
	// falling back to the cross-frame value (which folds to zero).
	if !inst.IsMember() {
		if obj, field, ok := c.memberCellFromSideEffectName(inst); ok {
			if objVal, objErr := c.getValue(inst, obj.GetId()); objErr == nil && !objVal.IsNil() {
				c.cacheValue(inst.GetId(), c.emitRuntimeGetField(objVal, field, inst.GetId()))
				return c.maybeEmitMemberSet(inst, inst, inst.GetId())
			}
		}
	}
	if val, ok := c.getCachedValue(inst, inst.GetId()); ok && !val.IsNil() {
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	actualID, err := c.resolveSideEffectActualID(inst)
	if err != nil {
		return err
	}

	// A side effect after a closure call (e.g. `f(); count` or
	// `retry(100, f); count` where f mutates the captured count) refers to a
	// value owned by the closure function. Resolve it through the closure's
	// free-value slots instead of the cross-function fallback (which returns 0).
	closureVal, closureIndex, closureByRef, closureOK := c.resolveClosureSideEffect(inst, actualID)
	if closureOK {
		if closureByRef {
			// Redirect this side-effect's storage to the closure's heap
			// free-value slot: reads at ANY later program point (including
			// after the closure is invoked indirectly through a wrapper such
			// as filesys.onFileStat -> Recursive) observe the final value.
			// The closure object is parked in an entry-block alloca so it
			// dominates every redirected use; the slot pointer itself is
			// resolved lazily at each use site.
			closureSlot := c.ensureRedirectedClosureAlloca(inst.GetId())
			if !closureSlot.IsNil() {
				c.storeInt64AfterDef(closureVal, closureSlot)
			}
			if c.function.redirectedSlots == nil {
				c.function.redirectedSlots = make(map[int64]redirectedSlotSource)
			}
			c.function.redirectedSlots[inst.GetId()] = redirectedSlotSource{
				closureSlot: closureSlot,
				index:       int64(closureIndex),
			}
			if c.function.storedValues == nil {
				c.function.storedValues = make(map[int64]struct{})
			}
			c.function.storedValues[inst.GetId()] = struct{}{}
			return c.maybeEmitMemberSet(inst, inst, inst.GetId())
		}
		readFn, readType := c.getOrInsertRuntimeReadClosureFreeValue()
		idx := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(closureIndex), false)
		byRefWord := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		val := c.Builder.CreateCall(readType, readFn, []llvm.Value{
			c.coerceToInt64(closureVal), idx, byRefWord,
		}, fmt.Sprintf("yak_closure_fv_read_%d", inst.GetId()))
		val = c.coerceToInt64(val)
		if inst.GetId() > 0 {
			c.cacheValue(inst.GetId(), val)
		}
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}

	actualVal, err := c.getValue(inst, actualID)
	if err != nil {
		return err
	}
	actualVal = c.coerceToInt64(actualVal)
	if inst.GetId() > 0 {
		c.cacheValue(inst.GetId(), actualVal)
	}

	return c.maybeEmitMemberSet(inst, inst, inst.GetId())
}

// resolveClosureSideEffect resolves a side-effect whose modified value belongs
// to a closure function called at the side-effect's call site (either directly
// as the call target or passed as a callback argument). It returns the
// materialized closure object, the free-value binding index for the captured
// variable, whether the capture is by-reference, and ok.
func (c *Compiler) resolveClosureSideEffect(inst *ssa.SideEffect, actualID int64) (llvm.Value, uint64, bool, bool) {
	if inst == nil || inst.CallSite <= 0 || actualID <= 0 || c.function == nil {
		return llvm.Value{}, 0, false, false
	}
	fn := inst.GetFunc()
	if fn == nil {
		return llvm.Value{}, 0, false, false
	}
	valObj, ok := fn.GetValueById(actualID)
	if !ok || valObj == nil {
		return llvm.Value{}, 0, false, false
	}
	closureFn := valObj.GetFunc()
	// An if/elif assignment inside the closure is a phi there. The front end
	// copies that phi into the caller (edges still point at the closure's
	// constants) and parks the copy on the entry block, which has no
	// predecessor to select GET vs POST. The value that actually ran is in
	// the closure's by-ref slot; use the closure that owns the edge values.
	if phi, ok := valObj.(*ssa.Phi); ok && phi != nil && (closureFn == nil || closureFn == fn) {
		for _, edgeID := range phi.Edge {
			edge, ok := fn.GetValueById(edgeID)
			if !ok || edge == nil {
				continue
			}
			owner := edge.GetFunc()
			if owner != nil && owner != fn {
				closureFn = owner
				break
			}
		}
	}
	if closureFn == nil || closureFn == fn {
		return llvm.Value{}, 0, false, false
	}
	// The modified value's variable name comes from the side-effect
	// instruction (e.g. `files` for `files = append(files, x)`), not from
	// the modified value's id: that id is the closure-local result, which
	// never equals the caller's free-value binding id. A later use such as
	// `"got %v" % [count]` also binds a member name (`#210[1]`) on the same
	// value. Map iteration would pick that member and miss the free value,
	// so the parent read falls through to 0. Prefer the name the side effect
	// was created with, then any binding that the closure actually captures.
	modifyName := c.sideEffectCaptureName(inst, closureFn)
	if modifyName == "" {
		return llvm.Value{}, 0, false, false
	}
	// A member side effect names the field (`#18.n`); the closure captures the
	// object (`m`). The object's cell is where the update is visible.
	if c.closureFreeValueIndex(closureFn, modifyName) < 0 {
		if base := c.memberSideEffectObjectName(closureFn, modifyName); base != "" {
			modifyName = base
		}
	}
	index := -1
	byRef := false
	declared := callframe.OrderedFreeValueBindings(closureFn)
	for i, binding := range declared {
		if binding.Variable.GetName() != modifyName {
			continue
		}
		index = i
		byRef = c.freeValueCaptureMode(closureFn, binding) != freeValueCaptureByValue
		break
	}
	if index < 0 {
		for i, extra := range c.extraCaptures(closureFn) {
			if extra.Name != modifyName {
				continue
			}
			index = len(declared) + i
			byRef = true
			break
		}
	}
	if index < 0 {
		return llvm.Value{}, 0, false, false
	}
	// Direct closure target.
	if c.function.materializedClosures != nil {
		if closureVal, ok := c.function.materializedClosures[inst.CallSite]; ok && !closureVal.IsNil() {
			return closureVal, uint64(index), byRef, true
		}
	}
	// Closure passed as a callback argument (extern/yaklib callee). A call can
	// take several callbacks (progress + finished, for example); pick the one
	// whose body performed the write, so this side effect reads that closure's
	// free-value slot instead of the first callback's.
	if c.function.materializedClosureArgs != nil {
		args := c.function.materializedClosureArgs[inst.CallSite]
		for _, arg := range args {
			if arg.fnID == closureFn.GetId() && !arg.value.IsNil() {
				return arg.value, uint64(index), byRef, true
			}
		}
		for _, arg := range args {
			if !arg.value.IsNil() {
				return arg.value, uint64(index), byRef, true
			}
		}
	}
	return llvm.Value{}, 0, false, false
}

// sideEffectCaptureName picks the variable a closure actually captures.
// inst.GetName() is the name passed to NewSideEffect. Later uses can add
// other variable bindings (a format-list element) that are not captures.
func (c *Compiler) sideEffectCaptureName(inst *ssa.SideEffect, closureFn *ssa.Function) string {
	if inst == nil {
		return ""
	}
	candidates := make([]string, 0, 4)
	if name := inst.GetName(); name != "" {
		candidates = append(candidates, name)
	}
	if last := inst.GetLastVariable(); last != nil && last.GetName() != "" {
		candidates = append(candidates, last.GetName())
	}
	for name := range inst.GetAllVariables() {
		if name != "" {
			candidates = append(candidates, name)
		}
	}
	for _, name := range candidates {
		if c.closureCapturesName(closureFn, name) {
			return name
		}
	}
	if name := inst.GetName(); name != "" {
		return name
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return ""
}

func (c *Compiler) closureCapturesName(fn *ssa.Function, name string) bool {
	if fn == nil || name == "" {
		return false
	}
	if c.closureFreeValueIndex(fn, name) >= 0 {
		return true
	}
	// A member write (`m.n++`) is recorded as `#<id>.n` while the closure
	// captures the object `m`. The update lives in the object's cell.
	if base := c.memberSideEffectObjectName(fn, name); base != "" && c.closureFreeValueIndex(fn, base) >= 0 {
		return true
	}
	for _, extra := range c.extraCaptures(fn) {
		if extra.Name == name {
			return true
		}
	}
	return false
}

// closureFreeValueIndex is the position of a captured variable in the
// closure's free-value list, or -1.
func (c *Compiler) closureFreeValueIndex(fn *ssa.Function, name string) int {
	if fn == nil || name == "" {
		return -1
	}
	for i, binding := range callframe.OrderedFreeValueBindings(fn) {
		if binding.Name == name {
			return i
		}
	}
	return -1
}

// memberSideEffectObjectName resolves a member side-effect name (`#18.n`)
// to the variable the closure captures (`m`). The id before the dot is the
// closure's free-value parameter.
func (c *Compiler) memberSideEffectObjectName(fn *ssa.Function, name string) string {
	if fn == nil || len(name) < 3 || name[0] != '#' {
		return ""
	}
	dot := strings.IndexByte(name, '.')
	if dot <= 1 {
		return ""
	}
	id, err := strconv.ParseInt(name[1:dot], 10, 64)
	if err != nil || id <= 0 {
		return ""
	}
	for _, binding := range callframe.OrderedFreeValueBindings(fn) {
		if binding.ValueID == id {
			return binding.Name
		}
	}
	return ""
}

func (c *Compiler) getOrInsertRuntimeGetClosureFreeSlot() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeGetClosureFreeSlotSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeReadClosureFreeValue() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeReadClosureFreeValueSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) isAsyncSideEffect(inst *ssa.SideEffect) bool {
	callInst, ok := c.sideEffectCallSite(inst)
	return ok && callInst.Async
}

func (c *Compiler) sideEffectCallSite(inst *ssa.SideEffect) (*ssa.Call, bool) {
	if inst == nil || inst.CallSite <= 0 || inst.GetFunc() == nil {
		return nil, false
	}
	callInstAny, ok := inst.GetFunc().GetInstructionById(inst.CallSite)
	if !ok || callInstAny == nil {
		return nil, false
	}
	callInst, ok := callInstAny.(*ssa.Call)
	return callInst, ok && callInst != nil
}

func (c *Compiler) resolveSideEffectActualID(inst *ssa.SideEffect) (int64, error) {
	if inst == nil {
		return 0, fmt.Errorf("resolveSideEffectActualID: nil side-effect")
	}
	fn := inst.GetFunc()
	if fn == nil {
		return 0, fmt.Errorf("compileSideEffect: missing function for side-effect %d", inst.GetId())
	}

	callInstAny, ok := fn.GetInstructionById(inst.CallSite)
	if !ok || callInstAny == nil {
		return 0, fmt.Errorf("compileSideEffect: callsite %d not found", inst.CallSite)
	}
	callInst, ok := callInstAny.(*ssa.Call)
	if !ok || callInst == nil {
		return 0, fmt.Errorf("compileSideEffect: callsite %d is %T, want *ssa.Call", inst.CallSite, callInstAny)
	}

	actualID := inst.Value
	if valueAny, ok := fn.GetValueById(inst.Value); ok && valueAny != nil {
		switch tmpl := valueAny.(type) {
		case *ssa.Parameter:
			idx := tmpl.FormalParameterIndex
			if idx >= 0 && idx < len(callInst.Args) {
				actualID = callInst.Args[idx]
			}
			// idx beyond the call site's arguments (or a negative index for a
			// receiver/free value) means this call simply does not carry the
			// parameter: yak allows calling with fewer arguments than the
			// callee declares, and a variadic tail is folded into one tuple
			// argument. There is no actual value to resolve here, so keep the
			// template and let the callers fall back instead of failing the
			// whole compile.
		case *ssa.ParameterMember:
			if actual, ok := tmpl.GetActualCallParam(callInst); ok && actual != nil {
				actualID = actual.GetId()
			}
			// Same as above: GetActualCallParam only fails when the call site
			// has no argument at this parameter's position.
		}
	}
	return actualID, nil
}

func (c *Compiler) resolveSideEffectActualValue(inst *ssa.SideEffect) ssa.Value {
	actualID, err := c.resolveSideEffectActualID(inst)
	if err != nil || actualID <= 0 {
		return nil
	}
	fn := inst.GetFunc()
	if fn == nil {
		return nil
	}
	value, ok := fn.GetValueById(actualID)
	if !ok || value == nil {
		return nil
	}
	return value
}
