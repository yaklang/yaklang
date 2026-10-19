package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
)

// The common case is no uses or one use. Allocate the set lazily and use a
// slice for small sets; build an index only when there are many references.
type variableUseRanges struct {
	mu    sync.Mutex
	first *memedit.Range
	rest  []*memedit.Range
	index *utils.SafeMapWithKey[*memedit.Range, struct{}]
}

func (s *variableUseRanges) add(r *memedit.Range) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.first == nil {
		s.first = r
		return true
	}
	if s.first == r {
		return false
	}
	if s.index != nil {
		if s.index.Have(r) {
			return false
		}
		s.index.Set(r, struct{}{})
	} else {
		for _, old := range s.rest {
			if old == r {
				return false
			}
		}
		if len(s.rest) == 7 {
			s.index = utils.NewSafeMapWithKey[*memedit.Range, struct{}]()
			s.index.Set(s.first, struct{}{})
			for _, old := range s.rest {
				s.index.Set(old, struct{}{})
			}
			s.index.Set(r, struct{}{})
		}
	}
	s.rest = append(s.rest, r)
	return true
}

func (s *variableUseRanges) each(fn func(*memedit.Range)) {
	s.mu.Lock()
	first := s.first
	rest := append([]*memedit.Range(nil), s.rest...)
	s.mu.Unlock()
	if first != nil {
		fn(first)
	}
	for _, r := range rest {
		fn(r)
	}
}

type variableRangePosition struct {
	start, end                                 int
	startLine, startColumn, endLine, endColumn int
}

// Variable ranges are source positions, shared for the lifetime of a program.
// Copy the input positions so later parser edits cannot change shared ranges.
// Different editor instances remain distinct even if their content is equal.
func (v *Variable) internRange(r *memedit.Range) *memedit.Range {
	prog := v.GetProgram()
	if prog == nil {
		if scope, ok := v.GetScope().(*ScopeInstance); ok && scope.fun != nil {
			prog = scope.fun.GetProgram()
		}
	}
	editor := r.GetEditor()
	if prog == nil || editor == nil {
		return r
	}
	key := variableRangePosition{r.GetStartOffset(), r.GetEndOffset(), r.GetStart().GetLine(), r.GetStart().GetColumn(), r.GetEnd().GetLine(), r.GetEnd().GetColumn()}
	prog.initConcurrentMaps()
	registry := prog.variableRanges
	ranges, _ := registry.GetOrLoad(editor, func() (*utils.SafeMapWithKey[variableRangePosition, *memedit.Range], error) {
		return utils.NewSafeMapWithKey[variableRangePosition, *memedit.Range](), nil
	})
	shared, _ := ranges.GetOrLoad(key, func() (*memedit.Range, error) { return r.Clone(), nil })
	return shared
}
