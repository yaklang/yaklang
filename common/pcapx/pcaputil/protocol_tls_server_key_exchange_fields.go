package pcaputil

import "encoding/hex"

func tlsServerKeyExchangeDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, key string
		bits      int
	}{
		{"tls.handshake.server_curve_type", "Curve Type", 8},
		{"tls.handshake.server_named_curve", "Named Group", 16},
		{"tls.handshake.server_point_len", "Server EC Point Length", 8},
		{"tls.handshake.server_point", "Server EC Point", 0},
		{"tls.handshake.p_len", "DH Modulus Length", 16},
		{"tls.handshake.p", "DH Modulus", 0},
		{"tls.handshake.g_len", "DH Generator Length", 16},
		{"tls.handshake.g", "DH Generator", 0},
		{"tls.handshake.ys_len", "Server DH Public Value Length", 16},
		{"tls.handshake.ys", "Server DH Public Value", 0},
		{"tls.handshake.sig_len", "Signature Length", 16},
		{"tls.handshake.sig", "Signature", 0},
	} {
		spec := spec
		kind := DisplayUnsigned
		if spec.bits == 0 {
			kind = DisplayString
		}
		b := displayCustom("tls", spec.name, "TLS 1.2 ServerKeyExchange "+spec.key, kind, spec.bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root == "" {
				return
			}
			p.walk(session, []string{root, "*", "Server Key Exchange", spec.key}, func(value any) {
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
		b.descriptor.Description = "Literal in an exact, cipher-selected TLS 1.2 signed ephemeral ServerKeyExchange body. Separate owned handshake source, no outer record/frame range. Does not validate public parameters, signatures, transcript or peer identity."
		out = append(out, b)
	}
	return out
}
