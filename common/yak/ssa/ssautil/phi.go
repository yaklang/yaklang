package ssautil

import "github.com/yaklang/yaklang/common/log"

// ForEachCapturedVariable call the handler for each captured by base scope Variable
func ForEachCapturedVariable[T versionedValue](
	scope ScopedVersionedTableIF[T],
	base ScopedVersionedTableIF[T],
	handler VariableHandler[T],
) {
	scope.ForEachCapturedVariable(func(name string, ver VersionedIF[T]) {
		if ver.CanCaptureInScope(base) || scope.GetForceCapture() {
			handler(name, ver)
		}
	})
}

func ForEachCapturedSideEffect[T versionedValue](
	scope ScopedVersionedTableIF[T],
	base ScopedVersionedTableIF[T],
	handler func(string, []VersionedIF[T]),
) {
	scope.ForEachCapturedSideEffect(func(name string, ver []VersionedIF[T]) {
		if ver[0].GetValue().IsSideEffect() {
			handler(name, ver)
		} else {
			log.Warnf("link-SideEffect must be side effect type")
		}
	})
}

func (base *ScopedVersionedTable[T]) CoverBy(scope ScopedVersionedTableIF[T]) {
	if scope == nil {
		panic("cover scope is nil")
	}

	baseScope := ScopedVersionedTableIF[T](base)
	ForEachCapturedVariable(scope, baseScope, func(name string, ver VersionedIF[T]) {
		// v := base.CreateVariable(name, false)
		base.AssignVariable(ver, ver.GetValue())
	})
	ForEachCapturedSideEffect(scope, baseScope, func(name string, ver []VersionedIF[T]) {
		// v := base.CreateVariable(name, false)
		if baseScope.GetParent() == ver[1].GetScope() {
			baseScope.AssignVariable(ver[0], ver[0].GetValue())
		} else {
			baseScope.SetCapturedSideEffect(ver[0].GetName(), ver[0], ver[1])
		}
	})
}

// Merge merge the sub-scope to current scope,
// if hasSelf is true: the current scope will be merged to the result
func (base *ScopedVersionedTable[T]) Merge(
	hasSelf bool,
	merge MergeHandle[T],
	subScopes ...ScopedVersionedTableIF[T],
) {
	var zero T
	// subScopes := s.child
	// handler []T must sort same with sub-scope
	length := len(subScopes)
	if hasSelf {
		length++
	}
	tmpVariable := make(map[VersionedIF[T]][]T)
	tmpName := make(map[string][]T)
	phi := make(map[VersionedIF[T]]T)
	_ = tmpVariable
	_ = tmpName

	addPhiContent := func(index int, name string, ver VersionedIF[T], sub ScopedVersionedTableIF[T]) {
		variable := ver
		if find := sub.GetParent().ReadVariable(name); find != nil {
			if sub.IsSameOrSubScope(find.GetScope()) {
				variable = find
			}
		}

		m, ok := tmpVariable[variable]
		if !ok {
			m = make([]T, length)
		}
		m[index] = ver.GetValue()
		tmpVariable[variable] = m
	}
	generatePhi := func(name string, m []T) {
		variable := base.ReadVariable(name)
		if variable == nil {
			return
		}
		origin := variable.GetValue()

		// fill the missing value
		// if len(m) != length {
		if hasSelf {
			// m[s] = origin
			m[len(m)-1] = origin
		}
		for index := range subScopes {
			v := m[index]
			if v == zero {
				m[index] = origin
			}
		}

		// generate phi
		// handler(name, m)
		//if len(m) > 1 {
		//	log.Infof("merge phi %s: edges count: %v", name, len(m))
		//}
		ret := merge(name, m)
		if base.GetParent().GetParent() == variable.GetScope() {
			v := base.CreateVariable(name, variable.GetLocal())
			phi[v] = ret
		} else {
			v := base.CreateVariable(name, false)
			phi[v] = ret
		}
	}

	defer func() {
		for v, ret := range phi {
			base.tryRegisterCapturedVariable(v.GetName(), v)
			base.AssignVariable(v, ret)
		}
	}()

	baseScope := ScopedVersionedTableIF[T](base)
	for index, sub := range subScopes {
		ForEachCapturedVariable(sub, baseScope, func(name string, ver VersionedIF[T]) {
			addPhiContent(index, name, ver, sub)
		})
		ForEachCapturedSideEffect(sub, baseScope, func(name string, ver []VersionedIF[T]) {
			addPhiContent(index, name, ver[0], sub)
			baseScope.SetCapturedSideEffect(ver[0].GetName(), ver[0], ver[1])
		})
	}

	for ver, m := range tmpVariable {
		generatePhi(ver.GetName(), m)
	}
}

// this handler merge [origin, last] to phi
func (condition *ScopedVersionedTable[T]) Spin(
	header, latch ScopedVersionedTableIF[T],
	handler SpinHandle[T],
) {
	condition.spin = false
	condition.createEmptyPhi = nil
	for name, ver := range condition.linkIncomingPhi {
		last := latch.ReadValue(name)
		origin := header.ReadValue(name)
		res := handler(name, ver.GetValue(), origin, last)
		for name, value := range res {
			v := condition.CreateVariable(name, ver.GetLocal())
			condition.AssignVariable(v, value)
		}
	}
}

func (s *ScopedVersionedTable[T]) SetSpin(create func(string) T) {
	s.spin = true
	s.createEmptyPhi = create
}

func (s *ScopedVersionedTable[T]) SetSpinRaw(b bool) {
	s.spin = b
}
