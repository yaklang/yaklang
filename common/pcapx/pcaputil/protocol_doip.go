package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// This TCP profile observes routing activation and UDS DiagnosticSessionControl.
// Wire fields were cross-checked with python-doipclient messages.py and Scapy
// automotive/doip.py. It does not establish vehicle identity or decode other UDS
// services, UDP discovery, TLS, or manufacturer routing activation semantics.
type doipDiagnosticHistory struct {
	seen        [2]uint64
	generations byte
}
type binDoIP struct {
	clientDir                                        int
	maxPending                                       int
	routePending, active, alivePending               bool
	tester, entity                                   uint16
	version                                          byte
	diagnosticPending, diagnosticAcknowledged        bool
	target                                           uint16
	request                                          [2]byte
	history                                          map[uint16]doipDiagnosticHistory
	identities                                       int
	diagnosticRetired                                bool
	diagnosticAmbiguous                              bool
	diagnosticReplyUncertain                         bool
	diagnosticSuperseded                             bool
	associationRefusal                               bool // complete bounded reply; no pending state consumed
	routeEvent, aliveEvent, diagnosticEvent, replyTo uint64
	reserveMemory                                    func(int64) error
}

func (s *binDoIP) sessionBytes() int64 { return 256 + int64(len(s.history))*128 }
func (s *binDoIP) retireDiagnostics() {
	s.diagnosticPending, s.diagnosticAcknowledged, s.diagnosticRetired = false, false, true
	s.diagnosticEvent, s.target, s.request = 0, 0, [2]byte{}
	s.diagnosticAmbiguous = false
	s.diagnosticReplyUncertain = false
	s.diagnosticSuperseded = false
}

// A rejected client diagnostic frame cannot leave old eligibility behind.
// Unknown response/direction errors do not consume a legitimate pending request.
func (s *binDoIP) refusedWire(dir int, w []byte) {
	if dir == s.clientDir && len(w) >= 8 && binary.BigEndian.Uint16(w[2:4]) == 0x8001 {
		s.retireDiagnostics()
	}
}

func (s *binDoIP) outstanding() int {
	n := 0
	if s.routePending {
		n++
	}
	if s.alivePending {
		n++
	}
	if s.diagnosticPending {
		n++
	}
	return n
}

func doipFrameSize(w []byte, maxMessage int) (int, error) {
	if len(w) < 8 {
		return 0, nil
	}
	if w[0] != 2 && w[0] != 3 || w[0]^w[1] != 255 {
		return 0, protocolError(ErrMalformedMessage, "DoIP version or inverse version is invalid")
	}
	n := uint64(binary.BigEndian.Uint32(w[4:8])) + 8
	if maxMessage < 8 || n > uint64(maxMessage) {
		return 0, protocolError(ErrResourceExceeded, "DoIP message exceeds configured limit")
	}
	length := n - 8
	switch binary.BigEndian.Uint16(w[2:4]) {
	case 5:
		if length != 7 && length != 11 {
			return 0, protocolError(ErrMalformedMessage, "DoIP routing request length must be 7 or 11")
		}
	case 6:
		if length != 9 && length != 13 {
			return 0, protocolError(ErrMalformedMessage, "DoIP routing response length must be 9 or 13")
		}
	case 7:
		if length != 0 {
			return 0, protocolError(ErrMalformedMessage, "DoIP alive request must be empty")
		}
	case 8:
		if length != 2 {
			return 0, protocolError(ErrMalformedMessage, "DoIP alive response must contain one address")
		}
	case 0x8001:
		if length < 5 {
			return 0, protocolError(ErrMalformedMessage, "DoIP diagnostic data is empty")
		}
	case 0x8002, 0x8003:
		if length < 5 {
			return 0, protocolError(ErrMalformedMessage, "DoIP diagnostic acknowledgement is truncated")
		}
	default:
		return 0, protocolError(ErrUnsupportedFeature, "DoIP TCP payload type is outside the routing/diagnostic profile")
	}
	return int(n), nil
}

