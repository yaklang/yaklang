package pcaputil

import (
	"bufio"
	"bytes"
	"net/http"
)

// Only previously observed request roles permit waiting. A first midstream
// response retains the existing immediate context-required header diagnostic.
func (f *binFlow) httpMayAwaitRequest(dir int) bool {
	return f.httpRequestDirections == byte(1<<uint(1-dir))
}

func (f *binFlow) awaitHTTPRequest(h *binHTTPState) error {
	// The exact stream bytes use the existing bounded direction buffer. Retain
	// only the owned raw header, summary, and this fixed state; never retain a
	// net/http response Body reader or the parsed MIME header map while waiting.
	// 512 conservatively covers the current fixed binHTTPState, while the
	// doubled summary length includes string allocator rounding.
	charge := int64(512 + cap(h.rawHeader) + 2*len(h.summary))
	if !f.a.reserveEvidence(charge) {
		return protocolError(ErrResourceExceeded, "HTTP response context wait exceeds capture memory budget")
	}
	h.awaitRequest = true
	h.awaitCharge = charge
	h.headerFields = nil
	return nil
}

func (f *binFlow) releaseHTTPAwait(h *binHTTPState) {
	if h != nil && h.awaitCharge != 0 {
		f.a.buffered.Add(-h.awaitCharge)
		h.awaitCharge = 0
	}
}

// The header was validated before entering awaitRequest. Reconstruct only its
// literal diagnostic tree when closing/refusing; no method or body boundary is
// inferred. This reuses the existing bounded header diagnostic path.
func (f *binFlow) prepareHTTPAwaitDiagnostic(dir int) {
	h := f.directions[dir].http
	if h == nil || !h.awaitRequest || h.headerFields != nil || len(h.rawHeader) == 0 {
		return
	}
	r := bufio.NewReaderSize(bytes.NewReader(h.rawHeader), min(len(h.rawHeader), 4096))
	response, err := http.ReadResponse(r, &http.Request{})
	if err != nil {
		return // Private validated bytes should make this unreachable; do not guess.
	}
	h.headerFields = map[string]any{"Protocol": response.Proto, "Status Code": response.StatusCode, "Status": response.Status, "Headers": httpHeaderFields(response.Header)}
}

// Called after publishing an ordinary complete request, or after retaining an
// incomplete request body whose validated header is already queued (Expect:
// 100-continue). The private queue is committed before either call site.
func (f *binFlow) httpAwaitingPeerReady(dir int) bool {
	if f.httpReplaying || f.protocol != "http" || f.directions[dir].stopped || len(f.httpMethods) == 0 || f.httpRequestDirections != byte(1<<uint(dir)) {
		return false
	}
	peer := &f.directions[1-dir]
	return !peer.stopped && len(peer.buffer) > 0 && peer.http != nil && peer.http.awaitRequest
}

func (f *binFlow) replayHTTPAwaitingPeer(dir int) {
	if !f.httpAwaitingPeerReady(dir) {
		return
	}
	peer := &f.directions[1-dir]
	f.httpReplaying = true
	defer func() { f.httpReplaying = false }()
	// Keep the peer's original timestamp and contribution ledger. The caller's
	// currently arriving request/TLS parent is not the response's byte source.
	f.feed(1-dir, nil, peer.ts)
}
