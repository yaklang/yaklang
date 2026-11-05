package pcaputil

import "crypto/sha256"

// WCFTLSCarrierObservation is a native observation of the outer MC-NMF
// upgrade, not a TLS authentication, closure or decrypted application result.
type WCFTLSCarrierObservation struct {
	WCFObservation
	ObservedSessionIndex uint64
	Returned             bool
}

type wcfTLSCarrierObservation = WCFTLSCarrierObservation

func (e *ProtocolEvent) freezeWCFTLSObservation() {
	if e.nativeWCFTLS == nil {
		return
	}
	owned := *e.nativeWCFTLS
	owned.Status, owned.Summary, owned.Error = e.Status, e.Summary, e.Error
	e.nativeWCFTLS = &owned
	if e.semanticFields == nil {
		e.semanticFields = cloneSession(e.Session)
	}
}

// NativeWCFTLSCarrier cannot be manufactured by public flags or JSON DTOs.
func (e *ProtocolEvent) NativeWCFTLSCarrier() (WCFTLSCarrierObservation, bool) {
	if e == nil || e.nativeWCFTLS == nil {
		return WCFTLSCarrierObservation{}, false
	}
	return *e.nativeWCFTLS, true
}

func (e *ProtocolEvent) NativeWCFTLSWireMatches() bool {
	v, ok := e.NativeWCFTLSCarrier()
	return ok && v.Bytes == len(e.Raw) && v.SHA256 == sha256.Sum256(e.Raw)
}

// NativeWCFTLSFields returns an owned projection, including an incomplete
// stream diagnostic's carrier context without granting it record admission.
func (e *ProtocolEvent) NativeWCFTLSFields() (map[string]any, bool) {
	if e == nil || e.nativeWCFTLS == nil {
		return nil, false
	}
	return cloneSession(e.semanticFields), true
}
