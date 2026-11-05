package sharkcli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// The real source394/control398 positive NST is decoded, but its old list row
// said only 1-RTT/pn. This projection verifies visibility independently of AEAD.
func quicTicketSummaryEvent() *pcaputil.ProtocolEvent {
	return &pcaputil.ProtocolEvent{ID: 9, FlowID: 3, Protocol: "quic", Transport: "udp", Status: "decoded", Admission: "authenticated-wire-and-observed-CID", Session: map[string]any{
		"Native Carrier": true, "Authentication Verified": true, "Packet Name": "1-RTT", "Packet Number": uint64(10), "Packet Number Space": "application",
		"TLS Handshakes": []map[string]any{{"Type": uint8(4), "Length": 22, "Authenticated": true, "New Session Ticket": map[string]any{"Lifetime Hint": uint32(60)}}},
	}}
}

func TestQUICTLSMessageNamePrecedesCoalescedPacketSamples(t *testing.T) {
	for _, ipc := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("ipc%v/reverse%v", ipc, reverse), func(t *testing.T) {
				var events []*pcaputil.ProtocolEvent
				for i := 0; i < 5; i++ {
					events = append(events, &pcaputil.ProtocolEvent{ID: uint64(i + 1), Protocol: "quic", Status: "decoded", Session: map[string]any{"Packet Name": "Handshake", "Packet Number": uint64(i), "Authentication Verified": true}})
				}
				if reverse {
					events = append([]*pcaputil.ProtocolEvent{quicTicketSummaryEvent()}, events...)
				} else {
					events = append(events, quicTicketSummaryEvent())
				}
				row := presentEvents(quicDiagnosticIPC(t, events, ipc)).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
				require.Equal(t, "QUIC", row.Protocol)
				require.True(t, strings.HasPrefix(row.Info, "TLS New Session Ticket"), row.Info)
				require.Contains(t, clipText(row.Info, 65), "New Session Ticket")
				require.Contains(t, row.Info, "1-RTT")
				require.Contains(t, row.Info, "pn=10")
				require.Contains(t, row.Info, "Handshake", "coalesced packet evidence remains")
			})
		}
	}
}

func TestQUICTLSMessageNameRequiresQualifiedCurrentMessage(t *testing.T) {
	for _, mode := range []string{"no-native-carrier", "packet-not-authenticated", "message-not-authenticated", "wrong-level", "prohibited-KeyUpdate", "no-envelope-length", "packet-refused"} {
		t.Run(mode, func(t *testing.T) {
			e := quicTicketSummaryEvent()
			message := e.Session["TLS Handshakes"].([]map[string]any)[0]
			switch mode {
			case "no-native-carrier":
				e.Session["Native Carrier"] = false
			case "packet-not-authenticated":
				e.Session["Authentication Verified"] = false
			case "message-not-authenticated":
				message["Authenticated"] = false
			case "wrong-level":
				e.Session["Packet Number Space"] = "handshake"
			case "prohibited-KeyUpdate":
				message["Type"] = uint8(24)
			case "no-envelope-length":
				delete(message, "Length")
			case "packet-refused":
				e.Status, e.Error = "malformed", "MalformedMessage: CRYPTO message refused"
			}
			for _, ipc := range []bool{false, true} {
				summary := applicationEventSummary(quicDiagnosticIPC(t, []*pcaputil.ProtocolEvent{e}, ipc)[0])
				require.NotContains(t, summary, "TLS New Session Ticket")
				require.NotContains(t, summary, "TLS Key Update")
			}
		})
	}
}

func TestQUICInitialHelloNameDoesNotInventNegotiatedVersion(t *testing.T) {
	e := quicTicketSummaryEvent()
	e.Session["Packet Name"], e.Session["Packet Number Space"] = "Initial", "initial"
	e.Session["TLS Handshakes"] = []map[string]any{{"Type": uint8(1), "Length": 100, "Authenticated": false}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.Contains(t, row.Info, "TLS Client Hello")
	require.Equal(t, "QUIC", row.Protocol)
	require.NotContains(t, row.Info, "TLSv1.3")
}

func TestQUICIncompleteCRYPTOIsVisibleWithoutInventingMessageType(t *testing.T) {
	e := quicTicketSummaryEvent()
	delete(e.Session, "TLS Handshakes")
	e.Session["CRYPTO Incomplete"] = true
	summary := applicationEventSummary(e)
	require.Contains(t, summary, "TLS handshake reassembly incomplete")
	require.NotContains(t, summary, "New Session Ticket")
}
