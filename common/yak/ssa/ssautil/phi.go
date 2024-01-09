package ssautil

import (
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

func (s *ScopedVersionedTable[T]) CoverByChild() {
	// cover origin value
	subs := s.child
	if len(subs) != 1 {
		log.Error("only support one child")
		panic("only support one child")
	}

	sub := subs[0]
	sub.captured.ForEach(func(name string, ver *Versioned[T]) bool {
		log.Infof("cover %s by %s", name, ver.String())
		s.CreateLexicalVariable(name, ver.Value)
		return true
	})

	s.finishChild = append(s.finishChild, sub)
	s.child = make([]*ScopedVersionedTable[T], 0)
}

func (s *ScopedVersionedTable[T]) Merge(hasSelf bool, handler func(name string, t []T) T) {
	var zero T
	subScopes := s.child
	// handler []T must sort same with sub-scope
	length := len(subScopes)
	if hasSelf {
		length += 1
	}
	tmp := make(map[string][]T)
	for index, sub := range subScopes {
		sub.captured.ForEach(func(name string, ver *Versioned[T]) bool {
			m, ok := tmp[name]
			if !ok {
				m = make([]T, length)
			}
			m[index] = ver.Value
			tmp[name] = m
			return true
		})
	}

	for name, m := range tmp {
		origin := s.GetLatestVersion(name)
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
		// }
		res := handler(name, m)
		s.CreateLexicalVariable(name, res)
	}

	s.finishChild = append(s.finishChild, subScopes...)
	s.child = make([]*ScopedVersionedTable[T], 0)
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
