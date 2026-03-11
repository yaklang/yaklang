package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

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

func GetLatestObjectKeyPair(member Value) (ObjectKeyPair, bool) {
	pairs := GetObjectKeyPairs(member)
	if len(pairs) == 0 {
		return ObjectKeyPair{}, false
	}
	return pairs[len(pairs)-1], true
}

func GetLatestObject(member Value) Value {
	pair, ok := GetLatestObjectKeyPair(member)
	if !ok {
		return nil
	}
	return pair.Object
}

func GetLatestKey(member Value) Value {
	pair, ok := GetLatestObjectKeyPair(member)
	if !ok {
		return nil
	}
	return pair.Key
}

func SetObjectKeyPairs(member Value, pairs []ObjectKeyPair) {
	if utils.IsNil(member) {
		return
	}
	anValue := member.getAnValue()
	anValue.ownerPairs = anValue.ownerPairs[:0]
	for _, pair := range pairs {
		if utils.IsNil(pair.Object) || utils.IsNil(pair.Key) {
			continue
		}
		anValue.appendOwnerPairIDs(pair.Object.GetId(), pair.Key.GetId())
	}
}

func SetMemberPairs(object Value, pairs []MemberPair) {
	if utils.IsNil(object) {
		return
	}
	anValue := object.getAnValue()
	anValue.memberPairs = anValue.memberPairs[:0]
	for _, pair := range pairs {
		if utils.IsNil(pair.Key) || utils.IsNil(pair.Member) {
			continue
		}
		anValue.appendMemberPairIDs(pair.Key.GetId(), pair.Member.GetId())
	}
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
	return nil
}

func GetLatestMemberByKey(object, key Value) (Value, bool) {
	members := GetMembersByKey(object, key)
	if len(members) == 0 {
		return nil, false
	}
	return members[0], true
}

func GetLatestMemberByKeyString(object Value, key string) (Value, bool) {
	if utils.IsNil(object) || key == "" {
		return nil, false
	}
	members := object.GetMembersByKeyString(key)
	if len(members) == 0 {
		return nil, false
	}
	return members[0], true
}

func GetLastWinsMemberPairs(object Value) []MemberPair {
	pairs := GetMemberPairs(object)
	if len(pairs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(pairs))
	ret := make([]MemberPair, 0, len(pairs))
	for index := len(pairs) - 1; index >= 0; index-- {
		pair := pairs[index]
		signature := memberKeySignature(pair.Key)
		if _, ok := seen[signature]; ok {
			continue
		}
		seen[signature] = struct{}{}
		ret = append(ret, pair)
	}
	return ret
}

func memberKeySignature(key Value) string {
	if key == nil {
		return ""
	}
	if lit, ok := ToConstInst(key); ok {
		return "const:" + fmt.Sprint(lit.value)
	}
	return fmt.Sprintf("id:%d", key.GetId())
}
