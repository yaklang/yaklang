package stream_parser

import (
	"fmt"
	"strconv"
	"strings"
)

type byteFieldsDecoder func([]byte) ([]tlsCertificateField, map[string]any, error)

// These helpers share parseExactByteFieldTreeWithEndian's contract: one bounded
// message, no reads of caller/session state, and a privately staged field tree.
// Keep adapters here, beside that contract; arbitrary native calls are NOT pure
// decoders. The selected profile/direction/version remains a caller decision.
func structuredNativeDecoder(source string) StructuredDecoder {
	const suffix = ")\nif err != nil { panic(err) }\n"
	if !strings.HasPrefix(source, "err = ") || !strings.HasSuffix(source, suffix) {
		return nil
	}
	name, arg, ok := strings.Cut(strings.TrimSuffix(strings.TrimPrefix(source, "err = "), suffix), "(")
	if !ok {
		return nil
	}
	var decode byteFieldsDecoder
	endian := "big"
	profile, stringErr := strconv.Unquote(arg)
	// A canonical literal is the complete argument, never an expression.
	if stringErr == nil && strconv.Quote(profile) == arg {
		var f func([]byte, string) ([]tlsCertificateField, map[string]any, error)
		switch name {
		case "parseTNSFields":
			f = decodeTNSFields
		case "parseTDSFields":
			f = decodeTDSFields
		case "parseLDAPFields":
			f = decodeLDAPFields
		case "parseMySQLFields":
			f, endian = decodeMySQLFields, "little"
		case "parsePostgreSQLFields":
			f = decodePostgreSQLFields
		case "parseIMAPFields":
			f = decodeIMAPFields
		case "parsePOP3Fields":
			f = decodePOP3Fields
		}
		if f != nil {
			decode = func(w []byte) ([]tlsCertificateField, map[string]any, error) { return f(w, profile) }
		}
	} else {
		switch name {
		case "parseMQTTFields":
			if arg == "3" || arg == "4" {
				level := int(arg[0] - '0')
				decode = func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodeMQTTFields(w, level) }
			}
		case "parseKerberosFields":
			if arg == "true" || arg == "false" {
				tcp := arg == "true"
				decode = func(w []byte) ([]tlsCertificateField, map[string]any, error) { return decodeKerberosFields(w, tcp) }
			}
		case "parseSMTPFields":
			if arg == "true" {
				decode = decodeSMTPDataFields
			} else if arg == "false" {
				decode = decodeSMTPCommandFields
			}
		case "parseSMB3TransformFields":
			if arg == "" {
				decode, endian = decodeSMB3TransformFields, "little"
			}
		case "parseTLSCertificateHandshake":
			if arg == "" {
				decode = decodeTLSCertificateHandshake
			}
		}
	}
	if decode == nil {
		return nil
	}
	return func(wire []byte) (any, map[string]any, error) {
		// This is the common Node wrapper's bound, in addition to each decoder's
		// own stricter profile limits. Check it before any allocation or decode.
		if len(wire) == 0 || len(wire) > tlsCertificateMaxBytes {
			return nil, nil, fmt.Errorf("structured decode: explicit 1..1048576 byte boundary required")
		}
		fields, metadata, err := decode(wire)
		if err != nil {
			return nil, nil, err
		}
		// An invocation-local byte arena needs no global pool or lock. Its output
		// storage belongs to the result; independent values have capped slices.
		arena := &fieldArena{outputReserve: min(4096, 2*len(wire))}
		value, err := projectStructuredFieldsWithArena(wire, fields, false, endian, arena)
		return value, metadata, err
	}
}
