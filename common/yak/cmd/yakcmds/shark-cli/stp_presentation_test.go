package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestSTPProgressiveVariantNamesAndActualByteRanges(t *testing.T) {
	for _, pvst := range []bool{false, true} {
		w := []byte{0, 0, 2, 2, 0x3c, 0x80, 1, 2, 0, 0, 0, 0, 1, 0, 0, 0, 19, 0x90, 1, 2, 0, 0, 0, 0, 2, 0x80, 3, 0, 128, 20, 0, 2, 0, 15, 0, 0}
		name := "RSTP"
		path := []string{"Ethernet", "LLC"}
		if pvst {
			w = append(w, 0, 0, 0, 2, 0, 121)
			name = "RPVST+"
			path = append(path, "SNAP")
		}
		m, err := stream_parser.DecodeSTPWire(w, pvst, 128)
		require.NoError(t, err)
		e := &pcaputil.ProtocolEvent{Protocol: "stp", Transport: "ethernet", Status: "decoded", Raw: w, Session: m, Summary: name + " Rapid Configuration · advertised root 32768/1/02:00:00:00:00:01"}
		r := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "LLC", Layers: path})
		require.Equal(t, name, r.Protocol)
		require.Equal(t, append(path, name), r.ProtocolStack)
		legacy := append(append([]string(nil), path...), "STP")
		r = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "STP", Layers: legacy})
		require.Equal(t, append(path, name), r.ProtocolStack, "generic observer STP and native variant are the same layer")
		for _, query := range []string{"proto:stp", "proto:" + name} {
			term, err := compileSearchTerm(query)
			require.NoError(t, err)
			require.True(t, term.matches(nil, r, nil), query)
		}
		fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		seen := map[string]bool{}
		want := map[string][]byte{"Root Priority: 32768": {0x80, 1}, "Root MAC: 02:00:00:00:00:01": {2, 0, 0, 0, 0, 1}, "Message Age Seconds: 0.5": {0, 128}}
		if pvst {
			want["Originating VLAN: 121"] = []byte{0, 121}
		}
		for _, f := range fs {
			require.NotContains(t, f.text, "Wire Field Ranges")
			for key, b := range want {
				if strings.HasPrefix(f.text, key) {
					require.True(t, f.hasRange, key)
					require.Equal(t, b, sources[f.source-1].Data[f.start:f.end], key)
					seen[key] = true
				}
			}
		}
		require.Len(t, seen, len(want))
	}
}
