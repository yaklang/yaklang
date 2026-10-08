package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
)

type binRFB struct {
	server        int
	version       int
	serverVersion int
	phase         string
	security      []byte
	selected      byte
	bpp           int
	width, height uint16
	encodings     map[int32]bool
}

func probeRFB(w []byte, _ int) ProbeResult {
	if len(w) < 4 {
		if len(w) > 0 && bytes.HasPrefix([]byte("RFB "), w) {
			return probeNeed("vnc", "rfb", len(w), 12)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	if !bytes.HasPrefix(w, []byte("RFB ")) {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 12 {
		return probeNeed("vnc", "rfb", len(w), 12)
	}
	if w[7] != '.' || w[11] != '\n' {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("vnc", "rfb", 99)
}
func rfbPixel(p []byte) (int, map[string]any, error) {
	if len(p) != 16 {
		return 0, nil, fmt.Errorf("rfb: pixel format length")
	}
	bpp, depth := int(p[0]), int(p[1])
	if (bpp != 8 && bpp != 16 && bpp != 32) || depth == 0 || depth > bpp || p[2] > 1 || p[3] > 1 {
		return 0, nil, fmt.Errorf("rfb: pixel format")
	}
	out := map[string]any{"Bits Per Pixel": bpp, "Depth": depth, "Big Endian": p[2] != 0, "True Color": p[3] != 0}
	if p[3] != 0 {
		var masks uint64
		for i, name := range []string{"Red", "Green", "Blue"} {
			m := binary.BigEndian.Uint16(p[4+i*2:])
			shift := p[10+i]
			if m == 0 || (uint32(m)&(uint32(m)+1)) != 0 || int(shift) >= bpp || uint64(m)<<shift >= uint64(1)<<bpp {
				return 0, nil, fmt.Errorf("rfb: color mask")
			}
			mask := uint64(m) << shift
			if masks&mask != 0 {
				return 0, nil, fmt.Errorf("rfb: overlapping color masks")
			}
			masks |= mask
			out[name+" Max"] = m
			out[name+" Shift"] = shift
		}
	}
	return bpp / 8, out, nil
}
func (f *binFlow) frameRFB(dir int, w []byte) (int, *binSpec, error) {
	s := f.rfb
	if err := f.reserveSession(512 + int64(len(s.encodings))*32); err != nil {
		return 0, nil, err
	}
	n, _, err := s.packet(dir, w, f.a.budget.MaxCollectionElements, f.a.budget.MaxMessageBytes, false)
	return n, f.a.specs["session_envelopes/RFB"], err
}
func (s *binRFB) packet(dir int, w []byte, max, maxBytes int, consume bool) (int, map[string]any, error) {
	out := map[string]any{"Phase": s.phase}
	need := func(n int) (int, map[string]any, error) {
		if n > maxBytes {
			return 0, nil, protocolError(ErrResourceExceeded, "RFB message size")
		}
		return n, out, nil
	}
	server := dir == s.server
	if s.phase == "server-version" || s.phase == "client-version" {
		if (s.phase == "server-version") != server {
			return 0, nil, protocolError(ErrDesynchronized, "RFB banner direction")
		}
		if len(w) < 12 {
			return need(12)
		}
		if !bytes.HasPrefix(w, []byte("RFB 003.")) || w[11] != '\n' {
			return 0, nil, protocolError(ErrUnsupportedVersion, "RFB requires standard 3.3/3.7/3.8")
		}
		v, err := strconv.Atoi(string(w[8:11]))
		if err != nil || (v != 3 && v != 7 && v != 8) {
			return 0, nil, protocolError(ErrUnsupportedVersion, "RFB version")
		}
		out["Version"] = string(w[4:11])
		if consume {
			if server {
				s.serverVersion = v
				s.phase = "client-version"
			} else {
				if v > s.serverVersion {
					return 0, nil, fmt.Errorf("rfb: client version exceeds offer")
				}
				s.version = v
				s.phase = "security"
			}
		}
		return need(12)
	}
	switch s.phase {
	case "security":
		if !server {
			return 0, nil, protocolError(ErrDesynchronized, "RFB security direction")
		}
		if s.version == 3 {
			if len(w) < 4 {
				return need(4)
			}
			v := binary.BigEndian.Uint32(w)
			out["Security Type"] = v
			if v == 0 {
				if len(w) < 8 {
					return need(8)
				}
				n := int(binary.BigEndian.Uint32(w[4:]))
				if n > maxBytes-8 {
					return 0, nil, protocolError(ErrResourceExceeded, "RFB reason")
				}
				if len(w) < 8+n {
					return need(8 + n)
				}
				out["Reason"] = string(w[8 : 8+n])
				if consume {
					s.phase = "failed"
				}
				return need(8 + n)
			}
			if v != 1 && v != 2 {
				return 0, nil, protocolError(ErrUnsupportedFeature, "RFB security type")
			}
			if consume {
				s.selected = byte(v)
				s.phase = "challenge"
				if v == 1 {
					s.phase = "client-init"
				}
			}
			return need(4)
		}
		if len(w) < 1 {
			return need(1)
		}
		count := int(w[0])
		if count == 0 {
			if len(w) < 5 {
				return need(5)
			}
			n := int(binary.BigEndian.Uint32(w[1:]))
			if n > maxBytes-5 {
				return 0, nil, protocolError(ErrResourceExceeded, "RFB failure reason")
			}
			if len(w) < 5+n {
				return need(5 + n)
			}
			out["Reason"] = string(w[5 : 5+n])
			if consume {
				s.phase = "failed"
			}
			return need(5 + n)
		}
		if count > max {
			return 0, nil, protocolError(ErrResourceExceeded, "RFB security types")
		}
		if len(w) < count+1 {
			return need(count + 1)
		}
		out["Security Types"] = bytes.Clone(w[1 : count+1])
		if consume {
			s.security = bytes.Clone(w[1 : count+1])
			s.phase = "selection"
		}
		return need(count + 1)
	case "selection":
		if server {
			return 0, nil, protocolError(ErrDesynchronized, "RFB security selection direction")
		}
		if len(w) < 1 {
			return need(1)
		}
		if !bytes.Contains(s.security, w[:1]) {
			return 0, nil, fmt.Errorf("rfb: unoffered security type")
		}
		if w[0] != 1 && w[0] != 2 {
			return 0, nil, protocolError(ErrUnsupportedFeature, "RFB security type")
		}
		out["Security Type"] = w[0]
		if consume {
			s.selected = w[0]
			s.phase = "challenge"
			if w[0] == 1 {
				if s.version == 8 {
					s.phase = "auth-result"
				} else {
					s.phase = "client-init"
				}
			}
		}
		return need(1)
	case "challenge", "challenge-response":
		if (s.phase == "challenge") != server {
			return 0, nil, protocolError(ErrDesynchronized, "RFB challenge direction")
		}
		if len(w) < 16 {
			return need(16)
		}
		out["Authentication Bytes"] = 16
		out["Authentication Verified"] = false
		if consume {
			if server {
				s.phase = "challenge-response"
			} else {
				s.phase = "auth-result"
			}
		}
		return need(16)
	case "auth-result":
		if !server {
			return 0, nil, protocolError(ErrDesynchronized, "RFB result direction")
		}
		if len(w) < 4 {
			return need(4)
		}
		status := binary.BigEndian.Uint32(w)
		out["Authentication Status"] = status
		n := 4
		if status != 0 && s.version == 8 {
			if len(w) < 8 {
				return need(8)
			}
			l := int(binary.BigEndian.Uint32(w[4:]))
			if l > maxBytes-8 {
				return 0, nil, protocolError(ErrResourceExceeded, "RFB reason")
			}
			n = 8 + l
			if len(w) < n {
				return need(n)
			}
			out["Reason"] = string(w[8:n])
		}
		if consume {
			if status == 0 {
				s.phase = "client-init"
			} else {
				s.phase = "failed"
			}
		}
		return need(n)
	case "client-init":
		if server {
			return 0, nil, protocolError(ErrDesynchronized, "RFB ClientInit direction")
		}
		if len(w) < 1 {
			return need(1)
		}
		if w[0] > 1 {
			return 0, nil, fmt.Errorf("rfb: shared flag")
		}
		out["Shared"] = w[0] == 1
		if consume {
			s.phase = "server-init"
		}
		return need(1)
	case "server-init":
		if !server {
			return 0, nil, protocolError(ErrDesynchronized, "RFB ServerInit direction")
		}
		if len(w) < 24 {
			return need(24)
		}
		l := int(binary.BigEndian.Uint32(w[20:]))
		if l > maxBytes-24 {
			return 0, nil, protocolError(ErrResourceExceeded, "RFB desktop name")
		}
		if len(w) < 24+l {
			return need(24 + l)
		}
		bpp, pixel, err := rfbPixel(w[4:20])
		if err != nil {
			return 0, nil, err
		}
		out["Width"] = binary.BigEndian.Uint16(w)
		out["Height"] = binary.BigEndian.Uint16(w[2:])
		out["Pixel Format"] = pixel
		out["Desktop Name"] = string(w[24 : 24+l])
		if consume {
			s.bpp = bpp
			s.width = binary.BigEndian.Uint16(w)
			s.height = binary.BigEndian.Uint16(w[2:])
			s.phase = "messages"
		}
		return need(24 + l)
	case "failed":
		return 0, nil, protocolError(ErrDesynchronized, "RFB authentication failed")
	}
	if len(w) < 1 {
		return need(1)
	}
	out["Message Type"] = w[0]
	if !server {
		switch w[0] {
		case 0:
			if len(w) < 20 {
				return need(20)
			}
			bpp, pixel, err := rfbPixel(w[4:20])
			if err != nil {
				return 0, nil, err
			}
			out["Pixel Format"] = pixel
			if consume {
				s.bpp = bpp
			}
			return need(20)
		case 2:
			if len(w) < 4 {
				return need(4)
			}
			n := int(binary.BigEndian.Uint16(w[2:]))
			if n > max {
				return 0, nil, protocolError(ErrResourceExceeded, "RFB encodings")
			}
			if len(w) < 4+n*4 {
				return need(4 + n*4)
			}
			encs := make([]int32, 0, n)
			for i := 0; i < n; i++ {
				encs = append(encs, int32(binary.BigEndian.Uint32(w[4+i*4:])))
			}
			out["Encodings"] = encs
			if consume {
				s.encodings = map[int32]bool{}
				for _, e := range encs {
					s.encodings[e] = true
				}
			}
			return need(4 + n*4)
		case 3:
			if len(w) < 10 {
				return need(10)
			}
			out["Incremental"] = w[1] != 0
			out["X"] = binary.BigEndian.Uint16(w[2:])
			out["Y"] = binary.BigEndian.Uint16(w[4:])
			out["Width"] = binary.BigEndian.Uint16(w[6:])
			out["Height"] = binary.BigEndian.Uint16(w[8:])
			return need(10)
		case 4:
			if len(w) < 8 {
				return need(8)
			}
			out["Down"] = w[1] != 0
			out["Key"] = binary.BigEndian.Uint32(w[4:])
			return need(8)
		case 5:
			if len(w) < 6 {
				return need(6)
			}
			out["Buttons"] = w[1]
			out["X"] = binary.BigEndian.Uint16(w[2:])
			out["Y"] = binary.BigEndian.Uint16(w[4:])
			return need(6)
		case 6:
			return rfbCut(w, out, maxBytes)
		default:
			return 0, nil, protocolError(ErrUnsupportedFeature, "RFB client message")
		}
	}
	switch w[0] {
	case 2:
		return need(1)
	case 3:
		return rfbCut(w, out, maxBytes)
	case 1:
		if len(w) < 6 {
			return need(6)
		}
		n := int(binary.BigEndian.Uint16(w[4:]))
		if n > max {
			return 0, nil, protocolError(ErrResourceExceeded, "RFB color map")
		}
		if len(w) < 6+n*6 {
			return need(6 + n*6)
		}
		out["First Color"] = binary.BigEndian.Uint16(w[2:])
		out["Color Count"] = n
		return need(6 + n*6)
	case 0:
	default:
		return 0, nil, protocolError(ErrUnsupportedFeature, "RFB server message")
	}
	if len(w) < 4 {
		return need(4)
	}
	count := int(binary.BigEndian.Uint16(w[2:]))
	if count > max {
		return 0, nil, protocolError(ErrResourceExceeded, "RFB rectangles")
	}
	pos := 4
	rects := []map[string]any{}
	for i := 0; i < count; i++ {
		if len(w) < pos+12 {
			return need(pos + 12)
		}
		r := w[pos : pos+12]
		x, y, wide, high := binary.BigEndian.Uint16(r), binary.BigEndian.Uint16(r[2:]), binary.BigEndian.Uint16(r[4:]), binary.BigEndian.Uint16(r[6:])
		encoding := int32(binary.BigEndian.Uint32(r[8:]))
		pos += 12
		rect := map[string]any{"X": x, "Y": y, "Width": wide, "Height": high, "Encoding": encoding}
		size := uint64(0)
		switch encoding {
		case 0:
			size = uint64(wide) * uint64(high) * uint64(s.bpp)
			rect["Content Status"] = "raw-pixels"
		case 1:
			size = 4
		case -239:
			size = uint64(wide)*uint64(high)*uint64(s.bpp) + uint64((int(wide)+7)/8)*uint64(high)
		case -223:
			if consume {
				s.width = wide
				s.height = high
			}
		case -224:
			rects = append(rects, rect)
			out["Rectangles"] = rects
			return need(pos)
		case 16, 6:
			if len(w) < pos+4 {
				return need(pos + 4)
			}
			size = 4 + uint64(binary.BigEndian.Uint32(w[pos:]))
			rect["Content Status"] = "compressed-pixels-not-decoded"
		case 2:
			if len(w) < pos+4 {
				return need(pos + 4)
			}
			n := uint64(binary.BigEndian.Uint32(w[pos:]))
			if n > uint64(max) {
				return 0, nil, protocolError(ErrResourceExceeded, "RFB RRE subrectangles")
			}
			size = 4 + uint64(s.bpp) + n*uint64(s.bpp+8)
		default:
			return 0, nil, protocolError(ErrUnsupportedFeature, fmt.Sprintf("RFB rectangle encoding %d", encoding))
		}
		if size > uint64(maxBytes) || uint64(pos)+size > uint64(maxBytes) {
			return 0, nil, protocolError(ErrResourceExceeded, "RFB rectangle payload")
		}
		if len(w) < pos+int(size) {
			return need(pos + int(size))
		}
		rect["Payload Bytes"] = size
		pos += int(size)
		rects = append(rects, rect)
	}
	out["Rectangles"] = rects
	return need(pos)
}
func rfbCut(w []byte, out map[string]any, max int) (int, map[string]any, error) {
	if len(w) < 8 {
		return 8, out, nil
	}
	n := uint64(binary.BigEndian.Uint32(w[4:]))
	if n > uint64(max-8) {
		return 0, nil, protocolError(ErrResourceExceeded, "RFB clipboard length or extended clipboard")
	}
	if len(w) < 8+int(n) {
		return 8 + int(n), out, nil
	}
	out["Text"] = string(w[8 : 8+int(n)])
	return 8 + int(n), out, nil
}
func (s *binRFB) consume(dir int, w []byte, max, limit int) (map[string]any, error) {
	n, out, err := s.packet(dir, w, max, limit, true)
	if err != nil {
		return nil, err
	}
	if n != len(w) {
		return nil, fmt.Errorf("rfb: message boundary")
	}
	return out, nil
}