func probeDoIP(w []byte, limit int) ProbeResult {
	if len(w) < 2 || (w[0] != 2 && w[0] != 3) || w[0]^w[1] != 255 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 8 {
		return probeNeed("doip", "tcp", len(w), 8)
	}
	if binary.BigEndian.Uint16(w[2:4]) != 5 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, err := doipFrameSize(w, limit)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < n {
		return probeNeed("doip", "tcp", len(w), n)
	}
	if binary.BigEndian.Uint16(w[8:10]) == 0 || w[10] != 0 || binary.BigEndian.Uint32(w[11:15]) != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("doip", "tcp-routing/dsc", 99)
}

func (s *binDoIP) consume(dir int, raw []byte) (map[string]any, error) {
	return s.consumeEvent(dir, raw, 0)
}
func (s *binDoIP) consumeEvent(dir int, raw []byte, eventID uint64) (map[string]any, error) {
	s.replyTo = 0
	s.associationRefusal = false
	n, err := doipFrameSize(raw, len(raw))
	if err != nil {
		return nil, err
	}
	if n == 0 || n != len(raw) {
		return nil, protocolError(ErrMalformedMessage, "DoIP message boundary differs from payload length")
	}
	if dir != 0 && dir != 1 {
		return nil, protocolError(ErrMalformedMessage, "DoIP direction is invalid")
	}
	if s.version != 0 && raw[0] != s.version {
		return nil, protocolError(ErrMalformedMessage, "DoIP version changed within connection")
	}
	kind := binary.BigEndian.Uint16(raw[2:4])
	p := raw[8:]
	out := map[string]any{"Protocol Version": int(raw[0]), "Payload Type": int(kind), "Payload Length": len(p), "Routing Active": s.active}
	client := dir == s.clientDir
	switch kind {
	case 5:
		if !client || s.routePending || s.active || s.diagnosticPending {
			return nil, protocolError(ErrMalformedMessage, "DoIP routing request direction or order is invalid")
		}
		if binary.BigEndian.Uint16(p[:2]) == 0 || binary.BigEndian.Uint32(p[3:7]) != 0 {
			return nil, protocolError(ErrMalformedMessage, "DoIP routing address or reserved field is invalid")
		}
		if p[2] != 0 {
			return nil, protocolError(ErrUnsupportedFeature, "DoIP activation type requires unsupported routing semantics")
		}
		if s.maxPending <= s.outstanding() {
			return nil, protocolError(ErrResourceExceeded, "DoIP pending limit exceeded")
		}
		s.tester, s.routePending, s.version = binary.BigEndian.Uint16(p[:2]), true, raw[0]
		s.routeEvent = eventID
		out["Packet Name"], out["Role"] = "Routing Activation Request", "request"
		out["Tester Address"], out["Activation Type"] = int(s.tester), int(p[2])
		if len(p) == 11 {
			out["OEM Data"] = append([]byte(nil), p[7:11]...)
		}
	case 6:
		if client || !s.routePending || binary.BigEndian.Uint16(p[:2]) != s.tester {
			return nil, protocolError(ErrMalformedMessage, "DoIP routing response has no matching request")
		}
		if binary.BigEndian.Uint32(p[5:9]) != 0 || binary.BigEndian.Uint16(p[2:4]) == 0 {
			return nil, protocolError(ErrMalformedMessage, "DoIP routing response address or reserved field is invalid")
		}
		code := p[4]
		if code > 7 && code != 0x10 && code != 0x11 {
			return nil, protocolError(ErrUnsupportedFeature, "DoIP routing response code is unsupported")
		}
		s.entity = binary.BigEndian.Uint16(p[2:4])
		// 0x11 requires confirmation: it does not activate this profile.
		s.active, s.routePending = code == 0x10, false
		s.replyTo = s.routeEvent
		s.routeEvent = 0
		out["Packet Name"], out["Role"], out["Association"] = "Routing Activation Response", "response", "matched"
		out["Tester Address"], out["Entity Address"], out["Response Code"] = int(s.tester), int(s.entity), int(code)
		out["Routing Active"] = s.active
		if code == 0x11 {
			out["Confirmation Required"] = true
		}
		if len(p) == 13 {
			out["OEM Data"] = append([]byte(nil), p[9:13]...)
		}
	case 7:
		if client || !s.active || s.alivePending {
			return nil, protocolError(ErrMalformedMessage, "DoIP alive request direction or order is invalid")
		}
		if s.maxPending <= s.outstanding() {
			return nil, protocolError(ErrResourceExceeded, "DoIP pending limit exceeded")
		}
		s.alivePending = true
		s.aliveEvent = eventID
		out["Packet Name"], out["Role"] = "Alive Check Request", "request"
	case 8:
		if !client || !s.alivePending || binary.BigEndian.Uint16(p) != s.tester {
			return nil, protocolError(ErrMalformedMessage, "DoIP alive response has no matching request")
		}
		s.alivePending = false
		s.replyTo = s.aliveEvent
		s.aliveEvent = 0
		out["Packet Name"], out["Role"], out["Association"] = "Alive Check Response", "response", "matched"
		out["Tester Address"] = int(s.tester)
	case 0x8001, 0x8002, 0x8003:
		if !s.active {
			return nil, sessionContext("DoIP routing activation was not observed")
		}
		source, target := binary.BigEndian.Uint16(p[:2]), binary.BigEndian.Uint16(p[2:4])
		out["Source Address"], out["Target Address"] = int(source), int(target)
		data := p[4:]
		if s.diagnosticRetired {
			return nil, sessionContext("DoIP diagnostic context retired after rejected request")
		}
		if kind == 0x8001 && client {
			replacingUncertain := s.diagnosticPending && s.diagnosticReplyUncertain &&
				source == s.tester && target != 0 && len(data) == 2 && data[0] == 0x10 && data[1]&0x7f != 0 && data[1]&0x80 == 0 &&
				(target != s.target || !bytes.Equal(data, s.request[:]))
			if source != s.tester || target == 0 || s.diagnosticPending && !replacingUncertain {
				if source == s.tester && (target != s.target || len(data) != 2 || !bytes.Equal(data, s.request[:])) {
					s.retireDiagnostics()
				}
				return nil, protocolError(ErrMalformedMessage, "DoIP diagnostic request address or order is invalid")
			}
			if data[0] != 0x10 {
				s.retireDiagnostics()
				return nil, protocolError(ErrUnsupportedFeature, "DoIP UDS service is outside DiagnosticSessionControl profile")
			}
			if len(data) != 2 || data[1]&0x7f == 0 {
				s.retireDiagnostics()
				return nil, protocolError(ErrMalformedMessage, "UDS DiagnosticSessionControl request is malformed")
			}
			if data[1]&0x80 != 0 {
				s.retireDiagnostics()
				return nil, protocolError(ErrUnsupportedFeature, "UDS suppressed positive response requires unsupported completion semantics")
			}
			outstanding := s.outstanding()
			if replacingUncertain {
				outstanding--
			}
			if s.maxPending <= outstanding {
				return nil, protocolError(ErrResourceExceeded, "DoIP pending limit exceeded")
			}
			h := s.history[target]
			index, bit := data[1]/64, uint64(1)<<(data[1]%64)
			reused := h.seen[index]&bit != 0
			if !reused && s.identities >= sessionCollectionLimit(s.maxPending) {
				s.retireDiagnostics()
				return nil, protocolError(ErrResourceExceeded, "DoIP diagnostic identity history limit exceeded")
			}
			if _, exists := s.history[target]; !exists {
				if s.reserveMemory != nil {
					if err := s.reserveMemory(s.sessionBytes() + 128); err != nil {
						s.retireDiagnostics()
						return nil, protocolError(ErrResourceExceeded, "DoIP diagnostic history memory reservation failed")
					}
				}
				if s.history == nil {
					s.history = make(map[uint16]doipDiagnosticHistory)
				}
			}
			if replacingUncertain {
				// Captured association ambiguity cannot prove the prior request completed.
				// A distinct descriptor stays observable with that explicit uncertainty;
				// old eligibility is retired, and history still prevents reused matches.
				s.diagnosticSuperseded = true
				out["Previous Diagnostic Completion Unverified"] = true
				out["Unresolved Previous Request Event"] = s.diagnosticEvent
				out["Unresolved Previous Session Type"] = int(s.request[1] & 0x7f)
				out["Unresolved Previous Target Address"] = int(s.target)
			}
			s.diagnosticReplyUncertain = false
			h.seen[index] |= bit
			if h.generations < 2 {
				h.generations++
			}
			s.history[target] = h
			if !reused {
				s.identities++
			}
			s.diagnosticAmbiguous = reused
			s.diagnosticEvent = eventID
			s.request, s.target = [2]byte{data[0], data[1]}, target
			s.diagnosticPending, s.diagnosticAcknowledged = true, false
			out["Packet Name"], out["Role"] = "DiagnosticSessionControl Request", "request"
			out["UDS Service"], out["Session Type"] = 0x10, int(data[1]&0x7f)
			out["Suppress Positive Response"] = data[1]&0x80 != 0
			if reused {
				out["Association"] = "unverified-reused-diagnostic-request"
				out["Generation Identifiable"] = false
			}
		} else {
			if client || !s.diagnosticPending || source != s.target || target != s.tester {
				if !client && s.diagnosticPending && s.diagnosticSuperseded && target == s.tester {
					if _, known := s.history[source]; known {
						if err := doipDiagnosticResponseShape(kind, data); err != nil {
							return nil, err
						}
						s.associationRefusal, s.diagnosticReplyUncertain = true, true
						return nil, protocolError(ErrContextRequired, "DoIP response belongs to a retired logical target context")
					}
				}
				return nil, protocolError(ErrMalformedMessage, "DoIP diagnostic response has no matching address/direction/request")
			}
			// Wire validity is independent of whether a captured response can be
			// associated. Never let ambiguity turn malformed or unsupported bytes
			// into the private, recoverable complete-message refusal scope.
			if err := doipDiagnosticResponseShape(kind, data); err != nil {
				return nil, err
			}
			if s.diagnosticSuperseded {
				descriptor, hasDescriptor := byte(0), false
				if kind == 0x8001 && data[0] == 0x50 {
					descriptor, hasDescriptor = data[1], true
				}
				if (kind == 0x8002 || kind == 0x8003) && len(data) == 3 {
					descriptor, hasDescriptor = data[2], true
				}
				h := s.history[source]
				if hasDescriptor && descriptor < 128 && descriptor != s.request[1] && h.seen[descriptor/64]&(uint64(1)<<(descriptor%64)) != 0 {
					s.associationRefusal, s.diagnosticReplyUncertain = true, true
					return nil, protocolError(ErrContextRequired, "DoIP response echoes a retired diagnostic descriptor")
				}
			}
			if s.diagnosticAmbiguous {
				s.associationRefusal = true
				s.diagnosticReplyUncertain = true
				return nil, protocolError(ErrContextRequired, "DoIP reply cannot identify a reused diagnostic identity generation")
			}
			out["Role"], out["Association"], out["In Reply To"] = "response", "matched", "DiagnosticSessionControl Request"
			out["Session Type"] = int(s.request[1] & 0x7f)
			if kind != 0x8001 {
				code := data[0]
				if kind == 0x8002 && code != 0 || kind == 0x8003 && (code < 2 || code > 8) {
					return nil, protocolError(ErrMalformedMessage, "DoIP diagnostic acknowledgement code is invalid")
				}
				if len(data) > 3 || !bytes.Equal(data[1:], s.request[:len(data)-1]) {
					return nil, protocolError(ErrMalformedMessage, "DoIP acknowledgement previous message differs from request")
				}
				if s.history[source].generations > 1 && len(data) != 3 {
					s.associationRefusal = true
					s.diagnosticReplyUncertain = true
					return nil, protocolError(ErrContextRequired, "DoIP acknowledgement omits a unique diagnostic descriptor")
				}
				out["Acknowledgement Code"] = int(code)
				if kind == 0x8002 {
					if s.diagnosticAcknowledged {
						return nil, protocolError(ErrMalformedMessage, "DoIP diagnostic acknowledgement repeated")
					}
					s.diagnosticAcknowledged = true
					out["Packet Name"] = "Diagnostic Message ACK"
					if s.request[1]&0x80 != 0 {
						s.diagnosticPending = false
					}
				} else {
					out["Packet Name"] = "Diagnostic Message NACK"
					s.diagnosticPending = false
				}
			} else {
				switch data[0] {
				case 0x50:
					if len(data) != 6 || data[1] != s.request[1]&0x7f || s.request[1]&0x80 != 0 {
						return nil, protocolError(ErrMalformedMessage, "UDS DiagnosticSessionControl response differs from request")
					}
					out["Packet Name"], out["UDS Service"] = "DiagnosticSessionControl Response", 0x50
					out["P2 Server Max Milliseconds"] = int(binary.BigEndian.Uint16(data[2:4]))
					out["P2 Star Server Max Milliseconds"] = int(binary.BigEndian.Uint16(data[4:6])) * 10
					s.diagnosticPending = false
				case 0x7f:
					if len(data) != 3 || data[1] != 0x10 || data[2] == 0 {
						return nil, protocolError(ErrMalformedMessage, "UDS negative response differs from request")
					}
					if s.history[source].generations > 1 {
						s.associationRefusal = true
						s.diagnosticReplyUncertain = true
						return nil, protocolError(ErrContextRequired, "DoIP service-only negative response cannot identify diagnostic generation")
					}
					out["Packet Name"], out["UDS Service"], out["Negative Response Code"] = "DiagnosticSessionControl Negative Response", 0x7f, int(data[2])
					out["Response Pending"] = data[2] == 0x78
					s.diagnosticPending = data[2] == 0x78
				default:
					return nil, protocolError(ErrUnsupportedFeature, "DoIP UDS response service is unsupported")
				}
			}
			s.replyTo = s.diagnosticEvent
			out["Diagnostic Acknowledged"] = s.diagnosticAcknowledged
			if !s.diagnosticPending {
				s.diagnosticAcknowledged = false
				s.request = [2]byte{}
				s.target = 0
				s.diagnosticEvent = 0
				s.diagnosticAmbiguous = false
				s.diagnosticReplyUncertain = false
				s.diagnosticSuperseded = false
			}
		}
	}
	out["Outstanding Requests"] = s.outstanding()
	return out, nil
}

