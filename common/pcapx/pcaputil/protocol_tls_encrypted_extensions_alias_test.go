package pcaputil

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTLSEncryptedExtensionsLiteralAliases(t *testing.T) {
	for _, protocol := range []string{"tls", "quic"} {
		for _, name := range []string{"http/1.1", string([]byte{0xff, 0, 0xfe})} {
			t.Run(protocol+"/"+hex.EncodeToString([]byte(name)), func(t *testing.T) {
				wire := reliabilityHS(8, eeScopeBody(tlsResumptionExt{16, eeScopeALPN(name)}))
				state := &binTLS{client: 0, version: 0x304}
				message, err := state.handshakeMessage(1, wire, true)
				require.NoError(t, err)
				root := "Handshake Messages"
				if protocol == "quic" {
					root = "TLS Handshakes"
				}
				e := &ProtocolEvent{Protocol: protocol, Session: map[string]any{root: []map[string]any{message}}}
				fields := e.DisplayFields()
				require.Equal(t, []any{uint64(len(wire) - 6)}, fields["tls.handshake.extensions_length"])
				require.Equal(t, []any{uint64(16)}, fields["tls.handshake.extension.type"])
				require.Equal(t, []any{uint64(3 + len(name))}, fields["tls.handshake.extension.len"])
				require.Equal(t, []any{uint64(1 + len(name))}, fields["tls.handshake.extensions_alpn_len"])
				require.Equal(t, []any{uint64(len(name))}, fields["tls.handshake.extensions_alpn_str_len"])
				require.Equal(t, []any{hex.EncodeToString([]byte(name))}, fields["tls.handshake.extensions_alpn_raw"])
				if name == "http/1.1" {
					require.Equal(t, []any{name}, fields["tls.handshake.extensions_alpn_str"])
				} else {
					require.NotContains(t, fields, "tls.handshake.extensions_alpn_str", "do not replace invalid UTF-8 octets")
				}
				for _, value := range e.DisplayFieldValues().Values {
					if value.Origin == "derived" {
						require.Nil(t, value.Range, "owned plaintext cannot borrow an outer record range")
					}
				}
			})
		}
	}
}
