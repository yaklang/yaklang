package ssa

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func TestVariableIndexResolvesEachInstructionOnce(t *testing.T) {
	for _, constants := range []bool{false, true} {
		s := &indexStore{
			variable: utils.NewSafeMapWithKey[string, []int64](),
			member:   utils.NewSafeMapWithKey[string, []int64](),
			class:    utils.NewSafeMapWithKey[string, []int64](),
			consts:   utils.NewSafeMapWithKey[string, []int64](),
		}
		s.variable.Set("a", []int64{1, 1, 2, 0, -1})
		s.member.Set("b", []int64{1, 3})
		s.class.Set("c", []int64{2, 3})
		s.consts.Set("x", []int64{1, 1, 2, 0, -1})
		s.consts.Set("y", []int64{1, 2, 3})
		mode := ssadb.NameMatch | ssadb.KeyMatch
		if constants {
			mode = ssadb.ConstType
		}
		calls := make(map[int64]int)
		result := s.FindByVariableEx(mode, func(string) bool { return true }, func(id int64) Instruction {
			calls[id]++
			if id == 3 {
				return nil
			}
			return NewConst(id)
		})
		require.Equal(t, map[int64]int{1: 1, 2: 1, 3: 1}, calls)
		require.Len(t, result, 2)
	}
}

func TestVariableIndexFiltersBeforeResolvingOutsideLock(t *testing.T) {
	s := &indexStore{
		variable: utils.NewSafeMapWithKey[string, []int64](),
		member:   utils.NewSafeMapWithKey[string, []int64](),
		class:    utils.NewSafeMapWithKey[string, []int64](),
	}
	s.variable.Set("keep", []int64{1})
	s.variable.Set("drop", []int64{2})
	var calls []int64
	got := s.FindByVariableEx(ssadb.NameMatch, func(k string) bool { return k == "keep" }, func(id int64) Instruction {
		calls = append(calls, id)
		// IR resolution may populate indexes. Holding the map lock here deadlocks.
		s.variable.Set("loaded", []int64{3})
		return NewConst(id)
	})
	require.Equal(t, []int64{1}, calls)
	require.Len(t, got, 1)
}
