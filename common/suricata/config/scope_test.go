package config

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

func parseScopeExp(exp string) *scope {
	indexs := utils.IndexAllSubstrings(exp, " + ", " - ")
	parseRawScope := func(raw string) *scope {
		scopeRaw := strings.Split(raw, "-")
		left, _ := strconv.Atoi(scopeRaw[0])
		right, _ := strconv.Atoi(scopeRaw[1])
		return newScope(uint32(left), uint32(right))
	}
	if len(indexs) == 0 {
		return parseRawScope(exp)
	}
	var res *scope
	var preIndex int
	preOp := -1
	for i, index := range indexs {
		if preOp != -1 {
			currentScope := parseRawScope(exp[preIndex:index[1]])
			preIndex = index[1] + 3
			if index[0] == 0 {
				res.add(currentScope)
			} else {
				res.sub(currentScope)
			}
		}
		preOp = index[0]
		if res == nil {
			res = parseRawScope(exp[:index[1]])
			preIndex = index[1] + 3
		}
		if i == len(indexs)-1 {
			currentScope := parseRawScope(exp[preIndex:])
			if index[0] == 0 {
				res.add(currentScope)
			} else {
				res.sub(currentScope)
			}
		}
	}
	return res
}

func TestScope(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		scopeExp string
		expect   string
	}{
		{
			name:     "left marginal value, add test 1",
			scopeExp: `1-5 + 0-1`,
			expect:   "0-5",
		},
		{
			name:     "left marginal value, add test 2",
			scopeExp: `1-5 + 1-1`,
			expect:   "1-5",
		},
		{
			name:     "left marginal value, add test 3",
			scopeExp: `1-5 + 1-2`,
			expect:   "1-5",
		},
		{
			name:     "left marginal value, add test 4",
			scopeExp: `1-5 + 0-2`,
			expect:   "0-5",
		},
		{
			name:     "right marginal value, add test 1",
			scopeExp: `1-5 + 4-5`,
			expect:   "1-5",
		},
		{
			name:     "right marginal value, add test 2",
			scopeExp: `1-5 + 5-5`,
			expect:   "1-5",
		},
		{
			name:     "right marginal value, add test 3",
			scopeExp: `1-5 + 5-6`,
			expect:   "1-6",
		},
		{
			name:     "right marginal value, add test 4",
			scopeExp: `1-5 + 4-6`,
			expect:   "1-6",
		},

		{
			name:     "left marginal value, sub test 1",
			scopeExp: `1-5 - 0-1`,
			expect:   "2-5",
		},
		{
			name:     "left marginal value, sub test 2",
			scopeExp: `1-5 - 1-1`,
			expect:   "2-5",
		},
		{
			name:     "left marginal value, sub test 3",
			scopeExp: `1-5 - 1-2`,
			expect:   "3-5",
		},
		{
			name:     "left marginal value, sub test 4",
			scopeExp: `1-5 - 0-2`,
			expect:   "3-5",
		},
		{
			name:     "right marginal value, sub test 1",
			scopeExp: `1-5 - 4-5`,
			expect:   "1-3",
		},
		{
			name:     "right marginal value, sub test 2",
			scopeExp: `1-5 - 5-5`,
			expect:   "1-4",
		},
		{
			name:     "right marginal value, sub test 3",
			scopeExp: `1-5 - 5-6`,
			expect:   "1-4",
		},
		{
			name:     "right marginal value, sub test 4",
			scopeExp: `1-5 - 4-6`,
			expect:   "1-3",
		},

		{
			name:     "add sub scope test",
			scopeExp: `1-5 + 2-3`,
			expect:   "1-5",
		},
		{
			name:     "add left scope test",
			scopeExp: `10-20 + 1-5`,
			expect:   "1-5 10-20",
		},
		{
			name:     "add right scope test",
			scopeExp: `10-20 + 25-30`,
			expect:   "10-20 25-30",
		},

		{
			name:     "subtract sub scope test",
			scopeExp: `1-5 - 2-3`,
			expect:   "1-1 4-5",
		},
		{
			name:     "subtract left scope test",
			scopeExp: `10-20 - 1-5`,
			expect:   "10-20",
		},
		{
			name:     "subtract right scope test",
			scopeExp: `10-20 - 25-30`,
			expect:   "10-20",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			scopeTestCase := parseScopeExp(testCase.scopeExp)
			assert.Equal(t, testCase.expect, scopeTestCase.String())
		})
	}
}
func TestScopeRandInt(t *testing.T) {
	rander := rand.New(rand.NewSource(1))
	for _, testCase := range []struct {
		name     string
		scopeExp string
		resScope []int
	}{
		{
			name:     "rand specific value",
			scopeExp: `1-1`,
			resScope: []int{1},
		},
		{
			name:     "rand value in scope",
			scopeExp: `1-5`,
			resScope: []int{1, 2, 3, 4, 5},
		},
		{
			name:     "rand value in two scope",
			scopeExp: `1-5 + 10-15`,
			resScope: []int{1, 2, 3, 4, 5, 10, 11, 12, 13, 14, 15},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			scopeTestCase := parseScopeExp(testCase.scopeExp)
			scopeTestCase.rander = rander
			times := map[int]float64{}
			for i := 0; i < 1000; i++ {
				n := scopeTestCase.randInt()
				times[int(n)]++
				assert.Contains(t, testCase.resScope, int(n))
			}
			resScopeL := len(testCase.resScope)
			p := 1.0 / float64(resScopeL)
			for _, n := range testCase.resScope {
				actualP := times[n] / 1000
				assert.Greater(t, actualP+0.03, p)
			}
		})
	}
}
