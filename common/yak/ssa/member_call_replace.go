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
	isEmptyPhi := isEmptyPhi(builder, nil, nil)

	// replace object member-call
	if v.IsObject() {
		replace := func(key, member Value) {
			if utils.IsNil(key) || utils.IsNil(member) {
				log.Errorf("BUG: replace member is nil key[%v] member[%v]", key, member)
				return
			}

			// replace this member object to to
			trueKey := member.GetKey()
			// remove this member from v
			v.DeleteMember(key)

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

			if isEmptyPhi(key) {
				// No need for recursion
				toKey := to
				v.DeleteMember(key)
				ReplaceAllValue(key, toKey)
				DeleteInst(key)
				setMemberCallRelationship(v, toKey, member)
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

			for n, v := range ReplaceMemberCall(member, toMember) {
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
		for key, member := range v.GetAllMember() {
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

	return ret
}
