package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

func applicationProgramName(prog *Program) string {
	if prog == nil {
		return ""
	}
	if app := prog.GetApplication(); app != nil && app.Name != "" {
		return app.Name
	}
	return prog.Name
}

func (pair MemberPair) KeyString() string {
	return GetKeyString(pair.Key)
}

func (pair ObjectKeyPair) KeyString() string {
	return GetKeyString(pair.Key)
}

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

// GetAllObjects returns every distinct parent object recorded on member
// (i.e. every object that holds this member under some key), deduplicated
// by value id. Order follows ownerPairs append order; callers that need
// "latest first" semantics should reverse-iterate. Use this on audit read
// paths where a member may legitimately belong to multiple objects; keep
// using GetLatestObject for display/debug single-parent convenience.
func GetAllObjects(member Value) []Value {
	if utils.IsNil(member) {
		return nil
	}
	pairs := GetObjectKeyPairs(member)
	if len(pairs) == 0 {
		return nil
	}
	ret := make([]Value, 0, len(pairs))
	seen := make(map[int64]struct{}, len(pairs))
	for _, pair := range pairs {
		if utils.IsNil(pair.Object) {
			continue
		}
		id := pair.Object.GetId()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, pair.Object)
	}
	return ret
}

// GetAllKeys returns every distinct parent key recorded on member,
// deduplicated by value id. Semantics mirror GetAllObjects.
func GetAllKeys(member Value) []Value {
	if utils.IsNil(member) {
		return nil
	}
	pairs := GetObjectKeyPairs(member)
	if len(pairs) == 0 {
		return nil
	}
	ret := make([]Value, 0, len(pairs))
	seen := make(map[int64]struct{}, len(pairs))
	for _, pair := range pairs {
		if utils.IsNil(pair.Key) {
			continue
		}
		id := pair.Key.GetId()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ret = append(ret, pair.Key)
	}
	return ret
}

// GetAllMembersByKey is the explicit "all matches" counterpart to
// GetLatestMemberByKey. It is a thin alias over GetMembersByKey so call
// sites can express intent ("I want every member under this key") without
// the reader having to know GetMembersByKey already returns all matches.
func GetAllMembersByKey(object, key Value) []Value {
	return GetMembersByKey(object, key)
}

// ForEachOwnerPair invokes fn for every (object, key) pair recorded on
// member. If fn returns false, iteration stops. Use this on hot paths to
// avoid allocating an intermediate []ObjectKeyPair when only a single
// matching pair is needed.
func ForEachOwnerPair(member Value, fn func(ObjectKeyPair) bool) {
	if utils.IsNil(member) || fn == nil {
		return
	}
	for _, pair := range GetObjectKeyPairs(member) {
		if !fn(pair) {
			return
		}
	}
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
