package parser

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseBinaryWithConfigNestedRulePath(t *testing.T) {
	wire := make([]byte, 48)
	reader := &benchmarkBoundedReader{Reader: bytes.NewReader(wire), bitLength: 48 * 8}
	node, err := ParseBinaryWithConfig(reader, "application-layer.ntp", map[string]any{"path": "caller metadata", "label": "nested rule"}, "NTP")
	require.NoError(t, err)
	require.Zero(t, reader.Len())
	require.Equal(t, "caller metadata", node.Ctx.GetString("path"))
	require.Equal(t, "nested rule", node.Ctx.GetString("label"))
	// A configured caller must resolve the same embedded rule as ParseBinary,
	// including on Windows; filesystem separators are not RuleFS separators.
	plain, err := ParseBinary(&benchmarkBoundedReader{Reader: bytes.NewReader(wire), bitLength: 48 * 8}, "application-layer.ntp", "NTP")
	require.NoError(t, err)
	require.Equal(t, plain.Name, node.Name)
	require.Equal(t, len(plain.Children), len(node.Children))
}
