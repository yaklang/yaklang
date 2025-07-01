package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// ReplaceMemberCall replace all member or object relationship
// and will fixup method function call
func ReplaceMemberCall(v, to Value) map[string]Value {
	ret := make(map[string]Value)
	builder := v.GetFunc().builder
	recoverScope := builder.SetCurrent(v)
	defer recoverScope()
	createPhi := generatePhi(builder, nil, nil)

	isDeclereMember := func(member Value) bool {
		un, ok := ToUndefined(member)
		if !ok {
			return false
		}
		return un.Kind == UndefinedMemberInValid || un.Kind == UndefinedMemberValid
	}
	var replace func(Value, Value)
	replace = func(key, member Value) {
		// create member of `to` value with key
		res := checkCanMemberCallExist(to, key)
		name, typ := res.name, res.typ
		toMember := builder.PeekValue(name)

		// then, we will replace value, `member` to `toMember`
		if !isDeclereMember(member) {
			// memeber exist, not declere, just reset type/name/object
			member.SetName(name)
			member.SetType(typ)
			setMemberCallRelationship(to, key, member)
			if utils.IsNil(toMember) {
				// if no toMember, just use member is fine
				ret[name] = member
			} else {
				// if toMember is exist, should create phi
				ret[name] = createPhi(name, []Value{toMember, member})
			}
			return
		}
		// if member call is declere
		if utils.IsNil(toMember) {
			// must create
			toMember = builder.ReadMemberCallValue(to, key)
		}
		// memberT := member

		// switch member.GetOpcode() {
		// // Do nothing, it will be replaced later
		// case SSAOpcodeBinOp:
		// case SSAOpcodeUnOp:
		// default:
		ReplaceAllValue(member, toMember)
		DeleteInst(member)
		if member.IsObject() {
			obj := member
			for key, member := range obj.GetAllMember() {
				obj.DeleteMember(key)
				replace(key, member)
			}
		}
		ret[name] = toMember
	}
	// replace object member-call
	if v.IsObject() {
		for key, member := range v.GetAllMember() {
			// remove this member from v
			v.DeleteMember(key)
			replace(key, member)
		}
	}
	if v.IsMember() {
		obj := v.GetObject()
		setMemberCallRelationship(obj, v.GetKey(), v)
	}
	return ret
}
