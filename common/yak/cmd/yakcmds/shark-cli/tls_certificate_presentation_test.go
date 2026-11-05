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

func certificatePresentation(t *testing.T) *pcaputil.ProtocolEvent {
	t.Helper()
	// Two entries repeat the same data/context bytes to expose borrowed ranges.
	entry := []byte{0, 0, 3, 0x30, 0, 0xff, 0, 7, 0xff, 1, 0, 3, 0xff, 0, 0xff}
	wire := []byte{11, 0, 0, 37, 3, 0xff, 0, 0xff, 0, 0, 30}
	wire = append(wire, entry...)
	wire = append(wire, entry...)
	view, err := pcaputil.DecodeTLSCertificate(wire, 0x304, 4)
	require.NoError(t, err)
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{23, 3, 3, 0, 64}, bytes.Repeat([]byte{0xa5}, 64)...), Session: map[string]any{
		"Negotiated Version": uint16(0x304), "Authentication Verified": true,
		"Handshake Messages": []map[string]any{{"Type": byte(11), "Length": len(wire) - 4, "Authenticated": true,
			"Certificate Body": view.Fields, "Certificates": view.Certificates, "Certificate Version": uint16(0x304),
			"Encoded Certificate": wire, "Certificate Body Status": "body vectors decoded; request context, certificate type and trust not evaluated"}},
	}}
}

func TestTLSCertificateProgressiveBodyOwnedRangesAndWorkers(t *testing.T) {
	e := certificatePresentation(t)
	before, err := json.Marshal(e)
	require.NoError(t, err)
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	for _, detail := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
		require.Len(t, detail.sources, 2)
		wire := e.Session["Handshake Messages"].([]map[string]any)[0]["Encoded Certificate"].([]byte)
		require.Equal(t, wire, detail.sources[1].Data)
		require.Contains(t, detail.sources[1].Name, "authenticated handshake plaintext")
		header, body, context := -1, -1, -1
		dataRanges, extRanges := [][2]int{}, [][2]int{}
		for i, field := range detail.fields {
			switch {
			case field.text == "Handshake Type: Certificate (11)":
				header = i
				require.Equal(t, 2, field.source)
				require.True(t, field.hasRange)
				require.Equal(t, 0, field.start)
				require.Equal(t, 1, field.end)
			case field.text == "Certificate body":
				body = i
			case strings.HasPrefix(field.text, "Context:"):
				context = i
				require.Equal(t, 2, field.source)
				require.Equal(t, 5, field.start)
				require.Equal(t, 8, field.end)
			case strings.HasPrefix(field.text, "Certificate Data:"):
				require.Equal(t, 2, field.source)
				dataRanges = append(dataRanges, [2]int{field.start, field.end})
			case strings.HasPrefix(field.text, "Data:") && field.source == 2:
				extRanges = append(extRanges, [2]int{field.start, field.end})
			case strings.Contains(field.text, "trust not evaluated"), strings.Contains(field.text, "request context, certificate type and trust not evaluated"):
				require.False(t, field.hasRange)
			}
			if field.hasRange && field.source == 2 {
				require.Greater(t, field.end, field.start)
				require.LessOrEqual(t, field.end, len(wire))
			}
		}
		require.GreaterOrEqual(t, header, 0)
		require.Greater(t, body, header)
		require.Greater(t, context, body)
		require.Equal(t, [][2]int{{14, 17}, {29, 32}}, dataRanges)
		require.Equal(t, [][2]int{{23, 26}, {38, 41}}, extRanges)
	}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{})
	require.Equal(t, "TLSv1.3", row.Protocol)
	require.Contains(t, clipText(row.Info, 120), "Certificate · 2 entries · context 3 bytes")
	require.Contains(t, row.Info, "trust not evaluated")
	after, err := json.Marshal(e)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestTLSCertificateSourceRefusesContradictoryMetadata(t *testing.T) {
	for _, kind := range []string{"version", "record-auth", "message-auth", "length", "context", "list-length", "certificate-data", "extension-data", "encoding", "malformed-extensions"} {
		t.Run(kind, func(t *testing.T) {
			e := certificatePresentation(t)
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			body := m["Certificate Body"].(map[string]any)
			switch kind {
			case "version":
				m["Certificate Version"] = uint16(0x303)
			case "record-auth":
				e.Session["Authentication Verified"] = false
			case "message-auth":
				m["Authenticated"] = false
			case "length":
				m["Length"] = 1
			case "context":
				body["Context"] = []byte{0xff}
			case "list-length":
				body["Certificate List Length"] = uint32(29)
			case "certificate-data":
				body["Certificate Entries"].([]map[string]any)[0]["Certificate Data"] = []byte{0x30}
			case "extension-data":
				body["Certificate Entries"].([]map[string]any)[1]["Extensions"].([]map[string]any)[0]["Data"] = []byte{0xff}
			case "encoding":
				m["Encoded Certificate"].([]byte)[0] = 13
			case "malformed-extensions":
				m["Encoded Certificate"].([]byte)[22] = 4
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			for _, field := range fields {
				require.LessOrEqual(t, field.source, 1)
			}
			require.NotContains(t, secureTransportSummary(e), "2 entries")
		})
	}
}

func TestTLSCertificateSourcesCannotDisplaceSignatureOrKeyUpdate(t *testing.T) {
	for _, kind := range []string{"signature", "key-update"} {
		t.Run(kind, func(t *testing.T) {
			cert := certificatePresentation(t)
			other := certificateVerifyPresentation(t, 0x304, "handshake")
			want := "Certificate Verify"
			if kind == "key-update" {
				other, want = keyUpdatePresentationForTest(0), "Key Update"
			}
			other.ID = 2
			for _, reversed := range []bool{false, true} {
				events := []*pcaputil.ProtocolEvent{cert, other}
				if reversed {
					events[0], events[1] = events[1], events[0]
				}
				row := presentEvents(events).apply(packetSummary{})
				require.True(t, strings.HasPrefix(row.Info, want), fmt.Sprint(row.Info))
			}
		})
	}
}
