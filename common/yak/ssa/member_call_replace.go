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
			if currentObj := root.GetObject(); !utils.IsNil(currentObj) && currentObj.GetId() != target.GetId() && currentObj.GetId() != holder.GetId() {
				// 已指向有效对象，无需修改
				return
			}
			root.SetObject(targetObj)
			if root.IsMember() {
				currentKey := root.GetKey()
				if utils.IsNil(currentKey) || currentKey.GetId() == target.GetId() || currentKey.GetId() == holder.GetId() {
					root.SetKey(pickMemberKey(root, rootKey))
				}
			}
		}

		// 替换成员：处理 container[key] 的替换
		replace := func(container Value, key Value, member Value) {
			if utils.IsNil(key) || utils.IsNil(member) {
				return
			}

			trueKey := member.GetKey()
			if _, ok := container.GetMember(key); ok {
				container.DeleteMember(key)
			}

			res := checkCanMemberCallExist(replacement, key)
			trueRes := checkCanMemberCallExist(replacement, trueKey)
			name, typ := res.name, res.typ
			toMember := builder.PeekValue(trueRes.name)

			if member.GetOpcode() != SSAOpcodeUndefined {
				member.SetName(name)
				member.SetType(typ)
				setMemberCallRelationship(replacement, key, member)
				if utils.IsNil(toMember) {
					holderRet[name] = member
				} else {
					if res.typ != nil {
						toMember.SetType(res.typ)
					}
					holderRet[name] = createPhi(name, []Value{toMember, member})
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

			if utils.IsNil(toMember.GetObject()) || toMember.GetObject().GetId() == target.GetId() || toMember.GetObject().GetId() == holder.GetId() {
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
				if memberID != toMemberID {
					_, memberVisited := visited[memberID]
					_, toMemberVisited := visited[toMemberID]
					holderID := holder.GetId()
					targetID := target.GetId()
					replacementID := replacement.GetId()

					// 检查循环引用和已访问状态
					if !memberVisited && !toMemberVisited &&
						memberID != holderID && toMemberID != holderID &&
						memberID != targetID && toMemberID != targetID &&
						memberID != replacementID && toMemberID != replacementID {
						memberForRecursion = member
						toMemberForRecursion = toMember
						shouldRecurse = true
					}
				}
			}

			// 处理 IsMember 情况：如果 member 是成员访问，需要递归处理其 object
			if !shouldRecurse && member.IsMember() && !utils.IsNil(toMember) {
				memberObj := member.GetObject()
				if !utils.IsNil(memberObj) && memberObj.IsObject() {
					memberObjID := memberObj.GetId()
					if memberObjID == target.GetId() {
						// member 的 object 就是 target，需要替换为 replacement
						memberForRecursion = memberObj
						toMemberForRecursion = replacement
						shouldRecurse = true
					} else {
						toMemberObj := toMember.GetObject()
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
			switch member.GetOpcode() {
			case SSAOpcodeBinOp, SSAOpcodeUnOp:
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
			callMap := make(map[Value]Value)
			for key, member := range holder.GetAllMember() {
				if _, ok := ToCall(member); ok {
					callMap[key] = member
					continue
				}
				replace(holder, key, member)
			}
			for key, member := range callMap {
				replace(holder, key, member)
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
		if k := member.GetKey(); !utils.IsNil(k) {
			return k
		}
	}
	return fallback
}
