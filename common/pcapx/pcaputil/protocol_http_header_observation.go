package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"time"
)

const httpUnframedPreviewLimit = 64 << 10

// A validated response header does not establish its body boundary when the
// request method is missing. Preserve the old header-only Raw/field contract,
// and retain a bounded, separately attributed preview of the remaining ordered
// bytes. No HTTP resynchronization, guessed method or message completion occurs.
// The returned evidence charge lives until stop has released/emitted the event.
func (f *binFlow) retainHTTPUnframedObservation(dir int, e *ProtocolEvent, wire []byte) int64 {
	h := f.directions[dir].http
	if h == nil || !h.response || h.method != "" || h.omitted || len(h.rawHeader) == 0 ||
		!bytes.Equal(e.Raw, h.rawHeader) || !bytes.HasPrefix(wire, h.rawHeader) {
		return 0
	}
	e.Profile = "http-response-header-observation"
	e.Admission = "complete-header-without-request-method"
	e.nativeHTTPHeaderObservation = true
	if e.sessionError == nil && e.Status == "context-required" {
		// Keep the established literal-header query and empty Error contract.
		// Classification is still explicitly partial/context-required.
		e.ExpertCode = string(ErrContextRequired)
	}
	e.Session = map[string]any{
		"Request Context Available": false,
		"Body Boundary Known":       false,
		"Retained Stream Bytes":     len(wire),
		"Unframed Stream Bytes":     len(wire) - len(h.rawHeader),
		"HTTP Observation Scope":    "validated response header; request method unavailable",
	}
	tail := wire[len(h.rawHeader):]
	if len(tail) == 0 {
		e.semanticSession = cloneSession(e.Session)
		return 0
	}
	// ProbeBytes bounds identity detection, not display retention. Keep small
	// observations complete, with a separate hard cap and message budget.
	prefix := tail[:min(len(tail), min(httpUnframedPreviewLimit, f.a.config.MaxMessageBytes))]
	// Two owned snapshots (public/query), plus two bounded provenance lists.
	// Count all existing contributions conservatively before constructing the
	// narrower prefix ledger. History/snapshot budgets also count these bytes.
	charge := int64(2048 + 2*len(prefix) + 160*len(f.directions[dir].contributions))
	if !f.a.reserveEvidence(charge) {
		e.Session["Unframed Preview Unavailable"] = "capture memory budget exhausted"
		e.semanticSession = cloneSession(e.Session)
		return 0
	}
	observation := &ProtocolEvent{Offset: e.Offset + uint64(len(h.rawHeader)), Length: len(prefix)}
	f.eventEvidence(dir, observation)
	kind := observation.SourceBytes.Kind
	if kind == "" {
		kind = "reassembled"
	}
	e.Session["Unframed Stream Observation"] = map[string]any{
		"Observation Scope":      "ordered bytes after observed header; body and next-message boundaries unknown",
		"Offset":                 observation.Offset,
		"Observed Bytes":         len(tail),
		"Observed SHA256":        fmt.Sprintf("%x", sha256.Sum256(tail)),
		"Prefix Bytes":           len(prefix),
		"Prefix SHA256":          fmt.Sprintf("%x", sha256.Sum256(prefix)),
		"Preview Limited":        len(prefix) < len(tail),
		"Unframed Stream Prefix": bytes.Clone(prefix),
		"Source Kind":            kind,
		"Packet References":      observation.SourceBytes.PacketRefs,
		"Parent PDUs":            observation.SourceBytes.ParentPDUs,
		"References Limited":     f.directions[dir].evidenceLimited,
	}
	e.semanticSession = cloneSession(e.Session)
	return charge
}

func (f *binFlow) rememberHTTPHeaderObservation(dir int, e *ProtocolEvent) {
	if e.Profile != "http-response-header-observation" || e.Status != "context-required" {
		return
	}
	d := &f.directions[dir]
	h := d.http
	if h == nil || len(h.rawHeader) == 0 {
		return
	}
	end := bytes.IndexByte(h.rawHeader, ' ')
	if end < 0 {
		return
	}
	// Commit private lineage before handing the editable public event to the
	// callback. Neither a changed ID nor edited Raw can replace this context.
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	d.httpUnframedHeaderID = e.ID
	d.httpUnframedVersion = string(h.rawHeader[:end])
	d.httpUnframedHeaderHash = sha256.Sum256(h.rawHeader)
}

func (f *binFlow) emitHTTPUnframedArrival(dir int, wire []byte, ts time.Time) {
	d := &f.directions[dir]
	d.ts = ts
	prefix := wire[:min(len(wire), min(httpUnframedPreviewLimit, f.a.config.MaxMessageBytes))]
	charge := int64(1024 + len(prefix) + 80*len(d.contributions))
	reserved := f.a.reserveEvidence(charge)
	if reserved {
		defer f.a.buffered.Add(-charge)
	} else {
		prefix = nil
		// Resource exhaustion retires the observation continuation. It does
		// not resume the parser or retain an unbounded queue of context.
		defer func() { d.httpUnframedHeaderID = 0 }()
	}
	e := f.event(dir, prefix, "context-required", fmt.Sprintf("HTTP response context · %d unframed stream bytes; request method unavailable", len(wire)))
	e.Profile, e.Admission = "http-unframed-stream-observation", "after-complete-header-without-request-method"
	e.Length = len(wire)
	e.sessionError = &ProtocolError{Kind: ErrContextRequired, Message: "HTTP body and next-message boundaries unknown: request method unavailable"}
	e.nativeDiagnosticError = e.sessionError.Error()
	e.Error = e.sessionError.Error()
	e.Session = map[string]any{
		"Observation Scope":         "ordered bytes after a response header with no request method; message boundary unknown",
		"Observed Header PDU":       d.httpUnframedHeaderID,
		"Observed Header Protocol":  d.httpUnframedVersion,
		"Observed Header SHA256":    fmt.Sprintf("%x", d.httpUnframedHeaderHash),
		"Body Boundary Known":       false,
		"Request Context Available": false,
		"Observed Bytes":            len(wire),
		"Observed SHA256":           fmt.Sprintf("%x", sha256.Sum256(wire)),
		"Preview Limited":           len(prefix) < len(wire),
	}
	if !reserved {
		e.Status = "limited"
		e.sessionError = &ProtocolError{Kind: ErrResourceExceeded, Message: "unframed HTTP stream preview exceeds capture memory budget"}
		e.Error, e.nativeDiagnosticError = e.sessionError.Error(), e.sessionError.Error()
		e.Session["Preview Unavailable"] = "capture memory budget exhausted"
	}
	e.semanticSession = cloneSession(e.Session)
	f.a.contextRequired.Add(1)
	f.a.emit(e)
	// No buffered grammar survives this path; each arrival owns its prefix
	// ledger and cannot make future bytes inherit stale contributors.
	f.release(d)
}
