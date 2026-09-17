package pcaputil

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// binDCERPC is the M0 session state for connection-oriented DCE/RPC v5.
// Stub decode stays separate from generic Bind/Request/Call ID session
// state. Ports including 135 are never consulted.
type binDCERPC struct {
	client  int
	ctx     map[uint16]string
	uuid    map[uint16][]byte
	pending map[uint32]string
	frags   map[uint32][]byte
}

const (
	dcerpcFirst  = 0x01
	dcerpcLast   = 0x02
	dcerpcObject = 0x80
)

var (
	dcerpcEPM    = []byte{0x08, 0x83, 0xaf, 0xe1, 0x1f, 0x5d, 0xc9, 0x11, 0x91, 0xa4, 0x08, 0x00, 0x2b, 0x14, 0xa0, 0xfa}
	dcerpcSRVSVC = []byte{0xc8, 0x4f, 0x32, 0x4b, 0x70, 0x16, 0xd3, 0x01, 0x12, 0x78, 0x5a, 0x47, 0xbf, 0x6e, 0xe1, 0x88}
	dcerpcNDR    = []byte{0x04, 0x5d, 0x88, 0x8a, 0xeb, 0x1c, 0xc9, 0x11, 0x9f, 0xe8, 0x08, 0x00, 0x2b, 0x10, 0x48, 0x60}
)

func probeDCERPC(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 16 {
		return probeNeed("dcerpc", "5.0", len(w), 16)
	}
	if w[1] != 0 || w[2] > 19 {
		return ProbeResult{Verdict: ProbeReject}
	}
	switch w[2] {
	case 0, 2, 3, 11, 12, 13, 14, 15:
	default:
		return ProbeResult{Verdict: ProbeReject}
	}
	n := dcerpcFragLen(w)
	if n < 16 || n > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("dcerpc", "5.0", 92)
}

func dcerpcFragLen(w []byte) int {
	if len(w) < 10 {
		return 0
	}
	if w[4]&0x10 != 0 {
		return int(binary.LittleEndian.Uint16(w[8:10]))
	}
	return int(binary.BigEndian.Uint16(w[8:10]))
}

