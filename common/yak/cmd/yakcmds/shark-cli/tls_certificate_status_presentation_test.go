package sharkcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestTLSCertificateStatusProgressiveOwnedFields(t *testing.T) {
	wire := []byte{22, 0, 0, 9, 1, 0, 0, 5, 0x30, 3, 0x0a, 1, 3}
	v, err := pcaputil.DecodeTLSCertificateStatus(wire, 0x303)
	require.NoError(t, err)
	m := map[string]any{"Type": byte(22), "Length": 9, "Authenticated": false, "Certificate Status": v.Fields, "Encoded Certificate Status": wire,
		"Certificate Status Version": uint16(0x303), "Certificate Status Validation": "complete OCSP vector decoded; DER, revocation status, signature and certificate trust not evaluated"}
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{22, 3, 3, 0, 13}, wire...), Session: map[string]any{
		"Negotiated Version": uint16(0x303), "Cipher Suite": uint16(0xc02f), "Authentication Verified": false, "Handshake Messages": []map[string]any{m}}}
	require.Contains(t, secureTransportSummary(e), "Certificate Status · OCSP · 5-byte response")
	require.Contains(t, secureTransportSummary(e), "trust not evaluated")
	fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
	worker := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false)
	packet := detail(&capturedPacket{number: 1, events: []*pcaputil.ProtocolEvent{e}})
	require.Equal(t, packet.fields, worker.fields)
	require.Equal(t, sources, worker.sources)
	stream := decodeStream(streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}})
	streamWorker := workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)
	require.Equal(t, stream.fields, streamWorker.fields)
	require.Equal(t, stream.sources, streamWorker.sources)
	for _, d := range []packetDetail{{fields: fields, sources: sources}, worker, streamWorker} {
		require.Len(t, d.sources, 2)
		require.Equal(t, wire, d.sources[1].Data)
		require.Contains(t, d.sources[1].Name, "OCSP unvalidated")
		seen := 0
		for _, field := range d.fields {
			if strings.HasPrefix(field.text, "Status Type:") || strings.HasPrefix(field.text, "OCSP Response Length:") || strings.HasPrefix(field.text, "OCSP Response:") {
				require.True(t, field.hasRange)
				require.Equal(t, 2, field.source)
				require.Equal(t, wire[field.start:field.end], d.sources[1].Data[field.start:field.end])
				seen++
			}
			if strings.HasPrefix(field.text, "OCSP DER Decoded:") || strings.HasPrefix(field.text, "OCSP Response Validated:") || strings.HasPrefix(field.text, "Certificate Trust Evaluated:") {
				require.False(t, field.hasRange)
				require.True(t, strings.HasSuffix(field.text, "false"))
			}
		}
		require.Equal(t, 3, seen)
	}
	for _, kind := range []string{"version", "body", "wire", "authenticated", "outer-authenticated"} {
		t.Run(kind, func(t *testing.T) {
			copy := make(map[string]any)
			for k, x := range m {
				copy[k] = x
			}
			outerAuth := false
			switch kind {
			case "version":
				copy["Certificate Status Version"] = uint16(0x304)
			case "body":
				copy["Certificate Status"] = map[string]any{"OCSP Response Length": uint32(5)}
			case "wire":
				b := bytes.Clone(wire)
				b[7] = 4
				copy["Encoded Certificate Status"] = b
			case "authenticated":
				copy["Authenticated"] = true
			case "outer-authenticated":
				outerAuth = true
			}
			view, b, name := tlsCertificateStatusDetail(copy, 0x303, outerAuth)
			require.Nil(t, view)
			require.Nil(t, b)
			require.Empty(t, name)
		})
	}
}
