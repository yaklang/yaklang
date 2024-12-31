package ssa

import (
	"github.com/yaklang/yaklang/common/log"
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
		for index, member := range v.GetAllMember() {
			// replace this member object to to
			key := member.GetKey()
			// remove this member from v
			v.DeleteMember(key)

			// create member of `to` value with key
			// if fun := GetMethod(value.GetType(), key.String()); fun != nil {
			// 	return NewClassMethod(fun, value)
			// }
			// re-set type
			resKey := checkCanMemberCallExist(to, key)
			resIndex := checkCanMemberCallExist(to, index)
			name, typ := resKey.name, resKey.typ
			// toMember := builder.getOriginMember(name, typ, to, key)
			toMember := builder.PeekValue(name)

			// then, we will replace value, `member` to `toMember`
			if member.GetOpcode() != SSAOpcodeUndefined {
				member.SetName(name)
				member.SetType(typ)
				setMemberCallRelationship(to, key, member)
				log.Warn("ReplaceMemberCall can create phi, but we cannot find cfgEntryBlock")
				if utils.IsNil(toMember) {
					ret[resIndex.name] = member
				} else {
					ret[resIndex.name] = createPhi(name, []Value{toMember, member})
				}
			}
			if utils.IsNil(toMember) {
				toMember = builder.ReadMemberCallValue(to, key)
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
				ret[resIndex.name] = memberT
			}
		}
	}

	// TODO : this need more test, i think this code error
	if v.IsMember() {
		obj := v.GetObject()
		setMemberCallRelationship(obj, v.GetKey(), v)
	}
	return ret
}
