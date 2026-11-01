package pcaputil

// Request contexts are scoped to this TLS connection, not a key epoch, tuple,
// certificate or global cache. Strings own their bytes; public decoded bodies
// and encoded handshakes cannot mutate this history. All updates replace the map.
func (t *binTLS) requestContextStorage() int64 {
	var size int64
	if len(t.requestContexts) != 0 {
		size += 256
		for context := range t.requestContexts {
			size += 128 + int64(len(context))
		}
	}
	if len(t.phaRequests) != 0 {
		size += 256 + 128*int64(len(t.phaRequests))
	}
	if t.phaResponse.stage != tlsPHAResponseNone {
		size += 128 + int64(len(t.phaResponse.context))
	}
	if len(t.phaEarlyResponses) != 0 {
		size += 256
		for context := range t.phaEarlyResponses {
			size += 192 + int64(len(context))
		}
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
	if t.phaOfferKnown {
		offer = "not-offered"
		if t.phaOffered {
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
	_, early := t.phaEarlyResponses[context]
	retained := len(t.requestContexts) + len(t.phaEarlyResponses)
	if early {
		retained-- // this request replaces its already charged early-response slot
	}
	if retained >= sessionCollectionLimit(limit) {
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
	response, observedEarly := t.retainPHARequest(context, uint64(len(next)))
	out := map[string]any{
		"Client PHA Offer":       offer,
		"Context Uniqueness":     "unique among observed requests",
		"Observed Request Count": uint64(len(next)),
	}
	if observedEarly {
		stage := "response block complete; transcript not verified"
		if response.stage == tlsPHAResponseVerify {
			stage = "awaiting CertificateVerify"
		} else if response.stage == tlsPHAResponseFinished {
			stage = "awaiting Finished"
		}
		out["Earlier Response"] = response.observation(stage)
		out["Response Observation Order"] = "response observed before request; earlier events retain their original observation"
	}
	return out, nil
}
