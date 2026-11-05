package sharkcli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// Authored projections of capture394/control398 physical frame 5: a single
// datagram contains two admitted packets before the refused 1-RTT packet.
// Native AEAD, CRYPTO parsing and the real capture remain independent gates.
func quicCoalescedDiagnosticEvents(code, cause string) []*pcaputil.ProtocolEvent {
	var events []*pcaputil.ProtocolEvent
	for i, name := range []string{"Initial", "Handshake"} {
		events = append(events, &pcaputil.ProtocolEvent{ID: uint64(i + 1), FlowID: 3, Protocol: "quic", Transport: "udp", Status: "decoded", Admission: "authenticated-wire-and-observed-CID", Session: map[string]any{
			"Packet Name": name, "Packet Number": uint64(i + 1), "Authentication Verified": true,
		}})
	}
	events = append(events, &pcaputil.ProtocolEvent{ID: 3, FlowID: 3, Protocol: "quic", Transport: "udp", Status: "malformed", ExpertCode: code, Error: code + ": " + cause, Admission: "authenticated-wire-and-observed-CID", Session: map[string]any{
		"Authentication Verified": code != string(pcaputil.ErrAuthenticationFailed), "Peer Identity Verified": false,
		"Rejected Packet Payload": []byte{6, 0, 5, 24, 0, 0, 1, 0},
		"Payload Delivery":        "not delivered to protocol state; packet AEAD verified",
	}})
	if code == string(pcaputil.ErrContextRequired) || code == string(pcaputil.ErrAuthenticationFailed) {
		last := events[len(events)-1]
		last.Session["Authentication Verified"] = false
		delete(last.Session, "Rejected Packet Payload")
		delete(last.Session, "Payload Delivery")
		if code == string(pcaputil.ErrContextRequired) {
			last.Status, last.Admission = "context-required", "observed-CID-unverified-payload"
		}
	} else if code == string(pcaputil.ErrResourceExceeded) {
		events[len(events)-1].Status = "limited"
	}
	return events
}

func quicDiagnosticIPC(t *testing.T, events []*pcaputil.ProtocolEvent, ipc bool) []*pcaputil.ProtocolEvent {
	t.Helper()
	if !ipc {
		return events
	}
	wire, err := json.Marshal(events)
	require.NoError(t, err)
	var copied []*pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(wire, &copied))
	return copied
}

func TestQUICPhysicalFrameShowsPacketRefusalBeforeSamples(t *testing.T) {
	for _, profile := range []struct{ code, cause string }{
		{string(pcaputil.ErrMalformedMessage), "QUIC TLS handshake type 13 is not permitted at application encryption level"},
		{string(pcaputil.ErrAuthenticationFailed), "QUIC packet authentication failed"},
		{string(pcaputil.ErrContextRequired), "QUIC packet requires observed handshake context"},
		{string(pcaputil.ErrResourceExceeded), "QUIC CRYPTO retained range byte budget"},
	} {
		for _, ipc := range []bool{false, true} {
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/ipc%v/reverse%v", profile.code, ipc, reverse), func(t *testing.T) {
					events := quicDiagnosticIPC(t, quicCoalescedDiagnosticEvents(profile.code, profile.cause), ipc)
					if reverse {
						events[0], events[2] = events[2], events[0]
					}
					before, err := json.Marshal(events)
					require.NoError(t, err)
					row := presentEvents(events).apply(packetSummary{Protocol: "UDP", Transport: "udp", Layers: []string{"Loopback", "IPv4", "UDP"}})
					require.Equal(t, "QUIC", row.Protocol)
					require.True(t, strings.HasPrefix(row.Info, "QUIC · "+profile.code+" · "+profile.cause), row.Info)
					require.Contains(t, clipText(row.Info, 65), profile.code, "the diagnosis survives the physical packet list width")
					require.Contains(t, row.Info, "Initial")
					require.Contains(t, row.Info, "Handshake")
					require.Equal(t, 1, strings.Count(row.Info, profile.cause))
					require.Equal(t, "mixed-packet-outcomes", row.ProtocolStatus, "admitted neighbours do not certify every packet")
					require.Equal(t, "mixed-packet-outcomes", row.Visibility)
					require.Equal(t, "mixed QUIC datagram outcomes", row.ProtocolScope)
					after, err := json.Marshal(events)
					require.NoError(t, err)
					require.Equal(t, before, after, "presentation cannot change parser events or byte sources")
				})
			}
		}
	}
}

