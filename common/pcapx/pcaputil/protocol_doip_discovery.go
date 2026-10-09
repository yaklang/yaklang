package pcaputil

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
)

// This independent UDP profile follows AUTOSAR CP R19-11 vehicle identification
// layouts and the pinned python-doipclient v1.1.7 field grammar. Discovery has
// no wire transaction identifier. An announcement is an unverified observation,
// including when it follows a request or reports a configured invalidity pattern.
const doipDiscoveryProjectionBytes int64 = 4096
const doipDiscoveryProfile = "doip-udp-vehicle-discovery-v2-v3"

func doipDiscoveryError(kind ProtocolErrorKind, why string) error {
	return protocolError(kind, "DoIP UDP discovery: %s", why)
}

func doipASCII(w []byte) any {
	for _, b := range w {
		if b >= 128 {
			return nil
		}
	}
	return string(w)
}

func doipDiscoveryFieldCount(w []byte) int {
	if len(w) < 4 {
		return 6
	}
	switch binary.BigEndian.Uint16(w[2:4]) {
	case 2:
		return 7
	case 3:
		return 8
	case 4:
		return 16
	}
	return 6
}

func decodeDoIPDiscovery(w []byte, maxBytes, maxFields int) (map[string]any, error) {
	bad := func(why string) (map[string]any, error) {
		return nil, doipDiscoveryError(ErrMalformedMessage, why)
	}
	if len(w) < 8 {
		return bad("datagram ends before complete eight-byte header")
	}
	if w[0]^w[1] != 255 {
		return bad("inverse protocol version mismatch")
	}
	typ := binary.BigEndian.Uint16(w[2:4])
	if w[0] != 2 && w[0] != 3 && w[0] != 255 || w[0] == 255 && (typ < 1 || typ > 3) {
		return nil, doipDiscoveryError(ErrUnsupportedVersion, "selected v2/v3 or request-only FF default required")
	}
	n := uint64(binary.BigEndian.Uint32(w[4:8]))
	if maxBytes < 8 || n+8 > uint64(maxBytes) || len(w) > maxBytes {
		return nil, doipDiscoveryError(ErrResourceExceeded, "announced or captured datagram exceeds frame/message budget")
	}
	if n != uint64(len(w)-8) {
		return bad("declared payload length differs from exact datagram boundary")
	}
	p := w[8:]
	names := map[uint16]string{1: "VehicleIdentificationRequest", 2: "VehicleIdentificationRequestWithEID", 3: "VehicleIdentificationRequestWithVIN", 4: "VehicleAnnouncementOrIdentificationResponse"}
	name, ok := names[typ]
	if !ok {
		return nil, doipDiscoveryError(ErrUnsupportedFeature, "payload type outside selected UDP identification profile")
	}
	if typ == 1 && len(p) != 0 || typ == 2 && len(p) != 6 || typ == 3 && len(p) != 17 || typ == 4 && len(p) != 32 && len(p) != 33 {
		return bad("identification payload must have exactly 0/6/17/32-or-33 bytes")
	}
	if doipDiscoveryFieldCount(w) > maxFields {
		return nil, doipDiscoveryError(ErrResourceExceeded, "field map exceeds collection budget")
	}
	f := map[string]any{"protocol_version": w[0], "inverse_protocol_version": w[1], "payload_type": typ, "payload_length": n, "payload_hex": hex.EncodeToString(p), "kind": name}
	switch typ {
	case 2:
		f["eid_hex"] = hex.EncodeToString(p)
	case 3:
		f["vin_raw_hex"], f["vin_ascii"] = hex.EncodeToString(p), doipASCII(p)
	case 4:
		f["vin_raw_hex"], f["vin_ascii"] = hex.EncodeToString(p[:17]), doipASCII(p[:17])
		f["logical_address"], f["eid_hex"], f["gid_hex"] = binary.BigEndian.Uint16(p[17:19]), hex.EncodeToString(p[19:25]), hex.EncodeToString(p[25:31])
		f["further_action"] = p[31]
		action := "unknown-or-manufacturer-specific"
		if p[31] == 0 {
			action = "none"
		} else if p[31] == 16 {
			action = "routing-activation-required"
		}
		f["further_action_meaning"] = action
		f["vin_gid_sync_present"], f["vin_gid_sync"], f["vin_gid_sync_meaning"] = len(p) == 33, nil, "absent"
		if len(p) == 33 {
			f["vin_gid_sync"] = p[32]
			meaning := "unknown"
			if p[32] == 0 {
				meaning = "synchronized"
			} else if p[32] == 16 {
				meaning = "incomplete"
			}
			f["vin_gid_sync_meaning"] = meaning
		}
	}
	return f, nil
}

