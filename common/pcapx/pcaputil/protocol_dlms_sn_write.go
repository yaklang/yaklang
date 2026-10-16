package pcaputil

import (
	"encoding/binary"
	"errors"
	"strings"
)

func dlmsSNWriteAPDU(p []byte) bool { return len(p) > 0 && (p[0] == 6 || p[0] == 13) }

// Reuse the bounded SN A-XDR cursor without projecting synthetic Wrapper
// carrier fields. The enclosing CRC-checked HDLC frame and LLC remain literal.
func decodeDLMSSNWrite(m *dlmsMessage, p []byte, llc byte, limit, depth int) (*dlmsMessage, error) {
	request := p[0] == 6
	if request && llc != 0xe6 || !request && llc != 0xe7 {
		return nil, dlmsError(ErrMalformedMessage, "SN Write LLC direction differs from command")
	}
	if depth < 4 {
		return nil, dlmsError(ErrResourceExceeded, "SN Write projection exceeds recursion budget")
	}
	wire := make([]byte, 8+len(p))
	wire[1] = 1
	binary.BigEndian.PutUint16(wire[6:8], uint16(len(p)))
	copy(wire[8:], p)
	sn, err := decodeWrapperSNWrite(wire, limit, depth)
	if err != nil {
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return nil, dlmsError(pe.Kind, "SN Write: "+strings.TrimPrefix(pe.Message, "DLMS Wrapper "))
		}
		return nil, err
	}
	for _, key := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex"} {
		delete(sn.fields, key)
	}
	m.snWrite, m.request, m.kind = true, request, "SN-WRITE"
	m.fields["Frame Kind"] = "SN Write Response"
	if request {
		m.fields["Frame Kind"] = "SN Write Request"
	}
	m.fields["SN Write"] = sn.fields
	return m, nil
}
