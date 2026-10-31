package stream_parser

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
)

func rsvpIntservTail(r *controlReader, owner map[string]any, name string) {
	if r.at < len(r.w) {
		rsvpOwned(owner, name, r.w, r.at, len(r.w))
		r.at = len(r.w)
	}
}

// Each RFC2210 length counts data words after its own four-byte header.
// The object, Intserv message, service and parameter remain separate owners.
func rsvpIntservValue(r *controlReader, n int, class uint64, budget *int) bool {
	start := r.at
	h := &controlReader{w: r.w, at: start, info: map[string]any{}}
	v := h.num("Version and Unused", 2, false)
	words := h.num("Message Length Words", 2, false)
	if _, ok := h.info["Version and Unused"]; ok {
		h.info["Version"], h.info["Unused Bits"] = v>>12, v&4095
	}
	if len(h.fields) != 0 {
		r.info["Intserv Header"] = rsvpRanges(h)
	}
	r.at, r.err = h.at, h.err
	if r.err != nil {
		rsvpIntservTail(r, r.info, "Unparsed Intserv Data")
		return true
	}
	if v>>12 != 0 {
		// A future message format cannot authorize v0 word-count semantics.
		// The independently proved RSVP object remains the opaque owner.
		rsvpIntservTail(r, r.info, "Opaque Intserv Data")
		r.err = fmt.Errorf("%w: Intserv message format version unselected", ErrObservedWireContext)
		return false
	}
	end := start + 4 + 4*int(words)
	var boundary error
	if end != start+n {
		boundary = rsvpFormat(nil, "Intserv message word length contradicts its RSVP object extent")
	}
	limit := min(end, len(r.w))
	selected := true
	var services []map[string]any
	c := &controlReader{w: r.w[:limit], at: r.at, info: map[string]any{}}
	for c.at < len(c.w) && c.err == nil {
		if !ospfv3Charge(c, budget) {
			break
		}
		service, complete := rsvpIntservService(c, end, class, budget)
		services = append(services, service)
		selected = selected && complete
	}
	r.info["Services"] = services
	rsvpIntservTail(c, r.info, "Unparsed Intserv Data")
	r.at, r.err = c.at, errors.Join(boundary, c.err)
	if end > len(r.w) {
		r.need(false, "Intserv message exceeds captured object")
	}
	rsvpIntservTail(r, r.info, "Trailing Intserv Bytes")
	return selected
}

func rsvpIntservServiceName(class, number uint64) (string, bool) {
	switch {
	case class == 12 && number == 1:
		return "Global Sender Traffic Specification", true
	case class == 9 && number == 5:
		return "Controlled-Load Receiver Traffic Specification", true
	case class == 13 && number == 1:
		return "Default General Characterization Parameters", true
	case class == 13 && number == 5:
		return "Controlled-Load Characterization", true
	case class == 9 && number == 2:
		return "Guaranteed Receiver Traffic Specification (unselected)", false
	case class == 13 && number == 2:
		return "Guaranteed Characterization (unselected)", false
	}
	return fmt.Sprintf("Unselected service %d in %s", number, RSVPObjectName(class)), false
}

