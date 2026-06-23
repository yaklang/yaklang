package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// ReplaceMemberCall 替换所有成员或对象关系，并修复方法函数调用
func ReplaceMemberCall(old, replacement Value) map[string]Value {
	ret := make(map[string]Value)
	if utils.IsNil(old) || utils.IsNil(replacement) {
		return ret
	}

	builder := old.GetFunc().builder
	if utils.IsNil(builder) {
		return ret
	}

	recoverScope := builder.SetCurrent(old)
	defer recoverScope()

	createPhi := generatePhi(builder, nil, nil)
	target := old
	visited := make(map[int64]struct{})

	// 递归处理嵌套成员替换
	var replaceMemberCallRecursive func(holder Value, replacement Value, visited map[int64]struct{}) map[string]Value
	replaceMemberCallRecursive = func(holder Value, replacement Value, visited map[int64]struct{}) map[string]Value {
		holderRet := make(map[string]Value)
		sourceKeyOverrides := make(map[int64]Value)

		if !utils.IsNil(holder) {
			holderID := holder.GetId()
			if _, alreadyVisited := visited[holderID]; alreadyVisited {
				return holderRet
			}
			visited[holderID] = struct{}{}
		}

		fixBranch := func(root Value, targetObj Value, rootKey Value) {
			if utils.IsNil(root) || utils.IsNil(targetObj) {
				return
			}
			pairs := GetObjectKeyPairs(root)
			if len(pairs) == 0 {
				root.AddObjectKeyPair(targetObj, pickMemberKey(root, rootKey))
				return
			}
			updated := make([]ObjectKeyPair, 0, len(pairs))
			changed := false
			for _, pair := range pairs {
				if !utils.IsNil(pair.Object) && pair.Object.GetId() != target.GetId() && pair.Object.GetId() != holder.GetId() {
					updated = append(updated, pair)
					continue
				}
				key := pair.Key
				if utils.IsNil(key) || key.GetId() == target.GetId() || key.GetId() == holder.GetId() {
					key = pickMemberKey(root, rootKey)
				}
				updated = append(updated, ObjectKeyPair{Object: targetObj, Key: key})
				changed = true
			}
			if changed {
				SetObjectKeyPairs(root, updated)
			}
		}

		// 替换成员：处理 container[key] 的替换
		replace := func(container Value, key Value, member Value) {
			if utils.IsNil(key) || utils.IsNil(member) {
				return
			}

			trueKey := pickMemberKey(member, key)
			if member.GetOpcode() == SSAOpcodeUndefined {
				if sourceKey, ok := sourceKeyOverrides[member.GetId()]; ok {
					trueKey = sourceKey
				} else {
					trueKey = pickUndefinedMemberSourceKey(member, key)
				}
			}
			if _, ok := GetLatestMemberByKey(container, key); ok {
				container.DeleteMember(key)
			}

			res := checkCanMemberCallExist(replacement, key)
			trueRes := checkCanMemberCallExist(replacement, trueKey)
			name, typ := res.name, res.typ
			targetMember := builder.PeekValue(res.name)
			if utils.IsNil(targetMember) {
				if existing, ok := GetLatestMemberByKey(replacement, key); ok {
					targetMember = existing
				}
			}
			movingMember := member
			if member.GetOpcode() == SSAOpcodeUndefined {
				if existing, ok := GetLatestMemberByKey(replacement, trueKey); ok {
					movingMember = existing
				} else if existing := builder.PeekValue(trueRes.name); !utils.IsNil(existing) {
					movingMember = existing
				}
			}
			toMember := targetMember
			var mergedMember Value

			if movingMember.GetOpcode() != SSAOpcodeUndefined {
				movingMember.SetName(name)
				movingMember.SetType(typ)
				setMemberCallRelationship(replacement, key, movingMember)
				if utils.IsNil(targetMember) || targetMember.GetId() == movingMember.GetId() {
					toMember = movingMember
					holderRet[name] = movingMember
				} else {
					if res.typ != nil {
						targetMember.SetType(res.typ)
					}
					mergedMember = createPhi(name, []Value{movingMember, targetMember})
					if !utils.IsNil(mergedMember) {
						setMemberCallRelationship(replacement, key, mergedMember)
						holderRet[name] = mergedMember
					}
				}
			}

			if key.GetId() == target.GetId() {
				toKey := replacement
				setMemberCallRelationship(container, toKey, member)
				if utils.IsNil(toMember) {
					toMember = member
				}
			}
			if utils.IsNil(toMember) {
				toMember = builder.ReadMemberCallValue(replacement, key)
			}

			if toMemberObj := GetLatestObject(toMember); utils.IsNil(toMemberObj) || toMemberObj.GetId() == target.GetId() || toMemberObj.GetId() == holder.GetId() {
				fixBranch(toMember, replacement, key)
			}

			// Check if we need to recursively process nested members BEFORE modifying member
			// This must be done before ReplaceAllValue/DeleteInst which may invalidate member's state
			shouldRecurse := false
			var memberForRecursion Value
			var toMemberForRecursion Value

			// 处理 IsObject 情况：如果 member 是对象，需要递归处理其成员
			if member.IsObject() && !utils.IsNil(toMember) {
				memberID := member.GetId()
				toMemberID := toMember.GetId()
				_, memberVisited := visited[memberID]
				_, toMemberVisited := visited[toMemberID]
				holderID := holder.GetId()
				targetID := target.GetId()
				replacementID := replacement.GetId()

				if memberID == toMemberID {
					if !memberVisited &&
						memberID != holderID &&
						memberID != targetID &&
						memberID != replacementID {
						memberForRecursion = member
						toMemberForRecursion = toMember
						shouldRecurse = true
					}
				} else if !memberVisited && !toMemberVisited &&
					memberID != holderID && toMemberID != holderID &&
					memberID != targetID && toMemberID != targetID &&
					memberID != replacementID && toMemberID != replacementID {
					memberForRecursion = member
					toMemberForRecursion = toMember
					shouldRecurse = true
				}
			}

			// 处理 IsMember 情况：如果 member 是成员访问，需要递归处理其 object
			if !shouldRecurse && member.IsMember() && !utils.IsNil(toMember) {
				memberObj := GetLatestObject(member)
				if !utils.IsNil(memberObj) && memberObj.IsObject() {
					memberObjID := memberObj.GetId()
					if memberObjID == target.GetId() {
						// member 的 object 就是 target，需要替换为 replacement
						memberForRecursion = memberObj
						toMemberForRecursion = replacement
						shouldRecurse = true
					} else {
						toMemberObj := GetLatestObject(toMember)
						if !utils.IsNil(toMemberObj) && toMemberObj.IsObject() && memberObjID != toMemberObj.GetId() {
							_, memberObjVisited := visited[memberObjID]
							_, toMemberObjVisited := visited[toMemberObj.GetId()]
							holderID := holder.GetId()
							targetID := target.GetId()
							replacementID := replacement.GetId()

							if !memberObjVisited && !toMemberObjVisited &&
								memberObjID != holderID && toMemberObj.GetId() != holderID &&
								memberObjID != targetID && toMemberObj.GetId() != targetID &&
								memberObjID != replacementID && toMemberObj.GetId() != replacementID {
								memberForRecursion = memberObj
								toMemberForRecursion = toMemberObj
								shouldRecurse = true
							}
						}
					}
				}
			}

			// 替换 member 的值引用
			memberT := member
			switch {
			case !utils.IsNil(mergedMember):
				memberT = mergedMember
			case !utils.IsNil(toMember) && toMember.GetId() == member.GetId():
				memberT = member
			case member.GetOpcode() == SSAOpcodeBinOp || member.GetOpcode() == SSAOpcodeUnOp:
				// 保留原始指令供后续替换
			default:
				ReplaceAllValue(member, toMember)
				DeleteInst(member)
				memberT = toMember
			}

			// 递归处理嵌套成员
			if shouldRecurse && !utils.IsNil(memberForRecursion) && !utils.IsNil(toMemberForRecursion) {
				for n, v2 := range replaceMemberCallRecursive(memberForRecursion, toMemberForRecursion, visited) {
					holderRet[n] = v2
				}
			}
			if !member.IsObject() {
				holderRet[name] = memberT
			}
		}

		// 处理 holder 的所有成员，先处理非 Call 成员，再处理 Call 成员
		if holder.IsObject() {
			callPairs := make([]MemberPair, 0)
			memberPairs := holder.GetMemberPairs()
			sourceKeyOverrides = repeatedMemberSourceKeys(memberPairs)
			for _, pair := range memberPairs {
				if _, ok := ToCall(pair.Member); ok {
					callPairs = append(callPairs, pair)
					continue
				}
				replace(holder, pair.Key, pair.Member)
			}
			for _, pair := range callPairs {
				replace(holder, pair.Key, pair.Member)
			}
		}

		return holderRet
	}

	for n, v2 := range replaceMemberCallRecursive(old, replacement, visited) {
		ret[n] = v2
	}

	return ret
}

