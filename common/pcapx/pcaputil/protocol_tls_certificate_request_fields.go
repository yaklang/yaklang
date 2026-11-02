package pcaputil

import "encoding/hex"

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
	// This alias is shared by CertificateRequest and CertificateVerify. Walk
	// messages first so a coalesced flight retains wire-message occurrence order.
	signatureAlgorithm := displayCustom("tls", "tls.handshake.sig_hash_alg", "Observed complete two-byte TLS signature algorithm", DisplayUnsigned, 16, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		session, root := tlsHandshakeDisplayRoot(e)
		if root == "" {
			return
		}
		p.walk(session, []string{root, "*"}, func(message any) {
			for _, path := range [][]string{
				{"Certificate Request", "Signature Algorithms", "*", "Algorithm"},
				{"Certificate Request", "Extensions", "*", "Signature Algorithms", "*", "Algorithm"},
				{"Certificate Verify", "Signature Algorithm"},
			} {
				p.walk(message, path, emit)
			}
		})
	})
	signatureAlgorithm.protocols = "|tls|quic|http3|doq|"
	signatureAlgorithm.descriptor.Description = "Literal identifier in a complete version-qualified handshake body; TLS 1.2 pairs do not select an ECDSA curve. Naming does not establish negotiation, scheme conformance or signature validity. No outer record/frame span."
	out = append(out, signatureAlgorithm)
	add("tls.handshake.dnames_len", "CertificateRequest distinguished names vector length", 16,
		[]string{"Certificate Authorities Length"}, []string{"Extensions", "*", "Certificate Authorities Length"})
	out = append(out, tlsCertificateDisplayBindings()...)
	out = append(out, tlsEncryptedExtensionsDisplayBindings()...)
	finished := displayCustom("tls", "tls.handshake.verify_data", "Observed TLS Finished verify_data", DisplayString, 0, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
		session, root := tlsHandshakeDisplayRoot(e)
		if root != "" {
			p.walk(session, []string{root, "*", "Finished Verify Data"}, func(value any) {
				if raw, ok := value.([]byte); ok {
					emit(hex.EncodeToString(raw))
				}
			})
		}
	})
	finished.protocols = "|tls|quic|http3|doq|"
	finished.descriptor.Description = "Literal hexadecimal bytes of a complete cipher-suite-sized Finished. Record authentication is distinct from transcript MAC or peer authentication. Owned handshake plaintext has no outer record/frame span."
	out = append(out, finished)
	for _, spec := range []struct {
		name, key string
		kind      DisplayFieldType
		bits      int
	}{
		{"tls.handshake.client_cert_vrfy.sig_len", "Signature Length", DisplayUnsigned, 16},
		{"tls.handshake.client_cert_vrfy.sig", "Signature", DisplayString, 0},
	} {
		spec := spec
		b := displayCustom("tls", spec.name, "CertificateVerify "+spec.key, spec.kind, spec.bits, "derived", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			session, root := tlsHandshakeDisplayRoot(e)
			if root != "" {
				p.walk(session, []string{root, "*", "Certificate Verify", spec.key}, func(value any) {
					if spec.key == "Signature" {
						if raw, ok := value.([]byte); ok {
							emit(hex.EncodeToString(raw))
						}
						return
					}
					emit(value)
				})
			}
		})
		b.protocols = "|tls|quic|http3|doq|"
		b.descriptor.Description = "Literal in a complete version-qualified CertificateVerify signature vector. Signature, certificate trust and exchange context are not verified. Owned handshake encoding has no outer record/frame span."
		b.descriptor.AllowEmpty = spec.key == "Signature"
		out = append(out, b)
	}
	return out
}