func rsvpIntservService(r *controlReader, parentEnd int, class uint64, budget *int) (map[string]any, bool) {
	start := r.at
	h := &controlReader{w: r.w, at: start, info: map[string]any{}}
	number := h.num("Service Number", 1, false)
	flags := h.num("Flags", 1, false)
	words := h.num("Data Length Words", 2, false)
	if _, ok := h.info["Flags"]; ok {
		h.info["Break Bit Observed"], h.info["Reserved Flag Bits"] = flags&128 != 0, flags&127
	}
	row := map[string]any{"Header": rsvpRanges(h)}
	name, selected := rsvpIntservServiceName(class, number)
	row["Service Name"], row["Source Layout Selected"] = name, selected
	r.at, r.err = h.at, h.err
	if r.err != nil {
		rsvpIntservTail(r, row, "Unparsed Service Data")
		return row, selected
	}
	end := start + 4 + 4*int(words)
	var boundary error
	if end > parentEnd {
		boundary = rsvpFormat(nil, "Intserv service exceeds its declared message")
	}
	c := &controlReader{w: r.w[:min(end, len(r.w))], at: r.at, info: map[string]any{}}
	if !selected {
		c.rest("Opaque Service Data")
		row["Body"] = rsvpRanges(c)
	} else {
		var parameters []map[string]any
		for c.at < len(c.w) && c.err == nil {
			if !ospfv3Charge(c, budget) {
				break
			}
			parameter, complete := rsvpIntservParameter(c, end, class, number)
			parameters = append(parameters, parameter)
			selected = selected && complete
		}
		row["Parameters"] = parameters
	}
	rsvpIntservTail(c, row, "Unparsed Service Data")
	r.at, r.err = c.at, errors.Join(boundary, c.err)
	if end > len(r.w) {
		r.need(false, "Intserv service exceeds captured message")
	}
	return row, selected
}

func rsvpIntservParameter(r *controlReader, parentEnd int, class, service uint64) (map[string]any, bool) {
	start := r.at
	h := &controlReader{w: r.w, at: start, info: map[string]any{}}
	number := h.num("Parameter Number", 1, false)
	flags := h.num("Flags", 1, false)
	words := h.num("Data Length Words", 2, false)
	if _, ok := h.info["Flags"]; ok {
		h.info["Invalid Bit Observed"], h.info["Reserved Flag Bits"] = flags&128 != 0, flags&127
	}
	row := map[string]any{"Header": rsvpRanges(h)}
	r.at, r.err = h.at, h.err
	if r.err != nil {
		rsvpIntservTail(r, row, "Unparsed Parameter Data")
		return row, true
	}
	end := start + 4 + 4*int(words)
	var boundary error
	if end > parentEnd {
		boundary = rsvpFormat(nil, "Intserv parameter exceeds its declared service")
	}
	c := &controlReader{w: r.w[:min(end, len(r.w))], at: r.at, info: map[string]any{}}
	selected, expected := true, -1
	switch {
	case number == 127 && (class == 12 && service == 1 || class == 9 && service == 5):
		row["Parameter Name"] = "Token Bucket TSpec"
		expected = 5
		for _, field := range []struct{ name, unit string }{{"Token Bucket Rate", "bytes/s"}, {"Token Bucket Size", "bytes"}, {"Peak Data Rate", "bytes/s"}} {
			rsvpIntservFloat(c, field.name, field.unit)
		}
		c.num("Minimum Policed Unit", 4, false)
		c.num("Maximum Packet Size", 4, false)
		rsvpIntservTokenIssues(c.info)
	case class == 13 && (service == 1 || service == 5) && (number == 4 || number == 6 || number == 8 || number == 10):
		expected = 1
		name := map[uint64]string{4: "Composed IS Hop Count", 6: "Composed Available Path Bandwidth", 8: "Composed Minimum Path Latency", 10: "Composed Path MTU"}[number]
		row["Parameter Name"] = name
		if number == 6 {
			rsvpIntservFloat(c, name, "bytes/s")
			if f, ok := c.info[name].(map[string]any); ok {
				bits := uint32(f["IEEE754 Bits"].(uint64))
				if bits>>31 != 0 || bits&0x7f800000 == 0x7f800000 {
					rsvpIntservIssue(c.info, name, "path-bandwidth-requires-finite-nonnegative")
				} else if bits == 0 {
					f["Advertised Meaning"] = "unknown available path bandwidth"
				}
			}
		} else {
			v := c.num(name, 4, false)
			if number == 8 && c.err == nil {
				c.info["Units"] = "microseconds"
				if v == 0xffffffff {
					c.info["Advertised Meaning"] = "indeterminate minimum path latency"
				}
			}
		}
	default:
		selected = false
		row["Parameter Name"] = fmt.Sprintf("Unselected parameter %d", number)
		c.rest("Opaque Parameter Data")
	}
	if expected >= 0 && int(words) != expected {
		boundary = rsvpFormat(boundary, "Intserv parameter length contradicts its selected layout")
	}
	rsvpIntservTail(c, c.info, "Unparsed Parameter Data")
	row["Source Layout Selected"], row["Body"] = selected, rsvpRanges(c)
	r.at, r.err = c.at, errors.Join(boundary, c.err)
	if end > len(r.w) {
		r.need(false, "Intserv parameter exceeds captured service")
	}
	return row, selected
}

