package stream_parser

import (
	"encoding/binary"
	"fmt"
)

// RFC 5246 sections 7.4/7.4.5; RFC 5077 section 3.3. These messages
// do not carry a protocol version. The caller supplies TLS 1.2 context;
// this decoder does not establish direction, phase, or ticket validity.
const tls12ControlMaxBytes = 4 + 4 + 2 + 65535

// Resource bound for the explicit RFC 6066 OCSP-vector profile. This is smaller
// than the wire's uint24 maximum; exceeding it is not a malformed DER verdict.
const TLS12CertificateStatusMaxBytes = 1 << 20

type TLS12CertificateStatusField = tls12ControlField

// DecodeTLS12CertificateStatusLayout shares the canonical grammar with the YAML
// entry. OCSP response bytes remain opaque; no DER, revocation or trust verdict.
func DecodeTLS12CertificateStatusLayout(wire []byte) ([]TLS12CertificateStatusField, map[string]any, error) {
	return decodeTLS12ControlHandshake(wire, 22)
}

func tls12ControlLimit(messageType uint8) int {
	if messageType == 22 {
		return TLS12CertificateStatusMaxBytes
	}
	return tls12ControlMaxBytes
}

type tls12ControlField struct {
	Name, Type string
	Start, End int
}

func decodeTLS12ControlHandshake(wire []byte, messageType uint8) ([]tls12ControlField, map[string]any, error) {
	fail := func(reason string) ([]tls12ControlField, map[string]any, error) {
		return nil, nil, fmt.Errorf("tls12-control: %s", reason)
	}
	if messageType != 4 && messageType != 14 && messageType != 22 {
		return fail("unsupported handshake profile")
	}
	if len(wire) < 4 || len(wire) > tls12ControlLimit(messageType) {
		return fail("complete bounded handshake required")
	}
	if wire[0] != messageType {
		return fail("unexpected handshake type")
	}
	bodyLength := int(wire[1])<<16 | int(wire[2])<<8 | int(wire[3])
	if bodyLength != len(wire)-4 {
		return fail("handshake length does not exactly consume input")
	}
	fields := []tls12ControlField{{"Handshake Type", "uint8", 0, 1}, {"Handshake Length", "uint32", 1, 4}}
	info := map[string]any{
		"Protocol Profile":                "caller-selected TLS 1.2 complete handshake",
		"Protocol Version Inferred":       false,
		"Direction Validated":             false,
		"Handshake Phase Validated":       false,
		"Handshake Completion Validated":  false,
		"TCP Reassembly Performed":        false,
		"Structured Generation Supported": false,
	}
	if messageType == 22 {
		if bodyLength < 5 || wire[4] != 1 {
			return fail("CertificateStatus requires a complete nonempty OCSP response vector")
		}
		n := int(wire[5])<<16 | int(wire[6])<<8 | int(wire[7])
		if n == 0 || n != len(wire)-8 {
			return fail("OCSP response length does not exactly consume body")
		}
		fields = append(fields, tls12ControlField{"Status Type", "uint8", 4, 5},
			tls12ControlField{"OCSP Response Length", "uint32", 5, 8},
			tls12ControlField{"OCSP Response", "raw", 8, len(wire)})
		info["Handshake Message"] = "CertificateStatus"
		info["OCSP DER Decoded"], info["OCSP Response Validated"] = false, false
		info["Certificate Trust Evaluated"] = false
		return fields, info, nil
	}
	if messageType == 14 {
		if bodyLength != 0 {
			return fail("ServerHelloDone body must be empty")
		}
		info["Handshake Message"] = "ServerHelloDone"
		return fields, info, nil
	}
	if bodyLength < 6 {
		return fail("NewSessionTicket fixed body is truncated")
	}
	if int(binary.BigEndian.Uint16(wire[8:10])) != len(wire)-10 {
		return fail("ticket length does not exactly consume body")
	}
	fields = append(fields,
		tls12ControlField{"Ticket Lifetime Hint", "uint32", 4, 8},
		tls12ControlField{"Ticket Length", "uint16", 8, 10},
		tls12ControlField{"Ticket", "raw", 10, len(wire)},
	)
	info["Handshake Message"] = "NewSessionTicket"
	info["Lifetime Hint Unit"] = "seconds relative to receipt"
	info["Lifetime Hint Unspecified"] = binary.BigEndian.Uint32(wire[4:8]) == 0
	info["Lifetime Hint Is Verified Expiry"] = false
	info["Ticket Contents Decoded"] = false
	info["Ticket Validated"] = false
	info["Session Resumption Validated"] = false
	return fields, info, nil
}
