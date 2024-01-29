package ssautil

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
)

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
// 		capturedValue := v.parent.GetLatestVersion(name)
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
// 		result := ctx.scope.CreateLexicalVariable(ctx.name, phi)
// 		result.SetPhi(true)
// 	}
// }
// }

// ForEachCapturedVariable call the handler for each captured by base scope Variable
func (ps *ScopedVersionedTable[T]) ForEachCapturedVariable(base *ScopedVersionedTable[T], handler func(name string, ver *Versioned[T])) {
	ps.captured.ForEach(func(name string, ver *Versioned[T]) bool {
		baseVariable := base.GetLatestVersionVersioned(name)
		if baseVariable == nil {
			// not exist in base scope, this variable just set in sub-scope,
			// just skip
			return true
		}
		if baseVariable.overWriteVariable != ver.overWriteVariable {
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

	scope.captured.ForEach(func(name string, ver *Versioned[T]) bool {
		log.Infof("cover %s by %s", name, ver.String())
		s.CreateLexicalVariable(name, ver.Value)
		return true
	})
}

// Merge merge the sub-scope to current scope,
// if hasSelf is true: the current scope will be merged to the result
func Merge[T comparable](
	base *ScopedVersionedTable[T],
	hasSelf bool,
	handler func(name string, t []T),
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

	addPhiContent := func(index int, name string, ver *Versioned[T]) {
		m, ok := tmp[name]
		if !ok {
			m = make([]T, length)
		}
		m[index] = ver.Value
		tmp[name] = m
	}
	generatePhi := func(name string, m []T) {
		origin := base.GetLatestVersion(name)
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
		handler(name, m)
	}

	for index, sub := range subScopes {
		sub.captured.ForEach(func(name string, ver *Versioned[T]) bool {
			baseVariable := base.GetLatestVersionVersioned(name)
			capturedVariable := sub.parent.GetLatestVersionVersioned(name)
			if capturedVariable == nil {
				panic(fmt.Sprintf("variable %s not exist in parent scope, but this variable is captured", name))
			}
			if baseVariable == nil {
				// not exist in base scope, this variable just set in sub-scope,
				// just skip, not need generate phi
				return true
			}

			if baseVariable.scope != capturedVariable.scope {
				// the ver scope is not equal to base scope,
				// this `ver` is captured, but not same variable with base-variable
				// just skip
				return true
			}
			addPhiContent(index, name, ver)
			return true
		})
	}

	for name, m := range tmp {
		generatePhi(name, m)
	}
}

// this handler merge [origin, last] to phi
func (s *ScopedVersionedTable[T]) Spin(handler func(name string, phi T, origin T, last T) T) {
	s.incomingPhi.ForEach(func(name string, ver *Versioned[T]) bool {
		last := s.GetLatestVersion(name)
		origin := ver.origin.Value
		res := handler(name, ver.Value, origin, last)
		s.CreateLexicalVariable(name, res)
		return true
	})
	s.spin = false
	s.CreateEmptyPhi = nil
}

func (s *ScopedVersionedTable[T]) SetSpin(create func() T) {
	s.spin = true
	s.CreateEmptyPhi = create
}
