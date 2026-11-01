package pcaputil

import "encoding/hex"

// Shared aliases retain handshake and entry occurrence order. These fields
// address owned body vectors, not record/ciphertext bytes or inferred context.
func tlsCertificateDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, key string
		bits      int
		binary    bool
		request   []string
		body      []string
	}{
		{"tls.handshake.certificate_request_context_length", "certificate request context length", 8, false, []string{"Context Length"}, []string{"Context Length"}},
		{"tls.handshake.certificate_request_context", "certificate request context bytes", 0, true, []string{"Context"}, []string{"Context"}},
		{"tls.handshake.extensions_length", "certificate extensions vector length", 16, false, []string{"Extensions Length"}, []string{"Certificate Entries", "*", "Extensions Length"}},
		{"tls.handshake.extension.type", "certificate extension type", 16, false, []string{"Extensions", "*", "Type"}, []string{"Certificate Entries", "*", "Extensions", "*", "Type"}},
		{"tls.handshake.extension.len", "certificate extension data length", 16, false, []string{"Extensions", "*", "Length"}, []string{"Certificate Entries", "*", "Extensions", "*", "Length"}},
		{"tls.handshake.certificates_length", "certificate list length", 24, false, nil, []string{"Certificate List Length"}},
		{"tls.handshake.certificate_length", "certificate entry length", 24, false, nil, []string{"Certificate Entries", "*", "Certificate Length"}},
		{"tls.handshake.certificate", "certificate entry data", 0, true, nil, []string{"Certificate Entries", "*", "Certificate Data"}},
	} {
		spec := spec
		kind := DisplayUnsigned
		if spec.binary {
			kind = DisplayString
		}
		binding := displayCustom("tls", spec.name, "Observed "+spec.key, kind, spec.bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root == "" {
				return
			}
			p.walk(session, []string{root, "*"}, func(message any) {
				for _, choice := range []struct {
					root string
					path []string
				}{{"Certificate Request", spec.request}, {"Certificate Body", spec.body}} {
					if choice.path == nil {
						continue
					}
					p.walk(message, append([]string{choice.root}, choice.path...), func(value any) {
						if spec.binary {
							if raw, ok := value.([]byte); ok {
								emit(hex.EncodeToString(raw))
							}
							return
						}
						emit(value)
					})
				}
			})
		})
		binding.protocols = "|tls|quic|http3|doq|"
		binding.descriptor.AllowEmpty = spec.binary
		binding.descriptor.Description = "Literal in a complete version-qualified CertificateRequest or Certificate body. Byte strings use lossless hexadecimal; occurrence order is retained. Owned handshake bytes have no outer record/frame span. Offers, request association, certificate type, signature and trust are not verified."
		out = append(out, binding)
	}
	return out
}
