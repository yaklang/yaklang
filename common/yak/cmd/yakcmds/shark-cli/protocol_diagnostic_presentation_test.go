package sharkcli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestProtocolDiagnosticCodeAndReasonRemainDistinct(t *testing.T) {
	for _, c := range []struct {
		name, protocol, code, raw, reason, decoder string
	}{
		{"tls-request-context", "tls", "MalformedMessage", "MalformedMessage: initial TLS CertificateRequest context must be empty", "initial TLS CertificateRequest context must be empty", ""},
		{"tls-client-context", "tls", "MalformedMessage", "MalformedMessage: initial client TLS Certificate context must be empty", "initial client TLS Certificate context must be empty", ""},
		{"dns-internal-colon", "dns", "MalformedMessage", "MalformedMessage: DNS response: truncated answer", "DNS response: truncated answer", ""},
		{"http-distinct-wrapper", "http", "MalformedMessage", "ContextRequired: response method unavailable", "ContextRequired: response method unavailable", ""},
		{"ftp-no-code", "ftp", "", "MalformedMessage: invalid passive endpoint", "MalformedMessage: invalid passive endpoint", ""},
		{"ssh-decoder", "ssh", "MalformedMessage", "MalformedMessage: ssh-plaintext: invalid packet length", "invalid packet length", "ssh-plaintext"},
		{"tls-preserves-other-namespace", "tls", "MalformedMessage", "MalformedMessage: ssh: literal peer text", "ssh: literal peer text", ""},
		{"tls-no-space", "tls", "MalformedMessage", "MalformedMessage:bad record", "MalformedMessage:bad record", ""},
		{"tls-one-wrapper-only", "tls", "MalformedMessage", "MalformedMessage: MalformedMessage: nested reason", "MalformedMessage: nested reason", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := &pcaputil.ProtocolEvent{ID: 7, Protocol: c.protocol, Transport: "tcp", Status: "malformed", Completeness: "malformed", Error: c.raw, ExpertCode: c.code, Raw: []byte{23, 3, 3, 0, 1, 42}, Length: 6, Session: map[string]any{"Observation": "body refused"}}
			before, err := json.Marshal(e)
			require.NoError(t, err)
			fields, sources := protocolEventDetail([]*pcaputil.ProtocolEvent{e})
			views := []packetDetail{{fields: fields, sources: sources}, workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, false), workerMetadataDetail(t, []*pcaputil.ProtocolEvent{e}, true)}
			for _, view := range views {
				var diagnostics []string
				for _, f := range view.fields {
					if strings.HasPrefix(f.text, "Error: ") || strings.HasPrefix(f.text, "Diagnostic code: ") || strings.HasPrefix(f.text, "Decoder: ") {
						diagnostics = append(diagnostics, f.text)
						require.False(t, f.hasRange, "diagnostics do not name wire byte ranges")
					}
				}
				var want []string
				if c.code != "" {
					want = append(want, "Diagnostic code: "+c.code)
				}
				if c.decoder != "" {
					want = append(want, "Decoder: "+c.decoder)
				}
				want = append(want, "Error: "+c.reason)
				require.Equal(t, want, diagnostics, "the typed code, decoder and literal reason each remain readable")
			}
			after, err := json.Marshal(e)
			require.NoError(t, err)
			require.Equal(t, before, after, "presentation must retain the original export error and evidence")
		})
	}
}
