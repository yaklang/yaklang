package pcaputil

import (
	"encoding/binary"
	"errors"
	"strings"
)

func dlmsSNReadAPDU(p []byte) bool { return len(p) > 0 && (p[0] == 5 || p[0] == 12) }

// Reuse the bounded SN A-XDR cursor without projecting synthetic Wrapper
// carrier fields. The enclosing CRC-checked HDLC frame and LLC remain literal.
func decodeDLMSSNRead(m *dlmsMessage, p []byte, llc byte, limit, depth int) (*dlmsMessage, error) {
	request := p[0] == 5
	if request && llc != 0xe6 || !request && llc != 0xe7 {
		return nil, dlmsError(ErrMalformedMessage, "SN Read LLC direction differs from command")
	}
	if depth < 4 {
		return nil, dlmsError(ErrResourceExceeded, "SN Read projection exceeds recursion budget")
	}
	wire := make([]byte, 8+len(p))
	wire[1] = 1
	binary.BigEndian.PutUint16(wire[6:8], uint16(len(p)))
	copy(wire[8:], p)
	var sn *wrapperMessage
	var err error
	if wrapperIsSNServiceBlock(wire) {
		sn, err = decodeWrapperSNServiceBlock(wire, limit)
	} else {
		sn, err = decodeWrapperSNRead(wire, limit, depth)
	}
	if err != nil {
		var pe *ProtocolError
		if errors.As(err, &pe) {
			return nil, dlmsError(pe.Kind, "SN Read: "+strings.TrimPrefix(pe.Message, "DLMS Wrapper "))
		}
		return nil, err
	}
	for _, key := range []string{"wrapper_version", "source_wport", "destination_wport", "wrapper_header_hex"} {
		delete(sn.fields, key)
	}
	m.snRead, m.request, m.kind = true, request, "SN-READ"
	if sn.snServiceBlock {
		m.snBlock = sn
	}
	m.fields["Frame Kind"] = "SN Read Response"
	if request {
		m.fields["Frame Kind"] = "SN Read Request"
	}
	m.fields["SN Read"] = sn.fields
	return m, nil
}
