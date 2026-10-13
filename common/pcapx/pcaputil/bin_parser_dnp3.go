package pcaputil

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Selected IEEE 1815 complete-link/header observer with bounded READ/WRITE
// context. Port 20000 is never consulted. Legacy mode does not assemble
// fragments or decode objects; those bounded observations require explicit options.
// Secure Authentication is not decoded.
type dnp3RequestKey struct {
	dir                 int
	source, destination uint16
	sequence            byte
}
type dnp3Request struct {
	function  byte
	classes   byte
	selectors []dnp3StaticSelector
	digest    [32]byte
}
type binDNP3 struct {
	pending            map[dnp3RequestKey]dnp3Request
	maxPending         int
	maxDepth           int
	readPoints         bool
	directReads        bool
	transportFragments bool
	fragments          map[dnp3TransportKey]*dnp3TransportFragment
	maxApplication     int
	reserve            func(int64) error
}

func probeDNP3(w []byte, limit int) ProbeResult {
	if len(w) < 2 || w[0] != 0x05 || w[1] != 0x64 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 10 {
		return probeNeed("dnp3", "ieee1815", len(w), 10)
	}
	if w[2] < 5 || w[2] > 5 && !dnp3UserDataControl(w[3]) || !dnp3HeaderCRCValid(w[:10]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("dnp3", "ieee1815", 94)
}

func dnp3UserDataControl(control byte) bool {
	return control&0x40 != 0 && (control&15 == 3 || control&15 == 4)
}

func dnp3CRC(data []byte) uint16 {
	crc := uint16(0)
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if crc&1 != 0 {
				crc = crc>>1 ^ 0xA6BC
			} else {
				crc >>= 1
			}
		}
	}
	return crc ^ 0xFFFF
}

func dnp3HeaderCRCValid(hdr []byte) bool {
	if len(hdr) < 10 {
		return false
	}
	return dnp3CRC(hdr[:8]) == binary.LittleEndian.Uint16(hdr[8:10])
}

func dnp3FrameLength(w []byte) (int, error) {
	if len(w) < 10 {
		return 0, nil
	}
	if w[0] != 0x05 || w[1] != 0x64 {
		return 0, fmt.Errorf("dnp3: start bytes must be 05 64")
	}
	user := int(w[2]) - 5
	if user < 0 {
		return 0, fmt.Errorf("dnp3: invalid length")
	}
	n := 10
	for left := user; left > 0; {
		chunk := min(left, 16)
		n += chunk + 2
		left -= chunk
	}
	return n, nil
}

