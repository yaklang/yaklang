package pcaputil

import (
	"fmt"
	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type binAJP struct {
	proxy                                                                int
	roleKnown, active, blocked, closed, expectBody, knownLength, headers bool
	request, pull, ping                                                  uint64
	remaining                                                            uint64
	method, uri                                                          string
	pingAmbiguous                                                        bool
}

func probeAJP(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != 0x12 && w[0] != 0x41 {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, err := stream_parser.AJPWireLength(w)
	if err != nil || n > limit {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("ajp", "13", len(w), 4)
	}
	if len(w) >= 5 {
		code := w[4]
		valid := false
		if w[0] == 0x12 {
			valid = code == 2 || code == 7 || code == 8 || code == 10
		} else {
			valid = code >= 3 && code <= 6 || code == 9
		}
		if !valid {
			return ProbeResult{Verdict: ProbeReject}
		}
		if (code == 8 || code == 9 || code == 10) && n != 5 || code == 5 && n != 6 || code == 6 && n != 7 {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	if len(w) < n {
		return probeNeed("ajp", "13", len(w), n)
	}
	if _, err = stream_parser.DecodeAJPMessage(w[:n], false, 4096); err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("ajp", "13", 99)
}
func (s *binAJP) storage() int64 { return int64(2048 + len(s.method) + len(s.uri)) }
func (f *binFlow) frameAJP(dir int, w []byte) (int, *binSpec, error) {
	if err := f.reserveSession(f.ajp.storage()); err != nil {
		return 0, nil, err
	}
	n, err := stream_parser.AJPWireLength(w)
	entry := "AJPMessageFields"
	if f.ajp.roleKnown && dir == f.ajp.proxy && f.ajp.expectBody {
		entry = "AJPRequestBodyFields"
	}
	return n, f.spec("ajp", entry), err
}
func (f *binFlow) consumeAJP(dir int, e *ProtocolEvent) error {
	s := f.ajp
	proxy := e.Raw[0] == 0x12
	body := s.roleKnown && dir == s.proxy && s.expectBody
	fields, err := stream_parser.DecodeAJPMessage(e.Raw, body, f.a.budget.MaxCollectionElements)
	e.nativeSessionMessage = true
	e.Profile, e.Admission, e.Completeness = "ajp13-observed", "complete-directional-envelope-and-message", "message"
	e.Session = fields
	if fields == nil {
		return observedWireError(err)
	}
	fields["Error Scope"] = "message"
	if !s.roleKnown && err == nil {
		s.proxy = dir
		if !proxy {
			s.proxy = 1 - dir
		}
		s.roleKnown = true
	}
	if err == nil && s.roleKnown && proxy != (dir == s.proxy) {
		err = protocolError(ErrMalformedMessage, "AJP directional magic changed inside captured TCP generation")
		s.blocked = true
	}
	if e.ID == 0 {
		e.ID = f.a.ids.Add(1)
	}
	if err == nil {
		name := fields["Packet Name"].(string)
		if body {
			n := uint64(fields["Body Length"].(int))
			if s.knownLength && n > s.remaining {
				err = protocolError(ErrMalformedMessage, "AJP body exceeds remaining Content-Length")
				s.blocked = true
			} else {
				if !s.blocked {
					e.TransactionID = s.request
					e.ResponseTo = s.pull
					if e.ResponseTo == 0 {
						e.ResponseTo = s.request
					}
				}
				if s.knownLength {
					s.remaining -= n
				}
				s.expectBody = false
				s.pull = 0
				if n == 0 && s.knownLength && s.remaining > 0 {
					fields["Body Incomplete"] = true
					s.blocked = true
					err = protocolError(ErrNeedMore, "AJP body EOF before observed Content-Length was satisfied")
				}
			}
		} else {
			switch name {
			case "Forward Request":
				if s.active || s.closed || s.blocked {
					s.blocked = true
					err = protocolError(ErrContextRequired, "AJP Forward Request cannot establish an unambiguous reusable request cycle")
				} else {
					method, _ := fields["Method"].(string)
					uri, _ := fields["URI"].(string)
					if err = f.reserveSession(int64(2048 + len(method) + len(uri))); err != nil {
						s.blocked = true
						break
					}
					s.active = true
					s.request = e.ID
					s.headers = false
					s.method, _ = fields["Method"].(string)
					s.uri, _ = fields["URI"].(string)
					s.knownLength = false
					s.remaining = 0
					s.expectBody = false
					s.pull = 0
					if n, ok := fields["Content Length"].(uint64); ok {
						s.knownLength = true
						s.remaining = n
						s.expectBody = n > 0
					}
					e.TransactionID = e.ID
				}
			case "Send Headers", "Send Body Chunk", "Get Body Chunk", "End Response":
				if s.active && !s.blocked {
					e.TransactionID, e.ResponseTo = s.request, s.request
					fields["Request Method"], fields["Request URI"] = s.method, s.uri
				} else {
					fields["Request Context"] = "unavailable; no unique observed Forward Request"
				}
				if name == "Send Headers" {
					status := fields["Status Code"].(uint16)
					if status >= 200 {
						if s.headers {
							s.blocked = true
							err = protocolError(ErrContextRequired, "AJP repeated final response headers in one request cycle")
						}
						s.headers = true
					}
				}
				if name == "Get Body Chunk" {
					s.expectBody = true
					s.pull = e.ID
				}
				if name == "End Response" {
					if s.knownLength && s.remaining > 0 {
						s.blocked = true
						fields["Reuse Context"] = "unread request body; subsequent request association quarantined"
					}
					s.active = false
					s.request = 0
					s.method = ""
					s.uri = ""
					s.expectBody = false
					s.pull = 0
					s.headers = false
					if fields["Reuse"] != true {
						s.closed = true
					}
				}
			case "CPing":
				if s.ping != 0 {
					s.pingAmbiguous = true
					s.ping = 0
				} else if !s.pingAmbiguous {
					s.ping = e.ID
				}
			case "CPong":
				if !s.pingAmbiguous {
					e.ResponseTo = s.ping
				}
				s.ping = 0
				s.pingAmbiguous = false
			case "Shutdown":
				s.closed = true
				s.active = false
				s.request = 0
				s.expectBody = false
			}
		}
	}
	state := "idle observed carrier"
	if s.active {
		state = "request cycle active"
	}
	if s.closed {
		state = "reuse disabled or Shutdown observed"
	}
	if s.blocked {
		state = "request association quarantined"
	}
	fields["Connection State"] = state
	if s.active {
		fields["Remaining Request Bytes Known"] = s.knownLength
		if s.knownLength {
			fields["Remaining Request Bytes"] = s.remaining
		}
	}
	if err != nil {
		e.TransactionID, e.ResponseTo = 0, 0
	}
	if err == nil {
		err = f.reserveSession(s.storage())
	}
	e.semanticFields = cloneSession(fields)
	e.semanticSession = cloneSession(fields)
	e.Summary = fmt.Sprintf("AJP13 %v", fields["Packet Name"])
	return observedWireError(err)
}
