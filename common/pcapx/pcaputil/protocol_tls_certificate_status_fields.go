package pcaputil

func tlsCertificateStatusDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, key string
		bits      int
	}{
		{"tls.handshake.extensions_status_request_type", "Status Type", 8},
		{"tls.handshake.ocsp_response_len", "OCSP Response Length", 24},
	} {
		spec := spec
		b := displayCustom("tls", spec.name, "CertificateStatus "+spec.key, DisplayUnsigned, spec.bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root != "" {
				p.walk(session, []string{root, "*", "Certificate Status", spec.key}, emit)
			}
		})
		b.protocols = "|tls|"
		b.descriptor.Description = "Literal in an owned complete TLS 1.0..1.2 OCSP CertificateStatus vector. No physical frame range; DER, revocation status, signature and trust are not evaluated."
		out = append(out, b)
	}
	return out
}
