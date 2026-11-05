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

// A full ServerHello declaration, deliberately using an illegal TLS 1.3
// legacy_version. No negotiated version or record authentication is claimed.
func rejectedServerHelloPresentation(t *testing.T) *pcaputil.ProtocolEvent {
	t.Helper()
	body := append([]byte{3, 1}, bytes.Repeat([]byte{0x52}, 32)...)
	body = append(body, 0, 0x13, 2, 0, 0, 6, 0, 43, 0, 2, 3, 4)
	handshake := append([]byte{2, 0, 0, byte(len(body))}, body...)
	raw := []byte{22, 3, 3}
	raw = binary.BigEndian.AppendUint16(raw, uint16(len(handshake)))
	raw = append(raw, handshake...)
	return &pcaputil.ProtocolEvent{ID: 7, FlowID: 9, TCPFlowID: 3, Protocol: "tls", Transport: "tcp", Profile: "tls",
		Source: "127.0.0.1:443", Destination: "127.0.0.1:50990", Status: "malformed", ExpertCode: string(pcaputil.ErrMalformedMessage),
		Error: "MalformedMessage: TLS 1.3 ServerHello legacy parameters", Raw: raw, Length: len(raw),
		SourceBytes: pcaputil.ByteSource{Kind: "reassembled", SHA256: fmt.Sprintf("%x", sha256.Sum256(raw)), PacketRefs: []pcaputil.PacketReference{{Number: 1}}},
		Session: map[string]any{"Record Type": byte(22), "Record Version": uint16(0x303), "Record Length": len(raw) - 5,
			"Authentication Verified": false, "Content Visibility": "cleartext"}}
}

func TestTLSRejectedServerHelloKeepsObservedIdentity(t *testing.T) {
	e := rejectedServerHelloPresentation(t)
	for _, events := range [][]*pcaputil.ProtocolEvent{{e}, {e, {ID: 8, FlowID: 9, TCPFlowID: 3, Protocol: "tls", Transport: "tcp", Profile: "tls", Status: "context-required", ExpertCode: string(pcaputil.ErrContextRequired), Error: "ContextRequired: decode failure invalidated stream affinity"}}} {
		base := packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}}
		var streamed packetPresentation
		for _, event := range events {
			streamed.add(event)
		}
		for _, presentation := range []packetPresentation{presentEvents(events), streamed} {
			row := presentation.apply(base)
			require.Equal(t, "TLS", row.Protocol)
			require.Equal(t, "malformed", row.ProtocolStatus)
			require.Equal(t, "rejected TLS ServerHello", row.ProtocolScope)
			require.Equal(t, "cleartext", row.Visibility)
			require.Contains(t, row.ProtocolStack, "TLS")
			require.Contains(t, row.Info, "TLS 1.3 ServerHello legacy parameters")
			require.Equal(t, 1, strings.Count(row.Info, "TLS 1.3 ServerHello legacy parameters"))
			require.Contains(t, row.ProtocolEvidence, "negotiation not established")
			require.NotContains(t, row.Info, "AEAD verified")
		}
	}
	packet := tcpPacket(t, 1, 100, string(e.Raw), false, false, false, false)
	packet.events = []*pcaputil.ProtocolEvent{e}
	view := workerMetadataDetailForPacket(t, packet)
	require.Equal(t, "TLS", view.protocol)
	require.Contains(t, detailLines(view.fields), "TLS 1.3 ServerHello legacy parameters")
	require.NotContains(t, detailLines(view.fields), "Negotiated Version: TLSv1.3")
	require.NotContains(t, detailLines(view.fields), "handshake not captured")
	require.NotContains(t, detailLines(view.fields), "TLS? candidate")
}

