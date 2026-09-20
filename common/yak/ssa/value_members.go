package ssa

import (
	"fmt"
	"slices"

	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// Published relation prefixes are immutable. Readers resolve IDs after releasing
// the value lock; replacing or removing relations must allocate a new slice.
func (n *anValue) snapshotMemberPairs() []memberPairRecord {
	if n == nil {
		return nil
	}
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	return n.memberPairs
}

func (n *anValue) snapshotOwnerPairs() []ownerPairRecord {
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	return n.ownerPairs
}

func (n *anValue) appendMemberPairIDs(keyID, memberID int64) {
	if keyID <= 0 || memberID <= 0 {
		return
	}
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	pair := memberPairRecord{key: keyID, member: memberID}
	if len(n.memberPairs) > 0 && n.memberPairs[len(n.memberPairs)-1] == pair {
		return
	}
	n.memberPairs = append(n.memberPairs, pair)
}

func (n *anValue) appendOwnerPairIDs(objectID, keyID int64) {
	if objectID <= 0 || keyID <= 0 {
		return
	}
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	pair := ownerPairRecord{object: objectID, key: keyID}
	if len(n.ownerPairs) > 0 && n.ownerPairs[len(n.ownerPairs)-1] == pair {
		return
	}
	for _, existed := range n.ownerPairs {
		if existed == pair {
			return
		}
	}
	n.ownerPairs = append(n.ownerPairs, pair)
}

func (n *anValue) memberPairMatchesKey(pair memberPairRecord, key Value) bool {
	if key == nil {
		return false
	}
	if pair.key == key.GetId() {
		return true
	}
	pairKey, ok := n.GetValueById(pair.key)
	if !ok {
		return false
	}
	return sameConstKeyValue(pairKey, key)
}

func (n *anValue) IsMember() bool {
	return len(n.snapshotOwnerPairs()) > 0
}

func (n *anValue) IsObject() bool {
	return len(n.snapshotMemberPairs()) > 0
}

func (n *anValue) AddMember(k, v Value) {
	if k == nil || v == nil {
		return
	}
	n.appendMemberPairIDs(k.GetId(), v.GetId())
}

func (n *anValue) DeleteMember(k Value) {
	if k == nil {
		return
	}
	// Resolve keys outside the relation lock: resolution may load IR and
	// traverse another value. Remove the observed matching keys atomically.
	remove := make(map[int64]bool)
	for _, pair := range n.snapshotMemberPairs() {
		if n.memberPairMatchesKey(pair, k) {
			remove[pair.key] = true
		}
	}
	if len(remove) == 0 {
		return
	}
	n.relationsMu.Lock()
	defer n.relationsMu.Unlock()
	n.memberPairs = slices.DeleteFunc(slices.Clone(n.memberPairs), func(pair memberPairRecord) bool {
		return remove[pair.key]
	})
}

func sameConstKeyValue(lhs, rhs Value) bool {
	if lhs == nil || rhs == nil {
		return false
	}
	leftConst, leftOK := ToConstInst(lhs)
	rightConst, rightOK := ToConstInst(rhs)
	if !leftOK || !rightOK {
		return false
	}
	return fmt.Sprint(leftConst.value) == fmt.Sprint(rightConst.value)
}

func (n *anValue) AddObjectKeyPair(obj, key Value) {
	if obj == nil || key == nil {
		return
	}
	n.appendOwnerPairIDs(obj.GetId(), key.GetId())
}

func (n *anValue) resolveLinkedValue(id int64) (Value, bool) {
	if id <= 0 {
		return nil, false
	}
	if val, ok := n.GetValueById(id); ok {
		return val, true
	}
	prog := n.GetProgram()
	if prog == nil {
		return nil, false
	}
	// A missing memory-mode ID cannot be recovered from a database. Apart
	// from unnecessary I/O, consulting the global DB can resolve an unrelated
	// program's instruction and start database initialization in a body worker.
	if prog.DatabaseKind == ProgramCacheMemory {
		return nil, false
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	progName := applicationProgramName(prog)
	if progName != "" {
		ir := ssadb.GetIrCodeByIdFast(ssadb.GetDB(), progName, id)
		if ir != nil && ir.Opcode == int64(SSAOpcodeConstInst) && ir.String != "" {
			return NewConst(ir.String), true
		}
	}
	cache := n.getProgramCache()
	if cache != nil {
		if loaded := cache.GetInstruction(id); loaded != nil {
			if val, ok := ToValue(loaded); ok {
				return val, true
			}
		}
	}
	if progName != "" {
		if inst, err := NewLazyInstruction(app, id); err == nil && inst != nil {
			if val, ok := ToValue(inst); ok {
				return val, true
			}
		}
	}
	return nil, false
}

func (n *anValue) GetObjectKeyPairs() []ObjectKeyPair {
	pairs := n.snapshotOwnerPairs()
	ret := make([]ObjectKeyPair, 0, len(pairs))
	for _, pair := range pairs {
		if pair.object <= 0 || pair.key <= 0 {
			continue
		}
		obj, ok1 := n.resolveLinkedValue(pair.object)
		key, ok2 := n.resolveLinkedValue(pair.key)
		if !ok1 || !ok2 {
			continue
		}
		ret = append(ret, ObjectKeyPair{Object: obj, Key: key})
	}
	return ret
}

func (n *anValue) GetMembersByExactKey(key Value) []Value {
	if key == nil {
		return nil
	}
	pairs := n.snapshotMemberPairs()
	ret := make([]Value, 0)
	for index := len(pairs) - 1; index >= 0; index-- {
		pair := pairs[index]
		if !n.memberPairMatchesKey(pair, key) {
			continue
		}
		member, ok := n.resolveLinkedValue(pair.member)
		if ok {
			ret = append(ret, member)
		}
	}
	return ret
}

func (n *anValue) GetMembersByKeyString(key string) []Value {
	if key == "" {
		return nil
	}
	pairs := n.snapshotMemberPairs()
	ret := make([]Value, 0)
	for index := len(pairs) - 1; index >= 0; index-- {
		pair := pairs[index]
		keyVal, ok1 := n.resolveLinkedValue(pair.key)
		member, ok2 := n.resolveLinkedValue(pair.member)
		if !ok1 || !ok2 {
			continue
		}
		lit, ok := ToConstInst(keyVal)
		if !ok || fmt.Sprint(lit.value) != key {
			continue
		}
		ret = append(ret, member)
	}
	return ret
}

func (n *anValue) GetMemberPairs() []MemberPair {
	pairs := n.snapshotMemberPairs()
	ret := make([]MemberPair, 0, len(pairs))
	for _, pair := range pairs {
		key, ok1 := n.resolveLinkedValue(pair.key)
		member, ok2 := n.resolveLinkedValue(pair.member)
		if !ok1 || !ok2 {
			continue
		}
		ret = append(ret, MemberPair{Key: key, Member: member})
	}
	return ret
}
