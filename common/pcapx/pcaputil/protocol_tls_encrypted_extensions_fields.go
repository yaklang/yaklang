package pcaputil

import (
	"encoding/hex"
	"unicode/utf8"
)

func tlsEncryptedExtensionsDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, key string
		kind      DisplayFieldType
		bits      int
	}{
		{"tls.handshake.extensions_alpn_len", "ALPN Length", DisplayUnsigned, 16},
		{"tls.handshake.extensions_alpn_str_len", "Protocol Length", DisplayUnsigned, 8},
		{"tls.handshake.extensions_alpn_str", "Protocol", DisplayString, 0},
		{"tls.handshake.extensions_alpn_raw", "Protocol", DisplayString, 0},
	} {
		spec := spec
		binding := displayCustom("tls", spec.name, "Observed EncryptedExtensions ALPN "+spec.key, spec.kind, spec.bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root == "" {
				return
			}
			p.walk(session, []string{root, "*", "Encrypted Extensions", "Extensions", "*", spec.key}, func(value any) {
				if spec.key != "Protocol" {
					emit(value)
					return
				}
				if raw, ok := value.([]byte); ok {
					if spec.name == "tls.handshake.extensions_alpn_raw" {
						emit(hex.EncodeToString(raw))
					} else if utf8.Valid(raw) {
						emit(string(raw))
					}
				}
			})
		})
		binding.protocols = "|tls|quic|http3|doq|"
		binding.descriptor.Description = "Literal ALPN selection in a complete TLS 1.3 EncryptedExtensions body; no outer record/frame range. Text is available only for valid UTF-8; extensions_alpn_raw always preserves arbitrary protocol-name octets as hexadecimal. This field alone does not certify the client's offer or handshake conformance."
		out = append(out, binding)
	}
	return out
}