func TestQUICPhysicalFrameRejectionDetailsPrecedeLargeGoodTree(t *testing.T) {
	for _, ipc := range []bool{false, true} {
		t.Run(fmt.Sprintf("ipc%v", ipc), func(t *testing.T) {
			events := quicCoalescedDiagnosticEvents(string(pcaputil.ErrMalformedMessage), "TLS KeyUpdate is prohibited in QUIC")
			for i := 0; i < 80; i++ {
				events[0].Session[fmt.Sprintf("Earlier wire field %02d", i)] = strings.Repeat("literal", 20)
			}
			events = quicDiagnosticIPC(t, events, ipc)
			fields, sources := protocolEventDetail(events)
			var lines []string
			diagnostic, payload := 0, false
			for i, f := range fields {
				if i < 8 {
					lines = append(lines, f.text)
				}
				if strings.HasPrefix(f.text, "Packet diagnostic:") {
					diagnostic++
				}
				if strings.HasPrefix(f.text, "Rejected Packet Payload:") {
					require.True(t, f.hasRange)
					require.Equal(t, []byte{6, 0, 5, 24, 0, 0, 1, 0}, sources[f.source-1].Data[f.start:f.end])
					payload = true
				}
			}
			require.Contains(t, strings.Join(lines, "\n"), "TLS KeyUpdate is prohibited in QUIC", "diagnosis precedes other coalesced packet trees")
			require.Equal(t, 1, diagnostic, "frontloading must not duplicate the same packet error")
			require.True(t, payload, "the rejected packet's owned plaintext remains inspectable")
		})
	}
}

func TestQUICPhysicalFrameDiagnosticKeepsApplicationIdentity(t *testing.T) {
	events := quicCoalescedDiagnosticEvents(string(pcaputil.ErrMalformedMessage), "TLS KeyUpdate is prohibited in QUIC")
	application := &pcaputil.ProtocolEvent{ID: 4, Protocol: "http3", Transport: "udp", Status: "decoded", Summary: "HEADERS stream=0"}
	application.SourceBytes.Kind = "decrypted"
	events = append(events, application)
	for _, ipc := range []bool{false, true} {
		row := presentEvents(quicDiagnosticIPC(t, events, ipc)).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
		require.Equal(t, "HTTP/3", row.Protocol, "a carrier error does not replace a separately decoded application identity")
		require.Contains(t, clipText(row.Info, 80), "MalformedMessage")
		require.Equal(t, "mixed-packet-outcomes", row.ProtocolStatus)
		require.Contains(t, row.ProtocolStack, "QUIC")
	}
}

func TestQUICPhysicalFrameKeepsOriginalRefusalBeforeLaterContext(t *testing.T) {
	events := quicCoalescedDiagnosticEvents(string(pcaputil.ErrMalformedMessage), "TLS KeyUpdate is prohibited in QUIC")
	consequence := quicCoalescedDiagnosticEvents(string(pcaputil.ErrContextRequired), "later packet requires unavailable context")[2]
	consequence.ID = 4
	events = append(events, consequence)
	row := presentEvents(events).apply(packetSummary{Protocol: "UDP", Transport: "udp"})
	require.True(t, strings.HasPrefix(row.Info, "QUIC · MalformedMessage · TLS KeyUpdate is prohibited in QUIC"), row.Info)
	require.Contains(t, row.Info, "later packet requires unavailable context")
	require.Equal(t, 1, strings.Count(row.Info, "TLS KeyUpdate is prohibited in QUIC"))
}

func TestQUICDiagnosticWithoutObservedContextDoesNotEstablishIdentity(t *testing.T) {
	e := &pcaputil.ProtocolEvent{Protocol: "quic", Transport: "udp", Status: "malformed", ExpertCode: string(pcaputil.ErrMalformedMessage), Error: "MalformedMessage: unconfirmed short header candidate", Session: map[string]any{"Authentication Verified": false}}
	for _, ipc := range []bool{false, true} {
		row := presentEvents(quicDiagnosticIPC(t, []*pcaputil.ProtocolEvent{e}, ipc)).apply(packetSummary{Protocol: "UDP", Transport: "udp", Layers: []string{"IPv4", "UDP"}})
		require.Equal(t, "UDP", row.Protocol)
		require.NotContains(t, row.ProtocolStack, "QUIC")
		require.NotEqual(t, "authenticated-plaintext", row.Visibility)
	}
}
