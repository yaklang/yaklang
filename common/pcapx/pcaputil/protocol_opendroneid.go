package pcaputil

import (
	"bytes"
	"encoding/binary"
)

// This selected profile decodes protocol-version 2 Basic ID and Location wire
// messages, individually or in a message pack. UDP is an explicit test/tunnel
// carrier, not a standard Remote ID broadcast transport or identity proof.
func openDroneIDLayout(w []byte, maxBytes, maxMessages int) (int, int, error) {
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
	basic, location := 0, 0
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
		case 15:
			return fail(ErrMalformedMessage, "nested packs are forbidden")
		default:
			return fail(ErrUnsupportedFeature, "message type is outside Basic ID/Location")
		}
	}
	// The pinned reference's default pack contract permits two Basic IDs and
	// one Location. Preserve their order; never merge conflicting observations.
	if basic > 2 || location > 1 {
		return fail(ErrMalformedMessage, "pack repeats a message beyond selected cardinality")
	}
	return at, count, nil
}

func decodeOpenDroneID(w []byte, maxBytes, maxMessages int) (map[string]any, error) {
	at, count, err := openDroneIDLayout(w, maxBytes, maxMessages)
	if err != nil {
		return nil, err
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
	e.Protocol, e.Profile, e.Admission, e.Completeness = "opendroneid", "opendroneid-v2-basic-location", "explicit-decode-as", "message"
	// Reserve expansion before constructing any field tree. A stateless
	// message owns no conversation, pending request, or inferred aircraft.
	f := &binFlow{a: a}
	err := f.reserveSession(512 + int64(len(w))*6)
	defer f.closeSession()
	if err == nil {
		e.Session, err = decodeOpenDroneID(w, min(a.budget.MaxFrameBytes, a.config.MaxMessageBytes), a.budget.MaxCollectionElements)
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
