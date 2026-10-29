package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

var ErrRSVPWireFormat = errors.New("RSVP wire format failure")
var ErrRSVPWireIntegrity = errors.New("RSVP wire checksum failure")

var rsvpMessageNames = map[uint64]string{1: "Path", 2: "Resv", 3: "PathErr", 4: "ResvErr", 5: "PathTear", 6: "ResvTear", 7: "ResvConf"}
var rsvpObjectNames = map[uint64]string{0: "NULL", 1: "SESSION", 3: "RSVP_HOP", 4: "INTEGRITY", 5: "TIME_VALUES", 6: "ERROR_SPEC", 7: "SCOPE", 8: "STYLE", 9: "FLOWSPEC", 10: "FILTER_SPEC", 11: "SENDER_TEMPLATE", 12: "SENDER_TSPEC", 13: "ADSPEC", 14: "POLICY_DATA", 15: "RESV_CONFIRM"}

func RSVPMessageName(typ uint64) string {
	if n, ok := rsvpMessageNames[typ]; ok {
		return n
	}
	return fmt.Sprintf("Unselected RSVP message type %d", typ)
}
func RSVPObjectName(typ uint64) string {
	if n, ok := rsvpObjectNames[typ]; ok {
		return n
	}
	return fmt.Sprintf("Unselected RSVP object class %d", typ)
}
func rsvpRanges(r *controlReader) map[string]any {
	ranges := routingFieldRanges(r.fields)
	if ranges == nil {
		ranges = map[string]any{}
	}
	for key, span := range rsvpRangeMap(r.info["Wire Field Ranges"]) {
		ranges[key] = span
	}
	r.info["Wire Field Ranges"] = ranges
	return r.info
}

func rsvpRangeMap(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}
func rsvpFormat(prior error, text string) error {
	return errors.Join(prior, fmt.Errorf("%w: %s", ErrRSVPWireFormat, text))
}

func rsvpOwned(result map[string]any, key string, w []byte, start, end int) {
	result[key] = bytes.Clone(w[start:end])
	ranges, _ := result["Wire Field Ranges"].(map[string]any)
	if ranges == nil {
		ranges = map[string]any{}
		result["Wire Field Ranges"] = ranges
	}
	ranges[key] = []any{uint64(start), uint64(end)}
}

// RSVPChecksum covers exactly the declared RSVP extent, without a pseudo-header.
// A transmitted zero means omitted; this arithmetic helper does not authenticate
// an INTEGRITY object, sender, reservation or installed flow.
func RSVPChecksum(w []byte) uint16 {
	if len(w) < 8 {
		return 0xffff
	}
	n := int(binary.BigEndian.Uint16(w[6:8]))
	if n < 8 || n > len(w) {
		return 0xffff
	}
	return ospfv2Sum(w[:n])
}

// DecodeRSVPWire observes v1 messages1..7 and selected base/Intserv layouts.
// Unknown objects retain owned bytes. Advertised SESSION selectors and
// HOP handles never create captured transports, confirmed reservations or peers.
func DecodeRSVPWire(w []byte, budget int) (map[string]any, error) {
	if len(w) > 65535 || budget < 0 {
		return nil, ErrObservedWireBudget
	}
	result := map[string]any{"Authentication Verified": false, "Reservation State Inferred": false, "Procedure Semantics Verified": false, "Wire Grammar Complete": false, "Field Layouts Complete": false, "Observation Scope": "bounded RSVP wire observations; advertisements are not captured flows, verified reservations, neighbors, replay decisions or authentication"}
	result["Required Captured Bytes"] = uint64(8)
	h := &controlReader{w: w, info: map[string]any{}}
	vf := h.num("Version and Flags", 1, false)
	if h.err == nil {
		h.info["Version"] = vf >> 4
		if vf>>4 != 1 {
			result["Header"] = rsvpRanges(h)
			return result, fmt.Errorf("%w: RSVP version unselected", ErrObservedWireContext)
		}
		h.info["Flags"] = vf & 15
	}
	typ := h.num("Message Type", 1, false)
	sum := h.num("Checksum", 2, false)
	h.num("Send TTL", 1, false)
	h.num("Reserved", 1, false)
	length := h.num("Message Length", 2, false)
	if _, ok := h.info["Message Type"]; ok {
		result["Message Name"] = RSVPMessageName(typ)
	}
	if len(h.info) != 0 {
		result["Header"] = rsvpRanges(h)
	}
	if h.err != nil {
		return result, h.err
	}
	n := int(length)
	result["Required Captured Bytes"] = length
	result["Checksum Omitted"] = sum == 0
	result["Checksum Verified"] = false
	if n < 8 {
		return result, rsvpFormat(nil, "message length is below fixed common header")
	}
	var aggregate error
	if n%4 != 0 {
		aggregate = rsvpFormat(aggregate, "message length is not a multiple of four")
	}
	if n <= len(w) && sum != 0 {
		verified := RSVPChecksum(w) == 0
		result["Checksum Verified"] = verified
		if !verified {
			aggregate = errors.Join(aggregate, ErrRSVPWireIntegrity)
		}
	}
	if n < len(w) {
		rsvpOwned(result, "Trailing Bytes", w, n, len(w))
		aggregate = rsvpFormat(aggregate, "bytes follow the declared RSVP message")
	}
	if _, selected := rsvpMessageNames[typ]; !selected {
		rsvpOwned(result, "Unparsed Body", w, 8, min(n, len(w)))
		if n > len(w) {
			h.need(false, "declared RSVP message exceeds captured payload")
		}
		return result, errors.Join(aggregate, h.err, fmt.Errorf("%w: RSVP message layout unselected; body not interpreted as objects", ErrObservedWireContext))
	}
	r := &controlReader{w: w[:min(n, len(w))], at: 8, info: map[string]any{}}
	var objects []map[string]any
	layoutsComplete := true
	for r.at < len(r.w) && r.err == nil {
		if !ospfv3Charge(r, &budget) {
			break
		}
		c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
		start := c.at
		size := c.num("Length", 2, false)
		class := c.num("Class", 1, false)
		ctype := c.num("C-Type", 1, false)
		headerObserved := len(c.fields) != 0
		row := map[string]any{"Header": rsvpRanges(c)}
		if _, ok := c.info["Class"]; ok {
			row["Object Name"] = RSVPObjectName(class)
		}
		if c.err == nil {
			end := start + int(size)
			if size < 4 || size%4 != 0 {
				c.err = rsvpFormat(nil, "object length/alignment exceeds its declared message")
			} else {
				var boundaryError error
				if end > n {
					boundaryError = rsvpFormat(nil, "object declared end exceeds RSVP declared extent")
				}
				value := &controlReader{w: r.w[:min(end, len(r.w))], at: c.at, info: map[string]any{}}
				selected := rsvpObjectValue(value, class, ctype, int(size)-4, &budget)
				row["Value Captured Complete"] = end <= len(r.w)
				row["Source Layout Selected"] = selected
				layoutsComplete = layoutsComplete && selected
				row["Body"] = rsvpRanges(value)
				c.at, c.err = value.at, errors.Join(boundaryError, value.err)
				if end > len(r.w) && c.err == nil {
					c.need(false, "object extent exceeds captured message")
				}
				if c.err != nil && end <= len(r.w) && !errors.Is(c.err, ErrObservedWireBudget) && !errors.Is(c.err, ErrObservedWireContext) && !errors.Is(c.err, ErrRSVPWireFormat) {
					c.err = rsvpFormat(c.err, "complete object contradicts selected value layout")
				}
			}
		}
		if headerObserved {
			objects = append(objects, row)
		}
		r.at, r.err = c.at, c.err
	}
	result["Objects"] = objects
	if r.err != nil && n <= len(w) && !errors.Is(r.err, ErrObservedWireBudget) && !errors.Is(r.err, ErrObservedWireContext) && !errors.Is(r.err, ErrRSVPWireFormat) {
		r.err = rsvpFormat(r.err, "complete message contains an incomplete object")
	}
	if n > len(w) {
		r.need(false, "declared RSVP message exceeds captured payload")
	}
	if r.at < len(r.w) {
		rsvpOwned(result, "Unparsed Body", r.w, r.at, len(r.w))
	}
	aggregate = errors.Join(aggregate, r.err)
	result["Wire Grammar Complete"] = aggregate == nil
	result["Field Layouts Complete"] = aggregate == nil && layoutsComplete
	return result, aggregate
}

