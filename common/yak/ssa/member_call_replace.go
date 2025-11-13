package ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// ReplaceMemberCall replace all member or object relationship
// and will fixup method function call
func ReplaceMemberCall(holder, old, replacement Value) map[string]Value {
	result := make(map[string]Value)
	if utils.IsNil(holder) || utils.IsNil(old) || utils.IsNil(replacement) {
		return result
	}

	builder := holder.GetFunc().builder
	if utils.IsNil(builder) {
		return result
	}

	restore := builder.SetCurrent(holder)
	defer restore()

	createPhi := generatePhi(builder, nil, nil)
	type entry struct {
		key    Value
		member Value
	}

	recordMember := func(obj, key, val Value) Value {
		if utils.IsNil(obj) || utils.IsNil(key) || utils.IsNil(val) {
			return val
		}
		memberKey := pickMemberKey(val, key)
		resolve := func(target Value) (checkMemberResult, bool) {
			if utils.IsNil(target) {
				return checkMemberResult{}, false
			}
			res := checkCanMemberCallExist(target, memberKey)
			if res.name != "" || res.typ != nil {
				return res, true
			}
			return res, false
		}

		res, ok := resolve(obj)
		if !ok {
			if phiObj, isPhi := ToPhi(obj); isPhi {
				for _, edgeID := range phiObj.Edge {
					edgeValue, exist := obj.GetValueById(edgeID)
					if !exist || utils.IsNil(edgeValue) {
						continue
					}
					if tmp, ok := resolve(edgeValue); ok {
						res = tmp
						ok = true
						break
					}
				}
			}
		}
		if !ok {
			candidates := []Value{replacement, holder, old}
			if objVal := val.GetObject(); !utils.IsNil(objVal) {
				candidates = append(candidates, objVal)
			}
			for _, candidate := range candidates {
				if utils.IsNil(candidate) || candidate.GetId() == obj.GetId() {
					continue
				}
				if tmp, ok := resolve(candidate); ok {
					res = tmp
					break
				}
			}
			if res.name == "" && val.GetName() != "" {
				res.name = val.GetName()
			}
			if res.typ == nil && val.GetType() != nil {
				res.typ = val.GetType()
			}
		}
		if res.name == "" {
			return val
		}

		existingMember := builder.PeekValue(res.name)
		if utils.IsNil(existingMember) {
			if !utils.IsNil(obj) && !utils.IsNil(memberKey) {
				existingMember = builder.ReadMemberCallValue(obj, memberKey)
			}
		}

		recorded := val
		if !utils.IsNil(existingMember) && existingMember.GetId() != recorded.GetId() {
			if _, isUndefined := ToUndefined(existingMember); !isUndefined {
				if !utils.IsNil(val.GetType()) {
					existingType := existingMember.GetType()
					if utils.IsNil(existingType) || existingType.GetTypeKind() == AnyTypeKind {
						existingMember.SetType(val.GetType())
					}
				}
				existingType := existingMember.GetType()
				preferExisting := !utils.IsNil(existingType) && existingType.GetTypeKind() != AnyTypeKind
				if preferExisting {
					recorded = existingMember
					if res.typ == nil && existingType != nil {
						res.typ = existingType
					}
				}
			}
		}

		if res.typ != nil && !utils.IsNil(recorded) {
			recorded.SetType(res.typ)
		}
		if recorded.GetName() != res.name {
			currentName := recorded.GetName()
			shouldRename := res.name != "" && (currentName == "" || strings.HasPrefix(currentName, "#") || !strings.HasPrefix(res.name, "#"))
			if shouldRename {
				recorded.SetName(res.name)
			}
		}
		if existed, ok := result[res.name]; ok && !utils.IsNil(existed) {
			if existed.GetId() != recorded.GetId() {
				merged := createPhi(res.name, []Value{existed, recorded})
				if !utils.IsNil(merged) && res.typ != nil {
					merged.SetType(res.typ)
				}
				recorded = merged
			} else {
				recorded = existed
			}
		}
		result[res.name] = recorded
		return recorded
	}

	visited := make(map[int64]struct{})
	var traverse func(Value)

	traverse = func(current Value) {
		if utils.IsNil(current) {
			return
		}
		if _, seen := visited[current.GetId()]; seen {
			return
		}
		visited[current.GetId()] = struct{}{}

		members := current.GetAllMember()
		if len(members) == 0 {
			return
		}

		var ordered, delayed []entry
		for key, member := range members {
			e := entry{key: key, member: member}
			if _, isCall := ToCall(member); isCall {
				delayed = append(delayed, e)
				continue
			}
			ordered = append(ordered, e)
		}
		ordered = append(ordered, delayed...)

		for _, item := range ordered {
			key := item.key
			member := item.member

			dest := current
			if dest.GetId() == old.GetId() {
				dest = replacement
			}
			if utils.IsNil(dest) {
				continue
			}

			newKey := key
			if key.GetId() == old.GetId() {
				newKey = replacement
			}

			newMember := member
			if member.GetId() == old.GetId() {
				newMember = replacement
			}

			needMove := dest.GetId() != current.GetId() ||
				newKey.GetId() != key.GetId() ||
				newMember.GetId() != member.GetId()

			updateObjectRef := func(m Value, targetObj Value) {
				if utils.IsNil(m) || utils.IsNil(targetObj) {
					return
				}
				memberObj := m.GetObject()
				if utils.IsNil(memberObj) || memberObj.GetId() == current.GetId() {
					m.SetObject(targetObj)
				}
			}

			if needMove {
				updateObjectRef(member, dest)
				if newMember.GetId() != member.GetId() {
					updateObjectRef(newMember, dest)
				}
				current.DeleteMember(key)
			}

			effective := newMember
			if existing, ok := dest.GetMember(newKey); ok && !utils.IsNil(existing) {
				if existing.GetId() != newMember.GetId() {
					res := checkCanMemberCallExist(dest, pickMemberKey(newMember, newKey))
					candidate := createPhi(res.name, []Value{existing, newMember})
					if !utils.IsNil(candidate) {
						if res.typ != nil {
							candidate.SetType(res.typ)
						}
						effective = candidate
					} else {
						effective = existing
					}
				} else {
					effective = existing
				}
			}

			effective = recordMember(dest, newKey, effective)
			setMemberCallRelationship(dest, newKey, effective)

			if needMove && !utils.IsNil(effective) {
				updateObjectRef(effective, dest)
			}

			if needMove && !utils.IsNil(member) && member.GetId() != effective.GetId() {
				ReplaceAllValue(member, effective)
				DeleteInst(member)
			}

			if member.IsObject() {
				traverse(member)
			}
			for _, nested := range member.GetValues() {
				traverse(nested)
			}
			if effective.IsObject() {
				traverse(effective)
			}
			if effective.GetId() != member.GetId() {
				for _, nested := range effective.GetValues() {
					traverse(nested)
				}
			}
		}
	}

	traverse(holder)
	if replacement.IsObject() {
		traverse(replacement)
	}
	return result
}

func pickMemberKey(member, fallback Value) Value {
	if !utils.IsNil(member) {
		if k := member.GetKey(); !utils.IsNil(k) {
			return k
		}
	}
	return fallback
}
