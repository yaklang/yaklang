package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// ReplaceMemberCall replace all member or object relationship
// and will fixup method function call
func ReplaceMemberCall(old, replacement Value) map[string]Value {
	return replaceMemberCall(old, old, replacement)
}

func replaceMemberCall(holder, target, replacement Value) map[string]Value {
	ret := make(map[string]Value)
	if utils.IsNil(holder) || utils.IsNil(target) || utils.IsNil(replacement) {
		return ret
	}

	builder := holder.GetFunc().builder
	if utils.IsNil(builder) {
		return ret
	}

	recoverScope := builder.SetCurrent(holder)
	defer recoverScope()

	createPhi := generatePhi(builder, nil, nil)

	fixBranch := func(root Value, targetObj Value, rootKey Value) {
		if utils.IsNil(root) || utils.IsNil(targetObj) {
			return
		}
		if currentObj := root.GetObject(); !utils.IsNil(currentObj) && currentObj.GetId() != target.GetId() && currentObj.GetId() != holder.GetId() {
			// already points to a valid object, no change needed
		} else {
			root.SetObject(targetObj)
		}
		if root.IsMember() {
			currentKey := root.GetKey()
			if utils.IsNil(currentKey) || currentKey.GetId() == target.GetId() || currentKey.GetId() == holder.GetId() {
				root.SetKey(pickMemberKey(root, rootKey))
			}
		}
	}

	replace := func(container Value, key Value, member Value) {}
	replace = func(container Value, key Value, member Value) {
		if utils.IsNil(key) || utils.IsNil(member) {
			return
		}

		trueKey := member.GetKey()
		if _, ok := container.GetMember(key); ok {
			container.DeleteMember(key)
		}

		res := checkCanMemberCallExist(replacement, key)
		trueRes := checkCanMemberCallExist(replacement, trueKey)
		name, typ := res.name, res.typ
		toMember := builder.PeekValue(trueRes.name)

		if member.GetOpcode() != SSAOpcodeUndefined {
			member.SetName(name)
			member.SetType(typ)
			setMemberCallRelationship(replacement, key, member)
			if utils.IsNil(toMember) {
				ret[name] = member
			} else {
				if res.typ != nil {
					toMember.SetType(res.typ)
				}
				ret[name] = createPhi(name, []Value{toMember, member})
			}
		}

		if key.GetId() == target.GetId() {
			toKey := replacement
			setMemberCallRelationship(container, toKey, member)
			if utils.IsNil(toMember) {
				toMember = member
			}
		}
		if utils.IsNil(toMember) {
			toMember = builder.ReadMemberCallValue(replacement, key)
		}

		if utils.IsNil(toMember.GetObject()) || toMember.GetObject().GetId() == target.GetId() || toMember.GetObject().GetId() == holder.GetId() {
			fixBranch(toMember, replacement, key)
		}

		memberT := member
		switch member.GetOpcode() {
		case SSAOpcodeBinOp, SSAOpcodeUnOp:
			// keep original instruction for later replacement
		default:
			ReplaceAllValue(member, toMember)
			DeleteInst(member)
			memberT = toMember
		}

		for n, v2 := range replaceMemberCall(member, target, toMember) {
			ret[n] = v2
		}
		if !member.IsObject() {
			ret[name] = memberT
		}
	}

	if holder.IsObject() {
		callMap := make(map[Value]Value)
		for key, member := range holder.GetAllMember() {
			if _, ok := ToCall(member); ok {
				callMap[key] = member
				continue
			}
			replace(holder, key, member)
		}
		for key, member := range callMap {
			replace(holder, key, member)
		}
	}

	return ret
}

func pickMemberKey(member, fallback Value) Value {
	if !utils.IsNil(member) {
		if k := member.GetKey(); !utils.IsNil(k) {
			return k
		}
	}
	return fallback
}
