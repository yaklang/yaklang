package ssa

import "github.com/yaklang/yaklang/common/utils"

func AddObjectKeyPair(member, object, key Value) {
	if utils.IsNil(member) || utils.IsNil(object) || utils.IsNil(key) {
		return
	}
	member.AddObjectKeyPair(object, key)
}

func GetObjectKeyPairs(member Value) []ObjectKeyPair {
	if utils.IsNil(member) {
		return nil
	}
	return member.GetObjectKeyPairs()
}

func GetMemberPairs(object Value) []MemberPair {
	if utils.IsNil(object) {
		return nil
	}
	return object.GetMemberPairs()
}

func GetMembersByKey(object, key Value) []Value {
	if utils.IsNil(object) || utils.IsNil(key) {
		return nil
	}
	if members := object.GetMembersByExactKey(key); len(members) > 0 {
		return members
	}
	if members := object.GetMembersByKeyString(GetKeyString(key)); len(members) > 0 {
		return members
	}
	if member, ok := object.GetMember(key); ok {
		return []Value{member}
	}
	if member, ok := object.GetStringMember(GetKeyString(key)); ok {
		return []Value{member}
	}
	return nil
}