func (f *binFlow) frameDCERPC(w []byte) (int, *binSpec, error) {
	d := f.dcerpc
	if d == nil {
		return 0, nil, sessionContext("DCE/RPC session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(d.pending)+len(d.frags)+len(d.ctx))*24); err != nil {
		return 0, nil, err
	}
	if len(w) < 16 {
		if len(w) > 0 && w[0] == 5 {
			return 0, nil, nil
		}
		return 0, nil, fmt.Errorf("dcerpc: truncated header")
	}
	if w[0] != 5 {
		return 0, nil, fmt.Errorf("dcerpc: RPC vers must be 5")
	}
	if w[1] != 0 {
		return 0, nil, protocolError(ErrUnsupportedVersion, "DCE/RPC connection-oriented session requires vers minor 0")
	}
	if w[2] > 19 {
		return 0, nil, fmt.Errorf("dcerpc: unknown ptype %d", w[2])
	}
	n := dcerpcFragLen(w)
	if n < 16 {
		return 0, nil, fmt.Errorf("dcerpc: frag length smaller than header")
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("dcerpc", "DCERPC"), nil
}

func (d *binDCERPC) consume(raw []byte, max int) (map[string]any, error) {
	if len(raw) < 16 {
		return nil, fmt.Errorf("dcerpc: truncated header")
	}
	n := dcerpcFragLen(raw)
	if n != len(raw) {
		return nil, fmt.Errorf("dcerpc: frag length disagrees with framed message")
	}
	ptype := raw[2]
	flags := raw[3]
	call := binary.LittleEndian.Uint32(raw[12:16])
	body := raw[16:]
	info := map[string]any{
		"PType":         ptype,
		"Packet Name":   dcerpcPTypeName(ptype),
		"PFC Flags":     flags,
		"Call ID":       call,
		"Context Level": "observed",
		"First Frag":    flags&dcerpcFirst != 0,
		"Last Frag":     flags&dcerpcLast != 0,
	}
	if flags&dcerpcFirst == 0 {
		d.frags[call] = append(d.frags[call], body...)
		info["Fragment"] = true
		if flags&dcerpcLast == 0 {
			info["Outstanding"] = true
			return info, nil
		}
		body = d.frags[call]
		delete(d.frags, call)
		info["Reassembled"] = true
	} else if flags&dcerpcLast == 0 {
		d.frags[call] = append([]byte(nil), body...)
		info["Fragment"] = true
		info["Outstanding"] = true
		if ptype == 0 && len(body) >= 8 {
			info["Context ID"] = binary.LittleEndian.Uint16(body[4:6])
			info["OpNum"] = binary.LittleEndian.Uint16(body[6:8])
		}
		return info, nil
	}
	switch ptype {
	case 11, 14:
		if err := d.parseBind(body, info, max); err != nil {
			return nil, err
		}
		if err := d.push(call, dcerpcPTypeName(ptype), max); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 12:
		d.match(call, "BindAck", info)
		if len(body) >= 8 {
			info["Max Xmit Frag"] = binary.LittleEndian.Uint16(body[0:2])
			info["Assoc Group"] = binary.LittleEndian.Uint32(body[4:8])
		}
	case 13:
		d.match(call, "BindNak", info)
		if len(body) >= 2 {
			info["Reject Reason"] = binary.LittleEndian.Uint16(body[:2])
		}
	case 0:
		if err := d.parseRequest(body, flags, info); err != nil {
			return nil, err
		}
		if err := d.push(call, fmt.Sprint(info["Packet Name"]), max); err != nil {
			return nil, err
		}
		info["Outstanding"] = true
	case 2:
		d.match(call, "Response", info)
		if len(body) >= 6 {
			info["Context ID"] = binary.LittleEndian.Uint16(body[4:6])
		}
	case 3:
		d.match(call, "Fault", info)
		if len(body) >= 12 {
			info["Context ID"] = binary.LittleEndian.Uint16(body[4:6])
			info["Status"] = binary.LittleEndian.Uint32(body[8:12])
		}
	}
	return info, nil
}

func (d *binDCERPC) parseBind(body []byte, info map[string]any, max int) error {
	if len(body) < 12 {
		return fmt.Errorf("dcerpc: truncated bind")
	}
	n := int(body[8])
	if n > 20 {
		return fmt.Errorf("dcerpc: too many presentation contexts")
	}
	if max <= 0 {
		max = 4096
	}
	var names []string
	off := 12
	for i := 0; i < n; i++ {
		if off+44 > len(body) {
			return fmt.Errorf("dcerpc: truncated presentation context")
		}
		id := binary.LittleEndian.Uint16(body[off : off+2])
		uuid := append([]byte(nil), body[off+4:off+20]...)
		name := dcerpcIfaceName(uuid)
		if len(d.ctx) >= max {
			return protocolError(ErrResourceExceeded, "DCE/RPC context count exceeds budget")
		}
		d.ctx[id] = name
		d.uuid[id] = uuid
		names = append(names, name)
		info["Context ID"] = id
		info["Interface"] = name
		info["Abstract Syntax"] = hex.EncodeToString(uuid)
		off += 44
	}
	if len(names) > 1 {
		info["Interfaces"] = names
	}
	return nil
}

func (d *binDCERPC) parseRequest(body []byte, flags byte, info map[string]any) error {
	need := 8
	if flags&dcerpcObject != 0 {
		need += 16
	}
	if len(body) < need {
		return fmt.Errorf("dcerpc: truncated request")
	}
	ctx := binary.LittleEndian.Uint16(body[4:6])
	op := binary.LittleEndian.Uint16(body[6:8])
	info["Context ID"] = ctx
	info["OpNum"] = op
	if name, ok := d.ctx[ctx]; ok {
		info["Interface"] = name
		info["Packet Name"] = dcerpcOpName(name, op)
		if u := d.uuid[ctx]; len(u) != 0 {
			info["Abstract Syntax"] = hex.EncodeToString(u)
		}
	} else {
		info["Context Level"] = "partial"
	}
	stub := body[need:]
	info["Stub Bytes"] = len(stub)
	return nil
}

func (d *binDCERPC) push(call uint32, name string, max int) error {
	if max <= 0 {
		max = 4096
	}
	if len(d.pending) >= max {
		return protocolError(ErrResourceExceeded, "DCE/RPC outstanding calls exceed budget")
	}
	d.pending[call] = name
	return nil
}

func (d *binDCERPC) match(call uint32, respName string, info map[string]any) {
	if want, ok := d.pending[call]; ok {
		delete(d.pending, call)
		info["Matched Request"] = want
		info["Association Status"] = "matched"
		info["Packet Name"] = respName
		return
	}
	info["Unmatched"] = true
	info["Association Status"] = "missing-request"
	info["Context Level"] = "partial"
	info["Packet Name"] = respName
}

func dcerpcPTypeName(p byte) string {
	switch p {
	case 0:
		return "Request"
	case 2:
		return "Response"
	case 3:
		return "Fault"
	case 11:
		return "Bind"
	case 12:
		return "BindAck"
	case 13:
		return "BindNak"
	case 14:
		return "AlterContext"
	case 15:
		return "AlterContextResp"
	}
	return fmt.Sprintf("PType %d", p)
}

func dcerpcIfaceName(uuid []byte) string {
	switch {
	case bytes.Equal(uuid, dcerpcEPM):
		return "EPM"
	case bytes.Equal(uuid, dcerpcSRVSVC):
		return "SRVSVC"
	default:
		return "unknown"
	}
}

func dcerpcOpName(iface string, op uint16) string {
	switch iface {
	case "EPM":
		if op == 3 {
			return "EPM.ept_map"
		}
	case "SRVSVC":
		if op == 15 {
			return "SRVSVC.NetrShareEnum"
		}
	}
	return fmt.Sprintf("%s.op_%d", iface, op)
}
