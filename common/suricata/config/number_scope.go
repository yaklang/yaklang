package config

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"math/rand"
	"sort"
	"strings"
)

type scopeAtom struct {
	start uint32
	end   uint32
}
type scope struct {
	subScopes []*scopeAtom
	rander    *rand.Rand
}

func newEmptyScope() *scope {
	return &scope{
		subScopes: []*scopeAtom{},
	}
}
func newScope(start, end uint32) *scope {
	return &scope{
		subScopes: []*scopeAtom{
			{
				start: start,
				end:   end,
			},
		},
	}
}

func (s *scope) size() uint32 {
	var sum uint32 = 0
	for _, item := range s.subScopes {
		sum += item.end - item.start + 1
	}
	return sum
}
func (s *scope) add(scopeVal *scope) {
	res := s.or(scopeVal)
	s.subScopes = res.subScopes
}
func (s *scope) or(scopeVal *scope) *scope {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("scope or operation failed: %v", err)
		}
	}()
	allScopes := []*scopeAtom{}
	allScopes = append(allScopes, s.subScopes...)
	allScopes = append(allScopes, scopeVal.subScopes...)
	sort.Slice(allScopes, func(i, j int) bool {
		return allScopes[i].start < allScopes[j].start
	})
	finalScopes := []*scopeAtom{}
	for _, ietm := range allScopes {
		if len(finalScopes) == 0 {
			finalScopes = append(finalScopes, ietm)
			continue
		}
		lastScope := finalScopes[len(finalScopes)-1]
		if lastScope.end >= ietm.start {
			if lastScope.end < ietm.end {
				lastScope.end = ietm.end
			} else {
				continue
			}
		} else {
			finalScopes = append(finalScopes, ietm)
		}
	}
	return &scope{
		subScopes: finalScopes,
	}
}
func (s *scope) and(scopeVal *scope) *scope {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("scope and operation failed: %v", err)
		}
	}()
	scopeAtomAnd := func(atom1, atom2 *scopeAtom) *scopeAtom {
		res := &scopeAtom{}
		res.start = max(atom1.start, atom2.start)
		res.end = min(atom1.end, atom2.end)
		if res.start > res.end {
			return nil
		}
		return res
	}
	allScopes := []*scopeAtom{}
	for _, subScope := range s.subScopes {
		for _, subScopeVal := range scopeVal.subScopes {
			res := scopeAtomAnd(subScope, subScopeVal)
			if res != nil {
				allScopes = append(allScopes, res)
			}
		}
	}
	return &scope{
		subScopes: allScopes,
	}
}
func (s *scope) not() *scope {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("scope not operation failed: %v", err)
		}
	}()
	notScope := []*scopeAtom{}
	var start uint32
	for _, subScope := range s.subScopes {
		if start < subScope.start {
			notScope = append(notScope, &scopeAtom{
				start: start,
				end:   subScope.start - 1,
			})
		}
		start = subScope.end + 1
	}
	if start < 0xFFFFFFFF {
		notScope = append(notScope, &scopeAtom{
			start: start,
			end:   0xFFFFFFFF,
		})
	}
	return &scope{
		subScopes: notScope,
	}
}
func (s *scope) addNumber(n uint32) {
	s.addRaw(n, n)
}
func (s *scope) subNumber(n uint32) {
	s.subRaw(n, n)
}
func (s *scope) addRaw(start, end uint32) {
	s.add(newScope(start, end))
}
func (s *scope) subRaw(start, end uint32) {
	s.sub(newScope(start, end))
}
func (s *scope) hasNumber(n uint32) bool {
	for _, item := range s.subScopes {
		if n >= item.start && n <= item.end {
			return true
		}
	}
	return false
}
func (s *scope) sub(scopeVal *scope) {
	// A - B = A and not B
	res := s.and(scopeVal.not())
	s.subScopes = res.subScopes
}
func (s *scope) randInt() uint32 {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("scope rand int operation failed: %v", err)
		}
	}()
	scopeAtomLs := []uint32{}
	var sum uint32 = 0
	var pre uint32 = 0
	scopeAtomLs = lo.Map(s.subScopes, func(item *scopeAtom, index int) uint32 {
		l := item.end - item.start + 1
		sum += l
		preN := pre
		pre = sum
		return preN
	})
	if sum == 0 {
		return 0
	}
	scopeAtomLs = append(scopeAtomLs, sum)
	var n uint32
	if s.rander != nil {
		n = s.rander.Uint32() % sum
	} else {
		n = rand.Uint32() % sum
	}
	index := sort.Search(len(scopeAtomLs), func(i int) bool {
		return scopeAtomLs[i] > n
	})
	delta := n - scopeAtomLs[index-1]
	return s.subScopes[index-1].start + delta
}
func (s *scope) String() string {
	resList := []string{}
	for _, item := range s.subScopes {
		resList = append(resList, fmt.Sprintf("%d-%d", item.start, item.end))
	}
	return strings.Join(resList, " ")
}
func (s *scope) Dump() {
	println(s.String())
}
