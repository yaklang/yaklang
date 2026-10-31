package sharkcli

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"testing"
)

func TestTLSCarriersUseProgressivePresentation(t *testing.T) {
	for _, layers := range [][]string{{"RDPUDP2"}, {"RADIUS", "PEAP"}, {"RADIUS", "EAP-TTLS"}} {
		parent := &pcaputil.ProtocolEvent{Protocol: "rdpudp", Status: "decoded", Session: map[string]any{"Protocol Version": uint16(0x101)}}
		e := &pcaputil.ProtocolEvent{Protocol: "tls", Status: "decoded", Transport: "udp", CarrierLayers: layers, Session: map[string]any{"Negotiated Version": uint16(0x303)}}
		require.Greater(t, eventPriority(e), eventPriority(parent))
		require.Equal(t, layers, eventCarriers(e))
		require.Equal(t, "TLSv1.2", eventProtocolName(e))
		copied := eventCarriers(e)
		copied[0] = "edited"
		require.NotEqual(t, copied, e.CarrierLayers)
		var p packetPresentation
		p.add(parent)
		p.add(e)
		require.Equal(t, "TLSv1.2", p.protocol)
	}
}

func TestEvictedProtocolDetailIsExplicit(t *testing.T) {
	h, err := pcaputil.NewProtocolInspector(1, 1<<20)
	require.NoError(t, err)
	h.OnEvent(&pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Fields: map[string]any{"Type": 23}})
	h.OnEvent(&pcaputil.ProtocolEvent{ID: 2, Protocol: "tls", Status: "decoded", Fields: map[string]any{"Type": 23}})
	p := &capturedPacket{history: h, eventIDs: []uint64{1}}
	es := p.protocolEvents()
	require.Len(t, es, 1)
	require.Equal(t, "evicted", es[0].Completeness)
	fields, _ := protocolEventDetail(es)
	require.NotEmpty(t, fields)
}

func TestRDPUDPSelectedVersionReplacesCarrierWithoutInventingNesting(t *testing.T) {
	var p packetPresentation
	for _, label := range []string{"RDPUDP", "RDPUDP2", "TLS", "TLSv1.2", "RDPUDP"} {
		p.addLayer(label)
	}
	require.Equal(t, []string{"RDPUDP2", "TLSv1.2"}, p.stack)
}
