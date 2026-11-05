package stream_parser

import "fmt"

type TLSEndOfEarlyDataField = tls12ControlField

// DecodeTLSEndOfEarlyDataLayout decodes exactly one caller-selected plaintext
// message (RFC8446 4.5). Syntax does not establish direction, early-key record
// authentication, server acceptance, or a transition in a captured connection.
func DecodeTLSEndOfEarlyDataLayout(wire []byte) ([]TLSEndOfEarlyDataField, map[string]any, error) {
	if len(wire) != 4 || wire[0] != 5 || wire[1] != 0 || wire[2] != 0 || wire[3] != 0 {
		return nil, nil, fmt.Errorf("tls-end-of-early-data: exactly one empty type 5 handshake required")
	}
	return []TLSEndOfEarlyDataField{{"Handshake Type", "uint8", 0, 1}, {"Handshake Length", "uint32", 1, 4}}, map[string]any{
		"Protocol Profile":                "caller-selected TLS 1.3 EndOfEarlyData plaintext",
		"Handshake Message":               "EndOfEarlyData",
		"Record Authentication Verified":  false,
		"Early Data Acceptance Validated": false,
		"Direction Validated":             false,
		"Handshake Phase Validated":       false,
		"Handshake Completion Validated":  false,
		"Structured Generation Supported": false,
	}, nil
}
