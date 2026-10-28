package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// DecodeESPHeaderWire observes the RFC4303 clear header only. The remainder
// can include an explicit IV, ciphertext, padding and an ICV; without an SA its
// boundaries and algorithms are unknown. This is not an ESP integrity check.
// maxBytes is checked before cloning any opaque body.
func DecodeESPHeaderWire(w []byte, maxBytes int) (map[string]any, error) {
	if maxBytes <= 0 || len(w) > maxBytes {
		return nil, ErrObservedWireBudget
	}
	ranges := map[string]any{}
	m := map[string]any{
		"Header Complete":                 len(w) >= 8,
		"Algorithm Context Available":     false,
		"Authentication Verified":         false,
		"Extended Sequence Reconstructed": false,
		"Observation Scope":               "clear ESP header and opaque remainder; no SA, algorithm, peer authentication or replay verdict",
		"Wire Field Ranges":               ranges,
	}
	if len(w) >= 4 {
		m["SPI"] = binary.BigEndian.Uint32(w[:4])
		ranges["SPI"] = []any{0, 4}
	}
	if len(w) < 8 {
		return m, fmt.Errorf("esp: clear header requires 8 bytes, got %d", len(w))
	}
	m["Sequence"] = binary.BigEndian.Uint32(w[4:8])
	ranges["Sequence"] = []any{4, 8}
	m["Opaque Body"] = bytes.Clone(w[8:])
	ranges["Opaque Body"] = []any{8, len(w)}
	if m["SPI"] == uint32(0) {
		return m, fmt.Errorf("esp: SPI zero is reserved")
	}
	// A zero low-order sequence number alone cannot diagnose replay or wrap:
	// the high ESN bits and negotiated anti-replay policy are not on this wire.
	return m, nil
}
