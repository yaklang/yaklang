package sfvm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// countingTopDefValue wraps a ValueOperator and counts how many times the
// dataflow fanout descends into it via GetSyntaxFlowTopDef.
type countingTopDefValue struct {
	ValueOperator
	calls *int
}

func (c *countingTopDefValue) GetSyntaxFlowTopDef(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (Values, error) {
	*c.calls++
	return c.ValueOperator.GetSyntaxFlowTopDef(sfResult, sfConfig, config...)
}

// TestDedupDataflowFanoutInput is the direct unit test of the exact-dedup
// helper: empty/single inputs pass through untouched, exact duplicates
// (same operator, or two wrappers of the same node id) collapse to one
// first-occurrence entry, distinct entries are all kept.
func TestDedupDataflowFanoutInput(t *testing.T) {
	require.Equal(t, 0, dedupDataflowFanoutInput(NewEmptyValues()).Count(),
		"empty input passes through")

	single := ValuesOf(NewSimpleValue("a", "a.php", 1, 1))
	require.Equal(t, 1, dedupDataflowFanoutInput(single).Count(),
		"single input passes through")

	a := NewSimpleValue("a", "a.php", 1, 1)
	b := NewSimpleValue("b", "b.php", 2, 2)
	dup := NewValues([]ValueOperator{a, a, a, b, a})
	got := dedupDataflowFanoutInput(dup)
	require.Equal(t, 2, got.Count(), "exact duplicates collapse, distinct values kept")
	require.True(t, got[0] == ValueOperator(a), "first occurrence is kept")
}

// TestValuesGetSyntaxFlowTopDefDedupsFanoutInput proves the per-source fanout
// descends once per unique input node: a Values carrying the same operator
// three times triggers exactly one GetSyntaxFlowTopDef for it. Without the
// dedup every duplicate paid its own recursive descent — on a large project
// one opcode fed 52,842 values (with duplicates) and hung the scan for 71+
// minutes.
func TestValuesGetSyntaxFlowTopDefDedupsFanoutInput(t *testing.T) {
	calls := 0
	mk := func(text string) *countingTopDefValue {
		return &countingTopDefValue{
			ValueOperator: NewSimpleValue(text, text+".php", 1, 1),
			calls:         &calls,
		}
	}
	x := mk("x")
	y := mk("y")

	in := NewValues([]ValueOperator{x, x, x, y, x})
	out, err := in.GetSyntaxFlowTopDef(nil, NewConfig())
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, 2, calls,
		"fanout must descend once per unique value (x once, y once), got %d descents", calls)
}

// TestValuesGetSyntaxFlowBottomUseDedupsFanoutInput is the symmetric guard
// for the bottomuse (`<-?`) fanout.
func TestValuesGetSyntaxFlowBottomUseDedupsFanoutInput(t *testing.T) {
	calls := 0
	mk := func(text string) *countingBottomUseValue {
		return &countingBottomUseValue{
			ValueOperator: NewSimpleValue(text, text+".php", 1, 1),
			calls:         &calls,
		}
	}
	x := mk("x")
	y := mk("y")

	in := NewValues([]ValueOperator{x, y, x})
	out, err := in.GetSyntaxFlowBottomUse(nil, NewConfig())
	require.NoError(t, err)
	require.NotNil(t, out)
	require.Equal(t, 2, calls,
		"fanout must descend once per unique value (x once, y once), got %d descents", calls)
}

type countingBottomUseValue struct {
	ValueOperator
	calls *int
}

func (c *countingBottomUseValue) GetSyntaxFlowBottomUse(sfResult *SFFrameResult, sfConfig *Config, config ...*RecursiveConfigItem) (Values, error) {
	*c.calls++
	return c.ValueOperator.GetSyntaxFlowBottomUse(sfResult, sfConfig, config...)
}
