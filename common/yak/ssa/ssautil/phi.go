package ssautil

// ForEachCapturedVariable call the handler for each captured by base scope Variable
func ForEachCapturedVariable[T versionedValue](
	scope ScopedVersionedTableIF[T],
	base ScopedVersionedTableIF[T],
	handler CaptureVariableHandler[T],
) {
	scope.ForEachCapturedVariable(func(name string, ver VersionedIF[T]) {
		if ver.CanCaptureInScope(base) {
			handler(name, ver)
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
func (s *ScopedVersionedTable[T]) Spin(
	header, latch ScopedVersionedTableIF[T],
	handler SpinHandle[T],
) {
	s.spin = false
	s.createEmptyPhi = nil
	s.incomingPhi.ForEach(func(name string, ver VersionedIF[T]) bool {
		last := latch.ReadValue(name)
		origin := header.ReadValue(name)
		res := handler(name, ver.GetValue(), origin, last)
		for name, value := range res {
			v := s.CreateVariable(name, false)
			s.AssignVariable(v, value)
		}
		return true
	})
}

func (s *ScopedVersionedTable[T]) SetSpin(create func(string) T) {
	s.spin = true
	s.createEmptyPhi = create
}

func (s *ScopedVersionedTable[T]) SetSpinRaw(b bool) {
	s.spin = b
}
