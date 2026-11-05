package sharkcli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestTLSMetadataVersionNamesDoNotReinterpretOtherFields(t *testing.T) {
	for _, key := range []string{"Certificate Status Version", "Certificate Request Version", "Certificate Version", "Certificate Verify Version", "Finished Version", "Server Key Exchange Version", "Client Key Exchange Version", "TLS Boundary Version", "TLS Gap Version"} {
		for code, name := range map[uint16]string{0x300: "SSL 3.0", 0x301: "TLS 1.0", 0x302: "TLS 1.1", 0x303: "TLS 1.2", 0x304: "TLS 1.3"} {
			t.Run(fmt.Sprintf("%s/%04x", key, code), func(t *testing.T) {
				require.Equal(t, fmt.Sprintf("%s (0x%04x)", name, code), formatFieldValue(key, code))
				require.Equal(t, formatFieldValue(key, code), formatFieldValue(key, float64(code)))
			})
		}
		for _, v := range []any{uint16(0), uint16(0xffff), -1, 771.5, "771", []byte{3, 3}} {
			_, ok := tlsMetadataVersion(key, v)
			require.False(t, ok)
		}
	}
	for _, key := range []string{"Version", "Protocol Version", "Record Version", "Negotiated Version", "IP Version", "HTTP Version", "QUIC Version"} {
		require.Equal(t, "771", formatFieldValue(key, uint16(0x303)))
	}
}

func TestTLSCertificateStatusDefaultViewPrioritizesMissingContext(t *testing.T) {
	wire := []byte{22, 0, 0, 9, 1, 0, 0, 5, 0x30, 3, 0x0a, 1, 3}
	v, err := pcaputil.DecodeTLSCertificateStatus(wire, 0x303)
	require.NoError(t, err)
	m := map[string]any{"Type": byte(22), "Length": 9, "Authenticated": false, "Certificate Status": v.Fields, "Encoded Certificate Status": wire,
		"Certificate Status Version": uint16(0x303), "Certificate Status Validation": "Hello negotiation unavailable; preceding message order unavailable; complete OCSP vector decoded; DER, revocation status, signature and certificate trust not evaluated"}
	e := &pcaputil.ProtocolEvent{ID: 1, Protocol: "tls", Status: "decoded", Raw: append([]byte{22, 3, 3, 0, 13}, wire...), Session: map[string]any{
		"Negotiated Version": uint16(0x303), "Cipher Suite": uint16(0xc02f), "Authentication Verified": false, "Handshake Messages": []map[string]any{m}}}
	require.Contains(t, secureTransportSummary(e), "Certificate Status · context unavailable · OCSP")
	for _, d := range []packetDetail{detail(&capturedPacket{number: 1, events: []*pcaputil.ProtocolEvent{e}}), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), decodeStream(streamDecodeRequest{ID: 1, Events: []*pcaputil.ProtocolEvent{e}})} {
		validation, body, version := -1, -1, -1
		for i, f := range d.fields {
			if strings.HasPrefix(f.text, "Certificate Status Validation:") {
				validation = i
				require.True(t, strings.HasPrefix(f.text, "Certificate Status Validation: Hello negotiation unavailable"))
				require.False(t, f.hasRange)
			}
			if f.text == "Certificate Status body" {
				body = i
			}
			if f.text == "Certificate Status Version: TLS 1.2 (0x0303)" {
				version = i
				require.False(t, f.hasRange)
			}
		}
		require.GreaterOrEqual(t, validation, 0)
		require.Greater(t, body, validation)
		require.Greater(t, version, body)
	}
}
