package ssa

// Legacy member accessors keep main-branch ssaapi compiling while pair-first
// member relations are rolled out. Field-sensitive analysis updates live on
// feature/ssa/field-sensitive-analysis.

func (n *anValue) GetObject() Value {
	pairs := n.GetObjectKeyPairs()
	if len(pairs) == 0 {
		return nil
	}
	return pairs[len(pairs)-1].Object
}

func (n *anValue) GetKey() Value {
	pairs := n.GetObjectKeyPairs()
	if len(pairs) == 0 {
		return nil
	}
	return pairs[len(pairs)-1].Key
}

func (n *anValue) GetAllMember() map[Value]Value {
	pairs := n.GetMemberPairs()
	if len(pairs) == 0 {
		return make(map[Value]Value)
	}
	seen := make(map[string]struct{}, len(pairs))
	ret := make(map[Value]Value)
	for index := len(pairs) - 1; index >= 0; index-- {
		pair := pairs[index]
		signature := memberKeySignature(pair.Key)
		if _, ok := seen[signature]; ok {
			continue
		}
		seen[signature] = struct{}{}
		ret[pair.Key] = pair.Member
	}
	return ret
}

func (n *anValue) GetStringMember(key string) (Value, bool) {
	members := n.GetMembersByKeyString(key)
	if len(members) == 0 {
		return nil, false
	}
	return members[0], true
}
