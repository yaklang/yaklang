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

	// replace object member-call
	if v.IsObject() {
		replace := func(index, member Value) {
			if utils.IsNil(index) || utils.IsNil(member) {
				log.Errorf("BUG: replace member is nil key[%v] member[%v]", index, member)
				return
			}
			// replace this member object to to
			key := member.GetKey()
			// remove this member from v
			v.DeleteMember(index)

			// create member of `to` value with key
			// if fun := GetMethod(value.GetType(), key.String()); fun != nil {
			// 	return NewClassMethod(fun, value)
			// }
			// re-set type
			res := checkCanMemberCallExist(to, index)
			resk := checkCanMemberCallExist(to, key)
			name, typ := res.name, res.typ
			// toMember := builder.getOriginMember(name, typ, to, key)
			toMember := builder.PeekValue(resk.name)

			// then, we will replace value, `member` to `toMember`
			if member.GetOpcode() != SSAOpcodeUndefined {
				member.SetName(name)
				member.SetType(typ)
				setMemberCallRelationship(to, index, member)
				log.Warn("ReplaceMemberCall can create phi, but we cannot find cfgEntryBlock")
				if utils.IsNil(toMember) {
					ret[name] = member
				} else {
					ret[name] = createPhi(name, []Value{toMember, member})
				}
			}
			if utils.IsNil(toMember) {
				toMember = builder.ReadMemberCallValue(to, index)
			}

			memberT := member
			switch member.GetOpcode() {
			// Do nothing, it will be replaced later
			case SSAOpcodeBinOp:
			case SSAOpcodeUnOp:
			default:
				ReplaceAllValue(member, toMember)
				DeleteInst(member)
				memberT = toMember
			}
			for n, v := range ReplaceMemberCall(member, toMember) {
				ret[n] = v
			}
			if !member.IsObject() {
				ret[name] = memberT
			}
		}
		// call value需要优先替换
		callMap := make(map[Value]Value)
		for index, member := range v.GetAllMember() {
			if _, ok := ToCall(member); ok {
				callMap[index] = member
				continue
			}
			replace(index, member)
		}
		for index, member := range callMap {
			replace(index, member)
		}
	}

	// TODO : this need more test, i think this code error
	if v.IsMember() {
		obj := v.GetObject()
		setMemberCallRelationship(obj, v.GetKey(), v)
	}
	return ret
}