func pickMemberKey(member, fallback Value) Value {
	if !utils.IsNil(member) {
		if k := GetLatestKey(member); !utils.IsNil(k) {
			return k
		}
	}
	return fallback
}

func repeatedMemberSourceKeys(pairs []MemberPair) map[int64]Value {
	firstKeys := make(map[int64]Value)
	sourceKeys := make(map[int64]Value)
	for _, pair := range pairs {
		if utils.IsNil(pair.Member) || utils.IsNil(pair.Key) {
			continue
		}
		memberID := pair.Member.GetId()
		if firstKey, ok := firstKeys[memberID]; ok {
			if memberKeySignature(firstKey) != memberKeySignature(pair.Key) {
				sourceKeys[memberID] = firstKey
			}
			continue
		}
		firstKeys[memberID] = pair.Key
	}
	return sourceKeys
}

func pickUndefinedMemberSourceKey(member, fallback Value) Value {
	pairs := GetObjectKeyPairs(member)
	if len(pairs) == 0 || utils.IsNil(fallback) {
		return fallback
	}
	fallbackSignature := memberKeySignature(fallback)
	latestFallbackIndex := -1
	for index := len(pairs) - 1; index >= 0; index-- {
		if memberKeySignature(pairs[index].Key) == fallbackSignature {
			latestFallbackIndex = index
			break
		}
	}
	if latestFallbackIndex == len(pairs)-1 {
		for index := latestFallbackIndex - 1; index >= 0; index-- {
			if memberKeySignature(pairs[index].Key) != fallbackSignature {
				return pairs[index].Key
			}
		}
	}
	return fallback
}
