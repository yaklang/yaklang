package sharkcli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestHTTP3MessageErrorKeepsSealedNativeIdentityInPacketList(t *testing.T) {
	packets, _ := top30Replay(t, top30Cases()[7])
	var e *pcaputil.ProtocolEvent
	for _, p := range packets {
		for _, candidate := range p.protocolEvents() {
			if candidate.AuthenticatedQUICApplicationProtocol() == "http3" {
				e = candidate
				break
			}
		}
		if e != nil {
			break
		}
	}
	require.NotNil(t, e)
	e.Status, e.Error, e.Protocol = "malformed", "HTTP3 invalid field section", "edited-public-label"
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp", Layers: []string{"Loopback", "IPv4", "UDP"}})
	require.Equal(t, "HTTP/3", row.Protocol)
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.Equal(t, "authenticated application bytes", row.ProtocolScope)
	require.Contains(t, row.ProtocolStack, "QUIC")
	require.Contains(t, row.ProtocolStack, "HTTP/3")
	require.Contains(t, row.Info, "invalid field section")
}

func TestPublicHTTP3FlagsCannotUpgradeUDPDiagnostic(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "http3", Transport: "udp", Profile: "http3-native", Admission: "authenticated-wire-and-observed-CID", Status: "malformed", Error: "invalid fields", Session: map[string]any{"Authentication Verified": true, "Native Carrier": true, "ALPN": "h3", "HTTP3": true}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Equal(t, "UDP", row.Protocol)
	require.NotContains(t, row.ProtocolStack, "HTTP/3")
}
