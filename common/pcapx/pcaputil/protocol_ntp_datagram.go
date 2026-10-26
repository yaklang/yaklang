package pcaputil

import "fmt"

func (a *binParser) decodeNTPDatagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit bool) bool {
	if len(w) == 0 {
		return false
	}
	v, mode := w[0]>>3&7, w[0]&7
	hint := src == 123 || dst == 123
	if !explicit {
		if v < 1 || v > 4 || mode < 1 || mode > 6 {
			return false
		}
		if mode == 6 {
			if !hint || v < 2 || w[0]>>6 != 0 || len(w) < 12 || w[2] == 0 && w[3] == 0 || ntpControlOpcode(w[1]&31) == "Reserved" {
				return false
			}
		} else if len(w) < 48 || w[1] > 16 || !hint && (len(w) != 48 || probeNTP(w, len(w)).Verdict != ProbeAccept) {
			return false
		}
	}
	e.Protocol, e.Rule, e.Entry = "ntp", "application-layer.ntp", "NTP"
	e.Profile, e.Completeness, e.Admission = "ntp-native", "message", "complete-wire-and-service-hint"
	if explicit {
		e.Admission = "explicit-decode-as"
	} else if !hint {
		e.Admission = "complete-client-server-wire"
	}
	e.nativeSessionMessage = true
	// UDP events normally receive IDs at emit time. Passive ownership and
	// byte-source witnesses need the final nonzero ID before state advances.
	if e.ID == 0 {
		e.ID = a.ids.Add(1)
	}
	var fields map[string]any
	var err error
	if a.config.MaxMessageBytes > 0 && len(w) > a.config.MaxMessageBytes {
		err = protocolError(ErrResourceExceeded, "NTP datagram exceeds configured byte limit")
	} else {
		fields, err = decodeNTPDatagramWire(w)
	}
	if fields != nil {
		fields["Observation Scope"] = "literal UDP datagram and capture domain; sender authentication is not verified"
		if err == nil && mode == 6 {
			a.observeNTPControl(e, w, fields)
			if why, ok := fields["Control Payload Failure"].(string); ok {
				err = protocolError(ErrMalformedMessage, "NTP control payload: %s", why)
			}
		}
		if err != nil {
			e.Completeness = "headers"
		}
		e.Session, e.semanticFields, e.semanticSession = fields, cloneSession(fields), cloneSession(fields)
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if fields != nil {
		e.Summary = fmt.Sprintf("NTPv%v · %v", fields["Version"], fields["Packet Name"])
		if mode == 6 {
			e.Summary += fmt.Sprintf(" · %v · %v · %v", fields["Control Summary"], fields["Association"], fields["Control Reassembly"])
		} else {
			e.Summary += fmt.Sprintf(" · stratum=%v", fields["Stratum"])
		}
	}
	if err != nil {
		e.Summary = err.Error() + " · " + e.Summary
	}
	return true
}
