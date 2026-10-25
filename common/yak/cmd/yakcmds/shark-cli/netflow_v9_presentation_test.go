package sharkcli

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestNetFlowV9ProgressiveNamesDataRangesAndReportedFlowScope(t *testing.T) {
	w, err := hex.DecodeString("00090002000003e86553f1000000000700000013000000100100000200080004000200040100000cc000020900000011")
	require.NoError(t, err)
	d, err := stream_parser.DecodeNetFlowV9Wire(w, nil, 128)
	require.NoError(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "netflow-v9", Status: "decoded", Profile: "netflow-v9-directed-UDP-template-observer", Raw: w, Session: d.Fields, Summary: "NetFlow v9 · 1 template, 1 data record"}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "NetFlow v9", row.Protocol)
	require.Equal(t, []string{"Ethernet", "IPv4", "UDP", "NetFlow v9"}, row.ProtocolStack)
	require.Contains(t, row.Info, "1 template, 1 data record")
	require.True(t, (searchTerm{kind: "proto", value: "cflow"}).matches(nil, row, nil))
	fs, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	found := false
	for _, f := range fs {
		require.NotContains(t, f.text, "Wire Field Ranges")
		if strings.HasPrefix(f.text, "Reported Source IPv4: 192.0.2.9") {
			require.True(t, f.hasRange)
			require.Equal(t, 40, f.start)
			require.Equal(t, 44, f.end)
			require.Equal(t, []byte{192, 0, 2, 9}, sources[f.source-1].Data[f.start:f.end])
			found = true
		}
		if strings.Contains(f.text, "Template PDU SHA256") || strings.HasPrefix(f.text, "Template Scope:") {
			require.False(t, f.hasRange, "schema provenance does not fabricate current data bytes")
		}
	}
	require.True(t, found)
}

func TestNetFlowV9MissingTemplateKeepsOnlyProvenFraming(t *testing.T) {
	w, err := hex.DecodeString("00090001000003e86553f10000000007000000130100000cc000020900000011")
	require.NoError(t, err)
	d, err := stream_parser.DecodeNetFlowV9Wire(w, nil, 128)
	require.Error(t, err)
	e := &pcaputil.ProtocolEvent{Protocol: "netflow-v9", Status: "context-required", Profile: "netflow-v9-directed-UDP-template-observer", Raw: w, Session: d.Fields, Summary: "NetFlow v9 · template unavailable", Error: err.Error()}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "NetFlow v9", row.Protocol)
	require.Equal(t, "context-required", row.ProtocolStatus)
	require.Equal(t, "headers", row.Visibility)
	require.Contains(t, row.Info, "template unavailable")
	e.Raw, e.Session = w[:20], map[string]any{"Fixed Header Complete": true}
	row = presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Layers: []string{"Ethernet", "IPv4", "UDP"}})
	require.Equal(t, "UDP", row.Protocol, "a header/port hint alone does not prove complete framing")
}

func TestProtocolFieldViewBudgetIsVisibleAndUnrangedValuesStayUnranged(t *testing.T) {
	many := make([]map[string]any, 2000)
	for i := range many {
		many[i] = map[string]any{"Field Type": uint64(8), "Field Name": "Reported Source IPv4", "Value": "192.0.2.9"}
	}
	e := &pcaputil.ProtocolEvent{Protocol: "netflow-v9", Status: "decoded", Session: map[string]any{"Fields": many}}
	fs, _ := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Contains(t, fs[0].text, "Field view limited")
	require.Less(t, len(fs), 4128)
	for _, f := range fs {
		require.False(t, f.hasRange, "a display label is not a physical-byte proof")
	}
}