func rsvpObjectValue(r *controlReader, class, ctype uint64, n int, budget *int) bool {
	if ctype == 2 && (class == 9 || class == 12 || class == 13) {
		return rsvpIntservValue(r, n, class, budget)
	}
	width := 4
	if ctype == 2 {
		width = 16
	}
	selected := false
	expected := -1
	if ctype == 1 || ctype == 2 {
		switch class {
		case 1:
			selected = true
			expected = width + 4
			routingAddress(r, "Destination Address", width)
			r.num("Protocol ID", 1, false)
			r.num("Flags", 1, false)
			r.num("Destination Port", 2, false)
		case 3:
			selected = true
			expected = width + 4
			routingAddress(r, "Hop Address", width)
			r.num("Logical Interface Handle", 4, false)
		case 6:
			selected = true
			expected = width + 4
			routingAddress(r, "Error Node Address", width)
			r.num("Flags", 1, false)
			r.num("Error Code", 1, false)
			r.num("Error Value", 2, false)
		case 7:
			selected = true
			var lengthError error
			if n%width != 0 {
				lengthError = rsvpFormat(nil, "SCOPE address list length is inconsistent with its address family")
			}
			var addresses []map[string]any
			for r.err == nil && r.at+width <= len(r.w) && ospfv3Charge(r, budget) {
				c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
				routingAddress(c, "Address", width)
				if len(c.info) != 0 {
					addresses = append(addresses, rsvpRanges(c))
				}
				r.at, r.err = c.at, c.err
			}
			r.info["Addresses"] = addresses
			if r.err == nil && r.at < len(r.w) {
				r.rest("Unselected Address Tail")
				if lengthError == nil {
					r.need(false, "captured SCOPE address prefix is incomplete")
				}
			}
			r.err = errors.Join(lengthError, r.err)
		case 10, 11:
			selected = true
			expected = width + 4
			routingAddress(r, "Source Address", width)
			r.num("Unused", 2, false)
			r.num("Source Port", 2, false)
		case 15:
			selected = true
			expected = width
			routingAddress(r, "Receiver Address", width)
		}
	}
	if ctype == 1 {
		switch class {
		case 5:
			selected = true
			expected = 4
			r.num("Refresh Period Milliseconds", 4, false)
		case 8:
			selected = true
			expected = 4
			r.num("Flags", 1, false)
			r.num("Option Vector", 3, false)
		}
	}
	if class == 0 && ctype == 0 {
		selected = true
		r.rest("Ignored NULL Data")
	}
	if !selected {
		r.rest("Opaque Body")
		r.info["Observation Scope"] = "unselected object value layout; opaque bytes only"
	}
	if expected >= 0 && n != expected {
		if r.err == nil && r.at < len(r.w) {
			r.rest("Unselected Value Tail")
		}
		r.err = rsvpFormat(r.err, "object length contradicts its selected base class/C-Type")
	}
	return selected
}
