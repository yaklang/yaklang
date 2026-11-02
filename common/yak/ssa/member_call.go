package ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// get field value
func (b *FunctionBuilder) getFieldValue(object, key Value, wantFunction bool) Value {
	res := checkCanMemberCallExist(object, key, wantFunction)
	if ret := b.PeekValueInThisFunction(res.name); ret != nil && shouldReuseMemberScopeValue(object, key, ret) {
		return ret
	}
	if !isStaticBlueprintMemberAccess(object, key) && !memberMutatedByNestedClosure(object, key) {
		if members := GetMembersByKey(object, key); len(members) > 0 {
			if wantFunction {
				for _, member := range members {
					if utils.IsNil(member) {
						continue
					}
					if typ := member.GetType(); typ != nil && typ.GetTypeKind() == FunctionTypeKind {
						return member
					}
				}
			} else if !utils.IsNil(members[0]) {
				return members[0]
			}
		}
	}

	// default member
	value := b.createDefaultMember(res, object, key, wantFunction)
	return value
}

func shouldReuseMemberScopeValue(object, key, value Value) bool {
	if utils.IsNil(value) {
		return false
	}
	if !memberMutatedByNestedClosure(object, key) {
		return true
	}
	// A literal cached when the object was built can be stale after a nested
	// function mutates this exact field. Keep propagated side effects and
	// computed values from the current scope; only bypass the old literal.
	_, isLiteral := ToConstInst(value)
	return !isLiteral
}

// memberMutatedByNestedClosure reports whether a nested function captures this
// object and writes the requested member through a parameter member.
func memberMutatedByNestedClosure(object, key Value) bool {
	if utils.IsNil(object) || utils.IsNil(key) || !valueCapturedByClosure(object) {
		return false
	}
	parent := object.GetFunc()
	program := object.GetProgram()
	if parent == nil || program == nil {
		return false
	}
	keyName := GetKeyString(key)
	if keyName == "" {
		keyName = key.String()
	}
	if keyName == "" {
		return false
	}
	wantSuffix := "." + keyName
	mutated := false
	program.EachFunction(func(fn *Function) {
		if mutated || fn == nil || !isNestedInFunction(fn, parent) {
			return
		}
		// The closure must be handed to a call whose body the front end cannot
		// see (filesys.onFileStat); otherwise handleArgumentFunctionSideEffect
		// already propagates the write.
		if holder := fn.GetParent(); holder == nil || !closurePassedToOpaqueCall(holder, fn) {
			return
		}
		for _, freeValueID := range fn.FreeValues {
			parameterValue, ok := fn.GetValueById(freeValueID)
			if !ok || parameterValue == nil {
				continue
			}
			parameter, ok := ToParameter(parameterValue)
			if !ok || parameter == nil || resolveCapturedObjectID(parameter) != object.GetId() {
				continue
			}
			prefix := fmt.Sprintf("#%d", freeValueID)
			for _, memberID := range fn.ParameterMembers {
				member, ok := fn.GetValueById(memberID)
				if !ok || member == nil {
					continue
				}
				name := member.GetName()
				if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, wantSuffix) {
					mutated = true
					return
				}
			}
		}
	})
	return mutated
}

// isNestedInFunction reports whether fn is nested anywhere below ancestor.
func isNestedInFunction(fn, ancestor *Function) bool {
	if fn == nil || ancestor == nil {
		return false
	}
	for current := fn.GetParent(); current != nil; current = current.GetParent() {
		if current.GetId() == ancestor.GetId() {
			return true
		}
	}
	return false
}

// resolveCapturedObjectID follows free-value defaults until it reaches the value
// the innermost closure actually captured (for `params.file_count`, the outer
// `params` object). A capture nested two levels deep defaults to the enclosing
// function's free value, not to the object itself.
func resolveCapturedObjectID(param *Parameter) int64 {
	for depth := 0; param != nil && depth < 8; depth++ {
		captured := param.GetDefault()
		if captured == nil {
			return 0
		}
		if next, ok := ToParameter(captured); ok && next != nil && next.IsFreeValue {
			param = next
			continue
		}
		return captured.GetId()
	}
	return 0
}

