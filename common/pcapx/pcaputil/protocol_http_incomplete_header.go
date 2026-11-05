package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
)

// The header was admitted by frameHTTP before the body stalled. Preserve that
// bounded literal observation without running HTTPExact, inferring a missing
// body suffix, publishing a response association, or replacing retained bytes.
func (f *binFlow) retainHTTPIncompleteHeader(dir int, e *ProtocolEvent) {
	h := f.httpIncompleteHeader(dir, e.Raw)
	if h == nil {
		return
	}
	e.Profile = "http-incomplete-message-header-observation"
	e.Admission = "validated-header-without-complete-message"
	e.Completeness, e.ExpertCode = "headers", string(ErrNeedMore)
	e.nativeHTTPHeaderObservation = true
	e.semanticFields = h.retainedHeaderFields()
	e.Session = map[string]any{
		"HTTP Observation Scope":    "validated response header; exact message boundary not reached",
		"Request Context Available": h.method != "",
		"Message Complete":          false,
		"Header Bytes":              len(h.rawHeader),
		"Observed Header SHA256":    fmt.Sprintf("%x", sha256.Sum256(h.rawHeader)),
		"Retained Stream Bytes":     len(e.Raw),
		"Bytes After Header":        len(e.Raw) - len(h.rawHeader),
	}
	e.semanticSession = cloneSession(e.Session)
}

func (f *binFlow) httpIncompleteHeader(dir int, raw []byte) *binHTTPState {
	h := f.directions[dir].http
	if f.protocol != "http" || h == nil || !h.response || h.awaitRequest || h.omitted ||
		len(h.rawHeader) == 0 || !bytes.HasPrefix(raw, h.rawHeader) {
		return nil
	}
	return h
}
