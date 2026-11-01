package stream_parser

import (
	"bytes"
	"encoding/base64"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const IMAPSASLMaxBytes = 64 << 10

// These are caller-selected SASL transport encodings, not protocol admission
// signatures. A base64-looking string never establishes an IMAP exchange.
func decodeIMAPSASL(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	if len(wire) > IMAPSASLMaxBytes {
		return nil, nil, fmt.Errorf("imap-sasl: transport value exceeds local byte bound")
	}
	info := map[string]any{"Profile": "IMAP SASL transport value", "Context Supplied By Caller": true, "Authentication Verified": false, "Peer Identity Verified": false, "Security Layer Verified": false, "Decoded Values Are Wire Spans": false}
	start, end := 0, len(wire)
	var fields []tlsCertificateField
	if profile == "response" || profile == "challenge" {
		if !bytes.HasSuffix(wire, []byte("\r\n")) {
			return nil, nil, fmt.Errorf("imap-sasl: a complete CRLF line is required")
		}
		end -= 2
		if profile == "challenge" {
			if end == 1 && wire[0] == '+' {
				start = 1
				info["Conformance Warning"] = "empty challenge omits the formal space after +"
			} else if end >= 2 && wire[0] == '+' && wire[1] == ' ' {
				start = 2
			} else {
				return nil, nil, fmt.Errorf("imap-sasl: challenge requires a + prefix and space")
			}
			fields = append(fields, tlsCertificateField{Name: "Challenge Prefix", Type: "string", Start: 0, End: start})
		}
	} else if profile != "initial-value" {
		return nil, nil, fmt.Errorf("imap-sasl: unknown caller-selected profile")
	}
	value := wire[start:end]
	cancel := profile == "response" && bytes.Equal(value, []byte{'*'})
	emptyInitial := profile == "initial-value" && bytes.Equal(value, []byte{'='})
	if profile == "initial-value" && len(value) == 0 {
		return nil, nil, fmt.Errorf("imap-sasl: empty initial response uses the = marker")
	}
	var decoded []byte
	if !cancel && !emptyInitial {
		// Go's base64 decoder permits line endings; the IMAP value grammar does
		// not. Validate every octet before decoding the bounded value.
		for _, b := range value {
			if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '/' || b == '=') {
				return nil, nil, fmt.Errorf("imap-sasl: invalid base64 alphabet")
			}
		}
		var err error
		decoded, err = base64.StdEncoding.DecodeString(string(value))
		if err != nil {
			return nil, nil, fmt.Errorf("imap-sasl: invalid base64 length or padding")
		}
		info["Base64 Canonical"] = base64.StdEncoding.EncodeToString(decoded) == string(value)
		if info["Base64 Canonical"] == false {
			info["Conformance Warning"] = "base64 has nonzero unused pad bits"
		}
	} else {
		decoded = []byte{}
		info["Base64 Canonical"] = true
	}
	name, typ := "Encoded Value", "raw"
	if cancel {
		name, typ = "Cancellation Marker", "string"
	}
	if emptyInitial {
		name, typ = "Empty Initial Response Marker", "string"
	}
	if end > start {
		fields = append(fields, tlsCertificateField{Name: name, Type: typ, Start: start, End: end})
	}
	if profile != "initial-value" {
		fields = append(fields, tlsCertificateField{Name: "Line Ending", Type: "string", Start: end, End: len(wire)})
	}
	info["Encoded Value"], info["Decoded Payload"] = bytes.Clone(value), bytes.Clone(decoded)
	info["Encoded Length"], info["Decoded Length"] = len(value), len(decoded)
	info["Canceled"], info["Empty Initial Response"] = cancel, emptyInitial
	if end > start {
		info["Wire Field Ranges"] = map[string]any{"Encoded Value": []uint64{uint64(start), uint64(end)}}
	}
	return fields, info, nil
}

func parseIMAPSASL(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits == 0 || bits > IMAPSASLMaxBytes*8 || bits%8 != 0 {
		return fmt.Errorf("imap-sasl: explicit bounded octet input required")
	}
	return parseCertificateFieldTree(node, process, func(wire []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeIMAPSASL(wire, profile)
	}, "imap-sasl")
}

// DecodeIMAPSASLValue validates the same grammar as the explicit bounded rule.
// Native callers must independently own the AUTHENTICATE/challenge context.
func DecodeIMAPSASLValue(wire []byte, profile string) (map[string]any, error) {
	_, info, err := decodeIMAPSASL(wire, profile)
	return info, err
}
