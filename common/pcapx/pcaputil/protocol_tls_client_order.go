package pcaputil

// RFC 8446 4.4: an initial client authentication flight observes Certificate,
// CertificateVerify, Finished in order. Missing captured prefixes/intermediate
// messages are allowed; only accepted, authenticated observations constrain the
// next one. Application-key PHA responses retain their separate context owner.
// This does not verify signatures, transcripts, negotiation or peer trust.
func (t *binTLS) observedClientHandshakeStage(dir int, kind byte, authenticated bool) (uint8, error) {
	if t.version != 0x304 || !authenticated || t.client < 0 || dir != t.client || t.app[dir] {
		return 0, nil
	}
	var stage uint8
	switch kind {
	case 11:
		stage = 1
	case 15:
		stage = 2
	case 20:
		stage = 3
	default:
		return 0, nil
	}
	if stage <= t.clientHandshakeStage {
		return 0, protocolError(ErrMalformedMessage, "TLS 1.3 client handshake order: repeated or reversed message")
	}
	if kind == 15 && t.clientCertificateEmpty {
		return 0, protocolError(ErrMalformedMessage, "TLS 1.3 CertificateVerify after empty client Certificate")
	}
	return stage, nil
}