// RSVPIntservIEEE32 returns exact JSON-safe text, including signed zero and
// nonfinite classifications. Only the raw bits are used to classify specials.
func RSVPIntservIEEE32(bits uint32) (value, classification string) {
	negative := bits>>31 != 0
	exponent, fraction := bits>>23&255, bits&0x7fffff
	if exponent == 255 {
		if fraction != 0 {
			return "NaN", "nan"
		}
		if negative {
			return "-Infinity", "negative-infinity"
		}
		return "+Infinity", "positive-infinity"
	}
	if exponent == 0 && fraction == 0 {
		if negative {
			return "-0", "negative-zero"
		}
		return "0", "positive-zero"
	}
	n := new(big.Rat).SetFloat64(float64(math.Float32frombits(bits)))
	value = n.FloatString(n.Denom().BitLen() - 1)
	if strings.Contains(value, ".") {
		value = strings.TrimRight(strings.TrimRight(value, "0"), ".")
	}
	return value, "finite"
}

func rsvpIntservFloat(r *controlReader, name, unit string) {
	start := r.at
	bits := r.num(name, 4, false)
	if r.err != nil {
		return
	}
	// A float is never exposed as an unsigned Wireshark numeric alias or as a
	// JSON NaN/Infinity. Bits and exact decimal interpretation share four bytes.
	value, classification := RSVPIntservIEEE32(uint32(bits))
	r.info[name] = map[string]any{"IEEE754 Bits": bits, "Value": value, "Classification": classification, "Units": unit, "Wire Field Ranges": map[string]any{"IEEE754 Bits": []any{uint64(start), uint64(r.at)}, "Value": []any{uint64(start), uint64(r.at)}}}
	// The parent range belongs to the composite numeric observation as well.
}

func rsvpIntservIssue(body map[string]any, field, code string) {
	issues, _ := body["Observed Value Domain Issues"].([]map[string]any)
	body["Observed Value Domain Issues"] = append(issues, map[string]any{"Field": field, "Code": code, "Scope": "advertised value constraint; no reservation or path state inferred"})
}

func rsvpIntservTokenIssues(body map[string]any) {
	for _, name := range []string{"Token Bucket Rate", "Token Bucket Size", "Peak Data Rate"} {
		f, ok := body[name].(map[string]any)
		if !ok {
			continue
		}
		bits := uint32(f["IEEE754 Bits"].(uint64))
		finite := bits&0x7f800000 != 0x7f800000
		allowed := bits>>31 == 0 && (finite || name == "Peak Data Rate" && bits == 0x7f800000)
		if !allowed {
			rsvpIntservIssue(body, name, "invalid-nonnegative-float-representation")
		} else if bits == 0 && name != "Peak Data Rate" {
			rsvpIntservIssue(body, name, "token-rate-and-bucket-size-must-be-positive")
		}
	}
	m, mok := body["Minimum Policed Unit"].(uint64)
	maximum, xok := body["Maximum Packet Size"].(uint64)
	if mok && m == 0 {
		rsvpIntservIssue(body, "Minimum Policed Unit", "minimum-policed-unit-must-be-positive")
	}
	if xok && maximum == 0 {
		rsvpIntservIssue(body, "Maximum Packet Size", "maximum-packet-size-must-be-positive")
	}
	if mok && xok && m > maximum {
		rsvpIntservIssue(body, "Minimum Policed Unit", "minimum-policed-unit-exceeds-maximum")
	}
}
