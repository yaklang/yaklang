package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestNetFlowV5ProgressiveRecordsHaveCurrentPDUAddressRanges(t *testing.T) {
	w := make([]byte, 72)
	w[1], w[3] = 5, 1
	copy(w[24:], []byte{192, 0, 2, 9})
	copy(w[28:], []byte{198, 51, 100, 7})
	d, err := stream_parser.DecodeNetFlowV5Wire(w, 4096)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "netflow-v5", Status: "decoded", Completeness: "message", Raw: w, Session: d, Summary: "NetFlow v5 · 1 reported record(s)"}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "NetFlow v5", row.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "NetFlow v5"}, row.ProtocolStack)
	require.True(t, (searchTerm{kind: "proto", value: "cflow"}).matches(nil, row, nil))
	require.True(t, (searchTerm{kind: "proto", value: "netflow-v5"}).matches(nil, row, nil))
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	found := false
	nanoseconds, firstRecord := -1, -1
	for index, f := range fs {
		if strings.HasPrefix(f.text, "Unix Nanoseconds:") {
			nanoseconds = index
		}
		if strings.HasPrefix(f.text, "Records[0]") {
			firstRecord = index
		}
		require.NotContains(t, f.text, "Wire Field Ranges")
		if strings.HasPrefix(f.text, "Reported Source IPv4: 192.0.2.9") {
			found = true
			require.True(t, f.hasRange)
			require.Equal(t, 24, f.start)
			require.Equal(t, 28, f.end)
			require.Equal(t, []byte{192, 0, 2, 9}, sources[f.source-1].Data[f.start:f.end])
		}
		if strings.HasPrefix(f.text, "Sequence Scope:") {
			require.False(t, f.hasRange)
		}
	}
	require.True(t, found)
	require.GreaterOrEqual(t, nanoseconds, 0)
	require.Greater(t, firstRecord, nanoseconds, "every export-time header must precede the long record list")
	fields := e.DisplayFieldValues()
	require.False(t, fields.Incomplete)
	require.False(t, fields.Truncated)
	for _, f := range fields.Values {
		if f.Name == "cflow.srcaddr" {
			require.Equal(t, 24, f.Range.Offset)
			require.Equal(t, 4, f.Range.Length)
		}
	}
}
