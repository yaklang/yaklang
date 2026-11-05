package sharkcli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func encryptedExtensionsPresentation(t *testing.T) *pcaputil.ProtocolEvent {
	t.Helper()
	wire := []byte{8, 0, 0, 17, 0, 15, 0, 16, 0, 11, 0, 9, 8}
	wire = append(wire, []byte("http/1.1")...)
	view, err := pcaputil.DecodeTLSEncryptedExtensions(wire, 0x304, 1)
	require.NoError(t, err)
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{23, 3, 3, 0, 38}, bytes.Repeat([]byte{0xa5}, 38)...), Session: map[string]any{
		"Negotiated Version": uint16(0x304), "Authentication Verified": true, "ALPN": "http/1.1",
		"Handshake Messages": []map[string]any{{"Type": byte(8), "Length": 17, "Authenticated": true,
			"Encrypted Extensions": view.Fields, "Encrypted Extensions Version": uint16(0x304), "Encoded Encrypted Extensions": wire}},
	}}
}

func TestTLSEncryptedExtensionsProgressiveOwnedSourcesAndWorkers(t *testing.T) {
	for _, protocol := range []string{"tls", "quic"} {
		t.Run(protocol, func(t *testing.T) {
			e := encryptedExtensionsPresentation(t)
			if protocol == "quic" {
				e.Protocol = protocol
				e.Session["TLS Handshakes"] = e.Session["Handshake Messages"]
				delete(e.Session, "Handshake Messages")
				e.Session["Native Carrier"], e.Session["Packet Number Space"] = true, "handshake"
			}
			before, err := json.Marshal(e)
			require.NoError(t, err)
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, detail := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				require.Len(t, detail.sources, 2)
				require.Contains(t, detail.sources[1].Name, "authenticated handshake plaintext")
				header, body, selected := -1, -1, -1
				for i, f := range detail.fields {
					if f.text == "Handshake Type: Encrypted Extensions (8)" {
						header = i
						require.Equal(t, [2]int{0, 1}, [2]int{f.start, f.end})
						require.Equal(t, 2, f.source)
						require.True(t, f.hasRange)
					}
					if f.text == "Encrypted Extensions body" {
						body = i
					}
					if strings.HasPrefix(f.text, "Protocol:") {
						selected = i
						require.Equal(t, [2]int{13, 21}, [2]int{f.start, f.end})
						require.Equal(t, 2, f.source)
						require.True(t, f.hasRange)
						require.Equal(t, []byte("http/1.1"), detail.sources[1].Data[f.start:f.end])
					}
					if f.source == 2 && f.hasRange {
						require.Greater(t, f.end, f.start)
						require.LessOrEqual(t, f.end, 21)
					}
				}
				require.GreaterOrEqual(t, header, 0)
				require.Greater(t, body, header)
				require.Greater(t, selected, body)
			}
			if protocol == "tls" {
				row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{})
				require.Contains(t, row.Info, "Encrypted Extensions · ALPN=\"http/1.1\"")
				require.Equal(t, 1, strings.Count(row.Info, "ALPN="), "current selection must not be repeated as a context hint")
				require.Equal(t, "TLSv1.3", row.Protocol)
			}
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestTLSEncryptedExtensionsViewRefusesContradictions(t *testing.T) {
	for _, kind := range []string{"version", "record-auth", "message-auth", "length", "protocol", "extension-length", "encoding", "wrong-QUIC-level", "missing-native-carrier"} {
		t.Run(kind, func(t *testing.T) {
			e := encryptedExtensionsPresentation(t)
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			ext := m["Encrypted Extensions"].(map[string]any)["Extensions"].([]map[string]any)[0]
			switch kind {
			case "version":
				m["Encrypted Extensions Version"] = uint16(0x303)
			case "record-auth":
				e.Session["Authentication Verified"] = false
			case "message-auth":
				m["Authenticated"] = false
			case "length":
				m["Length"] = 1
			case "protocol":
				ext["Protocol"] = []byte("h2")
			case "extension-length":
				ext["Length"] = 1
			case "encoding":
				m["Encoded Encrypted Extensions"].([]byte)[0] = 13
			case "wrong-QUIC-level", "missing-native-carrier":
				e.Protocol = "quic"
				e.Session["TLS Handshakes"] = e.Session["Handshake Messages"]
				delete(e.Session, "Handshake Messages")
				e.Session["Native Carrier"], e.Session["Packet Number Space"] = true, "application"
				if kind == "missing-native-carrier" {
					e.Session["Native Carrier"], e.Session["Packet Number Space"] = false, "handshake"
				}
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			for _, f := range fields {
				require.LessOrEqual(t, f.source, 1, fmt.Sprint(f))
			}
			require.NotContains(t, secureTransportSummary(e), "ALPN=\"http/1.1\"")
		})
	}
}
