package sharkcli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func finishedPresentation(version, suite uint16, phase string) *pcaputil.ProtocolEvent {
	n := 12
	if version == 0x304 {
		n = 32
		if suite == 0x1302 {
			n = 48
		}
	}
	body := bytes.Repeat([]byte{0x5a}, n)
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{23, 3, 3, 0, 80}, bytes.Repeat([]byte{0xa5}, 80)...), Session: map[string]any{
		"Negotiated Version": version, "Cipher Suite": suite, "Authentication Verified": true, "Content Visibility": "authenticated-plaintext",
		"Handshake Messages": []map[string]any{{"Type": byte(20), "Length": n, "Authenticated": true,
			"Finished Version": version, "Finished Cipher Suite": suite, "Finished Phase": phase,
			"Finished Verify Data": body, "Encoded Finished": append([]byte{20, 0, 0, byte(n)}, body...),
			"Finished Verification Status": "record AEAD authenticated; transcript MAC not evaluated"}},
	}}
}

func TestTLSFinishedProgressiveDetailAndOwnedPlaintext(t *testing.T) {
	for _, spec := range []struct {
		version, suite uint16
		phase          string
	}{{0x303, 0xc02f, "handshake"}, {0x304, 0x1301, "handshake"}, {0x304, 0x1302, "application"}, {0x304, 0x1303, "application"}} {
		t.Run(fmt.Sprintf("%04x/%04x/%s", spec.version, spec.suite, spec.phase), func(t *testing.T) {
			e := finishedPresentation(spec.version, spec.suite, spec.phase)
			require.Contains(t, secureTransportSummary(e), "Finished · "+spec.phase+" epoch")
			require.Contains(t, secureTransportSummary(e), "transcript MAC not evaluated")
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			for _, view := range []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)} {
				require.Len(t, view.sources, 2)
				require.Contains(t, view.sources[1].Name, "authenticated handshake plaintext")
				m := e.Session["Handshake Messages"].([]map[string]any)[0]
				require.Equal(t, m["Encoded Finished"], view.sources[1].Data)
				header, length, verify, phase := -1, -1, -1, -1
				for i, field := range view.fields {
					switch {
					case field.text == "Handshake Type: Finished (20)":
						header = i
						require.Equal(t, 2, field.source)
						require.Equal(t, 0, field.start)
						require.Equal(t, 1, field.end)
					case strings.HasPrefix(field.text, "Length:") && field.source == 2:
						length = i
						require.Equal(t, 1, field.start)
						require.Equal(t, 4, field.end)
					case strings.HasPrefix(field.text, "Finished Verify Data:"):
						verify = i
						require.Equal(t, 2, field.source)
						require.True(t, field.hasRange)
						require.Equal(t, 4, field.start)
						require.Equal(t, len(view.sources[1].Data), field.end)
					case strings.HasPrefix(field.text, "Finished Phase:"):
						phase = i
						require.False(t, field.hasRange, "phase is context, not a plaintext octet")
					}
				}
				require.GreaterOrEqual(t, header, 0)
				require.Greater(t, length, header)
				require.Greater(t, verify, length)
				require.Greater(t, phase, verify)
			}
		})
	}
}

func TestTLSFinishedSourceRejectsContradictoryMetadata(t *testing.T) {
	for _, mutation := range []string{"record-auth", "message-auth", "version", "suite", "length", "encoding", "data", "phase"} {
		t.Run(mutation, func(t *testing.T) {
			e := finishedPresentation(0x304, 0x1302, "application")
			m := e.Session["Handshake Messages"].([]map[string]any)[0]
			switch mutation {
			case "record-auth":
				e.Session["Authentication Verified"] = false
			case "message-auth":
				m["Authenticated"] = false
			case "version":
				m["Finished Version"] = uint16(0x303)
			case "suite":
				m["Finished Cipher Suite"] = uint16(0x1301)
			case "length":
				m["Length"] = 47
			case "encoding":
				m["Encoded Finished"].([]byte)[0] = 13
			case "data":
				m["Finished Verify Data"] = []byte{0x5a}
			case "phase":
				m["Finished Phase"] = "peer authenticated"
			}
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			for _, field := range fields {
				require.LessOrEqual(t, field.source, 1, "contradictory metadata cannot acquire an owned plaintext span")
			}
		})
	}
}
