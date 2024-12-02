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
	handler VariableHandler[T],
) {
	scope.ForEachCapturedSideEffect(func(name string, ver VersionedIF[T]) {
		if ver.GetValue().IsSideEffect() {
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
	ForEachCapturedSideEffect(scope, baseScope, func(name string, ver VersionedIF[T]) {
		// v := base.CreateVariable(name, false)
		base.AssignVariable(ver, ver.GetValue())
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
	tmp := make(map[string][]T)

	addPhiContent := func(index int, name string, ver VersionedIF[T]) {
		m, ok := tmp[name]
		if !ok {
			m = make([]T, length)
		}
		m[index] = ver.GetValue()
		tmp[name] = m
	}
	generatePhi := func(name string, m []T) {
		origin := base.ReadValue(name)
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
		v := base.CreateVariable(name, false)
		base.AssignVariable(v, ret)
	}

	baseScope := ScopedVersionedTableIF[T](base)
	for index, sub := range subScopes {
		ForEachCapturedVariable(sub, baseScope, func(name string, ver VersionedIF[T]) {
			addPhiContent(index, name, ver)
		})
	}

	for name, m := range tmp {
		generatePhi(name, m)
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
