package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// UDP already supplies message boundaries. Do not feed these bytes through TCP
// framing or keep a process-wide, unbounded map of guessed UDP conversations.
// Packet fields are decoded here; response association requires a caller session.
func (a *binParser) decodeSessionDatagram(e *ProtocolEvent, wire []byte) bool {
	var spec *binSpec
	var session map[string]any
	var err error
	switch {
	case a.decodeSTUNDatagram(e, wire):
		return true
	case a.decodeTFTPDatagram(e, wire):
		return true
	case len(wire) >= 240 && (wire[0] == 1 || wire[0] == 2) && binary.BigEndian.Uint32(wire[236:240]) == dhcpCookie:
		e.Protocol, spec = "dhcp", a.specs["application-layer.dhcp/DHCP"]
		session, err = decodeDHCP4(wire, a.budget.MaxCollectionElements)
	case probeRADIUS(wire, len(wire)).Verdict == ProbeAccept:
		e.Protocol, spec = "radius", a.specs["application-layer.radius/RADIUS"]
		session, err = (&binRADIUS{}).consume(wire, a.budget.MaxCollectionElements)
	case probeNTP(wire, len(wire)).Verdict == ProbeAccept:
		e.Protocol, spec = "ntp", a.specs["application-layer.ntp/NTP"]
		if len(wire) != 48 {
			err = protocolError(ErrUnsupportedFeature, "NTP extensions/authentication require an unsupported profile")
		} else {
			session, err = (&binNTP{}).consume(wire)
		}
	case probeCoAP(wire, len(wire)).Verdict == ProbeAccept:
		e.Protocol, spec = "coap", a.specs["application-layer.extended_protocols/CoAP"]
		session, err = (&binCoAP{maxPending: a.budget.MaxCollectionElements}).consume(0, wire)
	default:
		return false
	}
	e.Raw = append([]byte(nil), wire...)
	e.Status, e.Summary = "deferred", e.Protocol
	a.messages.Add(1)
	a.messageBytes.Add(uint64(len(wire)))
	if spec == nil && err == nil {
		err = fmt.Errorf("missing %s datagram rule", e.Protocol)
	}
	if spec != nil {
		e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
	}
	if err == nil {
		e.Session = session
		e.Session["Observation Scope"] = "datagram"
		if e.Protocol == "dhcp" {
			e.Completeness = "message"
			a.dhcpObservation(e)
		}
		if a.config.Deferred {
			a.deferred.Add(1)
			return true
		}
		e.Structured, err = e.Decode()
	}
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = err.Error()
		switch e.Status {
		case "limited":
			a.limited.Add(uint64(len(wire)))
		case "context-required":
			a.contextRequired.Add(1)
		default:
			a.malformed.Add(1)
		}
	} else {
		e.Status = "decoded"
		a.decoded.Add(1)
	}
	return true
}
