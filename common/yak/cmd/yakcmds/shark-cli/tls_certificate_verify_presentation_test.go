package sharkcli

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func certificateVerifyPresentation(t *testing.T, version uint16, phase string) *pcaputil.ProtocolEvent {
	t.Helper()
	wire := []byte{15, 0, 0, 0}
	if version >= 0x303 {
		wire = append(wire, 8, 4)
	}
	wire = append(wire, 1, 0)
	wire = append(wire, bytes.Repeat([]byte{0x93}, 256)...)
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)-4))
	v, err := pcaputil.DecodeTLSCertificateVerify(wire, version)
	require.NoError(t, err)
	auth := version == 0x304
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{23, 3, 3, 1, 44}, bytes.Repeat([]byte{0xa5}, 300)...), Session: map[string]any{
		"Negotiated Version": version, "Authentication Verified": auth,
		"Handshake Messages": []map[string]any{{"Type": byte(15), "Length": len(wire) - 4, "Authenticated": auth,
			"Certificate Verify": v.Fields, "Certificate Verify Version": version, "Certificate Verify Phase": phase,
			"Encoded Certificate Verify": wire,
			"Certificate Verify Status":  "body vector decoded; signature not evaluated; certificate trust and handshake context not evaluated"}},
	}}
}

func TestTLSCertificateVerifyProgressiveDetailAndOwnedBytes(t *testing.T) {
	for _, spec := range []struct {
		version uint16
		phase   string
	}{{0x301, "handshake"}, {0x302, "handshake"}, {0x303, "handshake"}, {0x304, "handshake"}, {0x304, "application"}} {
		t.Run(fmt.Sprintf("%04x/%s", spec.version, spec.phase), func(t *testing.T) {
			e := certificateVerifyPresentation(t, spec.version, spec.phase)
			require.Contains(t, secureTransportSummary(e), "signature not evaluated")
			if spec.version >= 0x303 {
				require.Contains(t, secureTransportSummary(e), "rsa_pss_rsae_sha256 (0x0804)")
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				require.Len(t, view.sources, 2)
				m := e.Session["Handshake Messages"].([]map[string]any)[0]
				require.Equal(t, m["Encoded Certificate Verify"], view.sources[1].Data)
				require.Contains(t, view.sources[1].Name, "TLS Certificate Verify")
				if spec.version == 0x304 {
					require.Contains(t, view.sources[1].Name, "authenticated handshake plaintext")
				} else {
					require.Contains(t, view.sources[1].Name, "reassembled handshake bytes")
				}
				header, length, algorithm, siglen, signature, context := -1, -1, -1, -1, -1, -1
				start := 6
				if spec.version >= 0x303 {
					start = 8
				}
				for i, f := range view.fields {
					switch {
					case f.text == "Handshake Type: Certificate Verify (15)":
						header = i
						require.Equal(t, 2, f.source)
						require.True(t, f.hasRange)
						require.Equal(t, 0, f.start)
						require.Equal(t, 1, f.end)
					case strings.HasPrefix(f.text, "Length:") && f.source == 2:
						length = i
						require.Equal(t, 1, f.start)
						require.Equal(t, 4, f.end)
					case strings.HasPrefix(f.text, "Signature Algorithm:"):
						algorithm = i
						require.Equal(t, 2, f.source)
						require.Equal(t, 4, f.start)
						require.Equal(t, 6, f.end)
					case strings.HasPrefix(f.text, "Signature Length:"):
						siglen = i
						require.Equal(t, 2, f.source)
						require.Equal(t, start-2, f.start)
						require.Equal(t, start, f.end)
					case strings.HasPrefix(f.text, "Signature:"):
						signature = i
						require.Equal(t, 2, f.source)
						require.True(t, f.hasRange)
						require.Equal(t, start, f.start)
						require.Equal(t, start+256, f.end)
					case strings.HasPrefix(f.text, "Certificate Verify Phase:"):
						context = i
						require.False(t, f.hasRange)
					}
				}
				require.GreaterOrEqual(t, header, 0)
				require.Greater(t, length, header)
				if spec.version >= 0x303 {
					require.Greater(t, algorithm, length)
					require.Greater(t, siglen, algorithm)
				} else {
					require.Equal(t, -1, algorithm)
					require.Greater(t, siglen, length)
				}
				require.Greater(t, signature, siglen)
				require.Greater(t, context, signature)
			}
		})
	}
}

