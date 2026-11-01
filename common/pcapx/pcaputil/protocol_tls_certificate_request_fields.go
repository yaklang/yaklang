package pcaputil

func tlsHandshakeDisplayRoot(e *ProtocolEvent) (map[string]any, string) {
	if e.nativeQUICObservation {
		return e.semanticSession, "TLS Handshakes"
	}
	switch e.Protocol {
	case "quic":
		return e.Session, "TLS Handshakes"
	case "tls":
		return e.Session, "Handshake Messages"
	}
	return nil, ""
}

// These literals belong to the complete handshake body. Their source may span
// records, or be authenticated plaintext, so none borrows an outer PDU range.
func tlsCertificateRequestDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, key string
		bits      int
	}{
		{"tls.handshake.type", "Type", 8}, {"tls.handshake.length", "Length", 24},
	} {
		spec := spec
		b := displayCustom("tls", spec.name, "Observed complete TLS handshake envelope "+spec.key, DisplayUnsigned, spec.bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root != "" {
				p.walk(session, []string{root, "*", spec.key}, emit)
			}
		})
		b.protocols = "|tls|quic|http3|doq|"
		b.descriptor.Description = "Literal complete handshake envelope, possibly reassembled or decrypted. Does not certify the body grammar or handshake conformance; no outer record or physical-frame span."
		out = append(out, b)
	}
	add := func(name, label string, bits int, paths ...[]string) {
		b := displayCustom("tls", name, label, DisplayUnsigned, bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root == "" {
				return
			}
			for _, tail := range paths {
				path := append([]string{root, "*", "Certificate Request"}, tail...)
				p.walk(session, path, emit)
			}
		})
		b.protocols = "|tls|quic|http3|doq|"
		b.descriptor.Description = "Literal in a bounded, version-qualified CertificateRequest handshake body. Separate owned handshake byte source; no ciphertext record or physical-frame range. Not a handshake conformance or certificate-trust assertion."
		out = append(out, b)
	}
	add("tls.handshake.cert_types_count", "CertificateRequest certificate types length", 8, []string{"Certificate Types Length"})
	add("tls.handshake.cert_type", "CertificateRequest certificate type", 8, []string{"Certificate Types", "*", "Type"})
	add("tls.handshake.sig_hash_alg_len", "CertificateRequest signature algorithms vector length", 16,
		[]string{"Signature Algorithms Length"}, []string{"Extensions", "*", "Signature Algorithms Length"})
	add("tls.handshake.sig_hash_alg", "CertificateRequest complete two-byte signature algorithm", 16,
		[]string{"Signature Algorithms", "*", "Algorithm"}, []string{"Extensions", "*", "Signature Algorithms", "*", "Algorithm"})
	add("tls.handshake.dnames_len", "CertificateRequest distinguished names vector length", 16,
		[]string{"Certificate Authorities Length"}, []string{"Extensions", "*", "Certificate Authorities Length"})
	add("tls.handshake.certificate_request_context_length", "CertificateRequest context length", 8, []string{"Context Length"})
	add("tls.handshake.extensions_length", "CertificateRequest extensions vector length", 16, []string{"Extensions Length"})
	add("tls.handshake.extension.type", "CertificateRequest extension type", 16, []string{"Extensions", "*", "Type"})
	add("tls.handshake.extension.len", "CertificateRequest extension data length", 16, []string{"Extensions", "*", "Length"})
	return out
}
