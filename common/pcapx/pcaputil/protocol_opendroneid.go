package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// This selected profile decodes protocol-version 2 Basic ID and Location wire
// messages, individually or in a message pack. UDP is an explicit test/tunnel
// carrier, not a standard Remote ID broadcast transport or identity proof.
const openDroneIDSystemProfile = "opendroneid-v2-basic-location-system"
const openDroneIDIdentityProfile = "opendroneid-v2-identity-observation"
const openDroneIDAuthProfile = "opendroneid-v2-auth-pages-observation"

type openDroneIDSelection uint8

const (
	odidBasicLocation openDroneIDSelection = iota
	odidSystem
	odidIdentity
	odidAuthPages
)

func odidSelection(systemProfile bool) openDroneIDSelection {
	if systemProfile {
		return odidSystem
	}
	return odidBasicLocation
}

func openDroneIDLayout(w []byte, maxBytes, maxMessages int) (int, int, error) {
	return openDroneIDSelectedLayout(w, maxBytes, maxMessages, false)
}

// The System extension is separately selected. Legacy Basic ID/Location
// admission and its unsupported-message errors retain their original meaning.
func openDroneIDSelectedLayout(w []byte, maxBytes, maxMessages int, systemProfile bool) (int, int, error) {
	return openDroneIDProfileLayout(w, maxBytes, maxMessages, odidSelection(systemProfile))
}

func openDroneIDProfileLayout(w []byte, maxBytes, maxMessages int, selection openDroneIDSelection) (int, int, error) {
	fail := func(k ProtocolErrorKind, s string) (int, int, error) {
		return 0, 0, protocolError(k, "OpenDroneID %s", s)
	}
	if len(w) > maxBytes {
		return fail(ErrResourceExceeded, "wire exceeds byte budget")
	}
	if len(w) == 0 {
		return fail(ErrNeedMore, "empty datagram")
	}
	if w[0]&15 != 2 {
		return fail(ErrUnsupportedVersion, "selected profile requires protocol version 2")
	}
	at, count := 0, 1
	if w[0]>>4 == 15 {
		if len(w) < 3 {
			return fail(ErrNeedMore, "message pack header is truncated")
		}
		if w[1] != 25 || w[2] == 0 || w[2] > 9 {
			return fail(ErrMalformedMessage, "message pack size/count is invalid")
		}
		at, count = 3, int(w[2])
	}
	if count > maxMessages {
		return fail(ErrResourceExceeded, "message count exceeds collection budget")
	}
	if len(w) < at+count*25 {
		return fail(ErrNeedMore, "message is truncated within this datagram")
	}
	if len(w) != at+count*25 {
		return fail(ErrMalformedMessage, "trailing bytes are outside declared message pack")
	}
	basic, location, system, selfID, operatorID, auth := 0, 0, 0, 0, 0, 0
	for i := 0; i < count; i++ {
		h := w[at+i*25]
		if h&15 != 2 {
			return fail(ErrUnsupportedVersion, "inner message protocol version differs")
		}
		switch h >> 4 {
		case 0:
			basic++
		case 1:
			location++
		case 2:
			if selection != odidAuthPages {
				return fail(ErrUnsupportedFeature, "message type is outside Basic ID/Location")
			}
			auth++
			p := w[at+i*25 : at+(i+1)*25]
			// Page zero alone carries the declared whole-auth length. The
			// pinned 16-page codec limits that length by page capacity. Later
			// pages contain only opaque bytes; never borrow another page's
			// metadata or infer an authenticated aircraft/session association.
			if p[1]&15 == 0 && (p[2] >= 16 || int(p[3]) > 17+23*int(p[2])) {
				return fail(ErrMalformedMessage, "authentication page zero exceeds declared page capacity")
			}
		case 3, 5:
			if selection != odidIdentity && selection != odidAuthPages {
				return fail(ErrUnsupportedFeature, "message type is outside Basic ID/Location")
			}
			if h>>4 == 3 {
				selfID++
			} else {
				operatorID++
			}
		case 4:
			if selection == odidBasicLocation {
				return fail(ErrUnsupportedFeature, "message type is outside Basic ID/Location")
			}
			system++
		case 15:
			return fail(ErrMalformedMessage, "nested packs are forbidden")
		default:
			return fail(ErrUnsupportedFeature, "message type is outside Basic ID/Location")
		}
	}
	// The pinned reference's default pack contract permits two Basic IDs and
	// one Location. Preserve their order; never merge conflicting observations.
	if basic > 2 || location > 1 || system > 1 || selfID > 1 || operatorID > 1 {
		return fail(ErrMalformedMessage, "pack repeats a message beyond selected cardinality")
	}
	if selection == odidSystem && system != 1 {
		return fail(ErrUnsupportedFeature, "System profile requires one System observation")
	}
	if selection == odidIdentity && selfID+operatorID == 0 {
		return fail(ErrUnsupportedFeature, "identity profile requires a Self ID or Operator ID observation")
	}
	if selection == odidAuthPages && auth == 0 {
		return fail(ErrUnsupportedFeature, "authentication profile requires an Authentication page observation")
	}
	return at, count, nil
}

