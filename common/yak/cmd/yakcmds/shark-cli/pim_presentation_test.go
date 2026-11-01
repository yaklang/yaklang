package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestPIMControlProgressiveFieldsAndDummyWording(t *testing.T) {
	w := []byte{0x23, 0, 0, 0, 1, 0, 192, 0, 2, 2, 0, 1, 0, 210, 1, 0, 0, 32, 239, 1, 2, 3, 0, 1, 0, 0, 1, 0, 7, 32, 192, 0, 2, 10}
	m, err := stream_parser.DecodePIMControl(w, stream_parser.PIMWireContext{}, 32)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "pim", Transport: "network", Status: "decoded", Raw: w, Session: m}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "PIMv2", Layers: []string{"Ethernet", "IPv4", "PIMv2"}})
	require.Equal(t, "PIMv2", r.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "PIMv2"}, r.ProtocolStack)
	require.Contains(t, r.Info, "Join/Prune")
	require.Contains(t, r.Info, "joins=1 prunes=0")
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	want := map[string][]byte{"Address: 239.1.2.3": {239, 1, 2, 3}, "Address: 192.0.2.10": {192, 0, 2, 10}}
	seen := map[string]bool{}
	for _, f := range fs {
		require.NotContains(t, f.text, "Wire Field Ranges")
		for text, b := range want {
			if strings.HasPrefix(f.text, text) {
				require.True(t, f.hasRange)
				require.Equal(t, b, sources[f.source-1].Data[f.start:f.end])
				seen[text] = true
			}
		}
	}
	require.Len(t, seen, 2)
	e.Session = map[string]any{"Version": uint64(2), "Type": uint64(1), "Packet Name": "Register", "Null Register": true, "Register IP Header": map[string]any{"Source": "192.0.2.10", "Group": "239.1.2.3"}}
	r = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "PIMv2", Layers: []string{"IPv4", "PIMv2"}})
	require.Contains(t, r.Info, "Null-Register")
	require.Contains(t, r.Info, "dummy header; advertised addresses only")
	inner := &pcaputil.ProtocolEvent{Protocol: "dns", Status: "decoded", Fields: map[string]any{"ID": uint64(1)}}
	r = presentEvents([]*pcaputil.ProtocolEvent{e, inner}).apply(packetSummary{Protocol: "UDP", Layers: []string{"IPv4", "PIMv2", "IPv4", "UDP"}})
	require.Equal(t, "DNS", r.Protocol, "Register control fields must not hide the admitted data protocol")
}

func TestPIMDummyFieldsLeadWithAdvertisedAddressesAndOwnedRanges(t *testing.T) {
	w := []byte{0x21, 0, 0, 0, 0x40, 0, 0, 0, 0x45, 0, 0, 20, 0, 0, 0, 0, 0, 103, 0, 0, 192, 0, 2, 10, 239, 1, 2, 3}
	m, err := stream_parser.DecodePIMControl(w, stream_parser.PIMWireContext{}, 32)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "pim", Status: "decoded", Raw: w, Session: m}
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	dummy, checksum := -1, -1
	seen := 0
	for i, f := range fs {
		require.NotContains(t, f.text, "Encapsulated Bytes:", "the complete PDU already has a byte source")
		if strings.HasPrefix(f.text, "Dummy Header:") {
			dummy = i
		}
		if strings.HasPrefix(f.text, "Checksum:") {
			checksum = i
		}
		for text, want := range map[string][]byte{"Source: 192.0.2.10": {192, 0, 2, 10}, "Group: 239.1.2.3": {239, 1, 2, 3}} {
			if strings.HasPrefix(f.text, text) {
				require.True(t, f.hasRange)
				require.Equal(t, want, sources[f.source-1].Data[f.start:f.end])
				seen++
			}
		}
	}
	require.GreaterOrEqual(t, dummy, 0)
	require.Greater(t, checksum, dummy)
	require.Equal(t, 2, seen)
}