// closurePassedToOpaqueCall reports whether parent passes nested as a callback
// argument to a call without an SSA function body. Such a call cannot have its
// callback side effects propagated by handleArgumentFunctionSideEffect.
func closurePassedToOpaqueCall(parent, nested *Function) bool {
	if parent == nil || nested == nil {
		return false
	}
	for _, blockID := range parent.Blocks {
		blockValue, ok := parent.GetValueById(blockID)
		if !ok {
			continue
		}
		block, ok := ToBasicBlock(blockValue)
		if !ok || block == nil {
			continue
		}
		for _, instructionID := range block.Insts {
			instruction, ok := parent.GetInstructionById(instructionID)
			if !ok || instruction == nil {
				continue
			}
			call, ok := ToCall(instruction)
			if !ok || call == nil {
				continue
			}
			passed := false
			for _, argumentID := range call.Args {
				argument, ok := parent.GetValueById(argumentID)
				if !ok || argument == nil {
					continue
				}
				if function, ok := ToFunction(argument); ok && function != nil && function.GetId() == nested.GetId() {
					passed = true
					break
				}
			}
			if !passed {
				continue
			}
			callee, ok := parent.GetValueById(call.Method)
			if !ok || callee == nil {
				return true
			}
			if _, hasSSAFunctionBody := ToFunction(callee); !hasSSAFunctionBody {
				return true
			}
		}
	}
	return false
}

// isStaticBlueprintMemberAccess reports whether object/key denote a class static
// member access such as A.value, where static/phi resolution must stay in charge.
func isStaticBlueprintMemberAccess(object, key Value) bool {
	if utils.IsNil(object) || utils.IsNil(key) {
		return false
	}
	blueprint, ok := ToBluePrintType(object.GetType())
	if !ok {
		return false
	}
	return isBlueprintStaticAccessValue(object, blueprint)
}

func (b *FunctionBuilder) getStaticFieldValue(object, key Value, wantFunction bool) Value {
	// only static member and method need to be checked
	if !b.isSupportClassStaticModifier() {
		return nil
	}
	// get member or method
	getValueFromClass := func(class *Blueprint) Value {
		keyName := GetKeyString(key)
		if keyName == "" {
			keyName = key.String()
		}
		var get func(string) Value
		if wantFunction {
			get = class.GetStaticMethod
		} else {
			get = class.GetStaticMember
		}
		return get(keyName)
	}
	// className.Key
	if un, ok := ToUndefined(object); ok && un.Kind == UndefinedValueInValid {
		if bp := b.GetBluePrint(un.GetName()); bp != nil {
			if ret := getValueFromClass(bp); ret != nil {
				return ret
			}
		}
	}

	// classInstance.Key
	if blueprint, ok := object.GetType().(*Blueprint); ok {
		if isBlueprintStaticAccessValue(object, blueprint) || b.allowStaticMemberAccessByInstance() {
			if ret := getValueFromClass(blueprint); ret != nil {
				return ret
			}
		}
	}
	return nil
}

func (b *FunctionBuilder) InterfaceAddFieldBuild(
	size int, keys func(int) Value, value func(int) Value,
) *Make {
	// lValueLen := NewConst(size)
	var lValueLen Value = nil
	itf := b.EmitMakeWithoutType(lValueLen, lValueLen)
	if utils.IsNil(itf) {
		return nil
	}
	if b.MarkedVariable != nil {
		itf.SetName(b.MarkedVariable.GetName())
		b.MarkedThisObject = itf
		defer func() {
			b.MarkedThisObject = nil
		}()
	}
	ityp := NewObjectType()
	itf.SetType(ityp)
	for i := 0; i < size; i++ {
		key := keys(i)
		value := value(i)
		v := b.CreateMemberCallVariable(itf, key)
		b.AssignVariable(v, value)
	}
	ityp.Finish()
	// ityp.Len = len(vs)
	return itf
}

func (b *FunctionBuilder) getDefaultMemberOrMethodByClass(object, key Value, method bool) Value {
	if !b.isSupportClass() {
		return nil
	}
	// class blue print
	bluePrint, ok := ToBluePrintType(object.GetType())
	if !ok {
		return nil
	}
	keyName := GetKeyString(key)
	if keyName == "" {
		keyName = key.String()
	}
	if method {
		if normalMethod := bluePrint.GetNormalMethod(keyName); !utils.IsNil(normalMethod) {
			return normalMethod
		}
	}
	if member := bluePrint.GetNormalMember(keyName); !utils.IsNil(member) {
		return member
	}
	return nil
}

