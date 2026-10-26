package pcaputil

// AuthenticatedQUICApplicationProtocol reports the observed application identity
// even when its message fields were rejected. It relies on the native decoder's
// private packet-authentication and ALPN snapshot, never editable public labels.
// It does not certify message validity, peer identity or delivery.
func (e *ProtocolEvent) AuthenticatedQUICApplicationProtocol() string {
	if e == nil || !e.nativeQUICObservation || e.semanticSession == nil {
		return ""
	}
	m := e.semanticSession
	if m["Authentication Verified"] != true || m["Native Carrier"] != true {
		return ""
	}
	if m["ALPN"] == "h3" && m["HTTP3"] == true {
		return "http3"
	}
	return ""
}
