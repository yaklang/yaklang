package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/netip"
)

type diameterKey struct {
	dir int
	id  uint32
}
type diameterRequest struct {
	command, app, end uint32
	hash              [32]byte
}
type binDiameter struct {
	pending map[diameterKey]diameterRequest
}

func diameter24(w []byte) uint32 { return uint32(w[0])<<16 | uint32(w[1])<<8 | uint32(w[2]) }
func probeDiameter(w []byte, _ int) ProbeResult {
	if len(w) == 0 || w[0] != 1 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) >= 2 && w[1] > 16 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) >= 4 && (diameter24(w[1:]) < 20 || diameter24(w[1:])%4 != 0) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) >= 5 && w[4]&15 != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 20 {
		return probeNeed("diameter", "1", len(w), 20)
	}
	n, c := diameter24(w[1:]), diameter24(w[5:])
	if n < 20 || n%4 != 0 || w[4]&15 != 0 || c == 0 || c > 0xffffff {
		return ProbeResult{Verdict: ProbeReject}
	}
	// Admit known base and credit-control commands, not every length-prefixed binary stream.
	switch c {
	case 257, 258, 271, 272, 274, 275, 280, 282:
		return probeAccept("diameter", "1", 95)
	}
	return ProbeResult{Verdict: ProbeReject}
}
func (f *binFlow) frameDiameter(w []byte) (int, *binSpec, error) {
	if err := f.reserveSession(256 + int64(len(f.diameter.pending)+1)*128); err != nil {
		return 0, nil, err
	}
	if len(w) < 4 {
		return 0, nil, nil
	}
	n := int(diameter24(w[1:]))
	if w[0] != 1 || n < 20 || n%4 != 0 {
		return 0, nil, fmt.Errorf("diameter: version/length")
	}
	return n, f.spec("extended_protocols", "Diameter"), nil
}

var diameterNames = map[uint32]string{1: "User-Name", 257: "Host-IP-Address", 258: "Auth-Application-Id", 259: "Acct-Application-Id", 260: "Vendor-Specific-Application-Id", 263: "Session-Id", 264: "Origin-Host", 266: "Vendor-Id", 268: "Result-Code", 269: "Product-Name", 273: "Disconnect-Cause", 279: "Failed-AVP", 283: "Destination-Realm", 293: "Destination-Host", 296: "Origin-Realm", 415: "CC-Request-Number", 416: "CC-Request-Type", 443: "Subscription-Id"}

func diameterAVPs(w []byte, max, depth int, total *int) ([]map[string]any, error) {
	if depth <= 0 {
		return nil, protocolError(ErrResourceExceeded, "Diameter AVP nesting")
	}
	out := []map[string]any{}
	for len(w) > 0 {
		if *total >= max {
			return nil, protocolError(ErrResourceExceeded, "Diameter AVP count")
		}
		*total++
		if len(w) < 8 {
			return nil, fmt.Errorf("diameter: truncated AVP header")
		}
		code := binary.BigEndian.Uint32(w)
		flags := w[4]
		n := int(diameter24(w[5:]))
		h := 8
		vendor := uint32(0)
		if flags&0x80 != 0 {
			h = 12
		}
		if flags&0x1f != 0 || n < h || n > len(w) || (n+3)&^3 > len(w) {
			return nil, fmt.Errorf("diameter: AVP flags/length/padding")
		}
		if h == 12 {
			vendor = binary.BigEndian.Uint32(w[8:])
		}
		v := w[h:n]
		a := map[string]any{"Code": code, "Flags": flags, "Vendor ID": vendor, "Length": n, "Value": append([]byte(nil), v...)}
		if vendor == 0 {
			a["Name"] = diameterNames[code]
			switch code {
			case 260, 279, 443, 456, 446:
				children, err := diameterAVPs(v, max, depth-1, total)
				if err != nil {
					return nil, err
				}
				a["Value"] = children
			case 258, 259, 266, 268, 273, 415, 416:
				if len(v) != 4 {
					return nil, fmt.Errorf("diameter: integer AVP length")
				}
				a["Value"] = binary.BigEndian.Uint32(v)
			case 1, 263, 264, 269, 283, 293, 296:
				a["Value"] = string(v)
			case 257:
				if len(v) < 2 {
					return nil, fmt.Errorf("diameter: address AVP")
				}
				fam := binary.BigEndian.Uint16(v)
				if (fam == 1 && len(v) != 6) || (fam == 2 && len(v) != 18) {
					return nil, fmt.Errorf("diameter: address length")
				}
				if fam == 1 || fam == 2 {
					ip, ok := netip.AddrFromSlice(v[2:])
					if !ok {
						return nil, fmt.Errorf("diameter: address")
					}
					a["Value"] = ip.String()
				}
			}
		}
		out = append(out, a)
		w = w[(n+3)&^3:]
	}
	return out, nil
}
func (s *binDiameter) consume(dir int, w []byte, max, depth int) (map[string]any, error) {
	if len(w) < 20 || w[0] != 1 || int(diameter24(w[1:])) != len(w) || w[4]&15 != 0 {
		return nil, fmt.Errorf("diameter: header")
	}
	request := w[4]&128 != 0
	if request && w[4]&32 != 0 {
		return nil, fmt.Errorf("diameter: request has error flag")
	}
	count := 0
	avps, err := diameterAVPs(w[20:], max, depth, &count)
	if err != nil {
		return nil, err
	}
	c, app, hop, end := diameter24(w[5:]), binary.BigEndian.Uint32(w[8:]), binary.BigEndian.Uint32(w[12:]), binary.BigEndian.Uint32(w[16:])
	out := map[string]any{"Command Code": c, "Application ID": app, "Hop-by-Hop ID": hop, "End-to-End ID": end, "Request": request, "AVPs": avps}
	if s.pending == nil {
		s.pending = map[diameterKey]diameterRequest{}
	}
	key := diameterKey{dir, hop}
	if request {
		// Retransmission bit may change while the transaction stays identical.
		copyWire := append([]byte(nil), w...)
		copyWire[4] &= ^byte(16)
		req := diameterRequest{c, app, end, sha256.Sum256(copyWire)}
		if old, ok := s.pending[key]; ok {
			if old != req {
				return nil, protocolError(ErrDesynchronized, "Diameter hop ID reused")
			}
			out["Retransmission"] = true
		} else if len(s.pending) >= max {
			return nil, protocolError(ErrResourceExceeded, "Diameter pending")
		}
		s.pending[key] = req
	} else {
		key.dir = 1 - dir
		old, ok := s.pending[key]
		out["Matched"] = ok
		if ok {
			if old.command != c || old.app != app || old.end != end {
				return nil, protocolError(ErrDesynchronized, "Diameter answer identifiers")
			}
			delete(s.pending, key)
		}
	}
	out["Pending"] = len(s.pending)
	return out, nil
}