func (b *FunctionBuilder) createDefaultMember(res checkMemberResult, object, key Value, wantFunction bool) Value {
	// create undefined memberCall value if the value can not be peeked
	name := res.name
	// normal method
	typ := res.typ
	if wantFunction {
		// this get will call function Builder, then should refresh the typ
		keyName := GetKeyString(key)
		if keyName == "" {
			keyName = key.String()
		}
		if fun := object.GetProgram().getMethod(object.GetType(), keyName); fun != nil {
			typ = fun.GetType()
		}
	}
	memberHandler := func(member Value) {
		// todo: phi type is anytype,unknown other value
		if typ == nil || typ.GetTypeKind() == AnyTypeKind {
			if wantFunction {
				t := NewFunctionTypeDefine(name, nil, nil, false)
				t.SetIsMethod(true, object.GetType())
				typ = t
			}
			if typ == nil {
				typ = CreateAnyType()
			}
			objType := object.GetType()
			if objType != nil {
				if fts := objType.GetFullTypeNames(); len(fts) != 0 {
					typ.SetFullTypeNames(fts)
				}
			}
		}
		if typ != nil {
			member.SetType(typ)
		}
		setMemberCallRelationship(object, key, member)
		setMemberVerboseName(member)
	}
	writeUndefined := func() *Undefined {
		// Missing members of a dependency are synthesized by the consuming
		// unit. Re-entering the declaration block would mutate a completed
		// dependency (and race with other consumers of that same object).
		recoverScope := func() {}
		if !b.GetProgram().ConcurrentUnitBuildEnabled() || (object.GetFunc() != nil && object.GetFunc().GetProgram() == b.GetProgram()) {
			recoverScope = b.SetCurrent(object, true)
		}
		displayName := name
		if _, ok := ToConstInst(key); !ok {
			if memberName := getMemberVerboseName(object, key); memberName != "" {
				displayName = memberName
			}
		}
		un := b.writeUndefine(name, displayName)
		recoverScope()
		if res.exist {
			un.Kind = UndefinedMemberValid
		} else {
			un.Kind = UndefinedMemberInValid
		}

		return un
	}
	if para, ok := ToParameter(object); ok {
		// if member, ok2 := para.GetStringMember(key.String()); ok2 {
		// 	return member
		// }
		member := b.NewParameterMember(name, para, key)

		memberHandler(member)
		return member
	}
	config := b.prog.config
	if member, ok := ToParameterMember(object); ok {
		if !wantFunction || config.isSupportConstMethod {
			parameterMember := b.NewMoreParameterMember(name, member, key)
			memberHandler(parameterMember)
			return parameterMember
		}
	}
	if ret := b.getStaticFieldValue(object, key, wantFunction); ret != nil {
		return ret
	}
	// this function only try get value field
	if field := b.getDefaultMemberOrMethodByClass(object, key, false); !utils.IsNil(field) {
		return field
	}
	un := writeUndefined()
	memberHandler(un)
	return un
}

func (b *FunctionBuilder) checkAndCreateDefaultMember(res checkMemberResult, object, key Value) {
	// 	recoverScope := b.SetCurrent(object, true)
	if ret := b.PeekValueInThisFunction(res.name); ret != nil {
		return
	}
	if !res.exist {
		object.NewError(Error, "ObjectError",
			InvalidField(res.ObjType.String(), GetKeyString(key)),
		)
	}

	currentScope := b.CurrentBlock.ScopeTable
	if utils.IsNil(object.GetBlock()) {
		return
	}
	objectScope := object.GetBlock().ScopeTable
	if utils.IsNil(currentScope) || utils.IsNil(objectScope) {
		return
	}
	// is sub-scope and not same, just child scope
	if currentScope.IsSameOrSubScope(objectScope) && !currentScope.Compare(objectScope) {
		b.createDefaultMember(res, object, key, false)
	}
}
