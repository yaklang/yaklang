package sharkcli

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"strings"
	"testing"
)

func TestRoutingProgressiveDisplayFieldsAndNestedByteRanges(t *testing.T) {
	w := []byte{1, 0, 0, 1, 0, 0, 0, 1, 2, 3, 4, 5, 2, 20, 1, 32, 0, 100, 50, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0xb4, 0, 1, 1, 2, 20, 2, 4, 0, 101, 51, 0, 0, 0, 0, 0, 0, 0, 0, 7, 0xb4, 0, 1, 2}
	m, err := stream_parser.DecodeRoutingControl(w, "glbp", 32)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "glbp", Status: "decoded", Raw: w, Session: m}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "GLBP"}, row.ProtocolStack)
	require.Contains(t, row.Info, "forwarder=1")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	var weights, priorities [][]byte
	for _, f := range fields {
		require.NotContains(t, f.text, "Wire Field Ranges")
		require.NotContains(t, f.text, "Strong Identity")
		if strings.HasPrefix(f.text, "Weight:") || strings.HasPrefix(f.text, "Priority:") {
			require.True(t, f.hasRange, f.text)
			b := sources[f.source-1].Data[f.start:f.end]
			if strings.HasPrefix(f.text, "Weight:") {
				weights = append(weights, b)
			} else {
				priorities = append(priorities, b)
			}
		}
	}
	require.Equal(t, [][]byte{{50}, {51}}, weights)
	require.Equal(t, [][]byte{{100}, {101}}, priorities)
	// The gateway priority must not be populated from a forwarder TLV.
	for _, v := range e.DisplayFieldValues().Values {
		require.NotEqual(t, "glbp.hello.priority", v.Name)
	}
	require.True(t, bytes.Equal(w, e.Raw))
}
