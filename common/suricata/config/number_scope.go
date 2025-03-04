package config

import (
	"fmt"
	"math/big"
	"math/rand"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

type scopeAtom struct {
	start *big.Int
	end   *big.Int
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

// 新的使用 big.Int 的接口
func newScopeBigInt(start, end *big.Int) *scope {
	return &scope{
		subScopes: []*scopeAtom{
			{
				start: new(big.Int).Set(start),
				end:   new(big.Int).Set(end),
			},
		},
	}
}

// 保持原有的 uint32 接口
func newScope(start, end uint32) *scope {
	startBig := new(big.Int).SetUint64(uint64(start))
	endBig := new(big.Int).SetUint64(uint64(end))
	return newScopeBigInt(startBig, endBig)
}

func (s *scope) size() *big.Int {
	sum := new(big.Int)
	one := big.NewInt(1)
	for _, item := range s.subScopes {
		diff := new(big.Int).Sub(item.end, item.start)
		diff.Add(diff, one)
		sum.Add(sum, diff)
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
		return allScopes[i].start.Cmp(allScopes[j].start) < 0
	})
	finalScopes := []*scopeAtom{}
	for _, ietm := range allScopes {
		if len(finalScopes) == 0 {
			finalScopes = append(finalScopes, ietm)
			continue
		}
		lastScope := finalScopes[len(finalScopes)-1]
		if lastScope.end.Cmp(ietm.start) >= 0 {
			if lastScope.end.Cmp(ietm.end) < 0 {
				lastScope.end = new(big.Int).Set(ietm.end)
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
		res := &scopeAtom{
			start: new(big.Int),
			end:   new(big.Int),
		}
		if atom1.start.Cmp(atom2.start) > 0 {
			res.start.Set(atom1.start)
		} else {
			res.start.Set(atom2.start)
		}
		if atom1.end.Cmp(atom2.end) < 0 {
			res.end.Set(atom1.end)
		} else {
			res.end.Set(atom2.end)
		}
		if res.start.Cmp(res.end) > 0 {
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
	start := big.NewInt(0)
	one := big.NewInt(1)
	maxUint32 := new(big.Int).SetUint64(0xFFFFFFFF)

	for _, subScope := range s.subScopes {
		if start.Cmp(subScope.start) < 0 {
			notScope = append(notScope, &scopeAtom{
				start: new(big.Int).Set(start),
				end:   new(big.Int).Sub(new(big.Int).Set(subScope.start), one),
			})
		}
		start = new(big.Int).Add(subScope.end, one)
	}
	if start.Cmp(maxUint32) < 0 {
		notScope = append(notScope, &scopeAtom{
			start: new(big.Int).Set(start),
			end:   new(big.Int).Set(maxUint32),
		})
	}
	return &scope{
		subScopes: notScope,
	}
}

// 新的使用 big.Int 的接口
func (s *scope) addNumberBigInt(n *big.Int) {
	s.addRawBigInt(n, n)
}

// 保持原有的 uint32 接口
func (s *scope) addNumber(n uint32) {
	nBig := new(big.Int).SetUint64(uint64(n))
	s.addNumberBigInt(nBig)
}

// 新的使用 big.Int 的接口
func (s *scope) subNumberBigInt(n *big.Int) {
	s.subRawBigInt(n, n)
}

// 保持原有的 uint32 接口
func (s *scope) subNumber(n uint32) {
	nBig := new(big.Int).SetUint64(uint64(n))
	s.subNumberBigInt(nBig)
}

// 新的使用 big.Int 的接口
func (s *scope) addRawBigInt(start, end *big.Int) {
	s.add(newScopeBigInt(start, end))
}

// 保持原有的 uint32 接口
func (s *scope) addRaw(start, end uint32) {
	startBig := new(big.Int).SetUint64(uint64(start))
	endBig := new(big.Int).SetUint64(uint64(end))
	s.addRawBigInt(startBig, endBig)
}

// 新的使用 big.Int 的接口
func (s *scope) subRawBigInt(start, end *big.Int) {
	s.sub(newScopeBigInt(start, end))
}

// 保持原有的 uint32 接口
func (s *scope) subRaw(start, end uint32) {
	startBig := new(big.Int).SetUint64(uint64(start))
	endBig := new(big.Int).SetUint64(uint64(end))
	s.subRawBigInt(startBig, endBig)
}

// 新的使用 big.Int 的接口
func (s *scope) hasNumberBigInt(n *big.Int) bool {
	for _, item := range s.subScopes {
		if item.start.Cmp(n) <= 0 && item.end.Cmp(n) >= 0 {
			return true
		}
	}
	return false
}

// 保持原有的 uint32 接口
func (s *scope) hasNumber(n uint32) bool {
	nBig := new(big.Int).SetUint64(uint64(n))
	return s.hasNumberBigInt(nBig)
}

func (s *scope) sub(scopeVal *scope) {
	// A - B = A and not B
	res := s.and(scopeVal.not())
	s.subScopes = res.subScopes
}

// 新的使用 big.Int 的接口
func (s *scope) randIntBigInt() *big.Int {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("scope rand int operation failed: %v", err)
		}
	}()

	scopeAtomLs := []*big.Int{}
	sum := big.NewInt(0)
	pre := big.NewInt(0)

	for _, item := range s.subScopes {
		l := new(big.Int).Sub(item.end, item.start)
		l.Add(l, big.NewInt(1))
		sum.Add(sum, l)
		scopeAtomLs = append(scopeAtomLs, new(big.Int).Set(pre))
		pre.Add(pre, l)
	}

	if sum.Sign() == 0 {
		return big.NewInt(0)
	}

	scopeAtomLs = append(scopeAtomLs, new(big.Int).Set(sum))

	var n *big.Int
	if s.rander != nil {
		n = new(big.Int).SetUint64(uint64(s.rander.Uint32()))
	} else {
		n = new(big.Int).SetUint64(uint64(rand.Uint32()))
	}
	n.Mod(n, sum)

	index := sort.Search(len(scopeAtomLs), func(i int) bool {
		return scopeAtomLs[i].Cmp(n) > 0
	})

	delta := new(big.Int).Sub(n, scopeAtomLs[index-1])
	result := new(big.Int).Add(s.subScopes[index-1].start, delta)

	return result
}

// 保持原有的 uint32 接口
func (s *scope) randInt() uint32 {
	result := s.randIntBigInt()
	// 如果结果超出 uint32 范围，取模
	if result.BitLen() > 32 {
		result.Mod(result, new(big.Int).SetUint64(0x100000000))
	}
	return uint32(result.Uint64())
}

func (s *scope) String() string {
	resList := []string{}
	for _, item := range s.subScopes {
		resList = append(resList, fmt.Sprintf("%v-%v", item.start, item.end))
	}
	return strings.Join(resList, " ")
}

func (s *scope) Dump() {
	println(s.String())
}
