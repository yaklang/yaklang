package pcaputil

// Request contexts are scoped to this TLS connection, not a key epoch, tuple,
// certificate or global cache. Strings own their bytes; public decoded bodies
// and encoded handshakes cannot mutate this history. All updates replace the map.
func (t *binTLS) requestContextStorage() int64 {
	if len(t.requestContexts) == 0 {
		return 0
	}
	size := int64(256)
	for context := range t.requestContexts {
		size += 128 + int64(len(context))
	}
	return size
}

// RFC 8446 4.2.6, 4.3.2 and 4.6.2. The passive decoder can establish uniqueness
// only among observed requests. Missing ClientHello is unknown, not an absent
// offer. These checks do not verify the client's subsequent response transcript,
// certificate trust, context unpredictability or requests lost before capture.
func (t *binTLS) admitCertificateRequest(dir int, request *TLSCertificateRequestView, limit int) (map[string]any, error) {
	if t.version != 0x304 || t.client < 0 {
		return nil, nil
	}
	offer := "unknown"
	if len(t.firstHello) != 0 {
		hello, err := tlsHello(t.firstHello, true)
		if err != nil {
			return nil, err
		}
		offer = "not-offered"
		if _, present := hello.ext[49]; present {
			offer = "offered"
		}
	}
	if t.app[dir] && offer == "not-offered" {
		return nil, protocolError(ErrMalformedMessage, "TLS post-handshake authentication not offered")
	}
	context := string(request.Fields["Context"].([]byte))
	if _, repeated := t.requestContexts[context]; repeated {
		return nil, protocolError(ErrMalformedMessage, "repeated TLS CertificateRequest context")
	}
	if len(t.requestContexts) >= sessionCollectionLimit(limit) {
		return nil, protocolError(ErrResourceExceeded, "TLS CertificateRequest context history limit")
	}
	next := make(map[string]struct{}, len(t.requestContexts)+1)
	for key := range t.requestContexts {
		next[key] = struct{}{}
	}
	next[context] = struct{}{}
	t.requestContexts = next
	if !t.app[dir] {
		return nil, nil
	}
	return map[string]any{
		"Client PHA Offer":       offer,
		"Context Uniqueness":     "unique among observed requests",
		"Observed Request Count": uint64(len(next)),
	}, nil
}