func decodeOpenDroneID(w []byte, maxBytes, maxMessages int) (map[string]any, error) {
	return decodeOpenDroneIDSelected(w, maxBytes, maxMessages, false)
}

func decodeOpenDroneIDSelected(w []byte, maxBytes, maxMessages int, systemProfile bool) (map[string]any, error) {
	return decodeOpenDroneIDProfile(w, maxBytes, maxMessages, odidSelection(systemProfile))
}

func decodeOpenDroneIDProfile(w []byte, maxBytes, maxMessages int, selection openDroneIDSelection) (map[string]any, error) {
	return decodeOpenDroneIDProfileBudget(w, maxBytes, maxMessages, DefaultParserBudget().MaxRecursionDepth, selection)
}

func decodeOpenDroneIDProfileBudget(w []byte, maxBytes, maxMessages, maxDepth int, selection openDroneIDSelection) (map[string]any, error) {
	at, count, err := openDroneIDProfileLayout(w, maxBytes, maxMessages, selection)
	if err != nil {
		return nil, err
	}
	// The root observation contains one level of per-message field maps.
	// Match the scalar native projection depth contract before expanding any
	// complete prefix. Message packs add siblings, not recursive field levels.
	if maxDepth < 2 {
		return nil, protocolError(ErrResourceExceeded, "OpenDroneID field projection exceeds configured depth")
	}
	messages := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		p := w[at+i*25 : at+(i+1)*25]
		f := map[string]any{"Protocol Version": p[0] & 15, "Message Type": p[0] >> 4, "Raw": bytes.Clone(p)}
		if p[0]>>4 == 0 {
			f["Message Name"], f["ID Type"], f["UA Type"] = "Basic ID", p[1]>>4, p[1]&15
			// IDs may be binary. Reserved enums/bytes are observations, not an
			// authenticated registration, serial number, or a future-type guess.
			f["ID"], f["Reserved"] = bytes.Clone(p[2:22]), bytes.Clone(p[22:25])
		} else if p[0]>>4 == 2 {
			f["Message Name"], f["Authentication Type"], f["Data Page"] = "Authentication", p[1]>>4, p[1]&15
			f["Page Zero"] = p[1]&15 == 0
			if p[1]&15 == 0 {
				f["Last Page Index"], f["Authentication Length"] = p[2], p[3]
				f["Timestamp"] = binary.LittleEndian.Uint32(p[4:8])
				f["Authentication Data"] = bytes.Clone(p[8:25])
			} else {
				f["Authentication Data"] = bytes.Clone(p[2:25])
			}
		} else if p[0]>>4 == 3 || p[0]>>4 == 5 {
			// The reference codec uses NUL-terminated byte strings. Preserve
			// both that decoded observation and every original wire byte;
			// neither charset conversion nor registration validation is implied.
			text := func(w []byte) []byte {
				if end := bytes.IndexByte(w, 0); end >= 0 {
					w = w[:end]
				}
				return bytes.Clone(w)
			}
			if p[0]>>4 == 3 {
				f["Message Name"], f["Description Type"] = "Self ID", p[1]
				f["Description"], f["Description Wire"] = text(p[2:25]), bytes.Clone(p[2:25])
			} else {
				f["Message Name"], f["Operator ID Type"] = "Operator ID", p[1]
				f["Operator ID"], f["Operator ID Wire"] = text(p[2:22]), bytes.Clone(p[2:22])
				f["Reserved"] = bytes.Clone(p[22:25])
			}
		} else if p[0]>>4 == 4 {
			u16 := func(at int) uint16 { return binary.LittleEndian.Uint16(p[at:]) }
			// Preserve wire values, reserved classifications and unsigned time.
			// These are unverified observations, not a validated operating area
			// or authenticated operator identity.
			f["Message Name"] = "System"
			f["Operator Location Type"], f["Classification Type"] = p[1]&3, (p[1]>>2)&7
			f["Operator Latitude"] = float64(int32(binary.LittleEndian.Uint32(p[2:6]))) / 1e7
			f["Operator Longitude"] = float64(int32(binary.LittleEndian.Uint32(p[6:10]))) / 1e7
			f["Area Count"], f["Area Radius"] = u16(10), uint16(p[12])*10
			f["Area Ceiling"], f["Area Floor"] = float64(u16(13))*0.5-1000, float64(u16(15))*0.5-1000
			f["Category EU"], f["Class EU"] = p[17]>>4, p[17]&15
			f["Operator Altitude Geo"] = float64(u16(18))*0.5 - 1000
			f["Timestamp"] = binary.LittleEndian.Uint32(p[20:24])
			f["Reserved Bits"], f["Reserved Octet"] = p[1]>>5, p[24]
		} else {
			u16 := func(at int) uint16 { return binary.LittleEndian.Uint16(p[at:]) }
			latitude := float64(int32(binary.LittleEndian.Uint32(p[5:9]))) / 1e7
			longitude := float64(int32(binary.LittleEndian.Uint32(p[9:13]))) / 1e7
			direction := float64(p[2]) + float64((p[1]>>1)&1)*180
			speed := float64(p[3]) * 0.25
			if p[1]&1 != 0 {
				speed = float64(p[3])*0.75 + 63.75
			}
			// The pinned endpoint exposes this measurement as float32. Keep
			// its exact numeric projection, including the unknown sentinel.
			stamp := float64(float32(u16(21)) / 10)
			if u16(21) == 65535 {
				stamp = 65535
			}
			f["Message Name"], f["Status"], f["Height Type"] = "Location", p[1]>>4, (p[1]>>2)&1
			f["Direction"], f["Speed Horizontal"], f["Speed Vertical"] = direction, speed, float64(int8(p[4]))*0.5
			f["Latitude"], f["Longitude"] = latitude, longitude
			f["Altitude Baro"], f["Altitude Geo"], f["Height"] = float64(u16(13))*0.5-1000, float64(u16(15))*0.5-1000, float64(u16(17))*0.5-1000
			f["Horizontal Accuracy"], f["Vertical Accuracy"] = p[19]&15, p[19]>>4
			f["Speed Accuracy"], f["Baro Accuracy"] = p[20]&15, p[20]>>4
			f["Timestamp"], f["Timestamp Accuracy"] = stamp, p[23]&15
			f["Reserved Bit"], f["Reserved Nibble"], f["Reserved Octet"] = (p[1]>>3)&1, p[23]>>4, p[24]
		}
		messages = append(messages, f)
	}
	return map[string]any{"Protocol Version": 2, "Message Count": count, "Message Pack": at == 3, "Messages": messages, "Observation": "unverified-remote-id-wire-values"}, nil
}