func TestTLSCertificateVerifySourceRejectsContradictoryMetadata(t *testing.T) {
	for _, kind := range []string{"version", "length", "algorithm", "signature-length", "signature", "encoding", "record-auth", "message-auth", "phase"} {
		t.Run(kind, func(t *testing.T) {
			e := certificateVerifyPresentation(t, 0x304, "application")
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			body := m["Certificate Verify"].(map[string]any)
			switch kind {
			case "version":
				m["Certificate Verify Version"] = uint16(0x303)
			case "length":
				m["Length"] = 1
			case "algorithm":
				body["Signature Algorithm"] = uint16(0x0805)
			case "signature-length":
				body["Signature Length"] = uint16(255)
			case "signature":
				body["Signature"] = []byte{0x93}
			case "encoding":
				m["Encoded Certificate Verify"].([]byte)[0] = 13
			case "record-auth":
				e.Session["Authentication Verified"] = false
			case "message-auth":
				m["Authenticated"] = false
			case "phase":
				m["Certificate Verify Phase"] = "peer-authenticated"
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			for _, f := range fields {
				require.LessOrEqual(t, f.source, 1, "contradictory metadata cannot acquire a plaintext range")
			}
		})
	}
}

func TestTLSCertificateVerifyVisibleBesideCoalescedCertificateRequest(t *testing.T) {
	for _, scope := range []string{"one-frame", "one-record"} {
		t.Run(scope, func(t *testing.T) {
			request := certificateRequestPresentation(t, true)
			verify := certificateVerifyPresentation(t, 0x304, "handshake")
			verify.ID = 2
			events := []*pcaputil.ProtocolEvent{request}
			messages := append([]map[string]any(nil), mapList(request.Session["Handshake Messages"])...)
			for i := 0; i < 32; i++ {
				opaque := keyUpdatePresentationForTest(0)
				opaque.ID = uint64(i + 3)
				opaque.Session["Handshake Messages"] = []map[string]any{{"Type": byte(2), "Length": 76, "Authenticated": false}}
				opaque.Session["SNI"] = strings.Repeat("a", 100) + ".example"
				events = append(events, opaque)
				messages = append(messages, mapList(opaque.Session["Handshake Messages"])...)
			}
			var summary string
			if scope == "one-frame" {
				events = append(events, verify)
				summary = presentEvents(events).apply(packetSummary{}).Info
			} else {
				messages = append(messages, mapList(verify.Session["Handshake Messages"])...)
				request.Session["Handshake Messages"] = messages
				summary = secureTransportSummary(request)
			}
			visible := clipText(summary, 384)
			require.Contains(t, visible, "Certificate Request")
			require.Contains(t, visible, "Certificate Verify")
			require.Contains(t, visible, "signature not evaluated")
		})
	}
}

func TestTLSCertificateVerifySummaryPreservesKeyUpdateAndApplication(t *testing.T) {
	verify := certificateVerifyPresentation(t, 0x304, "application")
	finished := finishedPresentation(0x304, 0x1302, "application")
	for _, events := range [][]*pcaputil.ProtocolEvent{{verify, finished}, {finished, verify}} {
		row := presentEvents(events).apply(packetSummary{})
		require.True(t, strings.HasPrefix(row.Info, "Certificate Verify"), "a later Finished must not push the complete signature scheme off the visible row: %s", row.Info)
		require.Contains(t, clipText(row.Info, 80), "rsa_pss_rsae_sha256 (0x0804)")
		require.Contains(t, row.Info, "Finished")
	}
	update := keyUpdatePresentationForTest(1)
	for _, events := range [][]*pcaputil.ProtocolEvent{{verify, update}, {update, verify}} {
		row := presentEvents(events).apply(packetSummary{})
		require.True(t, strings.HasPrefix(row.Info, "Key Update · update_requested"), row.Info)
	}
	application := &pcaputil.ProtocolEvent{Protocol: "http", Status: "decoded", Summary: "GET / HTTP/1.1", Fields: map[string]any{"Protocol": "HTTP/1.1"}}
	for _, events := range [][]*pcaputil.ProtocolEvent{{verify, application}, {application, verify}} {
		row := presentEvents(events).apply(packetSummary{})
		require.Equal(t, "HTTP/1.1", row.Protocol)
		require.Contains(t, row.Info, "GET /")
		require.NotContains(t, row.Info, "Certificate Verify")
	}
}