func TestTLSRejectedServerHelloDetailsKeepLiteralSource(t *testing.T) {
	for _, name := range []string{"legacy-version", "compression"} {
		t.Run(name, func(t *testing.T) {
			e := rejectedServerHelloPresentation(t)
			label, start, end, expected := "Declared legacy version:", 9, 11, []byte{3, 1}
			if name == "compression" {
				e.Raw[9], e.Raw[10], e.Raw[46] = 3, 3, 1
				label, start, end, expected = "Declared compression:", 46, 47, []byte{1}
			}
			e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			text := detailLines(fields)
			require.NotContains(t, text, "handshake not captured")
			require.NotContains(t, text, "TLS? candidate")
			require.Contains(t, text, "Rejected ServerHello declaration")
			require.Contains(t, text, "Declared selected version: TLSv1.3 (0x0304) · not negotiated")
			found := false
			for _, f := range fields {
				if strings.HasPrefix(f.text, "Error:") {
					require.False(t, f.hasRange, "diagnosis is not a wire field")
				}
				if !strings.HasPrefix(f.text, label) {
					continue
				}
				found = true
				require.True(t, f.hasRange)
				require.Greater(t, f.source, 0)
				require.Equal(t, start, f.start)
				require.Equal(t, end, f.end)
				require.Equal(t, e.Raw, sources[f.source-1].Data)
				require.Equal(t, expected, sources[f.source-1].Data[f.start:f.end])
			}
			require.True(t, found)
		})
	}
}

func TestTLSRejectedServerHelloDoesNotGuessFromPrefixOrMetadata(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(*pcaputil.ProtocolEvent)
	}{
		{"no-id", func(e *pcaputil.ProtocolEvent) { e.ID = 0 }},
		{"no-flow", func(e *pcaputil.ProtocolEvent) { e.FlowID = 0 }},
		{"no-generation", func(e *pcaputil.ProtocolEvent) { e.TCPFlowID = 0 }},
		{"other-protocol", func(e *pcaputil.ProtocolEvent) { e.Protocol = "tcp" }},
		{"other-transport", func(e *pcaputil.ProtocolEvent) { e.Transport = "udp" }},
		{"other-profile", func(e *pcaputil.ProtocolEvent) { e.Profile = "port-hint" }},
		{"other-status", func(e *pcaputil.ProtocolEvent) { e.Status = "context-required" }},
		{"other-code", func(e *pcaputil.ProtocolEvent) { e.ExpertCode = string(pcaputil.ErrContextRequired) }},
		{"other-reason", func(e *pcaputil.ProtocolEvent) { e.Error = "MalformedMessage: TLS Certificate body" }},
		{"record-not-clear", func(e *pcaputil.ProtocolEvent) { e.Session["Content Visibility"] = "encrypted" }},
		{"record-authenticated", func(e *pcaputil.ProtocolEvent) { e.Session["Authentication Verified"] = true }},
		{"missing-auth-observation", func(e *pcaputil.ProtocolEvent) { delete(e.Session, "Authentication Verified") }},
		{"fabricated-negotiation", func(e *pcaputil.ProtocolEvent) { e.Session["Negotiated Version"] = 0x304 }},
		{"record-length", func(e *pcaputil.ProtocolEvent) { e.Session["Record Length"] = len(e.Raw) }},
		{"record-type", func(e *pcaputil.ProtocolEvent) { e.Session["Record Type"] = 23 }},
		{"record-version", func(e *pcaputil.ProtocolEvent) { e.Session["Record Version"] = 0x301 }},
		{"other-source-kind", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.Kind = "captured" }},
		{"wrong-hash", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = strings.Repeat("0", 64) }},
		{"no-references", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs = nil }},
		{"unresolved-reference", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Number = 0 }},
		{"other-domain", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Domain.Interface = 1 }},
		{"prefix-only", func(e *pcaputil.ProtocolEvent) { e.Raw = e.Raw[:16] }},
		{"client-type", func(e *pcaputil.ProtocolEvent) { e.Raw[5] = 1 }},
		{"broken-vector", func(e *pcaputil.ProtocolEvent) { e.Raw[len(e.Raw)-3]++ }},
		{"unselected-version", func(e *pcaputil.ProtocolEvent) { e.Raw[len(e.Raw)-1] = 3 }},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := rejectedServerHelloPresentation(t)
			c.edit(e)
			if c.name == "prefix-only" || c.name == "client-type" || c.name == "broken-vector" || c.name == "unselected-version" {
				e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
				e.Length = len(e.Raw)
				e.Session["Record Length"] = len(e.Raw) - 5
			}
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}})
			require.Equal(t, "TCP", row.Protocol)
			require.NotEqual(t, "rejected TLS ServerHello", row.ProtocolScope)
		})
	}
}
