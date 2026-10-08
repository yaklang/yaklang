package stream_parser

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessedEmptyListResult(t *testing.T) {
	root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
	n := root.Children[0].Children[0]
	p := &DefParser{}
	require.NoError(t, p.OnRoot(root))
	n.Cfg.SetItem(CfgIsTerminal, false)
	n.Cfg.SetItem(CfgIsList, true)
	_, err := ToMap(n)
	require.ErrorIs(t, err, noResultError, "a dormant schema is not evidence")
	n.Cfg.SetItem(CfgNodeResult, [2]uint64{0, 0})
	value, err := ToMap(n)
	require.NoError(t, err)
	require.True(t, value.IsList())
	require.Empty(t, value.Children())
	require.Same(t, n, value.Origin)
	require.Zero(t, CalcNodeConsumedLength(n))
	for _, terminal := range []bool{false, true} {
		n.Cfg.SetItem(CfgIsTerminal, terminal)
		n.Cfg.SetItem(CfgIsList, terminal) // ordinary nonlist or terminal list flag
		value, err := ToMap(n)
		require.NoError(t, err)
		require.True(t, value.IsValue(), "do not retype scalar results")
	}
	n.Cfg.SetItem(CfgIsTerminal, false)
	n.Cfg.SetItem(CfgIsList, true)
	n.Cfg.SetItem(CfgNodeResult, [2]uint64{8, 8})
	_, err = ToMap(n)
	require.Error(t, err, "an empty vector still needs an available wire position")
	_, err = p.write([]byte{0xa5}, 8)
	require.NoError(t, err)
	n.Cfg.SetItem(CfgNodeResult, [2]uint64{0, 8})
	value, err = ToMap(n)
	require.NoError(t, err)
	require.True(t, value.IsValue(), "do not retype nonempty scalar results")
}
