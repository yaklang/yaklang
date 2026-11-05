package sharkcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func keyUpdatePresentationForTest(request byte) *pcaputil.ProtocolEvent {
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{23, 3, 3, 0, 22}, bytes.Repeat([]byte{0xa5}, 22)...), Session: map[string]any{
		"Record Type": byte(23), "Record Version": uint16(0x303), "Record Length": 22,
		"Negotiated Version": uint16(0x304), "Content Visibility": "authenticated-plaintext", "Authentication Verified": true, "Inner Content Type": byte(22),
		"Handshake Messages": []map[string]any{{"Type": byte(24), "Length": 1, "Authenticated": true, "Key Update Request": request,
			"Key Update Request Name": []string{"update_not_requested", "update_requested"}[request], "Encoded Key Update": []byte{24, 0, 0, 1, request}}},
	}}
}

func TestTLS13KeyUpdateDetailUsesAuthenticatedPlaintextSource(t *testing.T) {
	for _, request := range []byte{0, 1} {
		e := keyUpdatePresentationForTest(request)
		require.Contains(t, secureTransportSummary(e), []string{"update_not_requested", "update_requested"}[request])
		fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		views := []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)}
		for _, view := range views {
			require.Len(t, view.sources, 2)
			require.Equal(t, e.Raw, view.sources[0].Data)
			require.Equal(t, []byte{24, 0, 0, 1, request}, view.sources[1].Data)
			require.Contains(t, view.sources[1].Name, "authenticated handshake plaintext")
			var selected int
			for _, f := range view.fields {
				if strings.HasPrefix(f.text, "Key Update Request:") {
					selected++
					require.True(t, f.hasRange)
					require.Equal(t, 2, f.source)
					require.Equal(t, 4, f.start)
					require.Equal(t, 5, f.end)
				}
			}
			require.Equal(t, 1, selected)
		}
	}
}

func TestTLS13KeyUpdateSourceRejectsUnprovenPlaintext(t *testing.T) {
	for _, name := range []string{"record-authentication", "version", "handshake-authentication", "enum", "different-value", "bad-header", "trailing-byte"} {
		t.Run(name, func(t *testing.T) {
			e := keyUpdatePresentationForTest(1)
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			switch name {
			case "record-authentication":
				e.Session["Authentication Verified"] = false
			case "version":
				e.Session["Negotiated Version"] = uint16(0x303)
			case "handshake-authentication":
				m["Authenticated"] = false
			case "enum":
				m["Key Update Request"] = byte(2)
			case "different-value":
				m["Encoded Key Update"].([]byte)[4] = 0
			case "bad-header":
				m["Encoded Key Update"].([]byte)[0] = 1
			case "trailing-byte":
				m["Encoded Key Update"] = append(m["Encoded Key Update"].([]byte), 0)
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			for _, f := range fields {
				if strings.HasPrefix(f.text, "Key Update Request:") || strings.HasPrefix(f.text, "Encoded Key Update:") {
					require.False(t, f.hasRange, f.text)
					require.Zero(t, f.source, f.text)
				}
			}
		})
	}
}
