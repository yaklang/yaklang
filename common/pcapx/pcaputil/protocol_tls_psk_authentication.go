package pcaputil

const (
	tlsPSKAuthenticationUnconstrained uint8 = iota // no complete selected-PSK observation
	tlsPSKAuthenticationOrdinary
	tlsPSKAuthenticationCertificateExtension // observed RFC 8773 flag; not full negotiation validation
)

// RFC 8446 4.3.2/4.4: ordinary selected PSK replaces initial certificate
// authentication. RFC 8773 is a distinct possible mode, and application-key
// PHA retains its own owner. Missing handshakes, public metadata and port hints
// cannot manufacture the private complete-ServerHello observation below.
func (t *binTLS) checkInitialPSKAuthentication(dir int, kind byte, authenticated bool) error {
	if t.version != 0x304 || !authenticated || t.app[dir] || t.initialPSKAuthentication != tlsPSKAuthenticationOrdinary {
		return nil
	}
	var name string
	switch kind {
	case 11:
		name = "Certificate"
	case 13:
		name = "CertificateRequest"
	case 15:
		name = "CertificateVerify"
	case 25:
		name = "CompressedCertificate"
	default:
		return nil
	}
	return protocolError(ErrMalformedMessage, "TLS 1.3 PSK forbids initial "+name)
}
