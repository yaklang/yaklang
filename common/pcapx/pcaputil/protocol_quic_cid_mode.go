package pcaputil

// zeroCID[D] is the observed endpoint D's empty Source CID choice, so packets
// sent TOWARD D use no Destination CID. It is not inferred from a missing CID
// map, a short-header guess, peer token values, or editable event metadata.
func (q *binQUIC) validateObservedCIDMode(dir int, frames []map[string]any) error {
	if dir < 0 || dir > 1 {
		return protocolError(ErrMalformedMessage, "QUIC invalid captured endpoint direction")
	}
	for _, frame := range frames {
		switch frame["Frame Type"] {
		case "NEW_CONNECTION_ID":
			if q.zeroCID[dir] {
				return protocolError(ErrMalformedMessage, "QUIC NEW_CONNECTION_ID contradicts the sender's observed zero-length CID choice")
			}
		case "RETIRE_CONNECTION_ID":
			if q.zeroCID[1-dir] {
				return protocolError(ErrMalformedMessage, "QUIC RETIRE_CONNECTION_ID contradicts the receiver's observed zero-length CID choice")
			}
		}
	}
	return nil
}