// The selected modern DSC response profile has six positive bytes, three NRC
// bytes and at most two echoed DSC request bytes in a DoIP ACK/NACK. Context-
// dependent echo/type comparisons remain in consumeEvent after these checks.
func doipDiagnosticResponseShape(kind uint16, data []byte) error {
	if kind == 0x8002 || kind == 0x8003 {
		code := data[0]
		if kind == 0x8002 && code != 0 || kind == 0x8003 && (code < 2 || code > 8) {
			return protocolError(ErrMalformedMessage, "DoIP diagnostic acknowledgement code is invalid")
		}
		if len(data) > 3 {
			return protocolError(ErrMalformedMessage, "DoIP acknowledgement previous message exceeds DSC request")
		}
		if len(data) >= 2 && data[1] != 0x10 {
			return protocolError(ErrUnsupportedFeature, "DoIP acknowledgement echoes an unsupported UDS service")
		}
		if len(data) == 3 && data[2]&0x7f == 0 {
			return protocolError(ErrMalformedMessage, "DoIP acknowledgement echoes a zero DSC session")
		}
		if len(data) == 3 && data[2]&0x80 != 0 {
			return protocolError(ErrUnsupportedFeature, "DoIP acknowledgement echoes an unsupported suppressed DSC request")
		}

		return nil
	}
	switch data[0] {
	case 0x50:
		if len(data) != 6 || data[1] == 0 || data[1]&0x80 != 0 {
			return protocolError(ErrMalformedMessage, "UDS DiagnosticSessionControl response is malformed")
		}
	case 0x7f:
		if len(data) != 3 || data[1] != 0x10 || data[2] == 0 {
			return protocolError(ErrMalformedMessage, "UDS negative response differs from request")
		}
	default:
		return protocolError(ErrUnsupportedFeature, "DoIP UDS response service is unsupported")
	}
	return nil
}
