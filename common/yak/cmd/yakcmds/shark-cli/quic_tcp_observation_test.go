package sharkcli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestQUICCandidateOnTCPDoesNotCreateProtocolTree(t *testing.T) {
	for _, protocol := range []string{"quic", "http3", "doq"} {
		for _, claimed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/claimed-carrier%v", protocol, claimed), func(t *testing.T) {
				payload := append([]byte{0x80, 0, 0, 0, 0, 0, 0}, make([]byte, 44)...)
				event := &pcaputil.ProtocolEvent{ID: 1, FlowID: 1, Transport: "tcp", Protocol: protocol, Profile: "quic", Status: "context-required", Summary: "protocol context required: QUIC session was not observed", Raw: bytes.Clone(payload), Length: len(payload), SourceBytes: pcaputil.ByteSource{Kind: "reassembled"}}
				if claimed {
					event.Status, event.Completeness = "decoded", "message"
					event.Session = map[string]any{"Native Carrier": true, "Authentication Verified": true, "Header Form": "Long", "Packet Name": "Initial", "Version": 1, "TLS Handshakes": []map[string]any{{"Type": 1, "Length": 4, "Authenticated": true}}}
					event.Fields = map[string]any{"Unqualified QUIC field": "must not be projected"}
				}
				raw := tcpFailureRaw(tcpFailureBody(20, nil, payload))
				raw.events = []*pcaputil.ProtocolEvent{event}
				for name, d := range map[string]packetDetail{"detail": detail(raw), "quick": quickDetail(raw), "worker": workerMetadataDetailForPacket(t, raw)} {
					t.Run(name, func(t *testing.T) {
						require.Equal(t, "TCP", d.protocol, "the title and tree must agree on the observed carrier")
						text := detailLines(d.fields)
						require.Contains(t, text, "TCP payload")
						require.Contains(t, text, "UDP carrier required")
						require.NotContains(t, text, "QUIC wire header")
						require.NotContains(t, text, "QUIC · PDU")
						require.NotContains(t, text, "Unqualified QUIC field")
						found := false
						for _, s := range d.sources {
							if strings.HasPrefix(s.Name, "Unidentified TCP payload") {
								found = true
								require.Equal(t, payload, s.Data)
							}
						}
						require.True(t, found, "unidentified owned bytes remain inspectable")
					})
				}
				require.Equal(t, protocol, event.Protocol, "display qualification cannot rewrite the native event")
				require.Equal(t, payload, event.Raw)
			})
		}
	}
}

func TestUnrecognizedTCPBytesRemainAnObservation(t *testing.T) {
	payload := append([]byte{0x80, 0, 0, 0, 0, 0, 0}, make([]byte, 44)...)
	for _, size := range []int{len(payload), 17} {
		t.Run(fmt.Sprint("retained", size), func(t *testing.T) {
			e := &pcaputil.ProtocolEvent{ID: 3, Transport: "tcp", Status: "unrecognized", Raw: bytes.Clone(payload[:size]), Length: len(payload), SourceBytes: pcaputil.ByteSource{Kind: "reassembled"}}
			raw := tcpFailureRaw(tcpFailureBody(20, nil, payload))
			raw.events = []*pcaputil.ProtocolEvent{e}
			for name, d := range map[string]packetDetail{"detail": detail(raw), "quick": quickDetail(raw), "worker": workerMetadataDetailForPacket(t, raw)} {
				t.Run(name, func(t *testing.T) {
					require.Equal(t, "TCP", d.protocol)
					text := detailLines(d.fields)
					require.Contains(t, text, "TCP payload · application unrecognized · observation #3")
					require.NotContains(t, text, "PDU #3")
					require.NotContains(t, text, "QUIC")
					var found bool
					for _, s := range d.sources {
						if strings.HasPrefix(s.Name, "Unidentified TCP payload") {
							found = true
							require.Equal(t, payload[:size], s.Data)
							if size < len(payload) {
								require.Contains(t, s.Name, fmt.Sprintf("preview · %d/%d observed bytes", size, len(payload)))
							}
						}
					}
					require.True(t, found)
				})
			}
		})
	}
}
