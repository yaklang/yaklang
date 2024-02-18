package ssautil

type SpinHandle[T comparable] func(string, T, T, T) map[string]T
type MergeHandle[T comparable] func(string, []T) T

// ForEachCapturedVariable call the handler for each captured by base scope Variable
func (ps *ScopedVersionedTable[T]) ForEachCapturedVariable(base *ScopedVersionedTable[T], handler func(name string, ver VersionedIF[T])) {
	ps.captured.ForEach(func(name string, ver VersionedIF[T]) bool {
		if ver.CanCaptureInScope(base) {
			handler(name, ver)
		}
		return true
	})
}

func (s *ScopedVersionedTable[T]) CoverBy(scope *ScopedVersionedTable[T]) {
	if scope == nil {
		panic("cover scope is nil")
	}

	scope.ForEachCapturedVariable(s, func(name string, ver VersionedIF[T]) {
		s.writeVariable(name, ver.GetValue())
	})
}

// Merge merge the sub-scope to current scope,
// if hasSelf is true: the current scope will be merged to the result
func (base *ScopedVersionedTable[T]) Merge(
	hasSelf bool,
	merge MergeHandle[T],
	subScopes ...*ScopedVersionedTable[T],
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
		ret := merge(name, m)
		base.writeVariable(name, ret)
	}

	for index, sub := range subScopes {
		sub.ForEachCapturedVariable(base, func(name string, ver VersionedIF[T]) {
			addPhiContent(index, name, ver)
		})
	}

	for name, m := range tmp {
		generatePhi(name, m)
	}
}

// this handler merge [origin, last] to phi
func (s *ScopedVersionedTable[T]) Spin(
	header, latch *ScopedVersionedTable[T],
	handler SpinHandle[T],
) {
	s.incomingPhi.ForEach(func(name string, ver VersionedIF[T]) bool {
		last := latch.ReadValue(name)
		origin := header.ReadValue(name)
		res := handler(name, ver.GetValue(), origin, last)
		for name, value := range res {
			s.writeVariable(name, value)
		}
		return true
	})
}

func (s *ScopedVersionedTable[T]) SetSpin(create func(string) T) {
	s.spin = true
	s.CreateEmptyPhi = create
}
