package parser

import (
	"encoding/binary"
)

// captureDCERPCStructured decodes the connection-oriented DCE/RPC envelope
// when an authentication verifier is present. The generic YAML rule remains
// the authority for unauthenticated PDUs; here we only need to preserve the
// exact PDU/auth boundary and expose header fields before an opaque stub.
func captureDCERPCStructured(w []byte) (map[string]any, bool) {
	if len(w) < 16 || w[0] != 5 || w[1] != 0 {
		return nil, false
	}
	if w[4] != 0x10 || w[5] != 0 || w[6] != 0 || w[7] != 0 {
		return nil, false
	}
	if w[2] > 19 {
		return nil, false
	}
	fragLen := int(binary.LittleEndian.Uint16(w[8:10]))
	authLen := int(binary.LittleEndian.Uint16(w[10:12]))
	if fragLen != len(w) || fragLen < 16 || authLen <= 0 || len(w) < 16+8+authLen {
		return nil, false
	}

	trailerOffset := len(w) - 8 - authLen
	trailer := w[trailerOffset:]
	padLen := int(trailer[2])
	bodyEnd := trailerOffset
	if padLen > bodyEnd-16 {
		return nil, false
	}
	body := w[16 : bodyEnd-padLen]
	fields := map[string]any{
		"RPC Vers":       uint64(w[0]),
		"RPC Vers Minor": uint64(w[1]),
		"PType":          uint64(w[2]),
		"PFC Flags":      uint64(w[3]),
		"Packed Drep":    append([]byte(nil), w[4:8]...),
		"Frag Length":    uint64(fragLen),
		"Auth Length":    uint64(authLen),
		"Call ID":        uint64(binary.LittleEndian.Uint32(w[12:16])),
	}
	pdu := map[string]any{}
	switch w[2] {
	case 0:
		if len(body) < 8 {
			return nil, false
		}
		pdu["Request"] = map[string]any{
			"Alloc Hint": uint64(binary.LittleEndian.Uint32(body[0:4])),
			"Context ID": uint64(binary.LittleEndian.Uint16(body[4:6])),
			"OpNum":      uint64(binary.LittleEndian.Uint16(body[6:8])),
			"Stub":       append([]byte(nil), body[8:]...),
		}
	case 2:
		if len(body) < 8 {
			return nil, false
		}
		pdu["Response"] = map[string]any{
			"Alloc Hint":   uint64(binary.LittleEndian.Uint32(body[0:4])),
			"Context ID":   uint64(binary.LittleEndian.Uint16(body[4:6])),
			"Cancel Count": uint64(body[6]),
			"Reserved":     uint64(body[7]),
			"Stub":         append([]byte(nil), body[8:]...),
		}
	case 3:
		if len(body) < 16 {
			return nil, false
		}
		pdu["Fault"] = map[string]any{
			"Alloc Hint":   uint64(binary.LittleEndian.Uint32(body[0:4])),
			"Context ID":   uint64(binary.LittleEndian.Uint16(body[4:6])),
			"Cancel Count": uint64(body[6]),
			"Reserved":     uint64(body[7]),
			"Status":       uint64(binary.LittleEndian.Uint32(body[8:12])),
			"Reserved2":    uint64(binary.LittleEndian.Uint32(body[12:16])),
		}
	case 11, 14:
		bind, ok := captureDCERPCBind(body)
		if !ok {
			return nil, false
		}
		pdu["Bind"] = bind
	case 12, 15:
		ack, ok := captureDCERPCBindAck(body)
		if !ok {
			return nil, false
		}
		pdu["BindAck"] = ack
	case 13:
		if len(body) < 3 {
			return nil, false
		}
		pdu["BindNak"] = map[string]any{
			"Reject Reason": uint64(binary.LittleEndian.Uint16(body[0:2])),
			"Num Versions":  uint64(body[2]),
		}
	case 16:
		if len(body) < 4 {
			return nil, false
		}
		pdu["Auth3"] = map[string]any{
			"Max Xmit Frag": uint64(binary.LittleEndian.Uint16(body[0:2])),
			"Max Recv Frag": uint64(binary.LittleEndian.Uint16(body[2:4])),
		}
	default:
		return nil, false
	}

	pdu["Auth Verifier"] = map[string]any{
		"Auth Type":       uint64(trailer[0]),
		"Auth Level":      uint64(trailer[1]),
		"Auth Pad Length": uint64(trailer[2]),
		"Auth Reserved":   uint64(trailer[3]),
		"Auth Context ID": uint64(binary.LittleEndian.Uint32(trailer[4:8])),
		"Auth Value":      append([]byte(nil), trailer[8:]...),
	}
	fields["PDU"] = pdu
	return fields, true
}

