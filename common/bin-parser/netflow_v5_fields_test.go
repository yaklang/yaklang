package bin_parser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

func TestNetFlowV5CanonicalAndLegacyRecordScope(t *testing.T) {
	w := make([]byte, 120)
	w[1], w[3] = 5, 2
	w[24], w[72] = 192, 198
	binary.BigEndian.PutUint16(w[22:], 0x8123)
	protocolCorpusRequireBoundedRuleParse(t, w, "application-layer.netflow_v5", "NetFlowV5")
	v, err := ParseStructured(w, "netflow_v5", "NetFlowV5Fields")
	require.NoError(t, err)
	m := v["fields"].(map[string]any)["Message"].(map[string]any)
	direct, err := stream_parser.DecodeNetFlowV5Wire(w, 4096)
	require.NoError(t, err)
	require.Equal(t, direct["Protocol Name"], m["Protocol Name"])
	require.Equal(t, uint64(2), m["Data Record Count"])
	for cut := 0; cut < len(w); cut++ {
		_, err = ParseStructured(w[:cut], "netflow_v5", "NetFlowV5Fields")
		require.Error(t, err)
	}
}
