package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestLDPDiscoveryProgressionAndNestedByteCoordinates(t *testing.T) {
	w := []byte{0, 1, 0, 30, 192, 0, 2, 10, 0, 7, 1, 0, 0, 20, 0, 0, 0, 9, 4, 0, 0, 4, 0, 15, 0x20, 0, 4, 1, 0, 4, 192, 0, 2, 11}
	m, err := stream_parser.DecodeLDPDiscovery(w, 32)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "ldp", Status: "decoded", Raw: w, Session: m, Summary: "LDP Hello"}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "LDP", r.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "LDP"}, r.ProtocolStack)
	require.Contains(t, r.Info, "LSR 192.0.2.10:7")
	require.Contains(t, r.Info, "advertised hold=15 s")
	require.Contains(t, r.Info, "transport=192.0.2.11")
	require.NotContains(t, r.Info, "Packet Name=")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	want := map[string][]byte{"LSR ID:": {192, 0, 2, 10}, "Message ID:": {0, 0, 0, 9}, "Hold Time:": {0, 15}, "IPv4 Transport Address:": {192, 0, 2, 11}, "GTSM:": {0x20, 0}}
	seen := map[string]bool{}
	for _, f := range fields {
		require.NotContains(t, f.text, "Wire Field Ranges")
		for prefix, expected := range want {
			if strings.HasPrefix(f.text, prefix) {
				require.True(t, f.hasRange, f.text)
				require.Equal(t, expected, sources[f.source-1].Data[f.start:f.end], f.text)
				seen[prefix] = true
			}
		}
	}
	require.Len(t, seen, len(want))
	e.Status, e.Error = "incomplete", "missing Hello parameters"
	r = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP"})
	require.Equal(t, "UDP", r.Protocol, "partial header cannot establish LDP")
}
