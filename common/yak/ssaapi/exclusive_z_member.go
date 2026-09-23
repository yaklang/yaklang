package ssaapi

import (
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// memberEntry is one (key, member) pair of an object's member table.
type memberEntry struct {
	key    ssa.Value
	member ssa.Value
	keyID  int64
}

// sortedMemberPairs flattens a member table into a stable order. See the call
// site in the *ssa.Make case: traversal order is observable through the shared
// recursion budget and result cap, so it must not depend on map iteration.
func sortedMemberPairs(members map[ssa.Value]ssa.Value) []memberEntry {
	pairs := make([]memberEntry, 0, len(members))
	for key, member := range members {
		if utils.IsNil(key) || utils.IsNil(member) {
			continue
		}
		pairs = append(pairs, memberEntry{key: key, member: member, keyID: key.GetId()})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].keyID != pairs[j].keyID {
			return pairs[i].keyID < pairs[j].keyID
		}
		return pairs[i].member.GetId() < pairs[j].member.GetId()
	})
	return pairs
}

// isBlueprintValue reports whether a value is a class blueprint rather than a
// data instance. Blueprint members are keyed by the class name (plus companion
// keys such as "<Class>-destructor"), not by field name, so they are not
// candidates for field-wise resolution.
func isBlueprintValue(v *Value) bool {
	if utils.IsNil(v) {
		return false
	}
	typ := v.GetType()
	if utils.IsNil(typ) {
		return false
	}
	_, ok := ssa.ToBluePrintType(GetBareType(typ))
	return ok
}

// isCollectionLikeValue reports whether a value is an array/slice/map style
// container whose "members" are positional or keyed elements rather than named
// fields. A member read on such a value (e.g. `data.toString()` on a char[])
// still needs the whole element set followed, so these keep the enumeration
// path.
func isCollectionLikeValue(v *Value) bool {
	if utils.IsNil(v) {
		return false
	}
	typ := v.GetType()
	if utils.IsNil(typ) {
		return false
	}
	raw := GetBareType(typ)
	if utils.IsNil(raw) {
		return false
	}
	switch raw.GetTypeKind() {
	case ssa.SliceTypeKind, ssa.MapTypeKind, ssa.TupleTypeKind:
		return true
	}
	return false
}

// callableMemberValue reports whether an object member is a callable symbol
// (a method or a function-typed slot) rather than a data field.
//
// Such a member is reached as a callee: the call site that reads it passes the
// owner object in as the receiver, so the value flow the caller is looking for
// is already carried by the object's own uses. Walking it as if it were a data
// field makes bottom-use descend every call site of that method, and on a
// class blueprint that means every method of the class, each pulling in its
// own caller set -- the dominant source of the observed fan-out.
func callableMemberValue(v *Value) bool {
	if utils.IsNil(v) {
		return false
	}
	if v.IsFunction() || v.IsMethod() {
		return true
	}
	typ := v.GetType()
	if utils.IsNil(typ) {
		return false
	}
	_, ok := ssa.ToFunctionType(GetBareType(typ))
	return ok
}

// constKeyText returns the plain string content of a constant key. Member keys
// are rendered with surrounding quotes by the generic key accessors, which
// would defeat prefix comparison, so the constant content is read directly.
func constKeyText(key *Value) (string, bool) {
	if utils.IsNil(key) {
		return "", false
	}
	if raw, ok := key.GetConstValue().(string); ok {
		return raw, true
	}
	text := ssa.GetKeyString(key.getValue())
	if text == "" {
		return "", false
	}
	if unquoted, err := strconv.Unquote(text); err == nil {
		return unquoted, true
	}
	return text, true
}

// resolveKeyedMembers returns the members of an object that a keyed descent
// should follow.
//
// A descent that arrives with an (object, key) context does so because some
// site read `object.key`; the precise answer is that key's member set. Some
// languages additionally synthesise companion members under a key derived from
// it -- PHP registers the destructor as `<Class>-destructor` alongside the
// class member -- and rules rely on those, so derived keys are included too.
//
// Returning nil means the key resolves to nothing here, which tells the caller
// to fall back to walking the object's members.
func resolveKeyedMembers(object, key *Value) []*Value {
	if object == nil || key == nil {
		return nil
	}
	matched := object.GetMember(key)
	if len(matched) == 0 {
		return nil
	}
	// Companion symbols belong to a class blueprint. An ordinary object's
	// keys such as "foo" and "foo-other" are independent data fields.
	if !isBlueprintValue(object) {
		return matched
	}
	raw, ok := constKeyText(key)
	if !ok || raw == "" {
		return matched
	}
	seen := make(map[int64]struct{}, len(matched))
	for _, m := range matched {
		if !utils.IsNil(m) {
			seen[m.GetId()] = struct{}{}
		}
	}
	for _, pair := range ssa.GetMemberPairs(object.getValue()) {
		if utils.IsNil(pair.Key) || utils.IsNil(pair.Member) {
			continue
		}
		pairKey := object.NewValue(pair.Key)
		if utils.IsNil(pairKey) {
			continue
		}
		derived, ok := constKeyText(pairKey)
		if !ok {
			continue
		}
		if derived == raw || !strings.HasPrefix(derived, raw+"-") {
			continue
		}
		member := object.NewValue(pair.Member)
		if utils.IsNil(member) {
			continue
		}
		if _, ok := seen[member.GetId()]; ok {
			continue
		}
		seen[member.GetId()] = struct{}{}
		matched = append(matched, member)
	}
	return matched
}
