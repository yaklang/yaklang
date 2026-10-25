package sharkcli

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestRIPngProgressiveDetailsKeepMessageLocalByteRanges(t *testing.T) {
	w := []byte{2, 1, 0, 0}
	for _, a := range []string{"fe80::9", "2001:db8::"} {
		w = append(w, net.ParseIP(a).To16()...)
		if a == "fe80::9" {
			w = append(w, 0, 0, 0, 255)
		} else {
			w = append(w, 0, 7, 64, 16)
		}
	}
	m, err := stream_parser.DecodeRIPngWire(w, 8)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "ripng", Transport: "udp", Status: "decoded", Raw: w, Session: m, Summary: "RIPng Response"}
	r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv6", "UDP"}})
	require.Equal(t, "RIPng", r.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv6", "UDP", "RIPng"}, r.ProtocolStack)
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	seen := map[string]bool{}
	firstType, firstPrefix, firstAdvisory := -1, -1, -1
	for i, f := range fs {
		if strings.HasPrefix(f.text, "Entry Type:") && firstType < 0 {
			firstType = i
		}
		if strings.HasPrefix(f.text, "IPv6 Prefix:") && firstPrefix < 0 {
			firstPrefix = i
		}
		if strings.HasPrefix(f.text, "Effective Next Hop Advisory:") && firstAdvisory < 0 {
			firstAdvisory = i
		}
		require.NotContains(t, f.text, "Wire Field Ranges")
		for name, value := range map[string][]byte{"IPv6 Prefix: fe80::9": net.ParseIP("fe80::9").To16(), "IPv6 Prefix: 2001:db8::": net.ParseIP("2001:db8::").To16(), "Route Tag: 7": {0, 7}, "Metric: 255": {255}} {
			if strings.HasPrefix(f.text, name) {
				require.True(t, f.hasRange, name)
				require.Equal(t, value, sources[f.source-1].Data[f.start:f.end], name)
				seen[name] = true
			}
		}
		if strings.HasPrefix(f.text, "Effective Next Hop Advisory:") || strings.HasPrefix(f.text, "Canonical Prefix:") {
			require.False(t, f.hasRange, "derived fields must not invent physical or PDU byte offsets")
		}
	}
	require.Len(t, seen, 4)
	require.Greater(t, firstPrefix, firstType)
	require.Greater(t, firstAdvisory, firstPrefix)
}
