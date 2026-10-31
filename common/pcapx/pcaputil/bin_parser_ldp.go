package pcaputil

import (
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type ldpPeerObservation struct {
	seen               bool
	receiver           string
	keepalive, maximum uint64
}
type binLDP struct {
	identities      [2]string
	peers           [2]ldpPeerObservation
	keepalive       [2]bool
	blocked, closed bool
}

func probeLDP(w []byte, limit int) ProbeResult {
	n, err := stream_parser.LDPWireLength(w)
	if err != nil || n > limit {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("ldp", "1", len(w), 4)
	}
	if len(w) < n {
		return probeNeed("ldp", "1", len(w), n)
	}
	fields, err := stream_parser.DecodeLDPSessionPDU(w[:n], 4096)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	for _, m := range fields["Messages"].([]map[string]any) {
		if m["Packet Name"] != "Unknown" && m["Unknown Mandatory Parameters"] != true && m["Unsupported Parameter Value"] != true {
			return probeAccept("ldp", "1", 98)
		}
	}
	return ProbeResult{Verdict: ProbeReject}
}
func (f *binFlow) frameLDP(w []byte) (int, *binSpec, error) {
	if err := f.reserveSession(2048); err != nil {
		return 0, nil, err
	}
	n, err := stream_parser.LDPWireLength(w)
	return n, f.a.specs["ldp_session/LDPSessionFields"], observedWireError(err)
}
func (s *binLDP) exchangeObserved() bool {
	return !s.blocked && !s.closed && s.peers[0].seen && s.peers[1].seen && s.keepalive[0] && s.keepalive[1]
}
func (s *binLDP) maxPDULength() uint64 {
	if !s.exchangeObserved() {
		return 4096
	}
	return min(s.peers[0].maximum, s.peers[1].maximum)
}
func ldpIdentity(m map[string]any, receiver bool) string {
	if receiver {
		return fmt.Sprintf("%v:%d", m["Receiver LSR ID"], routingNumber(m, "Receiver Label Space ID"))
	}
	return fmt.Sprintf("%v:%d", m["LSR ID"], routingNumber(m, "Label Space ID"))
}
func (f *binFlow) consumeLDP(dir int, e *ProtocolEvent) error {
	fields, err := stream_parser.DecodeLDPSessionPDU(e.Raw, f.a.budget.MaxCollectionElements)
	e.nativeSessionMessage = true
	e.Profile, e.Admission, e.Completeness = "ldp1-observed-tcp", "complete-service-hinted-PDU-and-message-grammar", "message"
	e.Session = fields
	if err != nil {
		// Framing proved this complete PDU boundary. Keep inspecting later
		// PDUs, but do not carry negotiation through unparsed contents.
		f.ldp.blocked = true
		e.Session = map[string]any{"Error Scope": "message", "Connection State": "negotiation quarantined; malformed or limited complete PDU"}
		e.semanticFields, e.semanticSession = cloneSession(e.Session), cloneSession(e.Session)
		return observedWireError(err)
	}
	fields["Error Scope"] = "message"
	s := f.ldp
	next := *s
	identity := ldpIdentity(fields, false)
	if next.identities[dir] != "" && next.identities[dir] != identity {
		next.blocked = true
		err = protocolError(ErrContextRequired, "LDP sender identifier changed inside captured TCP generation; context quarantined")
	} else {
		next.identities[dir] = identity
	}
	if routingNumber(fields, "PDU Length") > s.maxPDULength() {
		if !s.exchangeObserved() {
			err = protocolError(ErrContextRequired, "LDP large PDU requires observed Initialization and KeepAlive exchange")
		} else {
			err = protocolError(ErrMalformedMessage, "LDP PDU exceeds observed negotiated maximum")
		}
	}
	var names []string
	for _, m := range fields["Messages"].([]map[string]any) {
		typ := routingNumber(m, "Message Type")
		names = append(names, fmt.Sprint(m["Packet Name"]))
		if m["Message Decoded"] == false && err == nil {
			err = protocolError(ErrContextRequired, "LDP unknown message body retained; semantics unavailable")
		}
		if m["Unknown Mandatory Parameters"] == true || m["Unsupported Parameter Value"] == true {
			if typ == 0x200 {
				next.blocked = true
			}
			if err == nil {
				err = protocolError(ErrContextRequired, "LDP unselected mandatory parameter or address/FEC value; wire bytes retained")
			}
		}
		if err != nil {
			continue
		}
		if next.blocked || next.closed && typ != 1 {
			err = protocolError(ErrContextRequired, "LDP negotiation is quarantined or a fatal Notification was observed")
			continue
		}
		switch typ {
		case 0x200:
			if next.peers[dir].seen {
				next.blocked = true
				err = protocolError(ErrContextRequired, "LDP repeated Initialization within one TCP generation; negotiation quarantined")
				continue
			}
			t := m["TLVs"].([]map[string]any)[0]
			maximum := routingNumber(t, "Max PDU Length")
			if maximum <= 255 {
				maximum = 4096
			}
			next.peers[dir] = ldpPeerObservation{seen: true, receiver: ldpIdentity(t, true), keepalive: routingNumber(t, "KeepAlive Time"), maximum: maximum}
		case 0x201:
			if next.peers[0].seen && next.peers[1].seen {
				next.keepalive[dir] = true
			}
		case 1:
			for _, t := range m["TLVs"].([]map[string]any) {
				if routingNumber(t, "TLV Type") == 0x300 && t["Fatal"] == true {
					next.closed = true
				}
			}
		}
		for i, p := range next.peers {
			if p.seen && next.identities[1-i] != "" && p.receiver != next.identities[1-i] {
				next.blocked = true
				err = protocolError(ErrContextRequired, "LDP Initialization receiver identifier conflicts with observed peer; context quarantined")
			}
		}
	}
	// Whole-wire parsing happened before any state mutation. A malformed PDU
	// cannot publish half an Initialization or borrow another flow's identity.
	if err == nil || next.blocked || next.closed {
		*s = next
	}
	state := "midstream; Initialization exchange incomplete"
	if s.peers[0].seen && s.peers[1].seen {
		state = "both Initializations observed; KeepAlive exchange incomplete"
	}
	if s.exchangeObserved() {
		state = "Initialization and KeepAlive exchange observed"
		fields["Observed Maximum PDU Length"], fields["Observed KeepAlive Time"] = s.maxPDULength(), min(s.peers[0].keepalive, s.peers[1].keepalive)
	}
	if s.closed {
		state = "fatal Notification observed; protocol context closed"
	}
	if s.blocked {
		state = "negotiation quarantined"
	}
	fields["Connection State"] = state
	fields["Packet Name"] = fmt.Sprintf("LDP %v", names)
	e.semanticFields, e.semanticSession = cloneSession(fields), cloneSession(fields)
	e.Summary = fields["Packet Name"].(string)
	return observedWireError(err)
}