func (f *binFlow) frameDNP3(w []byte) (int, *binSpec, error) {
	s := f.dnp3
	if s == nil {
		return 0, nil, sessionContext("DNP3 session was not observed")
	}
	if err := f.reserveSession(s.memoryTarget(0)); err != nil {
		return 0, nil, err
	}
	n, err := dnp3FrameLength(w)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 {
		return 0, nil, nil
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	if n > len(w) {
		return n, nil, nil
	}
	return n, f.spec("extended_protocols", "DNP3"), nil
}

func dnp3UserData(w []byte) ([]byte, error) {
	if !dnp3HeaderCRCValid(w[:10]) {
		return nil, fmt.Errorf("dnp3: header CRC mismatch")
	}
	user := int(w[2]) - 5
	if user < 0 {
		return nil, fmt.Errorf("dnp3: invalid length")
	}
	out := make([]byte, 0, user)
	at := 10
	for left := user; left > 0; {
		chunk := min(left, 16)
		if at+chunk+2 > len(w) {
			return nil, fmt.Errorf("dnp3: truncated user data")
		}
		block := w[at : at+chunk]
		if dnp3CRC(block) != binary.LittleEndian.Uint16(w[at+chunk:at+chunk+2]) {
			return nil, fmt.Errorf("dnp3: user data CRC mismatch")
		}
		out = append(out, block...)
		at += chunk + 2
		left -= chunk
	}
	return out, nil
}

func dnp3FuncName(fc byte) string {
	switch fc {
	case 0x00:
		return "CONFIRM"
	case 0x01:
		return "READ"
	case 0x02:
		return "WRITE"
	case 0x81:
		return "RESPONSE"
	case 0x82:
		return "UNSOLICITED"
	default:
		return fmt.Sprintf("FC %d", fc)
	}
}

// Loss of assembly context invalidates outstanding observations for the same
// link pair only; a different link address domain retains its own requests.
func (s *binDNP3) discardLink(source, destination uint16) {
	for key := range s.pending {
		if key.source == source && key.destination == destination || key.source == destination && key.destination == source {
			delete(s.pending, key)
		}
	}
}

func (s *binDNP3) consume(dir int, raw []byte) (map[string]any, error) {
	if len(raw) < 10 {
		return nil, fmt.Errorf("dnp3: truncated header")
	}
	dest := binary.LittleEndian.Uint16(raw[4:6])
	src := binary.LittleEndian.Uint16(raw[6:8])
	out := map[string]any{
		"Packet Name": "Link Frame", "Destination": int(dest), "Source": int(src),
		"Control": int(raw[3]),
	}
	user, err := dnp3UserData(raw)
	if err != nil {
		return nil, err
	}
	if len(user) > 0 && !dnp3UserDataControl(raw[3]) {
		return nil, protocolError(ErrMalformedMessage, "DNP3 application bytes require primary user-data link function")
	}
	if s.pending == nil {
		s.pending = map[dnp3RequestKey]dnp3Request{}
	}
	if len(user) == 0 {
		return out, nil
	}
	// Application bytes exist only after complete transport and application
	// fragments. A continuation is not a new application header. Full DNP3
	// fragment/object assembly remains a separate capability.
	th := user[0]
	out["Transport Header"] = int(th)
	if s.transportFragments {
		var complete bool
		user, complete, err = s.assembleTransport(dir, src, dest, raw[3], user, out)
		if err != nil || !complete {
			return out, err
		}
		// Parsing below operates on one complete APDU. Raw event bytes and
		// link-rule fields still describe the actual final captured link frame.
		th = user[0]
	}
	if th&0xc0 != 0xc0 {
		s.discardLink(src, dest)
		out["Error Scope"] = "application"
		return out, protocolError(ErrContextRequired, "DNP3 transport fragment requires reassembly")
	}
	if len(user) < 3 {
		return nil, fmt.Errorf("dnp3: truncated application layer")
	}
	ac, fc := user[1], user[2]
	out["Application Control"] = int(ac)
	out["Function"] = dnp3FuncName(fc)
	out["Packet Name"] = dnp3FuncName(fc)
	seq, uns := ac&15, ac&16 != 0
	out["Application Sequence"] = int(seq)
	out["Unsolicited"] = uns
	out["Confirm Required"] = ac&32 != 0
	if ac&0xc0 != 0xc0 {
		s.discardLink(src, dest)
		out["Error Scope"] = "application"
		return out, protocolError(ErrContextRequired, "DNP3 application fragment requires assembly")
	}
	if fc == 0x81 || fc == 0x82 {
		if len(user) < 5 {
			return nil, fmt.Errorf("dnp3: truncated IIN")
		}
		out["IIN1"] = int(user[3])
		out["IIN2"] = int(user[4])
		if uns != (fc == 0x82) {
			out["Error Scope"] = "application"
			return out, protocolError(ErrMalformedMessage, "dnp3: response function disagrees with UNS bit")
		}
	}
	classes := byte(0)
	var selectors []dnp3StaticSelector
	if s.readPoints {
		var err error
		classes, selectors, err = s.observePoints(out, dir, src, dest, user)
		if err != nil {
			if pe, ok := err.(*ProtocolError); ok && pe.Kind == ErrResourceExceeded {
				return nil, err
			}
			return out, err
		}
	}
	switch fc {
	case 0:
		// Confirm acknowledges application delivery, not a request for a
		// solicited response. Its UNS/SEQ domain must not retire requests.
		if len(user) != 3 || ac&32 != 0 {
			out["Error Scope"] = "application"
			return out, protocolError(ErrMalformedMessage, "dnp3: invalid application confirmation")
		}
		out["Role"] = "confirmation"
	case 0x82:
		out["Role"], out["Association"] = "response", "unsolicited"
	case 0x81:
		out["Role"] = "response"
		key := dnp3RequestKey{1 - dir, dest, src, seq}
		if req, ok := s.pending[key]; ok {
			out["In Reply To"] = dnp3FuncName(req.function)
			delete(s.pending, key)
		} else {
			out["Association"] = "missing-request"
		}
	case 1, 2:
		if uns || ac&32 != 0 {
			out["Error Scope"] = "application"
			return out, protocolError(ErrMalformedMessage, "dnp3: invalid solicited request control")
		}
		key := dnp3RequestKey{dir, src, dest, seq}
		req := dnp3Request{function: fc, classes: classes, selectors: selectors, digest: sha256.Sum256(user[1:])}
		if old, ok := s.pending[key]; ok {
			if old.function != req.function || old.classes != req.classes || old.digest != req.digest {
				delete(s.pending, key)
				out["Error Scope"] = "application"
				return out, protocolError(ErrContextRequired, "DNP3 sequence reused with conflicting outstanding request")
			}
		} else {
			if len(s.pending) >= sessionCollectionLimit(s.maxPending) {
				return nil, protocolError(ErrResourceExceeded, "DNP3 pending request count")
			}
			if s.reserve != nil {
				if err := s.reserve(s.memoryTarget(80 + int64(cap(selectors))*64)); err != nil {
					return nil, err
				}
			}
			s.pending[key] = req
		}
		out["Role"] = "request"
	default:
		// The selected observer does not infer response contracts for
		// unknown/private or no-response application functions.
		out["Association"] = "untracked-function"
		if fc&0x80 == 0 {
			out["Role"] = "request"
		} else {
			out["Role"] = "response"
		}
	}
	return out, nil
}
