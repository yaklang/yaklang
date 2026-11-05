package pcaputil

type tlsHTTPGapCause struct {
	pdu     uint64
	version uint16
}

func (f *binFlow) tlsHTTPGapPending(dir int) bool {
	return f.tls != nil && f.tls.child != nil && f.tls.child.directions[dir].tlsGapHTTPPending
}

// Record authentication failures and authenticated sequence/epoch recovery can
// both establish a discontinuity. Publish its cause first; it is context only,
// never an application-byte contributor or evidence of message completion.
func (f *binFlow) deliverTLSHTTPGap(dir int, parent *ProtocolEvent) {
	if f.tlsHTTPGapPending(dir) {
		f.tls.child.emitHTTPGapObservation(dir, &tlsHTTPGapCause{pdu: parent.ID, version: f.tls.version})
	}
}

func (f *binFlow) emitHTTPGapObservation(dir int, cause *tlsHTTPGapCause) {
	d := &f.directions[dir]
	if !d.tlsGapHTTPPending {
		return
	}
	e := f.event(dir, d.buffer, "incomplete", "TLS plaintext discontinuity: exact HTTP message boundary was not reached")
	f.retainHTTPIncompleteHeader(dir, e)
	if e.Session == nil {
		e.Session = map[string]any{}
	}
	e.Session["TLS Plaintext Gap"] = true
	e.Session["TLS Gap Direction"] = dir
	if cause != nil && cause.pdu != 0 {
		e.Session["TLS Gap Cause PDU"] = cause.pdu
		e.Session["TLS Gap Version"] = cause.version
	}
	e.semanticSession = cloneSession(e.Session)
	f.a.incomplete.Add(1)
	f.release(d)
	d.http = nil
	f.a.emit(e)
}
