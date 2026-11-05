package sharkcli

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func ckePresentation(t *testing.T, profile string) *pcaputil.ProtocolEvent {
	t.Helper()
	wire, suite := []byte{16, 0, 0, 0}, uint16(0xc02f)
	switch profile {
	case "ECDHEClient":
		wire = append(wire, 32)
		wire = append(wire, bytes.Repeat([]byte{7}, 32)...)
	case "DHEClient":
		suite = 0x009e
		wire = append(wire, 0, 2, 4, 5)
	case "RSAClient":
		suite = 0x009c
		wire = append(wire, 0, 3, 1, 2, 3)
	case "empty-RSA":
		suite = 0x009c
		wire = append(wire, 0, 0)
	}
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)-4))
	view, err := pcaputil.DecodeTLS12ClientKeyExchange(wire, 0x303, suite)
	require.NoError(t, err)
	return &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: []byte{22, 3, 3, 0, 0}, Session: map[string]any{
		"Negotiated Version": uint16(0x303), "Cipher Suite": suite, "Authentication Verified": false,
		"Handshake Messages": []map[string]any{{"Type": byte(16), "Length": len(wire) - 4, "Authenticated": false,
			"Client Key Exchange": view.Fields, "Encoded Client Key Exchange": wire,
			"Client Key Exchange Version": uint16(0x303), "Client Key Exchange Cipher Suite": suite,
			"Client Key Exchange Profile": view.Profile, "Client Key Exchange Status": "body vector decoded; peer trust not evaluated"}},
	}}
}

func TestTLSClientKeyExchangeProgressiveDetailAndOwnedBytes(t *testing.T) {
	for _, profile := range []string{"ECDHEClient", "DHEClient", "RSAClient", "empty-RSA"} {
		t.Run(profile, func(t *testing.T) {
			e := ckePresentation(t, profile)
			m := mapList(e.Session["Handshake Messages"])[0]
			view, err := pcaputil.DecodeTLS12ClientKeyExchange(bytesValue(m["Encoded Client Key Exchange"]), 0x303, uint16(number(e.Session["Cipher Suite"])))
			require.NoError(t, err)
			summary := secureTransportSummary(e)
			require.Contains(t, summary, "Client Key Exchange")
			if strings.Contains(profile, "RSA") {
				require.Contains(t, summary, "premaster not decrypted")
			} else {
				require.Contains(t, summary, "public value not evaluated")
			}
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
				require.Equal(t, m["Encoded Client Key Exchange"], d.sources[1].Data)
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
					require.Greater(t, found, last)
					last = found
				}
				if profile == "empty-RSA" {
					for _, f := range d.fields {
						if strings.HasPrefix(f.text, "Encrypted Premaster:") {
							require.False(t, f.hasRange, "empty vector must not fabricate selected bytes")
						}
					}
				}
			}
		})
	}
}

func TestTLSClientKeyExchangeContradictoryMetadataHasNoSource(t *testing.T) {
	for _, kind := range []string{"version", "suite", "length", "encoding", "point", "profile", "record-auth", "message-auth", "both-auth", "extra-field"} {
		t.Run(kind, func(t *testing.T) {
			e := ckePresentation(t, "ECDHEClient")
			m := mapList(e.Session["Handshake Messages"])[0]
			body := m["Client Key Exchange"].(map[string]any)
			switch kind {
			case "version":
				m["Client Key Exchange Version"] = 0x304
			case "suite":
				m["Client Key Exchange Cipher Suite"] = 0x009e
			case "length":
				m["Length"] = 1
			case "encoding":
				m["Encoded Client Key Exchange"].([]byte)[0] = 12
			case "point":
				body["Client EC Point"] = []byte{7}
			case "profile":
				m["Client Key Exchange Profile"] = "RSAClient"
			case "record-auth":
				e.Session["Authentication Verified"] = true
			case "message-auth":
				m["Authenticated"] = true
			case "both-auth":
				e.Session["Authentication Verified"], m["Authenticated"] = true, true
			case "extra-field":
				body["Peer Identity Validated"] = true
			}
			_, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			require.Len(t, sources, 1)
			require.NotContains(t, secureTransportSummary(e), "public value not evaluated")
		})
	}
}

func TestTLSClientKeyExchangeImplicitContextHasNoInventedBodySource(t *testing.T) {
	for _, profile := range []string{"ECDHEClient", "DHEClient"} {
		e := ckePresentation(t, profile)
		e.Raw = []byte{22, 3, 3, 0, 4, 16, 0, 0, 0}
		e.Session["Handshake Messages"] = []map[string]any{{"Type": byte(16), "Length": 0, "Authenticated": false,
			"Client Key Exchange Status": "implicit public-value certificate context unavailable; body uninterpreted"}}
		summary := secureTransportSummary(e)
		require.Contains(t, summary, "implicit public-value certificate context unavailable")
		require.NotContains(t, summary, "cipher layout unavailable")
		_, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
		require.Len(t, sources, 1)
		worker := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false)
		for _, source := range worker.sources {
			require.NotContains(t, source.Name, "TLS Client Key Exchange")
		}
	}
}
