package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const bacnetIPv4UDPPort = 47808

type bacnetIPEnvelope struct {
	bvlcFunction uint8
	npduControl  uint8
	apduOffset   int
	apduType     uint8
	service      uint8
	serviceName  string
}

// inspectBACnetIP recognizes the supported BACnet/IP original-unicast and
// original-broadcast envelope. The UDP port narrows candidates, but these
// complete BVLC, NPDU, and APDU checks decide whether the datagram is admitted.
func inspectBACnetIP(wire []byte) (bacnetIPEnvelope, bool) {
	var out bacnetIPEnvelope
	if len(wire) < 8 || wire[0] != 0x81 {
		return out, false
	}

	function := wire[1]
	if function != 0x0a && function != 0x0b {
		return out, false
	}
	length := int(binary.BigEndian.Uint16(wire[2:4]))
	if length != len(wire) {
		return out, false
	}
	if wire[4] != 0x01 {
		return out, false
	}

	control := wire[5]
	// This profile uses the rule's fixed two-byte NPDU header. Routed packets
	// and network-layer messages need a different NPDU layout and stay outside
	// this recognition slice rather than being mislabeled as APDU bytes.
	if control&(0x80|0x20|0x01) != 0 {
		return out, false
	}
	at := 6
	if at >= len(wire) {
		return out, false
	}

	apdu := wire[at:]
	pduType := apdu[0] >> 4
	segmented := apdu[0]&0x08 != 0
	if !segmented && apdu[0]&0x04 != 0 {
		return out, false
	}
	serviceOffset := -1
	switch pduType {
	case 0x0: // Confirmed-Request.
		if apdu[0]&0x01 != 0 {
			return out, false
		}
		serviceOffset = 3
		if segmented {
			serviceOffset = 5
		}
	case 0x1: // Unconfirmed-Request.
		if apdu[0]&0x0f != 0 {
			return out, false
		}
		serviceOffset = 1
	case 0x2: // SimpleACK.
		if apdu[0]&0x0f != 0 {
			return out, false
		}
		serviceOffset = 2
	case 0x3: // ComplexACK.
		if apdu[0]&0x03 != 0 {
			return out, false
		}
		serviceOffset = 2
		if segmented {
			serviceOffset = 4
		}
	default:
		// SegmentACK, Error, Reject, Abort, and reserved PDU types are outside
		// this M0 service-selection profile.
		return out, false
	}
	if len(apdu) <= serviceOffset {
		return out, false
	}

	out = bacnetIPEnvelope{
		bvlcFunction: function,
		npduControl:  control,
		apduOffset:   at,
		apduType:     pduType,
		service:      apdu[serviceOffset],
	}
	out.serviceName = bacnetIPServiceName(pduType, out.service)
	return out, true
}

func bacnetIPServiceName(pduType, service uint8) string {
	switch pduType {
	case 0x1: // Unconfirmed service choices.
		switch service {
		case 0:
			return "I-Am"
		case 8:
			return "Who-Is"
		}
	case 0x0, 0x2, 0x3: // Confirmed service choices and their responses.
		switch service {
		case 12:
			return "ReadProperty"
		case 15:
			return "WriteProperty"
		}
	}
	return ""
}

func (a *binParser) decodeBACnetDatagram(e *ProtocolEvent, wire []byte) bool {
	envelope, ok := inspectBACnetIP(wire)
	if !ok {
		return false
	}
	spec := a.specs["application-layer.extended_protocols/BACnetIP"]
	if spec == nil {
		return false
	}

	e.Protocol = "bacnet"
	e.Profile = "bacnet-ip-envelope"
	e.Admission = "wire-and-port-hint"
	e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
	e.Raw = bytes.Clone(wire)
	e.Length = len(wire)
	e.Completeness = "message"
	e.Summary = "BACnet/IP"
	e.Status = "deferred"
	e.Session = map[string]any{
		"BVLC Function":  envelope.bvlcFunction,
		"NPDU Control":   envelope.npduControl,
		"APDU Type":      bacnetPDUTypeName(envelope.apduType),
		"Service Choice": envelope.service,
		"APDU":           bytes.Clone(wire[envelope.apduOffset:]),
	}
	if envelope.serviceName != "" {
		e.Session["Service"] = envelope.serviceName
		e.Summary = fmt.Sprintf("BACnet/IP %s", envelope.serviceName)
	}

	a.messages.Add(1)
	a.messageBytes.Add(uint64(len(wire)))
	result, err := e.Decode()
	if err != nil {
		e.Status, e.Completeness = "malformed", "malformed"
		e.ExpertCode, e.Error = "BACnetDecodeFailed", err.Error()
		a.malformed.Add(1)
		return true
	}
	if fields := protocolFields(result); fields != nil {
		fields["APDU Type"] = bacnetPDUTypeName(envelope.apduType)
		fields["Service Choice"] = envelope.service
		if envelope.serviceName != "" {
			fields["Service"] = envelope.serviceName
		}
		fields["Complete BVLC PDU"] = bytes.Clone(wire)
	}
	if a.config.Deferred {
		e.Structured = nil
		a.deferred.Add(1)
	} else {
		e.Structured = result
		e.Status = "decoded"
		a.decoded.Add(1)
	}
	return true
}

func bacnetPDUTypeName(pduType uint8) string {
	switch pduType {
	case 0x0:
		return "Confirmed-Request"
	case 0x1:
		return "Unconfirmed-Request"
	case 0x2:
		return "SimpleACK"
	case 0x3:
		return "ComplexACK"
	default:
		return fmt.Sprintf("PDU-%d", pduType)
	}
}
