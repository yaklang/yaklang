package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// RFC 5246 4.7, 7.4.3, 7.4.7 and A.7; RFC 8422 5.4/5.7.
// These are caller-selected TLS 1.2 wire layouts, not algorithm detection.
// DH values, EC points, signatures and RSA ciphertext remain opaque bytes.
const tls12KeyExchangeMaxBytes = 4 + 3*(2+65535) + 4 + 65535

type tls12KeyExchangeField struct {
	Name, Type string
	Start, End int
}

func decodeTLS12KeyExchange(wire []byte, profile string) ([]tls12KeyExchangeField, map[string]any, error) {
	fail := func(reason string) ([]tls12KeyExchangeField, map[string]any, error) {
		return nil, nil, fmt.Errorf("tls12-key-exchange: %s", reason)
	}
	var server, ec bool
	switch profile {
	case "ECDHEServer":
		server, ec = true, true
	case "DHEServer":
		server = true
	case "ECDHEClient":
		ec = true
	case "DHEClient", "RSAClient":
	default:
		return fail("unknown caller-selected profile")
	}
	if len(wire) < 4 || len(wire) > tls12KeyExchangeMaxBytes {
		return fail("complete handshake outside 4..262154 byte bound")
	}
	wantType := byte(16)
	if server {
		wantType = 12
	}
	if wire[0] != wantType || int(wire[1])<<16|int(wire[2])<<8|int(wire[3]) != len(wire)-4 {
		return fail("handshake type or exact length differs from selected profile")
	}
	fields := []tls12KeyExchangeField{{"Handshake Type", "uint8", 0, 1}, {"Handshake Length", "uint32", 1, 4}}
	at := 4
	vector := func(name string, width, minimum int) error {
		if len(wire)-at < width {
			return fmt.Errorf("truncated %s length", name)
		}
		n := int(wire[at])
		typ := "uint8"
		if width == 2 {
			n = int(binary.BigEndian.Uint16(wire[at : at+2]))
			typ = "uint16"
		}
		if n < minimum || n > len(wire)-at-width {
			return fmt.Errorf("%s vector exceeds enclosing handshake or has disallowed empty value", name)
		}
		fields = append(fields, tls12KeyExchangeField{name + " Length", typ, at, at + width})
		at += width
		fields = append(fields, tls12KeyExchangeField{name, "raw", at, at + n})
		at += n
		return nil
	}
	if ec {
		if server {
			if len(wire)-at < 3 || wire[at] != 3 {
				return fail("only named-curve ECDHE server parameters are supported")
			}
			fields = append(fields, tls12KeyExchangeField{"Curve Type", "uint8", at, at + 1}, tls12KeyExchangeField{"Named Group", "uint16", at + 1, at + 3})
			at += 3
		}
		name := "Client EC Point"
		if server {
			name = "Server EC Point"
		}
		if err := vector(name, 1, 1); err != nil {
			return fail(err.Error())
		}
	} else if server {
		for _, name := range []string{"DH Modulus", "DH Generator", "Server DH Public Value"} {
			if err := vector(name, 2, 1); err != nil {
				return fail(err.Error())
			}
		}
	} else {
		name, minimum := "Client DH Public Value", 1
		if profile == "RSAClient" {
			name, minimum = "Encrypted Premaster", 0
		}
		if err := vector(name, 2, minimum); err != nil {
			return fail(err.Error())
		}
	}
	if server {
		if len(wire)-at < 2 {
			return fail("truncated signature identifiers")
		}
		fields = append(fields, tls12KeyExchangeField{"Hash Algorithm", "uint8", at, at + 1}, tls12KeyExchangeField{"Signature Algorithm", "uint8", at + 1, at + 2})
		at += 2
		if err := vector("Signature", 2, 0); err != nil {
			return fail(err.Error())
		}
	}
	if at != len(wire) {
		return fail("unconsumed bytes after selected message layout")
	}
	info := map[string]any{
		"Profile": "TLS 1.2 " + profile + " layout", "Explicit Caller Selected Profile": true,
		"Version Is Caller Supplied": true, "Cipher Suite Correlation Validated": false,
		"Sender Role Validated": false, "Sender Conformance Validated": false,
		"Registry Values Validated": false, "Algorithm Usage Validated": false,
		"Group Parameters Validated": false, "Public Value Validated": false,
		"Point Representation Decoded": false, "Signature Verified": false,
		"Premaster Decrypted": false, "Handshake Transcript Validated": false,
		"Peer Identity Validated": false, "Handshake Completion Validated": false,
		"TCP Reassembly Performed": false, "Record Reassembly Performed": false,
		"Structured Generation Supported": false,
	}
	return fields, info, nil
}
