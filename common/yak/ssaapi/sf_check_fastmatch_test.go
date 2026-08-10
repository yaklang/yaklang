package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	sf "github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func fastMatchTestValue(t *testing.T, prog *Program, id int64) *Value {
	inst := ssa.NewConst(id)
	inst.SetId(id)
	v, err := prog.NewValue(inst)
	require.NoError(t, err)
	return v
}

func fastMatchTestCheck(t *testing.T, sourceIDs ...int64) *sfCheck {
	prog := NewTmpProgram("sf-check-fastmatch")
	vals := make(sf.Values, 0, len(sourceIDs))
	for _, id := range sourceIDs {
		vals = append(vals, fastMatchTestValue(t, prog, id))
	}
	table := omap.NewEmptyOrderedMap[string, sf.Values]()
	table.Set("source", vals)
	result := &sf.SFFrameResult{SymbolTable: table}
	return &sfCheck{contextResult: result}
}

func TestSFCheck_FastPathMatch_SourceMembership(t *testing.T) {
	check := fastMatchTestCheck(t, 1, 2, 3)
	prog := NewTmpProgram("sf-check-fastmatch-path")

	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* & $source",
	}}

	path := sf.Values{fastMatchTestValue(t, prog, 9), fastMatchTestValue(t, prog, 2)}
	match, ok := check.fastPathMatch(item, path)
	require.True(t, ok, "simple source intersection must be handled by fast path")
	require.True(t, match, "path containing a source id must match")

	path = sf.Values{fastMatchTestValue(t, prog, 8), fastMatchTestValue(t, prog, 9)}
	match, ok = check.fastPathMatch(item, path)
	require.True(t, ok)
	require.False(t, match, "path without source id must not match")
}

func TestSFCheck_FastPathMatch_UnknownPatternFallsBack(t *testing.T) {
	check := fastMatchTestCheck(t, 1)
	prog := NewTmpProgram("sf-check-fastmatch-unknown")
	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* ?{opcode:call}?{!<self> & $source}",
	}}
	_, ok := check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 1)})
	require.False(t, ok, "complex include must keep the full sub-query path")
}

func TestSFCheck_FastPathMatch_MissingSymbolFallsBack(t *testing.T) {
	check := fastMatchTestCheck(t)
	prog := NewTmpProgram("sf-check-fastmatch-missing")
	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* & $source",
	}}
	_, ok := check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 1)})
	require.False(t, ok, "unbound source symbol must fall back to the full query")
}

func TestFastPathMatchStats_TracksHitsAndFallbacks(t *testing.T) {
	beforeHit, beforeFallback := FastPathMatchStats()
	check := fastMatchTestCheck(t, 1)
	prog := NewTmpProgram("sf-check-fastmatch-stats")
	item := &checkItem{RecursiveConfigItem: &sf.RecursiveConfigItem{
		Key:   string(sf.RecursiveConfig_Include),
		Value: "* & $source",
	}}
	_, ok := check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 1)})
	require.True(t, ok)
	_, ok = check.fastPathMatch(item, sf.Values{fastMatchTestValue(t, prog, 99)})
	require.True(t, ok)
	afterHit, afterFallback := FastPathMatchStats()
	require.GreaterOrEqual(t, afterHit-beforeHit, int64(2), "two simple patterns must be fast-path hits")
	require.GreaterOrEqual(t, afterFallback-beforeFallback, int64(0))
}

var _ = utils.Error // keep import
