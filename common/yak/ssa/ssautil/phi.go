package ssautil

type PhiContext[T comparable] struct {
	scope *ScopedVersionedTable[T]
	name  string
	phi   []*Versioned[T]
}

func (p *PhiContext[T]) AddPhi(i *Versioned[T]) {
	p.phi = append(p.phi, i)
}

// ProducePhi produce the phi for the captured variable
// note: this function will be called after the block sealed
// func ProducePhi[T comparable](
// 	factory func(...T) T,
// 	scopes ...*ScopedVersionedTable[T]) {
// var ctxs = map[*ScopedVersionedTable[T]]map[string]*PhiContext[T]{}
// for _, v := range scopes {
// 	if v.IsRoot() {
// 		return
// 	}
// 	for _, name := range v.GetAllCapturedVariableNames() {
// 		capturedValue := v.parent.ReadValue(name)
// 		if capturedValue == nil {
// 			continue
// 		}
// 		currentValue := v.GetLatestVersionInCurrentLexicalScope(name)
// 		if currentValue == nil {
// 			continue
// 		}
// 		targetScope := capturedValue.scope
// 		if _, ok := ctxs[targetScope]; !ok {
// 			ctxs[targetScope] = map[string]*PhiContext[T]{}
// 		}
// 		if _, ok := ctxs[targetScope][name]; !ok {
// 			ctxs[targetScope][name] = &PhiContext[T]{
// 				scope: targetScope,
// 				name:  name,
// 				phi:   []*Versioned[T]{},
// 			}
// 		}
// 		ctxs[targetScope][name].AddPhi(currentValue)
// 	}
// }

// for _, vars := range ctxs {
// 	for _, ctx := range vars {
// 		var vals = make([]T, 0, len(ctx.phi))
// 		for _, v := range ctx.phi {
// 			vals = append(vals, v.Value)
// 		}
// 		phi := factory(vals...)
// 		result := ctx.scope.CreateVariable(ctx.name, phi)
// 		result.SetPhi(true)
// 	}
// }
// }

// ForEachCapturedVariable call the handler for each captured by base scope Variable
func (ps *ScopedVersionedTable[T]) ForEachCapturedVariable(base *ScopedVersionedTable[T], handler func(name string, ver VersionedIF[T])) {
	ps.captured.ForEach(func(name string, ver VersionedIF[T]) bool {
		baseVariable := base.ReadVariable(name)
		if baseVariable == nil {
			// not exist in base scope, this variable just set in sub-scope,
			// just skip
			return true
		}

		if baseVariable.GetCaptured() != ver.GetCaptured() {
			return true
		}

		handler(name, ver)
		return true
	})
}

func (s *ScopedVersionedTable[T]) CoverBy(scope *ScopedVersionedTable[T]) {
	if scope == nil {
		panic("cover scope is nil")
	}

	scope.ForEachCapturedVariable(s, func(name string, ver VersionedIF[T]) {
		s.WriteVariable(name, ver.GetValue())
	})
}

// Merge merge the sub-scope to current scope,
// if hasSelf is true: the current scope will be merged to the result
func (base *ScopedVersionedTable[T]) Merge(
	hasSelf bool,
	merge func(name string, t []T) T,
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
		base.WriteVariable(name, ret)
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
func (s *ScopedVersionedTable[T]) Spin(header, latch *ScopedVersionedTable[T], handler func(name string, phi, origin, last T) T) {
	s.incomingPhi.ForEach(func(name string, ver VersionedIF[T]) bool {
		last := latch.ReadValue(name)
		origin := header.ReadValue(name)
		res := handler(name, ver.GetValue(), origin, last)
		s.WriteVariable(name, res)
		return true
	})
}

func (s *ScopedVersionedTable[T]) SetSpin(create func(string) T) {
	s.spin = true
	s.CreateEmptyPhi = create
}
