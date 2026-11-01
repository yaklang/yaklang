package bin_parser

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
)

// The historical catalog entry keeps its original leaf tree. A separately
// tested canonical/native profile is required before accepting its old frozen
// todo metadata as a historical snapshot rather than today's coverage status.
func requireNetFlowV9Scopes(t *testing.T) bool {
	t.Helper()
	w, err := hex.DecodeString("00090002000003e86553f1000000000700000013000000100100000200080004000200040100000cc000020900000011")
	require.NoError(t, err)
	n, err := parser.ParseBinary(newProtocolCorpusBoundedReader(w), "application-layer.extended_protocols", "NetFlowV9")
	require.NoError(t, err)
	require.Equal(t, w, NodeToBytes(n))
	v, err := ParseStructured(w, "netflow_v9", "NetFlowV9Fields")
	require.NoError(t, err)
	m := v["fields"].(map[string]any)["Message"].(map[string]any)
	require.Equal(t, "NetFlow v9", m["Protocol Name"])
	require.Equal(t, uint64(19), m["Source ID"])
	require.Equal(t, true, m["Record Count Verified"])
	fields := m["FlowSets"].([]map[string]any)[1]["Records"].([]map[string]any)[0]["Fields"].([]map[string]any)
	require.Equal(t, "192.0.2.9", fields[0]["Value"])
	require.Equal(t, uint64(17), fields[1]["Value"])
	require.Equal(t, []any{uint64(40), uint64(44)}, fields[0]["Wire Field Ranges"].(map[string]any)["Value"])
	return true
}

func TestNetFlowV9CanonicalAndLegacyScope(t *testing.T) {
	require.True(t, requireNetFlowV9Scopes(t))
	w, err := hex.DecodeString("00090002000003e86553f1000000000700000013000000100100000200080004000200040100000cc000020900000011")
	require.NoError(t, err)
	for cut := range w {
		_, err := ParseStructured(w[:cut], "netflow_v9", "NetFlowV9Fields")
		require.Error(t, err)
	}
	_, err = ParseStructured(append(w, 0), "netflow_v9", "NetFlowV9Fields")
	require.Error(t, err)
}
