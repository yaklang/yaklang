package ssautil

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
		if ver[0].GetValue().IsSideEffect() || ver[0].GetValue().IsPhi() {
			handler(name, ver)
		} else {
			log.Warnf("link-SideEffect must be side effect or phi")
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
	hasSelf, setLocal bool,
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
	tmpPhiScope := make(map[VersionedIF[T]]T)
	tmpPhiCapture := make(map[VersionedIF[T]]T)
	tmpIsCapture := make(map[VersionedIF[T]]bool)

	addPhiContent := func(index int, name string, ver VersionedIF[T], sub ScopedVersionedTableIF[T], forceScope ...ScopedVersionedTableIF[T]) VersionedIF[T] {
		variable := ver
		parentScope := sub.GetParent()
		if len(forceScope) > 0 {
			parentScope = forceScope[0]
		}

		var Check func(scope ScopedVersionedTableIF[T])
		Check = func(scope ScopedVersionedTableIF[T]) {
			if scope == nil {
				return
			}
			if find := scope.ReadVariable(name); find != nil {
				if sub.IsSameOrSubScope(find.GetScope()) {
					variable = find
				}
				if !find.GetLocal() {
					Check(scope.GetParent())
				}
			}
		}
		Check(parentScope)

		m, ok := tmpVariable[variable]
		if !ok {
			m = make([]T, length)
		}
		m[index] = ver.GetValue()
		tmpVariable[variable] = m
		return variable
	}

	// pointerCheck := make(map[string]T)
	generatePhi := func(ver VersionedIF[T], m []T, canCapture bool) {
		var v VersionedIF[T]
		name := ver.GetName()
		variable := base.ReadVariable(name)
		if variable == nil {
			return
		}
		origin := variable.GetValue()
		if capturedVariable := variable.GetCaptured(); capturedVariable.GetLocal() {
			if canCapture {
				origin = ver.GetValue()
			}
		}

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

		// rev := regexp.MustCompile(`#(\d+)\.@value`)
		// idvs := rev.FindStringSubmatch(name)

		ret := merge(name, m)
		// if len(idvs) > 0 {
		// 	pointerCheck[fmt.Sprintf("#%s.@pointer", idvs[1])] = ret
		// }

		if base.GetParent().GetParent() == variable.GetScope() && setLocal {
			v = base.CreateVariable(name, variable.GetLocal())
		} else {
			v = base.CreateVariable(name, false)
		}
		// v.SetPointHandler(variable.GetPointHandler())
		v.SetKind(variable.GetKind())
		if canCapture {
			// 在当前scope中尝试修改外部的某个variable
			tmpPhiCapture[v] = ret
		}
		if variable.GetCaptured().GetScope().Compare(ver.GetScope()) {
			tmpPhiScope[v] = ret
		}
	}

	defer func() {
		for v, ret := range tmpPhiScope {
			// if v.GetKind() == PointerVariable {

			// } else {
			base.AssignVariable(v, ret)
			base.tryRegisterCapturedVariable(v.GetName(), v)
			// }
		}
		for v, ret := range tmpPhiCapture {
			err := v.Assign(ret)
			if err != nil {
				log.Warnf("BUG: variable.Assign error: %v", err)
				return
			}
			base.ChangeCapturedSideEffect(v.GetName(), v)
		}
	}()

	baseScope := ScopedVersionedTableIF[T](base)
	for index, sub := range subScopes {
		ForEachCapturedVariable(sub, baseScope, func(name string, ver VersionedIF[T]) {
			tmpIsCapture[addPhiContent(index, name, ver, sub)] = false
		})
		ForEachCapturedSideEffect(sub, baseScope, func(name string, ver []VersionedIF[T]) {
			tmpIsCapture[addPhiContent(index, name, ver[0], sub, ver[1].GetScope())] = true
			baseScope.SetCapturedSideEffect(ver[0].GetName(), ver[0], ver[1])
		})
	}

	for ver, m := range tmpVariable {
		generatePhi(ver, m, tmpIsCapture[ver])
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
		_ = res
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
