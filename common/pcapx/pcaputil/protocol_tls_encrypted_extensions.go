package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Qualify this message before the shared extension reader can change selected
// version or ALPN. An AEAD tag authenticates bytes, not handshake conformance.
func (t *binTLS) admitEncryptedExtensions(dir int, body []byte, limits ...int) error {
	if t.version == 0 {
		return protocolError(ErrContextRequired, "TLS EncryptedExtensions needs an observed TLS 1.3 version")
	}
	if t.version != 0x304 {
		return protocolError(ErrMalformedMessage, "TLS EncryptedExtensions requires TLS 1.3")
	}
	if t.client >= 0 && dir == t.client {
		return protocolError(ErrMalformedMessage, "client sent TLS EncryptedExtensions")
	}
	if t.app[dir] {
		return protocolError(ErrMalformedMessage, "TLS EncryptedExtensions in application epoch")
	}
	if len(body) < 2 || int(binary.BigEndian.Uint16(body)) != len(body)-2 {
		return protocolError(ErrMalformedMessage, "TLS extensions length")
	}
	max := 4096
	if len(limits) > 0 {
		max = limits[0]
	}
	var seen [1024]uint64
	count := 0
	var alpn []byte
	var grease uint16
	hasGREASE := false
	for rest := body[2:]; len(rest) > 0; {
		if len(rest) < 4 {
			return protocolError(ErrMalformedMessage, "TLS extension header")
		}
		typ, n := binary.BigEndian.Uint16(rest), int(binary.BigEndian.Uint16(rest[2:]))
		rest = rest[4:]
		word, bit := typ/64, uint64(1)<<(typ%64)
		if n > len(rest) || seen[word]&bit != 0 {
			return protocolError(ErrMalformedMessage, "TLS duplicate or truncated extension")
		}
		seen[word] |= bit
		value := rest[:n]
		rest = rest[n:]
		count++
		if tlsGREASE16(typ) {
			hasGREASE, grease = true, typ
		}
		// RFC 8446 4.2 and RFC 8773 5 place these in CH/SH/HRR/CR,
		// never EE. Unknown extensions remain opaque rather than inventing
		// a location rule for future registered extensions.
		switch typ {
		case 5, 13, 18, 21, 33, 41, 43, 44, 45, 47, 48, 50, 51:
			return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS extension %d is not permitted in EncryptedExtensions", typ))
		case 49:
			return protocolError(ErrMalformedMessage, "TLS post_handshake_auth must be an empty ClientHello extension")
		case 42:
			if len(value) != 0 {
				return protocolError(ErrMalformedMessage, "TLS EncryptedExtensions early_data must be empty")
			}
			if err := t.admitEarlyDataAcceptance(); err != nil {
				return err
			}
		case 16:
			selected, err := tlsALPNName(value, true, limits...)
			if err != nil {
				return err
			}
			alpn = selected
		}
	}
	if hasGREASE {
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS EncryptedExtensions selects reserved GREASE extension 0x%04x", grease))
	}
	if tlsGREASEALPN(alpn) {
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS EncryptedExtensions selects reserved GREASE ALPN 0x%04x", binary.BigEndian.Uint16(alpn)))
	}
	if count > max {
		return protocolError(ErrResourceExceeded, "TLS EncryptedExtensions collection limit")
	}
	if alpn != nil && t.alpnOfferKnown {
		// Retained bytes were admitted as a complete ClientHello. Never infer
		// an empty offer from a missing ClientHello or borrow another flow's.
		found := false
		if len(t.alpnOffer) >= 2 {
			for rest := t.alpnOffer[2:]; len(rest) > 0; {
				n := int(rest[0])
				rest = rest[1:]
				if n > len(rest) { // Defensive if an internal caller supplied unqualified state.
					return protocolError(ErrContextRequired, "TLS retained ALPN offer is incomplete")
				}
				found = found || bytes.Equal(rest[:n], alpn)
				rest = rest[n:]
			}
		}
		if !found {
			return protocolError(ErrMalformedMessage, "TLS EncryptedExtensions ALPN was not offered")
		}
	}
	return nil
}
