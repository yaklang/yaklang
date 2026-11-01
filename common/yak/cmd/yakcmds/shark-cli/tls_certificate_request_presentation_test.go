package sharkcli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func certificateRequestPresentation(t *testing.T, tls13 bool) *pcaputil.ProtocolEvent {
	t.Helper()
	text := "0d000030030102400028040305030603080708080809080a080b0804080508060401050106010303030103020402050206020000"
	version := uint16(0x303)
	if tls13 {
		version = 0x304
		text = "0d00003100002e000d002a002809050906090404030503060308070808081a081b081c0809080a080b080408050806040105010601"
	}
	wire, err := hex.DecodeString(text)
	require.NoError(t, err)
	view, err := pcaputil.DecodeTLSCertificateRequest(wire, version, 128)
	require.NoError(t, err)
	visibility := "cleartext"
	if tls13 {
		visibility = "authenticated-plaintext"
	}
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{23, 3, 3, 0, 75}, bytes.Repeat([]byte{0xa5}, 75)...), Session: map[string]any{
		"Negotiated Version": version, "Authentication Verified": tls13, "Content Visibility": visibility,
		"Handshake Messages": []map[string]any{{"Type": byte(13), "Length": len(wire) - 4, "Authenticated": tls13,
			"Certificate Request Version": version, "Certificate Request": view.Fields, "Encoded Certificate Request": wire}},
	}}
}

func TestTLSCertificateRequestProgressiveBodyAndOwnedSource(t *testing.T) {
	for _, tls13 := range []bool{false, true} {
		e := certificateRequestPresentation(t, tls13)
		require.Contains(t, secureTransportSummary(e), "Certificate Request · 20 signature algorithms")
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
			require.Len(t, view.sources, 2)
			require.Contains(t, view.sources[1].Name, "handshake")
			if tls13 {
				require.Contains(t, view.sources[1].Name, "authenticated handshake plaintext")
			}
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			require.Equal(t, m["Encoded Certificate Request"], view.sources[1].Data)
			body, algorithm, count, ranges := -1, -1, 0, 0
			for i, f := range view.fields {
				if f.text == "Certificate Request body" {
					body = i
					require.True(t, f.branch)
				}
				if strings.HasPrefix(f.text, "Algorithm:") {
					algorithm = i
					count++
					require.True(t, f.hasRange)
					require.Equal(t, 2, f.source)
					require.Equal(t, 2, f.end-f.start)
				}
				if f.source == 2 && f.hasRange {
					ranges++
					require.GreaterOrEqual(t, f.start, 0)
					require.Greater(t, f.end, f.start)
					require.LessOrEqual(t, f.end, len(view.sources[1].Data))
				}
				if strings.HasPrefix(f.text, "Context:") {
					require.False(t, f.hasRange, "empty context has no octets")
				}
			}
			require.GreaterOrEqual(t, body, 0)
			require.Greater(t, algorithm, body)
			require.Equal(t, 20, count)
			require.Equal(t, map[bool]int{false: 29, true: 28}[tls13], ranges)
		}
	}
}

func TestTLSCertificateRequestSourceRefusesContradictoryEvidence(t *testing.T) {
	for _, name := range []string{"record-auth", "message-auth", "context-version", "message-version", "length", "encoding", "body", "nil-body"} {
		t.Run(name, func(t *testing.T) {
			e := certificateRequestPresentation(t, true)
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			switch name {
			case "record-auth":
				e.Session["Authentication Verified"] = false
			case "message-auth":
				m["Authenticated"] = false
			case "context-version":
				e.Session["Negotiated Version"] = uint16(0x303)
			case "message-version":
				m["Certificate Request Version"] = uint16(0x303)
			case "length":
				m["Length"] = 48
			case "encoding":
				m["Encoded Certificate Request"].([]byte)[4] = 1
			case "body":
				m["Certificate Request"].(map[string]any)["Context Length"] = byte(1)
			case "nil-body":
				m["Certificate Request"] = nil
			}
			_, wire, _ := tlsCertificateRequestDetail(m, number(e.Session["Negotiated Version"]), e.Session["Authentication Verified"] == true)
			require.Nil(t, wire)
			_, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, source := range sources {
				require.NotContains(t, source.Name, "TLS Certificate Request ·", "contradictory metadata must not publish an authoritative handshake range")
			}
		})
	}
}

func TestTLSCertificateRequestIPCTypesBudgetAndOwnership(t *testing.T) {
	e := certificateRequestPresentation(t, true)
	wire := e.Session["Handshake Messages"].([]map[string]any)[0]["Encoded Certificate Request"].([]byte)
	encoded, err := json.Marshal(e)
	require.NoError(t, err)
	var transferred pcaputil.ProtocolEvent
	require.NoError(t, json.Unmarshal(encoded, &transferred))
	m := mapList(transferred.Session["Handshake Messages"])[0]
	view, data, _ := tlsCertificateRequestDetail(m, number(transferred.Session["Negotiated Version"]), true)
	require.NotNil(t, view)
	require.Equal(t, wire, data)
	fields, limited := tlsCertificateRequestFields(view, 2, 3, 5, 0x304)
	require.Len(t, fields, 5)
	require.True(t, limited)
	_, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	before := bytes.Clone(sources[1].Data)
	clear(wire)
	require.Equal(t, before, sources[1].Data)
}

