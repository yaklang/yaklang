package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// QUIC carries unprotected TLS handshake bytes in three independent CRYPTO
// streams. Successful packet AEAD does not permit a message at another level.
// RFC9001 4.1.3/4.4/6/8.3; compressed_certificate(25) is a Handshake message
// (RFC8879), even when its body decompression is outside the selected profile.
func quicTLSMessageLevel(typ byte, space int) error {
	want := quicSpaceHandshake
	switch typ {
	case 1, 2: // ClientHello, ServerHello (including HelloRetryRequest)
		want = quicSpaceInitial
	case 4: // NewSessionTicket
		want = quicSpaceApplication
	case 8, 11, 13, 15, 20, 25:
	case 5, 24: // QUIC has no EndOfEarlyData or TLS KeyUpdate.
		return protocolError(ErrMalformedMessage, fmt.Sprintf("TLS handshake type %d is prohibited in QUIC", typ))
	default:
		return protocolError(ErrUnsupportedFeature, fmt.Sprintf("QUIC TLS handshake type %d is outside the selected profile", typ))
	}
	if space != want {
		return protocolError(ErrMalformedMessage, fmt.Sprintf("QUIC TLS handshake type %d is not permitted at %s encryption level", typ, quicSpaceName(space)))
	}
	return nil
}

// RFC9001 4.2 permits future compatible versions but forbids offering or
// negotiating older TLS. Unknown future-only profiles are unsupported, not
// malformed. The legacy Hello word does not select the negotiated version.
func quicTLSHelloVersion(encoded []byte, max int) error {
	if encoded[0] != 1 && encoded[0] != 2 {
		return nil
	}
	client := encoded[0] == 1
	body := encoded[4:]
	hello, err := tlsHello(body, client, max)
	if err != nil {
		return err
	}
	versions := hello.ext[43]
	if client {
		if len(versions) < 3 {
			return protocolError(ErrMalformedMessage, "QUIC ClientHello requires supported_versions")
		}
		for p := 1; p < len(versions); p += 2 {
			if binary.BigEndian.Uint16(versions[p:]) < 0x304 {
				return protocolError(ErrMalformedMessage, "QUIC ClientHello offers TLS older than 1.3")
			}
		}
		if !tlsContains16(versions[1:], 0x304) {
			return protocolError(ErrUnsupportedVersion, "QUIC ClientHello has no supported TLS 1.3 profile")
		}
	} else {
		version := binary.BigEndian.Uint16(body)
		if len(versions) == 2 {
			version = binary.BigEndian.Uint16(versions)
		}
		if version < 0x304 {
			return protocolError(ErrMalformedMessage, "QUIC ServerHello selects TLS older than 1.3")
		}
		if version != 0x304 || len(versions) != 2 {
			return protocolError(ErrUnsupportedVersion, "QUIC ServerHello selects an unsupported TLS profile")
		}
	}
	if binary.BigEndian.Uint16(body) != 0x303 || !bytes.Equal(hello.compression, []byte{0}) {
		return protocolError(ErrMalformedMessage, "QUIC TLS 1.3 Hello legacy parameters")
	}
	return nil
}

func quicTLSTicketValues(message map[string]any) error {
	ticket, ok := message["New Session Ticket"].(map[string]any)
	if !ok {
		return nil
	}
	extensions, _ := ticket["Extensions"].([]map[string]any)
	for _, extension := range extensions {
		if extension["Type"] == uint16(42) && extension["Maximum Early Data Size"] != uint32(0xffffffff) {
			return protocolError(ErrMalformedMessage, "QUIC NewSessionTicket early_data maximum must be 0xffffffff")
		}
	}
	return nil
}
