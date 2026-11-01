package pcaputil

type tlsPHARequest struct {
	ordinal                   uint64
	certificateSeen, finished bool
}

const (
	tlsPHAResponseNone uint8 = iota
	tlsPHAResponseVerify
	tlsPHAResponseFinished
)

type tlsPHAResponse struct {
	context          string
	ordinal          uint64
	stage            uint8
	empty            bool
	uncertainHistory bool
}

// Request and response history belong to the connection, including across
// KeyUpdate. Neither public decoded bytes nor a new traffic key may rewrite it.
func (t *binTLS) retainPHARequest(context string, ordinal uint64) {
	next := make(map[string]tlsPHARequest, len(t.phaRequests)+1)
	for key, value := range t.phaRequests {
		next[key] = value
	}
	next[context] = tlsPHARequest{ordinal: ordinal}
	t.phaRequests = next
}

func (t *binTLS) updatePHARequest(context string, request tlsPHARequest) {
	next := make(map[string]tlsPHARequest, len(t.phaRequests))
	for key, value := range t.phaRequests {
		next[key] = value
	}
	next[context] = request
	t.phaRequests = next
}

func (t *binTLS) phaClientApplication(dir int) bool {
	return t.version == 0x304 && t.client >= 0 && dir == t.client && t.app[dir]
}

// A missing record can contain an entire authentication message. An observed
// Certificate can anchor a later response block, but CV/Finished alone cannot.
// Sticky gaps prevent absent capture history from becoming a normative error.
func (t *binTLS) notePHAGap(dir int) {
	t.phaGap[dir] = true
	if dir == t.client {
		t.phaResponse = tlsPHAResponse{}
	}
}

func (t *binTLS) checkPHAHandshakeOrder(dir int, kind byte) error {
	if t.phaClientApplication(dir) && t.phaResponse.stage != tlsPHAResponseNone && kind != 15 && kind != 20 {
		return protocolError(ErrMalformedMessage, "TLS post-handshake response interleaved")
	}
	return nil
}

func (response tlsPHAResponse) observation(stage string) map[string]any {
	association := "request not observed; association unavailable"
	if response.ordinal != 0 {
		association = "matched observed CertificateRequest"
	}
	selection := "certificate list supplied"
	if response.empty {
		selection = "empty certificate list (declined)"
	}
	out := map[string]any{
		"Context Association":   association,
		"Response Stage":        stage,
		"Certificate Selection": selection,
		"Verification Scope":    "observed message order; signature, Finished MAC and certificate trust not evaluated",
	}
	if response.ordinal != 0 {
		out["Request Ordinal"] = response.ordinal
	}
	if response.uncertainHistory {
		out["Prior Client History"] = "unavailable; association covers observed messages only"
	}
	return out
}

func unavailablePHAResponse() map[string]any {
	return map[string]any{
		"Context Association":   "response prefix not observed; association unavailable",
		"Response Stage":        "response prefix unavailable",
		"Certificate Selection": "unavailable",
		"Verification Scope":    "signature, Finished MAC and certificate trust not evaluated",
	}
}

// RFC 8446 4.4.2 and 4.6.2: contexts identify requests, responses may arrive in
// a different order, and an empty Certificate is a valid decline. This checks
// observed association and order only, never transcript integrity or PKI trust.
func (t *binTLS) admitPHACertificate(dir int, context []byte, empty, authenticated bool) (map[string]any, error) {
	if !authenticated || !t.phaClientApplication(dir) {
		return nil, nil
	}
	key := string(context) // own bytes independently of the public body vector
	request, matched := t.phaRequests[key]
	if matched && request.certificateSeen {
		return nil, protocolError(ErrMalformedMessage, "repeated TLS Certificate response context")
	}
	if !matched && t.phaInitialFinished[0] && t.phaInitialFinished[1] && !t.phaGap[1-dir] {
		return nil, protocolError(ErrMalformedMessage, "unobserved TLS CertificateRequest context")
	}
	response := tlsPHAResponse{context: key, empty: empty, stage: tlsPHAResponseVerify, uncertainHistory: !t.phaInitialFinished[dir] || t.phaGap[dir]}
	stage := "awaiting CertificateVerify"
	if empty {
		response.stage, stage = tlsPHAResponseFinished, "awaiting Finished"
	}
	if matched {
		response.ordinal = request.ordinal
		request.certificateSeen = true
		t.updatePHARequest(key, request)
	}
	t.phaResponse = response
	return response.observation(stage), nil
}

func (t *binTLS) admitPHACertificateVerify(dir int) (map[string]any, error) {
	if !t.phaClientApplication(dir) {
		return nil, nil
	}
	response := t.phaResponse
	if response.stage == tlsPHAResponseNone {
		if t.phaInitialFinished[dir] && !t.phaGap[dir] {
			return nil, protocolError(ErrMalformedMessage, "TLS CertificateVerify without an observed Certificate")
		}
		return unavailablePHAResponse(), nil
	}
	if response.stage != tlsPHAResponseVerify {
		if response.empty {
			return nil, protocolError(ErrMalformedMessage, "TLS CertificateVerify after an empty Certificate")
		}
		return nil, protocolError(ErrMalformedMessage, "repeated TLS CertificateVerify in post-handshake response")
	}
	response.stage = tlsPHAResponseFinished
	t.phaResponse = response
	return response.observation("awaiting Finished"), nil
}

func (t *binTLS) admitPHAFinished(dir int) (map[string]any, error) {
	if !t.phaClientApplication(dir) {
		return nil, nil
	}
	response := t.phaResponse
	if response.stage == tlsPHAResponseNone {
		if t.phaInitialFinished[dir] && !t.phaGap[dir] {
			return nil, protocolError(ErrMalformedMessage, "TLS post-handshake Finished without an observed Certificate")
		}
		return unavailablePHAResponse(), nil
	}
	if response.stage != tlsPHAResponseFinished {
		return nil, protocolError(ErrMalformedMessage, "TLS post-handshake Finished before CertificateVerify")
	}
	if response.ordinal != 0 {
		request := t.phaRequests[response.context]
		request.finished = true
		t.updatePHARequest(response.context, request)
	}
	t.phaResponse = tlsPHAResponse{}
	return response.observation("response block complete; transcript not verified"), nil
}
