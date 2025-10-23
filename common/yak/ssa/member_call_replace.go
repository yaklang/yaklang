package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// ReplaceMemberCall replace all member or object relationship
// and will fixup method function call
func ReplaceMemberCall(t, v, to Value) map[string]Value {
	ret := make(map[string]Value)
	builder := t.GetFunc().builder
	if utils.IsNil(builder) {
		return ret
	}
	recoverScope := builder.SetCurrent(t)
	defer recoverScope()
	createPhi := generatePhi(builder, nil, nil)

	// replace object member-call
	if t.IsObject() {
		replace := func(key, member Value) {
			if utils.IsNil(key) || utils.IsNil(member) {
				log.Errorf("BUG: replace member is nil key[%v] member[%v]", key, member)
				return
			}
			// replace this member object to to
			trueKey := member.GetKey()
			// remove this member from v
			if _, ok := t.GetMember(key); ok {
				t.DeleteMember(key)
			}

			// create member of `to` value with key
			// if fun := GetMethod(value.GetType(), key.String()); fun != nil {
			// 	return NewClassMethod(fun, value)
			// }
			// re-set type
			res := checkCanMemberCallExist(to, key)
			trueRes := checkCanMemberCallExist(to, trueKey)
			name, typ := res.name, res.typ
			// toMember := builder.getOriginMember(name, typ, to, key)
			toMember := builder.PeekValue(trueRes.name)

			// then, we will replace value, `member` to `toMember`
			if member.GetOpcode() != SSAOpcodeUndefined {
				member.SetName(name)
				member.SetType(typ)
				setMemberCallRelationship(to, key, member)
				log.Warn("ReplaceMemberCall can create phi, but we cannot find cfgEntryBlock")
				if utils.IsNil(toMember) {
					ret[name] = member
				} else {
					ret[name] = createPhi(name, []Value{toMember, member})
				}
			}

			if key.GetId() == v.GetId() {
				// No need for recursion
				toKey := to
				setMemberCallRelationship(t, toKey, member)
				if utils.IsNil(toMember) {
					toMember = member
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
			for n, v := range ReplaceMemberCall(member, v, toMember) {
				ret[n] = v
			}
			if !member.IsObject() {
				ret[name] = memberT
			}
			// } else {
			// 	log.Errorf("BUG: replace key[%v]/member[%v] is not EmptyPhi", key, member)
			// 	return
			// }
		}
		// call value需要优先替换
		callMap := make(map[Value]Value)
		for key, member := range t.GetAllMember() {
			if _, ok := ToCall(member); ok {
				callMap[key] = member
				continue
			}
			replace(key, member)
		}
		for key, member := range callMap {
			replace(key, member)
		}
	}

	// TODO : this need more test, i think this code error
	// if t.IsMember() {
	// 	obj := t.GetObject()
	// 	setMemberCallRelationship(obj, t.GetKey(), t)
	// }
	return ret
}
