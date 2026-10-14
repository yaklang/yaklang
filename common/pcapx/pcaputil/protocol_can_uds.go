package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// Selected ISO14229 wire observations, not an authenticated ECU service. The
// identity includes the echoed descriptor; NRC only echoes a service and loses
// eligibility once that service has multiple captured generations.
func (a *binParser) decodeCANUDS(s *canISOConnection, dir int, body []byte, e *ProtocolEvent) (map[string]any, error) {
	fail := func(kind ProtocolErrorKind, why string) (map[string]any, error) {
		return nil, protocolError(kind, "CAN UDS "+why)
	}
	if s.udsDisabled {
		return fail(ErrResourceExceeded, "request history exhausted")
	}
	if s.pending != nil && !e.Timestamp.IsZero() && !s.pending.at.IsZero() && e.Timestamp.Sub(s.pending.at) > s.options.UDSResponseTimeout {
		s.pending = nil
	}
	f := map[string]any{"Observation": "unverified-selected-diagnostic-message", "Payload": bytes.Clone(body), "Service": body[0], "Role": "request", "Transaction ID Present": false}
	if dir == 0 {
		key := canUDSIdentity{service: body[0]}
		if s.blockedServices[key.service] {
			s.pending = nil
			return fail(ErrContextRequired, "service retired after unknown request identity")
		}
		suppressed := false
		switch body[0] {
		case 0x10:
			if len(body) != 2 || body[1]&0x7f == 0 {
				s.pending = nil
				s.serviceSeen[0x10] = 2
				s.blockedServices[0x10] = true
				return fail(ErrMalformedMessage, "DiagnosticSessionControl request shape is invalid")
			}
			key.descriptor = uint16(body[1] & 0x7f)
			suppressed = body[1]&0x80 != 0
			f["Session Type"] = key.descriptor
			f["Suppress Positive Response"] = suppressed
			f["Positive Response Expected"] = !suppressed
		case 0x11, 0x3e:
			service := body[0]
			if len(body) != 2 {
				s.pending = nil
				s.serviceSeen[service] = 2
				s.blockedServices[service] = true
				return fail(ErrMalformedMessage, "control request requires one subfunction")
			}
			key.descriptor = uint16(body[1] & 0x7f)
			if (service == 0x3e && key.descriptor != 0) || (service == 0x11 && (key.descriptor < 1 || key.descriptor > 5)) {
				s.pending = nil
				s.serviceSeen[service] = 2
				s.blockedServices[service] = true
				return fail(ErrUnsupportedFeature, "control subfunction outside selected TesterPresent/ECUReset profile")
			}
			suppressed = body[1]&0x80 != 0
			f["Suppress Positive Response"] = suppressed
			f["Positive Response Expected"] = !suppressed
			if service == 0x11 {
				f["Reset Type"] = key.descriptor
			} else {
				f["Subfunction"] = key.descriptor
			}
		case 0x22:
			if len(body) != 3 {
				s.pending = nil
				s.serviceSeen[0x22] = 2
				s.blockedServices[0x22] = true
				return fail(ErrUnsupportedFeature, "ReadDataByIdentifier requires one complete selected DID")
			}
			key.descriptor = binary.BigEndian.Uint16(body[1:])
			f["Data Identifier"] = key.descriptor
		default:
			s.pending = nil
			return fail(ErrUnsupportedFeature, "service outside selected DSC/control/single-DID ReadDataByIdentifier profile")
		}
		if s.pending != nil {
			if s.pending.key == key {
				if s.pending.suppressed != suppressed {
					s.pending = nil
					return fail(ErrContextRequired, "retry changed response-suppression bit")
				}
				f["Association"] = "observed-active-request-retry"
				f["Original Request Event"] = s.pending.id
				return f, nil
			}
			if !s.pending.suppressed {
				s.pending = nil
				if _, used := s.used[key]; !used {
					if len(s.used) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveEvidence(128) {
						s.udsDisabled = true
						return fail(ErrResourceExceeded, "diagnostic identity byte/element budget")
					}
					if s.used == nil {
						s.used = make(map[canUDSIdentity]uint64)
					}
					s.used[key] = e.ID
					s.cost += 128
					s.serviceSeen[key.service] = 2
				}
				return fail(ErrContextRequired, "overlapping diagnostic requests are ambiguous")
			}
		}
		if _, used := s.used[key]; used {
			s.pending = nil
			return fail(ErrContextRequired, "diagnostic request identity was already used")
		}
		cost := int64(128)
		if len(s.used) >= sessionCollectionLimit(a.budget.MaxCollectionElements) || !a.reserveEvidence(cost) {
			s.pending = nil
			s.udsDisabled = true
			return fail(ErrResourceExceeded, "diagnostic identity byte/element budget")
		}
		if s.used == nil {
			s.used = make(map[canUDSIdentity]uint64)
		}
		s.used[key] = e.ID
		s.cost += cost
		if s.serviceSeen[key.service] < 2 {
			s.serviceSeen[key.service]++
		}
		s.pending = &canUDSPending{key: key, id: e.ID, at: e.Timestamp, suppressed: suppressed, ref: e.SourceBytes.PacketRefs[0]}
		f["Association"] = "observed-configured-request"
		f["Original Request Event"] = e.ID
		return f, nil
	}
	f["Role"] = "response"
	p := s.pending
	if p == nil {
		return fail(ErrContextRequired, "response has no eligible configured request")
	}
	switch body[0] {
	case 0x50:
		if p.key.service != 0x10 || len(body) != 6 || uint16(body[1]) != p.key.descriptor || p.suppressed {
			return fail(ErrMalformedMessage, "DiagnosticSessionControl response differs from pending request")
		}
		f["Session Type"], f["P2 Server Max Milliseconds"], f["P2 Star Server Max Milliseconds"] = p.key.descriptor, binary.BigEndian.Uint16(body[2:4]), uint32(binary.BigEndian.Uint16(body[4:6]))*10
		s.pending = nil
	case 0x51, 0x7e:
		service := byte(0x11)
		if body[0] == 0x7e {
			service = 0x3e
		}
		wantLength := 2
		if service == 0x11 && p.key.descriptor == 4 {
			wantLength = 3
		}
		if p.key.service != service || len(body) != wantLength || uint16(body[1]) != p.key.descriptor || p.suppressed {
			return fail(ErrMalformedMessage, "control response differs from pending service/subfunction")
		}
		if service == 0x11 {
			f["Reset Type"] = p.key.descriptor
			if p.key.descriptor == 4 {
				f["Power Down Time Raw"] = body[2]
				f["Power Down Time Known"] = body[2] != 0xff
				if body[2] != 0xff {
					f["Power Down Time Seconds"] = body[2]
				}
			}
		} else {
			f["Subfunction"] = p.key.descriptor
		}
		s.pending = nil
	case 0x62:
		if p.key.service != 0x22 || len(body) < 4 || binary.BigEndian.Uint16(body[1:3]) != p.key.descriptor {
			return fail(ErrMalformedMessage, "ReadDataByIdentifier response differs from pending DID")
		}
		f["Data Identifier"], f["Data Record"], f["Data Interpretation"] = p.key.descriptor, bytes.Clone(body[3:]), "unverified raw DID data record"
		s.pending = nil
	case 0x7f:
		if len(body) != 3 || body[1] != p.key.service || body[2] == 0 {
			return fail(ErrMalformedMessage, "negative response differs from pending service")
		}
		if s.serviceSeen[p.key.service] > 1 {
			return fail(ErrContextRequired, "negative response cannot identify reused service generation")
		}
		f["Request Service"], f["Negative Response Code"], f["Response Pending"] = body[1], body[2], body[2] == 0x78
		if body[2] != 0x78 {
			s.pending = nil
		}
	default:
		return fail(ErrUnsupportedFeature, "response service outside selected diagnostic profile")
	}
	e.ResponseTo = p.id
	f["Association"], f["Response To"] = "observed-request-match", p.id
	return f, nil
}