func captureDCERPCBind(body []byte) (map[string]any, bool) {
	if len(body) < 12 {
		return nil, false
	}
	count := int(body[8])
	if count > 20 || count > (len(body)-12)/44 {
		return nil, false
	}
	contexts := make([]any, 0, count)
	for i := 0; i < count; i++ {
		off := 12 + i*44
		contexts = append(contexts, map[string]any{
			"Context ID":       uint64(binary.LittleEndian.Uint16(body[off : off+2])),
			"Num Trans Items":  uint64(body[off+2]),
			"Reserved":         uint64(body[off+3]),
			"Abstract Syntax":  append([]byte(nil), body[off+4:off+20]...),
			"Abstract Version": uint64(binary.LittleEndian.Uint32(body[off+20 : off+24])),
			"Transfer Syntax":  append([]byte(nil), body[off+24:off+40]...),
			"Transfer Version": uint64(binary.LittleEndian.Uint32(body[off+40 : off+44])),
		})
	}
	return map[string]any{
		"Max Xmit Frag": uint64(binary.LittleEndian.Uint16(body[0:2])),
		"Max Recv Frag": uint64(binary.LittleEndian.Uint16(body[2:4])),
		"Assoc Group":   uint64(binary.LittleEndian.Uint32(body[4:8])),
		"Num Ctx Items": uint64(body[8]),
		"Reserved":      uint64(body[9]),
		"Reserved2":     uint64(binary.LittleEndian.Uint16(body[10:12])),
		"Contexts":      contexts,
	}, true
}

func captureDCERPCBindAck(body []byte) (map[string]any, bool) {
	if len(body) < 12 {
		return nil, false
	}
	secondaryLen := int(binary.LittleEndian.Uint16(body[8:10]))
	addressEnd := 10 + secondaryLen
	if addressEnd > len(body) {
		return nil, false
	}
	resultOffset := (addressEnd + 3) &^ 3
	if resultOffset+4 > len(body) || body[resultOffset+1] != 0 || body[resultOffset+2] != 0 || body[resultOffset+3] != 0 {
		return nil, false
	}
	count := int(body[resultOffset])
	if count > 20 || len(body) != resultOffset+4+count*24 {
		return nil, false
	}
	results := make([]any, 0, count)
	for off := resultOffset + 4; off < len(body); off += 24 {
		results = append(results, map[string]any{
			"Result":           uint64(binary.LittleEndian.Uint16(body[off : off+2])),
			"Reason":           uint64(binary.LittleEndian.Uint16(body[off+2 : off+4])),
			"Transfer Syntax":  append([]byte(nil), body[off+4:off+20]...),
			"Transfer Version": uint64(binary.LittleEndian.Uint32(body[off+20 : off+24])),
		})
	}
	return map[string]any{
		"Max Xmit Frag": uint64(binary.LittleEndian.Uint16(body[0:2])),
		"Max Recv Frag": uint64(binary.LittleEndian.Uint16(body[2:4])),
		"Assoc Group":   uint64(binary.LittleEndian.Uint32(body[4:8])),
		"Sec Addr Len":  uint64(secondaryLen),
		"Sec Addr":      string(body[10:addressEnd]),
		"Results":       results,
	}, true
}
