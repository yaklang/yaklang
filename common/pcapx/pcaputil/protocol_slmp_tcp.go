package pcaputil

import (
	"bytes"
	"encoding/binary"
	"errors"
)

const slmpTCPProfile = "slmp-tcp-binary-self-test"

// TCP carries length-framed 3E/4E messages, not UDP conversations. Completed
// 4E serials and 3E echoes are bounded tombstones: application retries are not
// removed by TCP reassembly. These conservative association limits are parser
// policy, not a prohibition on legal SLMP serial/echo reuse.
type binSLMPTCP struct {
	clientDir       int
	pending3        *slmpRequest
	pending4        map[uint16]*slmpRequest
	seen4           map[uint16]bool
	completed3      map[string]bool
	ambiguous3      bool
	ambiguous4      bool
	completed3Count int
}

func slmpTCPFrameSize(w []byte, limit int) (int, error) {
	if len(w) < 2 {
		return 0, nil
	}
	if !slmpStart(w) {
		return 0, slmpError(ErrDesynchronized, "TCP binary subheader changed")
	}
	h := 9
	if w[0] == 0x54 || w[0] == 0xd4 {
		h = 13
	}
	if len(w) < h {
		return 0, nil
	}
	n := int(binary.LittleEndian.Uint16(w[h-2 : h]))
	if h+n > limit {
		return 0, slmpError(ErrResourceExceeded, "announced TCP frame exceeds byte budget")
	}
	minimum := 5
	if w[0] == 0x50 || w[0] == 0x54 {
		minimum = 9
	}
	if n < minimum {
		return 0, slmpError(ErrMalformedMessage, "announced TCP body cannot contain selected fixed fields")
	}
	if len(w) < h+n {
		return 0, nil
	}
	return h + n, nil
}

func probeSLMPTCP(w []byte, limit int, portHint bool) ProbeResult {
	if len(w) < 2 || !slmpStart(w) || (w[0] != 0x50 && w[0] != 0x54) {
		return ProbeResult{Verdict: ProbeReject}
	}
	// A port hint admits only a binary request prefix, never arbitrary bytes or
	// an isolated commandless response. Off-port admission needs complete wire
	// evidence; a possible prefix can be retained without positive admission.
	n, err := slmpTCPFrameSize(w, limit)
	if portHint {
		return probeAccept("slmp", slmpTCPProfile, 90)
	}
	if err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}
	}
	if n == 0 {
		return probeNeed("slmp", slmpTCPProfile, len(w), len(w)+1)
	}
	if slmpRequestEvidence(w[:n]) {
		return probeAccept("slmp", slmpTCPProfile, 99)
	}
	return ProbeResult{Verdict: ProbeReject}
}

