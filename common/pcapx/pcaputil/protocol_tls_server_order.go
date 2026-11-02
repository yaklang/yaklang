package pcaputil

// RFC 8446 4.3/4.4: EE, optional CR, Certificate, CertificateVerify, Finished.
// Only compare accepted, authenticated messages in one known server direction.
// A passive capture may miss any prefix or intermediate record, so absence is
// not an error and a gap cannot manufacture completion or erase observed order.
// This is not a transcript, negotiation, signature or Finished-MAC validator.
func (t *binTLS) observedServerHandshakeStage(dir int, kind byte, authenticated bool) (uint8, error) {
	if t.version != 0x304 || !authenticated || t.client < 0 || dir == t.client || t.app[dir] {
		return 0, nil
	}
	var stage uint8
	switch kind {
	case 8:
		stage = 1
	case 13:
		stage = 2
	case 11:
		stage = 3
	case 15:
		stage = 4
	case 20:
		stage = 5
	default:
		return 0, nil
	}
	if stage <= t.serverHandshakeStage {
		return 0, protocolError(ErrMalformedMessage, "TLS 1.3 server handshake order repeats or precedes an accepted message")
	}
	return stage, nil
}