func TestTLSCertificateRequestAlgorithmNamesRespectVersion(t *testing.T) {
	require.Equal(t, "sha256 / ecdsa (0x0403)", tlsCertificateRequestAlgorithm(0x0403, 0x303))
	require.Equal(t, "ecdsa_secp256r1_sha256 (0x0403)", tlsCertificateRequestAlgorithm(0x0403, 0x304))
	require.Equal(t, "rsa_pss_rsae_sha256 (0x0804)", tlsCertificateRequestAlgorithm(0x0804, 0x303))
	require.Equal(t, "mldsa65 (0x0905)", tlsCertificateRequestAlgorithm(0x0905, 0x304))
	require.Equal(t, "0xfafa (name unavailable)", tlsCertificateRequestAlgorithm(0xfafa, 0x304))
}

func TestTLSCertificateRequestQUICProgressivePlaintext(t *testing.T) {
	e := certificateRequestPresentation(t, true)
	e.Protocol = "quic"
	e.Session["TLS Handshakes"] = e.Session["Handshake Messages"]
	delete(e.Session, "Handshake Messages")
	delete(e.Session, "Negotiated Version")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	require.Len(t, sources, 2)
	require.Contains(t, sources[1].Name, "authenticated handshake plaintext")
	count := 0
	for _, f := range fields {
		if strings.HasPrefix(f.text, "Algorithm:") {
			count++
			require.Equal(t, 2, f.source)
			require.True(t, f.hasRange)
		}
	}
	require.Equal(t, 20, count)
}

// The real241 server flight places CertificateRequest after three other TLS
// PDUs in one physical frame. The important result must survive bounded rows.
func TestTLSCertificateRequestSurvivesCoalescedFrameSummary(t *testing.T) {
	for _, count := range []int{3, 5} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("prefix%d/reverse%t", count, reverse), func(t *testing.T) {
				request := certificateRequestPresentation(t, true)
				var events []*pcaputil.ProtocolEvent
				for i := 0; i < count; i++ {
					e := keyUpdatePresentationForTest(0)
					e.ID = uint64(i + 2)
					e.Session["Handshake Messages"] = []map[string]any{{"Type": byte(2), "Length": 76, "Authenticated": false}}
					e.Session["SNI"] = strings.Repeat("a", 100) + ".example"
					events = append(events, e)
				}
				if reverse {
					events = append([]*pcaputil.ProtocolEvent{request}, events...)
				} else {
					events = append(events, request)
				}
				before, err := json.Marshal(events)
				require.NoError(t, err)
				row := presentEvents(events).apply(packetSummary{})
				require.True(t, strings.HasPrefix(row.Info, "Certificate Request"), row.Info)
				require.Contains(t, clipText(row.Info, 80), "20 signature algorithms")
				after, err := json.Marshal(events)
				require.NoError(t, err)
				require.Equal(t, before, after)
			})
		}
	}
}

func TestTLSCertificateRequestSurvivesCoalescedHandshakeSummary(t *testing.T) {
	e := certificateRequestPresentation(t, true)
	request := e.Session["Handshake Messages"].([]map[string]any)[0]
	var messages []map[string]any
	for i := 0; i < 10; i++ {
		messages = append(messages, map[string]any{"Type": byte(2), "Length": 76, "Authenticated": false})
	}
	e.Session["Handshake Messages"] = append(messages, request)
	require.True(t, strings.HasPrefix(secureTransportSummary(e), "Certificate Request · 20 signature algorithms"))
}

func TestTLSCertificateRequestSummaryPromotionRequiresBodyEvidence(t *testing.T) {
	for _, name := range []string{"record-auth", "message-auth", "body", "encoding", "version", "status"} {
		t.Run(name, func(t *testing.T) {
			e := certificateRequestPresentation(t, true)
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			switch name {
			case "record-auth":
				e.Session["Authentication Verified"] = false
			case "message-auth":
				m["Authenticated"] = false
			case "body":
				m["Certificate Request"].(map[string]any)["Context Length"] = byte(1)
			case "encoding":
				m["Encoded Certificate Request"].([]byte)[4] = 0xff
			case "version":
				delete(m, "Certificate Request Version")
			case "status":
				e.Status = "malformed"
			}
			require.Zero(t, tlsDetailedHandshakeRank(e), "a matching message name is not a decoded/authenticated body")
		})
	}
}

func TestTLSCertificateRequestSummaryDoesNotDisplaceKeyUpdateOrApplication(t *testing.T) {
	request, update := certificateRequestPresentation(t, true), keyUpdatePresentationForTest(1)
	for _, events := range [][]*pcaputil.ProtocolEvent{{request, update}, {update, request}} {
		row := presentEvents(events).apply(packetSummary{})
		require.True(t, strings.HasPrefix(row.Info, "Key Update · update_requested"), row.Info)
		require.Equal(t, "TLSv1.3", row.Protocol)
	}
	application := &pcaputil.ProtocolEvent{Protocol: "http", Status: "decoded", Summary: "GET / HTTP/1.1", Fields: map[string]any{"Protocol": "HTTP/1.1"}}
	row := presentEvents([]*pcaputil.ProtocolEvent{request, application}).apply(packetSummary{})
	require.Equal(t, "HTTP/1.1", row.Protocol)
	require.Contains(t, row.Info, "GET /")
	require.NotContains(t, row.Info, "20 signature algorithms")
}