func (s *binSLMPTCP) outstanding() int {
	n := len(s.pending4)
	if s.pending3 != nil {
		n++
	}
	return n
}
func (s *binSLMPTCP) storage() int64 {
	n := int64(512 + 128*len(s.seen4))
	for echo := range s.completed3 {
		n += 128 + int64(len(echo))
	}
	add := func(r *slmpRequest) {
		if r != nil {
			n += 256 + int64(len(r.wire)+len(r.data))
		}
	}
	add(s.pending3)
	for _, r := range s.pending4 {
		add(r)
	}
	return n
}
func slmpTCPRecoverable(err *ProtocolError) bool {
	return err != nil && (err.Kind == ErrContextRequired || err.Kind == ErrMalformedMessage)
}
func (f *binFlow) frameSLMPTCP(w []byte) (int, *binSpec, error) {
	n, err := slmpTCPFrameSize(w, min(f.a.budget.MaxFrameBytes, f.a.config.MaxMessageBytes))
	if err == nil && n > 0 {
		// Before event() copies raw bytes and before parsing allocates maps,
		// charge projections, a possible pending slot and retained tombstones.
		err = f.reserveSession(f.slmpTCP.storage() + 4096 + 256*int64(n))
		var typed *ProtocolError
		if errors.As(err, &typed) {
			err = typed
		}
	}
	return n, &binSpec{}, err
}
func (s *binSLMPTCP) consume(w []byte, dir int, id uint64, elements, depth int) (map[string]any, uint64, error) {
	context := func(why string) (map[string]any, uint64, error) { return nil, 0, slmpError(ErrContextRequired, why) }
	if depth < 2 {
		return nil, 0, slmpError(ErrResourceExceeded, "field projection nesting exceeds depth budget")
	}
	m, err := decodeSLMPMessage(w)
	if err != nil {
		// A rejected requester frame may still cause an error reply without an
		// echo. Never leave older requests available to claim that reply. The
		// frame boundary is known, so retain diagnostics without resynchronizing.
		if dir == s.clientDir && len(w) > 0 && (w[0] == 0x50 || w[0] == 0x54) {
			s.pending3, s.pending4, s.ambiguous3, s.ambiguous4 = nil, nil, true, true
		}
		return nil, 0, err
	}
	if len(m.fields)+1 > elements {
		return nil, 0, slmpError(ErrResourceExceeded, "field projection exceeds collection budget")
	}
	if m.request {
		if dir != s.clientDir {
			return context("request differs from observed requester direction")
		}
		var r *slmpRequest
		if m.frame == 3 {
			if s.ambiguous3 {
				return context("3E request generation is ambiguous")
			}
			r = s.pending3
		} else {
			if s.ambiguous4 {
				return context("rejected requester makes 4E association generation ambiguous")
			}
			r = s.pending4[m.serial]
		}
		if r != nil {
			if !bytes.Equal(r.wire, w) {
				if m.frame == 3 {
					s.pending3 = nil
					s.ambiguous3 = true
				} else {
					delete(s.pending4, m.serial)
				}
				return context("outstanding identifier reused for a distinct request")
			}
			m.fields["Association"] = "retransmitted-request"
			return m.fields, 0, nil
		}
		if m.frame == 3 && s.completed3[string(m.data)] || m.frame == 4 && s.seen4[m.serial] {
			return context("completed echo/serial reuse cannot distinguish a late application reply")
		}
		if s.outstanding() >= elements || m.frame == 3 && len(s.completed3) >= elements || m.frame == 4 && len(s.seen4) >= elements {
			return nil, 0, slmpError(ErrResourceExceeded, "retained request/identifier limit")
		}
		r = &slmpRequest{wire: bytes.Clone(w), data: bytes.Clone(m.data), frame: m.frame, serial: m.serial, route: m.route, id: id}
		if m.frame == 3 {
			s.pending3 = r
		} else {
			if s.pending4 == nil {
				s.pending4 = make(map[uint16]*slmpRequest)
			}
			if s.seen4 == nil {
				s.seen4 = make(map[uint16]bool)
			}
			s.pending4[m.serial], s.seen4[m.serial] = r, true
		}
		m.fields["Association"] = "request"
		return m.fields, 0, nil
	}
	if dir == s.clientDir {
		return context("response came from requester direction")
	}
	r := s.pending3
	if m.frame == 4 {
		r = s.pending4[m.serial]
	}
	if r == nil || r.frame != m.frame || r.route != m.route {
		return context("reply lacks matching frame/route/serial request")
	}
	end := m.fields["End Code"].(uint16)
	if m.frame == 3 && (s.ambiguous3 || end != 0 && s.completed3Count > 0 || end == 0 && s.completed3[string(m.data)]) {
		return context("unsequenced reply may belong to an earlier 3E exchange")
	}
	if end == 0 && !bytes.Equal(r.data, m.data) {
		return nil, 0, slmpError(ErrMalformedMessage, "loopback response differs from request data")
	}
	if m.frame == 3 {
		if s.completed3 == nil {
			s.completed3 = make(map[string]bool)
		}
		s.completed3[string(r.data)] = true
		s.completed3Count++
		s.pending3 = nil
	} else {
		delete(s.pending4, m.serial)
	}
	m.fields["Association"] = "response"
	return m.fields, r.id, nil
}

func (s *binSLMPTCP) requestID(w []byte, fallback uint64) uint64 {
	r := s.pending3
	if len(w) >= 4 && w[0] == 0x54 {
		r = s.pending4[binary.LittleEndian.Uint16(w[2:4])]
	}
	if r != nil {
		return r.id
	}
	return fallback
}
