package sharkcli

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

// The presentation layer consumes native authentication evidence; it does not
// authenticate this synthetic record itself. Real AEAD controls are replayed
// separately through the SDK, worker and Windows page.
func authenticatedTLSDiagnosticFixture(t *testing.T, reason string) *pcaputil.ProtocolEvent {
	t.Helper()
	e := certificateRequestPresentation(t, true)
	e.FlowID, e.TCPFlowID, e.Transport = 9, 3, "tcp"
	e.Status, e.ExpertCode, e.Error = "malformed", string(pcaputil.ErrMalformedMessage), "MalformedMessage: "+reason
	delete(e.Session, "Handshake Messages")
	e.Session["Record Type"], e.Session["Record Version"], e.Session["Record Length"] = byte(23), uint16(0x303), len(e.Raw)-5
	e.Session["Inner Content Type"], e.Session["Cipher Suite"] = byte(22), uint16(0x1302)
	e.SourceBytes = pcaputil.ByteSource{Kind: "reassembled", SHA256: fmt.Sprintf("%x", sha256.Sum256(e.Raw)), PacketRefs: []pcaputil.PacketReference{{Number: 1}}}
	return e
}

func TestTLSAuthenticatedHandshakeDiagnosticKeepsRecordIdentity(t *testing.T) {
	for _, reason := range []string{"TLS post-handshake authentication not offered", "repeated TLS CertificateRequest context"} {
		t.Run(reason, func(t *testing.T) {
			e := authenticatedTLSDiagnosticFixture(t, reason)
			base := packetSummary{Protocol: "TCP", Transport: "tcp", Layers: []string{"IPv4", "TCP"}}
			var streamed packetPresentation
			streamed.add(e)
			for _, p := range []packetPresentation{presentEvents([]*pcaputil.ProtocolEvent{e}), streamed} {
				row := p.apply(base)
				require.Equal(t, "TLSv1.3", row.Protocol)
				require.Equal(t, "malformed", row.ProtocolStatus)
				require.Equal(t, "authenticated TLS record diagnostic", row.ProtocolScope)
				require.Equal(t, "authenticated-plaintext", row.Visibility)
				require.Equal(t, 1, strings.Count(row.Info, reason))
				require.Contains(t, row.ProtocolStack, "TLSv1.3")
			}
			packet := tcpPacket(t, 1, 100, string(e.Raw), false, false, false, false)
			packet.events = []*pcaputil.ProtocolEvent{e}
			view := workerMetadataDetailForPacket(t, packet)
			require.Equal(t, "TLSv1.3", view.protocol)
			require.Contains(t, detailLines(view.fields), reason)
			stream := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)
			require.Contains(t, detailLines(stream.fields), reason)
			require.Contains(t, detailLines(stream.fields), "Negotiated Version: TLSv1.3")
		})
	}
}

func TestTLSUnauthenticatedDiagnosticDoesNotBorrowRecordIdentity(t *testing.T) {
	for _, c := range []struct {
		name string
		edit func(*pcaputil.ProtocolEvent)
	}{
		{"unverified", func(e *pcaputil.ProtocolEvent) { e.Session["Authentication Verified"] = false }},
		{"unknown-version", func(e *pcaputil.ProtocolEvent) { e.Session["Negotiated Version"] = 0 }},
		{"unknown-suite", func(e *pcaputil.ProtocolEvent) { e.Session["Cipher Suite"] = 0 }},
		{"record-version", func(e *pcaputil.ProtocolEvent) {
			e.Raw[2] = 1
			e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
		}},
		{"record-length", func(e *pcaputil.ProtocolEvent) {
			e.Raw[4]--
			e.SourceBytes.SHA256 = fmt.Sprintf("%x", sha256.Sum256(e.Raw))
		}},
		{"foreign-source-hash", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.SHA256 = strings.Repeat("0", 64) }},
		{"missing-contributors", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs = nil }},
		{"foreign-domain", func(e *pcaputil.ProtocolEvent) { e.SourceBytes.PacketRefs[0].Domain.Interface = 1 }},
		{"missing-flow", func(e *pcaputil.ProtocolEvent) { e.FlowID = 0 }},
		{"ciphertext", func(e *pcaputil.ProtocolEvent) { e.Session["Content Visibility"] = "encrypted" }},
		{"application-kind", func(e *pcaputil.ProtocolEvent) { e.Session["Inner Content Type"] = byte(23) }},
		{"other-protocol", func(e *pcaputil.ProtocolEvent) { e.Protocol = "tcp" }},
		{"missing-error", func(e *pcaputil.ProtocolEvent) { e.Error = "" }},
		{"other-refusal", func(e *pcaputil.ProtocolEvent) { e.ExpertCode = string(pcaputil.ErrContextRequired) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := authenticatedTLSDiagnosticFixture(t, "repeated TLS CertificateRequest context")
			c.edit(e)
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
			require.Equal(t, "TCP", row.Protocol)
		})
	}
}

func TestTLSAuthenticatedDiagnosticKeepsCoalescedOutcomes(t *testing.T) {
	for _, application := range []bool{false, true} {
		for _, diagnosticFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("application%t/diagnostic-first%t", application, diagnosticFirst), func(t *testing.T) {
				reason := "repeated TLS CertificateRequest context"
				bad := authenticatedTLSDiagnosticFixture(t, reason)
				good := certificateRequestPresentation(t, true)
				label := "TLSv1.3"
				if application {
					good = &pcaputil.ProtocolEvent{ID: 2, Protocol: "http", Status: "decoded", Summary: "GET /keep HTTP/1.1", Raw: []byte("GET /keep HTTP/1.1\r\n\r\n"), Fields: map[string]any{"Protocol": "HTTP/1.1"}, SourceBytes: pcaputil.ByteSource{Kind: "decrypted"}}
					label = "HTTP/1.1"
				}
				events := []*pcaputil.ProtocolEvent{good, bad}
				if diagnosticFirst {
					events[0], events[1] = events[1], events[0]
				}
				row := presentEvents(events).apply(packetSummary{Protocol: "TCP", Transport: "tcp"})
				require.Equal(t, label, row.Protocol)
				require.Equal(t, "mixed-record-outcomes", row.ProtocolStatus)
				require.Equal(t, 1, strings.Count(row.Info, reason))
				if application {
					require.Contains(t, row.Info, "GET")
				}
			})
		}
	}
}