func doipDiscoveryPortEvidence(w []byte, src, dst uint16) bool {
	if (src != 13400 && dst != 13400) || len(w) == 0 || (w[0] != 2 && w[0] != 3 && w[0] != 4 && w[0] != 255) {
		return false
	}
	// A standard UDP port does not turn TCP routing/alive/diagnostic traffic
	// into this native profile. Explicit selection can report those as an
	// unsupported carrier/function without admitting a TCP conversation.
	return len(w) < 4 || binary.BigEndian.Uint16(w[2:4]) >= 1 && binary.BigEndian.Uint16(w[2:4]) <= 4
}

// Short requests need transport context. Only a complete, exact announcement
// layout can establish this profile away from its standard UDP port.
func doipStrongAnnouncement(w []byte) bool {
	return (len(w) == 40 || len(w) == 41) && (w[0] == 2 || w[0] == 3) && w[0]^w[1] == 255 && binary.BigEndian.Uint16(w[2:4]) == 4 && uint64(binary.BigEndian.Uint32(w[4:8]))+8 == uint64(len(w))
}

func (a *binParser) decodeDoIPDiscoveryDatagram(e *ProtocolEvent, w []byte, src, dst uint16, explicit bool) bool {
	if !explicit && !doipDiscoveryPortEvidence(w, src, dst) && !doipStrongAnnouncement(w) {
		return false
	}
	e.Protocol, e.Profile, e.Admission, e.Completeness = "doip", doipDiscoveryProfile, "wire-and-port-hint", "message"
	if explicit {
		e.Admission = "explicit-decode-as"
	} else if !doipDiscoveryPortEvidence(w, src, dst) {
		e.Admission = "wire-signature"
	}
	temp := &binFlow{a: a}
	defer temp.closeSession()
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	var fields map[string]any
	var err error
	if len(w) > limit {
		err = doipDiscoveryError(ErrResourceExceeded, "datagram exceeds frame/message budget")
	} else if a.budget.MaxRecursionDepth < 2 {
		err = doipDiscoveryError(ErrResourceExceeded, "field depth exceeds recursion budget")
	} else if err = temp.reserveSession(doipDiscoveryProjectionBytes + 128*int64(len(w))); err == nil {
		fields, err = decodeDoIPDiscovery(w, limit, a.budget.MaxCollectionElements)
	}
	if err == nil {
		e.semanticFields = cloneSession(fields)
		e.Session = map[string]any{"Association": "unassociated-discovery", "Authentication Verified": false, "Vehicle Identity Verified": false}
	}
	// Do not allocate a raw event copy when the shared projection reservation
	// failed or the captured bytes exceeded the configured limit.
	raw := w
	if len(w) > limit || temp.sessionBytes == 0 {
		raw = nil
	}
	var typed *ProtocolError
	if errors.As(err, &typed) && typed.Kind == ErrResourceExceeded {
		err = typed
	}
	a.finishProtocolDatagram(e, raw, nil, err)
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}

func (s *captureSession) probeDoIPDiscovery(w []byte) (ProbeResult, bool) {
	if !doipDiscoveryPortEvidence(w, s.f.ports[0], s.f.ports[1]) && !doipStrongAnnouncement(w) {
		return ProbeResult{}, false
	}
	a := s.f.a
	temp := &binFlow{a: a}
	defer temp.closeSession()
	var err error
	limit := min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes)
	if len(w) > limit || a.budget.MaxRecursionDepth < 2 {
		err = doipDiscoveryError(ErrResourceExceeded, "probe exceeds configured budget")
	} else if err = temp.reserveSession(doipDiscoveryProjectionBytes + 128*int64(len(w))); err == nil {
		_, err = decodeDoIPDiscovery(w, limit, a.budget.MaxCollectionElements)
	}
	if err != nil {
		return ProbeResult{Verdict: ProbeReject, Reason: err.Error()}, true
	}
	return probeAccept("doip", doipDiscoveryProfile, 98), true
}
