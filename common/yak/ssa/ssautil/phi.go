package ssautil

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

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
	if utils.IsNil(scope) {
		log.Errorf("cover scope is nil")
		return
	}

	baseScope := ScopedVersionedTableIF[T](base)
	ForEachCapturedVariable(scope, baseScope, func(name string, ver VersionedIF[T]) {
		base.AssignVariable(ver, ver.GetValue())
	})
	ForEachCapturedSideEffect(scope, baseScope, func(name string, ver []VersionedIF[T]) {
		if baseScope.GetParent() == ver[1].GetScope() {
			baseScope.AssignVariable(ver[0], ver[0].GetValue())
		} else {
			baseScope.SetCapturedSideEffect(ver[0].GetName(), ver[0], ver[1])
		}
	})
}

func (base *ScopedVersionedTable[T]) Merge(
	hasSelf, setLocal bool,
	merge MergeHandle[T],
	subScopes ...ScopedVersionedTableIF[T],
) {
	var zero T
	length := len(subScopes)
	if hasSelf {
		length++
	}
	tmpVariable := omap.NewEmptyOrderedMap[VersionedIF[T], []T]()
	tmpPhiScope := omap.NewEmptyOrderedMap[VersionedIF[T], T]()
	tmpPhiCapture := omap.NewEmptyOrderedMap[VersionedIF[T], T]()
	tmpIsCapture := omap.NewEmptyOrderedMap[VersionedIF[T], bool]()

	addPhiContent := func(index int, name string, ver VersionedIF[T], sub ScopedVersionedTableIF[T], forceScope ...ScopedVersionedTableIF[T]) VersionedIF[T] {
		variable := ver
		parentScope := sub.GetParent()
		if len(forceScope) > 0 {
			parentScope = forceScope[0]
		}

		var Check func(scope ScopedVersionedTableIF[T])
		Check = func(scope ScopedVersionedTableIF[T]) {
			if utils.IsNil(scope) {
				return
			}
			if find := scope.ReadVariable(name); !utils.IsNil(find) {
				if sub.IsSameOrSubScope(find.GetScope()) {
					variable = find
				}
				if !find.GetLocal() {
					Check(scope.GetParent())
				}
			}
		}
		Check(parentScope)

		m, ok := tmpVariable.Get(variable)
		if !ok {
			m = make([]T, length)
			tmpVariable.Set(variable, m)
		}
		m[index] = ver.GetValue()
		tmpVariable.Set(variable, m)
		return variable
	}

	generatePhi := func(ver VersionedIF[T], m []T, canCapture bool) {
		var v VersionedIF[T]
		name := ver.GetName()
		variable := base.ReadVariable(name)
		if utils.IsNil(variable) {
			return
		}
		origin := variable.GetValue()
		if capturedVariable := variable.GetCaptured(); capturedVariable.GetLocal() {
			if canCapture {
				origin = ver.GetValue()
			}
		}

		if hasSelf {
			m[len(m)-1] = origin
		}
		for index := range subScopes {
			v := m[index]
			if v == zero {
				m[index] = origin
			}
		}

		ret := merge(name, m)

		if base.GetParent().GetParent() == variable.GetScope() && setLocal {
			v = base.CreateVariable(name, variable.GetLocal())
		} else {
			v = base.CreateVariable(name, false)
		}
		v.SetKind(variable.GetKind())
		if canCapture {
			tmpPhiCapture.Set(v, ret)
		}
		if variable.GetCaptured().GetScope().Compare(ver.GetCaptured().GetScope()) {
			tmpPhiScope.Set(v, ret)
		}
	}

	defer func() {
		tmpPhiScope.ForEach(func(v VersionedIF[T], ret T) bool {
			base.AssignVariable(v, ret)
			base.tryRegisterCapturedVariable(v.GetName(), v)
			return true
		})
		tmpPhiCapture.ForEach(func(v VersionedIF[T], ret T) bool {
			err := v.Assign(ret)
			if !utils.IsNil(err) {
				log.Warnf("BUG: variable.Assign error: %v", err)
				return false
			}
			base.ChangeCapturedSideEffect(v.GetName(), v)
			return true
		})
	}()

	baseScope := ScopedVersionedTableIF[T](base)
	for index, sub := range subScopes {
		ForEachCapturedVariable(sub, baseScope, func(name string, ver VersionedIF[T]) {
			tmpIsCapture.Set(addPhiContent(index, name, ver, sub), false)
		})
		ForEachCapturedSideEffect(sub, baseScope, func(name string, ver []VersionedIF[T]) {
			tmpIsCapture.Set(addPhiContent(index, name, ver[0], sub, ver[1].GetScope()), true)
			baseScope.SetCapturedSideEffect(ver[0].GetName(), ver[0], ver[1])
		})
	}

	tmpVariable.ForEach(func(ver VersionedIF[T], m []T) bool {
		canCapture, _ := tmpIsCapture.Get(ver)
		generatePhi(ver, m, canCapture)
		return true
	})
}

func (condition *ScopedVersionedTable[T]) Spin(
	header, latch ScopedVersionedTableIF[T],
	handler SpinHandle[T],
) {
	condition.spin = false
	condition.createEmptyPhi = nil
	condition.linkIncomingPhi.ForEach(func(name string, ver VersionedIF[T]) bool {
		last := latch.ReadValue(name)
		origin := header.ReadValue(name)
		if utils.IsNil(last) || utils.IsNil(origin) {
			return true
		}
		res := handler(name, ver.GetValue(), origin, last)
		for name, value := range res {
			if !utils.IsNil(condition.spinReplaceFilter) && condition.spinReplaceFilter(value) {
				continue
			}
			v := condition.CreateVariable(name, ver.GetLocal())
			condition.AssignVariable(v, value)
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
