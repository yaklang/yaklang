package tcp

// SYNSequenceNumber returns the initial sequence number of this endpoint's
// active handshake. A stepped probe uses it to distinguish its SYN from an
// older packet still queued on a link after a source tuple was reused.
// It does not advance the handshake or change retransmission behavior.
func (e *Endpoint) SYNSequenceNumber() (uint32, bool) {
	e.LockUser()
	defer e.UnlockUser()
	if e.EndpointState() != StateSynSent || e.h == nil {
		return 0, false
	}
	return uint32(e.h.iss), true
}
