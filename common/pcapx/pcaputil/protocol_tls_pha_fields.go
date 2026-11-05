package pcaputil

func tlsPHADisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, spec := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}{
		{"tls.pha.offer_status", "Client PHA Offer", DisplayString, 0},
		{"tls.pha.context_status", "Context Uniqueness", DisplayString, 0},
		{"tls.pha.requests_seen", "Observed Request Count", DisplayUnsigned, 64},
	} {
		spec := spec
		b := displayCustom("tls", spec.name, spec.key, spec.typ, spec.bits, "context", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if e.Protocol == "tls" {
				p.walk(e.Session, []string{"Handshake Messages", "*", "Certificate Request Admission", spec.key}, emit)
			}
		})
		b.descriptor.Description = "Current-connection observation for this admitted TLS 1.3 post-handshake CertificateRequest; unknown is distinct from absent. Uniqueness covers observed requests only, not missing history, response transcript verification or certificate trust. No wire range."
		out = append(out, b)
	}
	for _, spec := range []struct {
		name, key string
		typ       DisplayFieldType
		bits      int
	}{
		{"tls.pha.request_index", "Request Ordinal", DisplayUnsigned, 64},
		{"tls.pha.response_context", "Context Association", DisplayString, 0},
		{"tls.pha.response_stage", "Response Stage", DisplayString, 0},
		{"tls.pha.certificate_selection", "Certificate Selection", DisplayString, 0},
	} {
		spec := spec
		binding := displayCustom("tls", spec.name, spec.key, spec.typ, spec.bits, "context", func(e *ProtocolEvent, emit func(any), p *displayProjection) {
			if e.Protocol == "tls" {
				p.walk(e.Session, []string{"Handshake Messages", "*", "Post Handshake Response", spec.key}, emit)
				p.walk(e.Session, []string{"Handshake Messages", "*", "Certificate Request Admission", "Earlier Response", spec.key}, emit)
			}
		})
		binding.descriptor.Description = "Observed TLS 1.3 client post-handshake response association/order in this connection. Request ordinal counts observed requests, not PDU IDs. Missing history stays unavailable; signature, Finished MAC and trust are not verified. No wire range."
		out = append(out, binding)
	}
	return out
}
