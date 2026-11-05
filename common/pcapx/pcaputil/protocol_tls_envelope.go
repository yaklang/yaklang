package pcaputil

import "encoding/binary"

// TLSCiphertextRefusal retains observed negotiation separately from a rejected
// record. It grants neither plaintext nor successful field-query admission.
type TLSCiphertextRefusal struct {
	Version uint16
	Error   string
	Fields  map[string]any
}

// NativeTLSCiphertextRefusal cannot be created by a public event, JSON DTO or
// edited status/session map. The private query refusal and negotiation snapshot
// survive together; byte highlighting must still validate the current source.
func (e *ProtocolEvent) NativeTLSCiphertextRefusal() (TLSCiphertextRefusal, bool) {
	if e == nil || !e.nativeTLSObservation || e.sessionError == nil || e.sessionError.Kind != ErrMalformedMessage || e.semanticFields == nil {
		return TLSCiphertextRefusal{}, false
	}
	version, ok := e.semanticFields["Negotiated Version"].(uint16)
	if !ok || version != 0x303 && version != 0x304 || e.semanticFields["Authentication Verified"] != false || e.semanticFields["Content Visibility"] != "encrypted" {
		return TLSCiphertextRefusal{}, false
	}
	switch e.sessionError.Message {
	case "TLS ciphertext shorter than negotiated AEAD envelope", "TLS ciphertext exceeds negotiated AEAD plaintext limit":
		suite, ok := e.semanticFields["Cipher Suite"].(uint16)
		if !ok || !tlsSupportedSuite(version, suite) {
			return TLSCiphertextRefusal{}, false
		}
	case "TLS 1.3 protected record legacy version must be 0x0303":
		if version != 0x304 {
			return TLSCiphertextRefusal{}, false
		}
	default:
		return TLSCiphertextRefusal{}, false
	}
	return TLSCiphertextRefusal{Version: version, Error: e.sessionError.Error(), Fields: cloneSession(e.semanticFields)}, true
}

// Check only structure fixed by the privately observed negotiation. An opaque
// envelope that passes this check is not authenticated application content.
func (t *binTLS) checkCiphertextEnvelope(w []byte) error {
	if t.version == 0x304 && binary.BigEndian.Uint16(w[1:3]) != 0x303 {
		return protocolError(ErrMalformedMessage, "TLS 1.3 protected record legacy version must be 0x0303")
	}
	if !tlsSupportedSuite(t.version, t.suite) {
		return nil
	}
	// RFC 8446 5.2: a protected TLS 1.3 record includes at least the
	// inner content type and the 16-byte tag of each supported AEAD.
	minimum := 17
	if t.version == 0x303 {
		// RFC 5288 3: AES-GCM carries an 8-byte explicit nonce and a
		// 16-byte tag. RFC 7905 ChaCha20-Poly1305 omits that nonce.
		minimum = 24
		if t.suite == 0xcca8 || t.suite == 0xcca9 || t.suite == 0xccaa {
			minimum = 16
		}
	}
	if len(w)-5 < minimum {
		return protocolError(ErrMalformedMessage, "TLS ciphertext shorter than negotiated AEAD envelope")
	}
	// The supported AEAD profiles all append a 16-byte tag. TLS1.2 uses
	// null compression; TLS1.3's bound includes content type and all padding.
	// RFC5246 6.2.1 and RFC8446 5.4 limit these plaintexts, even when a
	// larger ciphertext would fit the general record-layer expansion bound.
	if len(w)-5 > 16384+minimum {
		return protocolError(ErrMalformedMessage, "TLS ciphertext exceeds negotiated AEAD plaintext limit")
	}
	return nil
}
