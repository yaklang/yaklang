package ssa

// inheritPhiMemberReceiver preserves a bound member across control-flow joins
// only when every incoming value denotes the same object/key. In particular,
// a method Phi must not lose its implicit receiver just because two branches
// materialized separate undefined method values in their own scopes.
func inheritPhiMemberReceiver(phi *Phi, incoming Values) {
	method, ok := phi.GetType().(*FunctionType)
	if !ok || !method.IsMethod {
		return
	}
	var common ObjectKeyPair
	visited := make(map[int64]bool)
	var visit func(Value) bool
	visit = func(value Value) bool {
		if value == nil {
			return false
		}
		if visited[value.GetId()] {
			return true
		}
		visited[value.GetId()] = true
		pairs := GetObjectKeyPairs(value)
		if len(pairs) == 0 {
			if nested, ok := ToPhi(value); ok && len(nested.Edge) > 0 {
				for _, id := range nested.Edge {
					edge, ok := nested.GetValueById(id)
					if !ok || !visit(edge) {
						return false
					}
				}
				return true
			}
			return false
		}
		pair := pairs[len(pairs)-1]
		if common.Object == nil {
			common = pair
			return true
		}
		return common.Object.GetId() == pair.Object.GetId() &&
			(common.Key.GetId() == pair.Key.GetId() || sameConstKeyValue(common.Key, pair.Key))
	}
	for _, value := range incoming {
		if !visit(value) {
			return
		}
	}
	if common.Object != nil {
		phi.AddObjectKeyPair(common.Object, common.Key)
	}
}