func (a *binParser) decodeOpenDroneIDDatagram(e *ProtocolEvent, w []byte) bool {
	return a.decodeOpenDroneIDSelectedDatagram(e, w, false)
}

func (a *binParser) decodeOpenDroneIDSelectedDatagram(e *ProtocolEvent, w []byte, systemProfile bool) bool {
	return a.decodeOpenDroneIDProfileDatagram(e, w, odidSelection(systemProfile))
}

func (a *binParser) decodeOpenDroneIDProfileDatagram(e *ProtocolEvent, w []byte, selection openDroneIDSelection) bool {
	e.Protocol, e.Profile, e.Admission, e.Completeness = "opendroneid", "opendroneid-v2-basic-location", "explicit-decode-as", "message"
	switch selection {
	case odidSystem:
		e.Profile = openDroneIDSystemProfile
	case odidIdentity:
		e.Profile = openDroneIDIdentityProfile
	case odidAuthPages:
		e.Profile = openDroneIDAuthProfile
	}
	// Reserve expansion before constructing any field tree. A stateless
	// message owns no conversation, pending request, or inferred aircraft.
	f := &binFlow{a: a}
	err := f.reserveSession(512 + int64(len(w))*6)
	defer f.closeSession()
	if err == nil {
		e.Session, err = decodeOpenDroneIDProfileBudget(w, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes), a.budget.MaxCollectionElements, a.budget.MaxRecursionDepth, selection)
	}
	if err == nil {
		e.semanticFields = cloneSession(e.Session)
	}
	if len(w) > min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes) {
		w = nil
	}
	a.finishProtocolDatagram(e, w, nil, err)
	if err != nil {
		e.Completeness = e.Status
	}
	return true
}
