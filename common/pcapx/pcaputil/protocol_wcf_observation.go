package pcaputil

import "crypto/sha256"

// WCFObservation describes a native framing observation, including a refusal
// inside an already observed WCF connection. It does not authenticate a peer,
// certify an upgrade, or decode an opaque SOAP/Binary XML body.
type WCFObservation struct {
	Status, Summary, Error string
	Bytes                  int
	SHA256                 [32]byte
}

type wcfObservation struct{ WCFObservation }

func newWCFObservation(raw []byte) *wcfObservation {
	return &wcfObservation{WCFObservation{Bytes: len(raw), SHA256: sha256.Sum256(raw)}}
}

func (e *ProtocolEvent) freezeWCFObservation() {
	if e.nativeWCF == nil {
		return
	}
	owned := *e.nativeWCF
	owned.Status, owned.Summary, owned.Error = e.Status, e.Summary, e.Error
	e.nativeWCF = &owned
	if e.semanticSession == nil {
		e.semanticSession = cloneSession(e.Session)
	}
}

// NativeWCFObservation cannot be created by public Session/Profile/Protocol
// edits or restored from a JSON DTO. The returned scalars are an owned copy.
func (e *ProtocolEvent) NativeWCFObservation() (WCFObservation, bool) {
	if e == nil || e.nativeWCF == nil {
		return WCFObservation{}, false
	}
	return e.nativeWCF.WCFObservation, true
}

// NativeWCFWireMatches gates display-only slicing of an admitted record. A
// byte-identical owned Raw is required; editable byte-source hashes are not proof.
func (e *ProtocolEvent) NativeWCFWireMatches() bool {
	v, ok := e.NativeWCFObservation()
	return ok && v.Bytes == len(e.Raw) && v.SHA256 == sha256.Sum256(e.Raw)
}

// NativeWCFFields returns an owned copy of the record and observed context.
// Display code must keep retained context separate from this record's spans.
func (e *ProtocolEvent) NativeWCFFields() (map[string]any, bool) {
	if e == nil || e.nativeWCF == nil {
		return nil, false
	}
	return cloneSession(e.semanticSession), true
}
