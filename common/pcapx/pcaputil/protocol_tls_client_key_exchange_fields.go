package pcaputil

import "encoding/hex"

func tlsClientKeyExchangeDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, key string
		bits      int
	}{
		{"tls.handshake.client_point_len", "Client EC Point Length", 8},
		{"tls.handshake.client_point", "Client EC Point", 0},
		{"tls.handshake.yc_len", "Client DH Public Value Length", 16},
		{"tls.handshake.yc", "Client DH Public Value", 0},
		{"tls.handshake.epms_len", "Encrypted Premaster Length", 16},
		{"tls.handshake.epms", "Encrypted Premaster", 0},
	} {
		spec := spec
		kind := DisplayUnsigned
		if spec.bits == 0 {
			kind = DisplayString
		}
		b := displayCustom("tls", spec.name, "TLS 1.2 ClientKeyExchange "+spec.key, kind, spec.bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root == "" {
				return
			}
			p.walk(session, []string{root, "*", "Client Key Exchange", spec.key}, func(value any) {
				if spec.bits == 0 {
					if raw, ok := value.([]byte); ok {
						emit(hex.EncodeToString(raw))
					}
					return
				}
				emit(value)
			})
		})
		b.protocols = "|tls|"
		b.descriptor.AllowEmpty = spec.bits == 0
		b.descriptor.Description = "Literal in an exact, observed-cipher-selected TLS 1.2 ClientKeyExchange body. Owned handshake source, no outer record/frame span. Public value, premaster, transcript and peer trust are not verified."
		out = append(out, b)
	}
	return out
}
