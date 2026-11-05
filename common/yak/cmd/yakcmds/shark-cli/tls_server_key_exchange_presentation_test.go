package sharkcli

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func skePresentation(t *testing.T, ec bool) *pcaputil.ProtocolEvent {
	t.Helper()
	wire := []byte{12, 0, 0, 0}
	suite := uint16(0xc02f)
	if ec {
		wire = append(wire, 3, 0, 29, 32)
		wire = append(wire, bytes.Repeat([]byte{7}, 32)...)
	} else {
		suite = 0x009e
		wire = append(wire, 0, 2, 1, 3, 0, 1, 2, 0, 2, 4, 5)
	}
	wire = append(wire, 8, 4, 0, 3, 1, 2, 3)
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)-4))
	view, err := pcaputil.DecodeTLS12ServerKeyExchange(wire, 0x303, suite)
	require.NoError(t, err)
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: []byte{22, 3, 3, 0, 0}, Session: map[string]any{
		"Negotiated Version": uint16(0x303), "Cipher Suite": suite, "Authentication Verified": false,
		"Handshake Messages": []map[string]any{{"Type": byte(12), "Length": len(wire) - 4, "Authenticated": false,
			"Server Key Exchange": view.Fields, "Encoded Server Key Exchange": wire,
			"Server Key Exchange Version": uint16(0x303), "Server Key Exchange Cipher Suite": suite,
			"Server Key Exchange Status": "body vectors decoded; signature not evaluated"}},
	}}
}

func TestTLSServerKeyExchangeProgressiveDetailAndOwnedBytes(t *testing.T) {
	for _, ec := range []bool{false, true} {
		t.Run(map[bool]string{true: "ECDHE", false: "DHE"}[ec], func(t *testing.T) {
			e := skePresentation(t, ec)
			m := mapList(e.Session["Handshake Messages"])[0]
			view, err := pcaputil.DecodeTLS12ServerKeyExchange(bytesValue(m["Encoded Server Key Exchange"]), 0x303, uint16(number(e.Session["Cipher Suite"])))
			require.NoError(t, err)
			require.Contains(t, secureTransportSummary(e), "rsa_pss_rsae_sha256 (0x0804)")
			require.Contains(t, secureTransportSummary(e), "signature not evaluated")
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			direct := packetDetail{fields: fields, sources: sources}
			packet := detail(&capturedPacket{number: 1, events: []*pcaputil.ProtocolEvent{e}})
			workerPacket := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false)
			require.Equal(t, packet.fields, workerPacket.fields)
			require.Equal(t, packet.sources, workerPacket.sources)
			stream := decodeStream(streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}})
			workerStream := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)
			require.Equal(t, stream.fields, workerStream.fields)
			require.Equal(t, stream.sources, workerStream.sources)
			for _, d := range []packetDetail{direct, workerPacket, workerStream} {
				require.Len(t, d.sources, 2)
				require.Equal(t, m["Encoded Server Key Exchange"], d.sources[1].Data)
				require.Contains(t, d.sources[1].Name, "reassembled handshake bytes")
				require.NotContains(t, d.sources[1].Name, "authenticated")
				last := -1
				for _, r := range view.Ranges {
					found := -1
					for i, f := range d.fields {
						if strings.HasPrefix(f.text, r.Path[0]+":") {
							found = i
							require.True(t, f.hasRange)
							require.Equal(t, 2, f.source)
							require.Equal(t, r.Offset, f.start)
							require.Equal(t, r.Offset+r.Length, f.end)
						}
					}
					require.Greater(t, found, last, "fields remain in actual wire order")
					last = found
				}
			}
		})
	}
}

func TestTLSServerKeyExchangeContradictoryMetadataHasNoSource(t *testing.T) {
	for _, kind := range []string{"version", "suite", "length", "encoding", "signature", "point", "record-auth", "message-auth", "extra-field"} {
		t.Run(kind, func(t *testing.T) {
			e := skePresentation(t, true)
			m := mapList(e.Session["Handshake Messages"])[0]
			body := m["Server Key Exchange"].(map[string]any)
			switch kind {
			case "version":
				m["Server Key Exchange Version"] = 0x304
			case "suite":
				m["Server Key Exchange Cipher Suite"] = 0x009e
			case "length":
				m["Length"] = 1
			case "encoding":
				m["Encoded Server Key Exchange"].([]byte)[0] = 13
			case "signature":
				body["Signature"] = []byte{9}
			case "point":
				body["Server EC Point"] = []byte{7}
			case "record-auth":
				e.Session["Authentication Verified"] = true
			case "message-auth":
				m["Authenticated"] = true
			case "extra-field":
				body["Peer Identity Validated"] = true
			}
			_, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
		})
	}
}
