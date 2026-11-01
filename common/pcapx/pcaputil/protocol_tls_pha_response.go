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
func (t *binTLS) retainPHARequest(context string, ordinal uint64) (tlsPHAResponse, bool) {
	next := make(map[string]tlsPHARequest, len(t.phaRequests)+1)
	for key, value := range t.phaRequests {
		next[key] = value
	}
	response, early := t.phaEarlyResponses[context]
	next[context] = tlsPHARequest{ordinal: ordinal, certificateSeen: early, finished: early && response.stage == tlsPHAResponseNone}
	t.phaRequests = next
	if early {
		var pending map[string]tlsPHAResponse
		if len(t.phaEarlyResponses) > 1 {
			pending = make(map[string]tlsPHAResponse, len(t.phaEarlyResponses)-1)
		}
		for key, value := range t.phaEarlyResponses {
			if key != context {
				pending[key] = value
			}
		}
		t.phaEarlyResponses = pending
		response.ordinal = ordinal
		if t.phaResponse.stage != tlsPHAResponseNone && t.phaResponse.context == context {
			t.phaResponse.ordinal = ordinal
		}
	}
	return response, early
}

// Only this record's captured ACKs can prove that the peer prefix acknowledged
// by a response has already reached the parser. Initial Finished, an empty TCP
// pending queue, and a later packet's ACK are insufficient evidence. Reliable
// UDP carriers and explicit sessions have no equivalent captured TCP proof.
func (f *binFlow) phaPeerPrefixComplete(dir int, e *ProtocolEvent) bool {
	if !f.captureTCP || f.transportFlow == nil || e.Length <= 0 {
		return false
	}
	tf := f.transportFlow
	peer := tf.ServerConn
	if dir == 1 {
		peer = tf.ClientConn
	}
	if peer == nil || !peer.initialed || !peer.synSeen || tf.reassemblyIncomplete || len(f.directions[1-dir].buffer) != 0 || f.directions[1-dir].stopped {
		return false
	}
	if f.tls != nil && (len(f.tls.handshake[1-dir]) != 0 || f.tls.recordGap[1-dir] || f.tls.handshakeGap[1-dir]) {
		return false
	}
	end, covered := e.Offset+uint64(e.Length), e.Offset
	if end < e.Offset || f.directions[dir].evidenceLimited {
		return false
	}
	for _, part := range f.directions[dir].contributions {
		if part.start >= end || part.end <= e.Offset {
			continue
		}
		// The complete interval must have captured ACK evidence. Differences
		// at half the sequence space are ambiguous, including around wrap.
		if !part.tcpACKKnown || part.start > covered || uint32(part.tcpACK-(peer.isn+1)) >= 1<<31 || uint32(peer.nextSeq-part.tcpACK) >= 1<<31 {
			return false
		}
		covered = max(covered, part.end)
	}
	return covered >= end
}

func (t *binTLS) retainEarlyPHAResponse(response tlsPHAResponse) {
	next := make(map[string]tlsPHAResponse, len(t.phaEarlyResponses)+1)
	for key, value := range t.phaEarlyResponses {
		next[key] = value
	}
	next[response.context] = response
	t.phaEarlyResponses = next
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
func (t *binTLS) admitPHACertificate(dir int, context []byte, empty, authenticated bool, limit int) (map[string]any, error) {
	if !authenticated || !t.phaClientApplication(dir) {
		return nil, nil
	}
	key := string(context) // own bytes independently of the public body vector
	request, matched := t.phaRequests[key]
	if matched && request.certificateSeen {
		return nil, protocolError(ErrMalformedMessage, "repeated TLS Certificate response context")
	}
	if _, repeated := t.phaEarlyResponses[key]; !matched && repeated {
		return nil, protocolError(ErrMalformedMessage, "repeated TLS Certificate response context")
	}
	if !matched && t.phaPeerPrefixComplete && t.phaInitialFinished[0] && t.phaInitialFinished[1] && !t.phaGap[1-dir] {
		return nil, protocolError(ErrMalformedMessage, "unobserved TLS CertificateRequest context")
	}
	if !matched && len(t.requestContexts)+len(t.phaEarlyResponses) >= sessionCollectionLimit(limit) {
		return nil, protocolError(ErrResourceExceeded, "TLS post-handshake response context history limit")
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
	} else {
		t.retainEarlyPHAResponse(response)
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
	if response.ordinal == 0 {
		t.retainEarlyPHAResponse(response)
	}
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
	} else {
		response.stage = tlsPHAResponseNone
		t.retainEarlyPHAResponse(response)
	}
	t.phaResponse = tlsPHAResponse{}
	return response.observation("response block complete; transcript not verified"), nil
}
