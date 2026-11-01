package sharkcli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestTLSPHAResponseAssociationFollowsOwnedBody(t *testing.T) {
	for _, kind := range []string{"certificate", "verify", "finished", "prefix-unavailable"} {
		t.Run(kind, func(t *testing.T) {
			e := certificatePresentation(t)
			body, encoded := "Certificate body", "Encoded Certificate"
			stage := "awaiting CertificateVerify"
			if kind == "verify" {
				e, body, encoded, stage = certificateVerifyPresentation(t, 0x304, "application"), "Certificate Verify body", "Encoded Certificate Verify", "awaiting Finished"
			} else if kind == "finished" || kind == "prefix-unavailable" {
				e, body, encoded, stage = finishedPresentation(0x304, 0x1301, "application"), "Finished body", "Encoded Finished", "response block complete; transcript not verified"
			}
			association := map[string]any{"Context Association": "matched observed CertificateRequest", "Request Ordinal": uint64(2), "Response Stage": stage, "Certificate Selection": "certificate list supplied", "Verification Scope": "observed message order; signature, Finished MAC and certificate trust not evaluated"}
			if kind == "prefix-unavailable" {
				delete(association, "Request Ordinal")
				association["Context Association"], association["Response Stage"] = "response prefix not observed; association unavailable", "response prefix unavailable"
			}
			message := e.Session["Handshake Messages"].([]map[string]any)[0]
			message["Post Handshake Response"] = association
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				bodyIndex, associationIndex := -1, -1
				for i, field := range view.fields {
					if field.text == body {
						bodyIndex = i
					}
					if strings.HasPrefix(field.text, "Post Handshake Response") {
						associationIndex = i
						require.True(t, field.branch)
						require.False(t, field.hasRange)
					}
					if strings.HasPrefix(field.text, "Request Ordinal:") || strings.HasPrefix(field.text, "Context Association:") || strings.HasPrefix(field.text, "Response Stage:") || strings.HasPrefix(field.text, "Certificate Selection:") || strings.HasPrefix(field.text, "Verification Scope:") {
						require.Greater(t, i, associationIndex)
						require.False(t, field.hasRange)
						require.Zero(t, field.source)
					}
				}
				require.GreaterOrEqual(t, bodyIndex, 0)
				require.Greater(t, associationIndex, bodyIndex)
				require.Len(t, view.sources, 2)
				require.Equal(t, message[encoded], view.sources[1].Data)
				if kind == "prefix-unavailable" {
					require.NotContains(t, detailLines(view.fields), "Request Ordinal:")
				}
			}
		})
	}
}

func TestTLSAuthenticatedPHAApplicationInterruptionKeepsIdentity(t *testing.T) {
	e := authenticatedTLSDiagnosticFixture(t, "TLS post-handshake response interleaved")
	e.Session["Inner Content Type"] = byte(23)
	packet := tcpPacket(t, 1, 100, string(e.Raw), false, false, false, false)
	packet.events = []*pcaputil.ProtocolEvent{e}
	packet.connectionDiagnostic = "TLS decoding halted after MalformedMessage; subsequent plaintext unavailable"
	row := summarize(packet)
	require.Equal(t, "TLSv1.3", row.Protocol)
	require.Equal(t, "malformed", row.ProtocolStatus)
	require.Equal(t, "authenticated TLS record diagnostic", row.ProtocolScope)
	require.Equal(t, packet.connectionDiagnostic, row.ConnectionDiagnostic)
	require.Contains(t, row.Info, "Response interrupted")
	require.Contains(t, row.Info, "TLS post-handshake response interleaved")
	view := workerMetadataDetailForPacket(t, packet)
	require.Equal(t, "TLSv1.3", view.protocol)
	require.Contains(t, detailLines(view.fields), "TLS post-handshake response interleaved")
}
