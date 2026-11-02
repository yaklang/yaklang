package sharkcli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func rejectedCertificatePSKPresentation(t *testing.T, client bool) *pcaputil.ProtocolEvent {
	body := append([]byte{3, 3}, bytes.Repeat([]byte{0x31}, 32)...)
	body = append(body, 0)
	kind := byte(2)
	ext := []byte{0, 33, 0, 1, 0xff, 0, 43, 0, 2, 3, 4}
	if client {
		kind = 1
		body = append(body, 0, 2, 0x13, 2, 1, 0)
		ext = []byte{0, 33, 0, 1, 0xff, 0, 43, 0, 3, 2, 3, 4}
	} else {
		body = append(body, 0x13, 2, 0)
	}
	body = binary.BigEndian.AppendUint16(body, uint16(len(ext)))
	body = append(body, ext...)
	hs := append([]byte{kind, 0, 0, byte(len(body))}, body...)
	raw := append(binary.BigEndian.AppendUint16([]byte{22, 3, 3}, uint16(len(hs))), hs...)
	e := rejectedServerHelloPresentation(t)
	e.Raw, e.Length = raw, len(raw)
	e.Error = "MalformedMessage: TLS certificate-with-external-PSK flag must be empty"
	e.Session["Record Length"] = len(raw) - 5
	e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(raw))
	return e
}

func TestTLSCertificatePSKRejectedHelloPresentation(t *testing.T) {
	for _, client := range []bool{false, true} {
		t.Run(fmt.Sprintf("client%v", client), func(t *testing.T) {
			e := rejectedCertificatePSKPresentation(t, client)
			name := "ServerHello"
			if client {
				name = "ClientHello"
			}
			stop := &pcaputil.ProtocolEvent{ID: 8, FlowID: 9, TCPFlowID: 3, Protocol: "tls", Transport: "tcp", Profile: "tls", Status: "context-required", ExpertCode: string(pcaputil.ErrContextRequired), Error: "ContextRequired: decode failure invalidated stream affinity"}
			stop.Raw, stop.Length = []byte{23, 3, 3, 0, 1, 0}, 6
			stop.SourceBytes.Kind = "reassembled"
			for _, events := range [][]*pcaputil.ProtocolEvent{{e}, {e, stop}} {
				row := presentEvents(events).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
				require.Equal(t, "TLS", row.Protocol)
				require.Equal(t, "rejected TLS "+name, row.ProtocolScope)
				require.Contains(t, row.ProtocolEvidence, "negotiation not established")
				packet := tcpPacket(t, 1, 100, string(e.Raw), false, false, false, false)
				packet.events = events
				view := workerMetadataDetailForPacket(t, packet)
				require.Equal(t, "TLS", view.protocol)
				text := detailLines(view.fields)
				require.NotContains(t, text, "handshake not captured")
				require.NotContains(t, text, "TLS? candidate")
				require.Contains(t, text, "TLS "+name+" (rejected)")
				require.Contains(t, text, "Certificate/PSK extension data: 1 bytes")
				found := false
				for _, f := range view.fields {
					if strings.HasPrefix(f.text, "Error:") {
						require.False(t, f.hasRange)
					}
					if !strings.HasPrefix(f.text, "Certificate/PSK extension data:") {
						continue
					}
					found = true
					require.True(t, f.hasRange)
					require.Greater(t, f.source, 0)
					source := view.sources[f.source-1].Data
					require.Equal(t, e.Raw, source)
					require.Equal(t, []byte{0xff}, source[f.start:f.end])
				}
				require.True(t, found)
			}
		})
	}
}

func TestTLSCertificatePSKRejectedHelloNeedsOwnedCompleteEvidence(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(*pcaputil.ProtocolEvent)
	}{
		{"no-ID", func(e *pcaputil.ProtocolEvent) { e.ID = 0 }},
		{"no-flow", func(e *pcaputil.ProtocolEvent) { e.FlowID = 0 }},
		{"no-generation", func(e *pcaputil.ProtocolEvent) { e.TCPFlowID = 0 }},
		{"other-protocol", func(e *pcaputil.ProtocolEvent) { e.Protocol = "tcp" }},
		{"other-transport", func(e *pcaputil.ProtocolEvent) { e.Transport = "udp" }},
		{"other-status", func(e *pcaputil.ProtocolEvent) { e.Status = "decoded" }},
		{"other-code", func(e *pcaputil.ProtocolEvent) { e.ExpertCode = "ContextRequired" }},
		{"port-hint", func(e *pcaputil.ProtocolEvent) { e.Profile = "port-hint" }},
		{"other-error", func(e *pcaputil.ProtocolEvent) { e.Error = "MalformedMessage: TLS arbitrary failure" }},
		{"invented-version", func(e *pcaputil.ProtocolEvent) { e.Session["Negotiated Version"] = 0x304 }},
		{"no-auth-observation", func(e *pcaputil.ProtocolEvent) { delete(e.Session, "Authentication Verified") }},
		{"authenticated", func(e *pcaputil.ProtocolEvent) { e.Session["Authentication Verified"] = true }},
		{"encrypted", func(e *pcaputil.ProtocolEvent) { e.Session["Content Visibility"] = "encrypted" }},
		{"record-type", func(e *pcaputil.ProtocolEvent) { e.Session["Record Type"] = 23 }},
		{"record-version", func(e *pcaputil.ProtocolEvent) { e.Session["Record Version"] = 0x301 }},
		{"record-length", func(e *pcaputil.ProtocolEvent) { e.Session["Record Length"] = len(e.Raw) }},
		{"event-length", func(e *pcaputil.ProtocolEvent) { e.Length++ }},
		{"captured-prefix", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.Kind = "captured" }},
		{"wrong-hash", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = "00" }},
		{"missing-source", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs = nil }},
		{"zero-source-number", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Number = 0 }},
		{"other-domain", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Domain.Interface = 1 }},
		{"truncated-tail", func(e *pcaputil.ProtocolEvent) {
			e.Raw = e.Raw[:len(e.Raw)-1]
			e.Length = len(e.Raw)
			e.Session["Record Length"] = len(e.Raw) - 5
			binary.BigEndian.PutUint16(e.Raw[3:], uint16(len(e.Raw)-5))
			e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := rejectedCertificatePSKPresentation(t, true)
			c.edit(e)
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
			require.NotEqual(t, "rejected TLS ClientHello", row.ProtocolScope)
		})
	}
}
