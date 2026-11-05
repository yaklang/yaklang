package pcaputil

// Boundary evidence is context, not an HTTP byte contributor. In particular the
// TLS alert must not acquire an HTTP field highlight or replace CompletedIn,
// which identifies the last numbered contributor of the message's own bytes.
type tlsHTTPBoundary struct {
	pdu       uint64
	direction int
	version   uint16
}

// Called only for a clean cleartext FIN or the authenticated, continuous TLS
// sender EOF. It completes this one close-delimited response, not both flow
// directions, fixed/chunked bodies, or another protocol's unfinished message.
func (f *binFlow) finishHTTPCloseDelimited(dir int, boundary *tlsHTTPBoundary) bool {
	d := &f.directions[dir]
	h := d.http
	if h == nil || !h.closeDelimited || h.awaitRequest {
		return false
	}
	if h.omitted {
		f.finishHTTP(dir)
		f.release(d)
		return true
	}
	if len(d.buffer) == 0 {
		return false
	}
	e := f.event(dir, d.buffer, "deferred", h.summary)
	e.Rule, e.Entry, e.decodeConfig = "application-layer.http", "HTTPExact", h.config(f.a.config.MaxMessageBytes)
	e.ipp = h.ipp
	f.httpEvidence(dir, e)
	f.a.messages.Add(1)
	f.a.messageBytes.Add(uint64(len(d.buffer)))
	if !f.a.config.Deferred || h.doh || h.ipp {
		result, err := e.Decode()
		if err == nil && (h.doh || h.ipp) {
			err = f.consumeSession(dir, e, result)
		}
		if err != nil {
			e.Status, e.sessionError = classifySessionError(err)
			f.lastSessionError = e.sessionError
			e.Error = err.Error()
		} else if !f.a.config.Deferred {
			e.Status, e.Structured = "decoded", result
		}
	}
	if boundary != nil {
		if e.Session == nil {
			e.Session = map[string]any{}
		}
		e.Session["Message Boundary"] = "authenticated TLS close_notify"
		e.Session["TLS Boundary PDU"] = boundary.pdu
		e.Session["TLS Boundary Direction"] = boundary.direction
		e.Session["TLS Boundary Version"] = boundary.version
		e.semanticSession = cloneSession(e.Session)
	}
	switch e.Status {
	case "decoded":
		f.a.decoded.Add(1)
	case "deferred":
		f.a.deferred.Add(1)
	case "limited":
		f.a.limited.Add(uint64(len(d.buffer)))
	case "context-required":
		f.a.contextRequired.Add(1)
	default:
		f.a.malformed.Add(1)
	}
	f.release(d)
	f.finishHTTP(dir)
	f.a.emit(e)
	return true
}
