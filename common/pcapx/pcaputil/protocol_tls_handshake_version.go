package pcaputil

// RFC 8446 B.3 reserves these earlier-protocol message types. Authentication
// proves their bytes, not their validity in the selected protocol version.
// Initial QUIC Hello messages use authenticated=false at this TLS layer; its
// Initial packet protection is not TLS handshake-traffic record protection.
func (t *binTLS) checkHandshakeVersion(kind byte, authenticated bool) error {
	if t.version != 0x304 {
		return nil
	}
	var name string
	switch kind {
	case 0:
		name = "HelloRequest"
	case 3:
		name = "HelloVerifyRequest"
	case 12:
		name = "ServerKeyExchange"
	case 14:
		name = "ServerHelloDone"
	case 16:
		name = "ClientKeyExchange"
	case 21:
		name = "CertificateURL"
	case 22:
		name = "CertificateStatus"
	case 23:
		name = "SupplementalData"
	case 254:
		// RFC 8446 4.4.1 synthesizes this inside the transcript hash. It is
		// never an additional message received from the peer on the wire.
		return protocolError(ErrMalformedMessage, "TLS 1.3 message_hash is transcript-only, not a wire handshake")
	case 1, 2:
		if authenticated {
			return protocolError(ErrMalformedMessage, "TLS 1.3 Hello inside a protected handshake record")
		}
	}
	if name != "" {
		return protocolError(ErrMalformedMessage, "TLS 1.3 reserved handshake type %d (%s)", kind, name)
	}
	return nil
}
