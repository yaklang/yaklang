package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
)

// ErrGTPv2WireFormat marks a contradiction in a declared wire envelope, rather
// than unavailable procedure, subscriber, bearer or authentication context.
var ErrGTPv2WireFormat = errors.New("GTPv2 wire format failure")

func gtpv2Ranges(r *controlReader) map[string]any {
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	return r.info
}

func gtpv2Format(prior error, text string) error {
	return errors.Join(prior, fmt.Errorf("%w: %s", ErrGTPv2WireFormat, text))
}

func gtpv2CompleteError(err error) error {
	if err == nil || errors.Is(err, ErrObservedWireBudget) || errors.Is(err, ErrObservedWireContext) || errors.Is(err, ErrGTPv2WireFormat) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrGTPv2WireFormat, err)
}

func gtpv2EPCType(typ uint64) bool {
	_, known := gtpv2MessageNames[typ]
	// 178 retains a historical display name, but is explicitly reserved.
	return known && typ != 178 && (typ >= 1 && typ <= 3 || typ >= 32 && typ <= 239)
}

// DecodeGTPv2Wire observes one UDP payload, including at most one indicated
// piggybacked message. Its TLIV/selected value grammar does not prove mandatory
// procedure IEs, a bearer, TEID ownership, NAS content or an authenticated peer.
// Each datagram supplies its own fields; no TEID/sequence state is inherited.
func DecodeGTPv2Wire(w []byte, budget int) (map[string]any, error) {
	if len(w) > 65535 || budget < 0 {
		return nil, ErrObservedWireBudget
	}
	result := map[string]any{"Authentication Verified": false, "Procedure Semantics Verified": false,
		"Bearer State Inferred": false, "Wire Grammar Complete": false,
		"Observation Scope": "bounded GTPv2-C wire observations only; mandatory IE/procedure, NAS, bearer/session ownership and authentication unverified"}
	var messages []map[string]any
	var aggregate error
	start := 0
	for index := 0; index < 2; index++ {
		row, end, piggyback, err := gtpv2Message(w, start, index, &budget)
		if row != nil {
			messages = append(messages, row)
		}
		if err != nil {
			aggregate = errors.Join(aggregate, err)
			break
		}
		start = end
		if !piggyback {
			if start < len(w) {
				result["Unindicated Tail"] = bytes.Clone(w[start:])
				aggregate = gtpv2Format(aggregate, "bytes follow a message without its P flag")
			}
			break
		}
		if start == len(w) {
			aggregate = gtpv2Format(aggregate, "P flag set but the following message is unavailable")
			break
		}
	}
	result["Messages"] = messages
	result["Observed Message Count"] = uint64(len(messages))
	result["Wire Grammar Complete"] = aggregate == nil
	return result, aggregate
}

func gtpv2Message(w []byte, start, index int, budget *int) (map[string]any, int, bool, error) {
	h := &controlReader{w: w, at: start, info: map[string]any{}}
	row := map[string]any{}
	flags := h.num("Flags", 1, false)
	if h.err == nil {
		h.info["Version"] = flags >> 5
		if flags>>5 != 2 {
			row["Header"] = gtpv2Ranges(h)
			return row, start, false, gtpv2Format(nil, "version is not two")
		}
		h.info["Piggyback"] = flags&0x10 != 0
		h.info["TEID Present"] = flags&8 != 0
		h.info["MP Bit Observed"] = flags&4 != 0
		h.info["Ignored Header Spare Bits"] = flags & 3
	}
	typ := h.num("Message Type", 1, false)
	length := h.num("Message Length", 2, false)
	if _, present := h.info["Message Type"]; present {
		row["Message Name"] = GTPv2MessageName(typ)
	}
	if h.err != nil {
		if len(h.info) != 0 {
			row["Header"] = gtpv2Ranges(h)
			return row, start, false, h.err
		}
		return nil, start, false, h.err
	}
	end := start + 4 + int(length)
	minimum := 8
	if flags&8 != 0 {
		minimum = 12
	}
	// Even an incomplete/contradictory header never reads outside its own
	// declared message. Complete captured primitives survive a later shortage.
	h.w = w[:min(end, len(w))]
	if flags&8 != 0 {
		h.num("TEID", 4, false)
	}
	h.num("Sequence Number", 3, false)
	last := h.num("Priority or Spare Octet", 1, false)
	if h.err == nil && gtpv2EPCType(typ) {
		if gtpv2EPCType(typ) && typ >= 32 && flags&8 != 0 && flags&4 != 0 {
			h.info["Message Priority"] = last >> 4
			h.info["Ignored Last Octet Spare"] = last & 15
		} else {
			h.info["Ignored Last Octet Spare"] = last
		}
	}
	row["Header"] = gtpv2Ranges(h)
	var format error
	if int(length)+4 < minimum {
		format = gtpv2Format(format, "declared message length is below conditional header")
	}
	if gtpv2EPCType(typ) && index == 1 && flags&0x10 != 0 {
		format = gtpv2Format(format, "second piggybacked message must clear P; third message is not admitted")
	}
	if gtpv2EPCType(typ) && (typ <= 3) == (flags&8 != 0) {
		format = gtpv2Format(format, "TEID presence contradicts the selected EPC message type")
	}
	if h.err != nil || int(length)+4 < minimum {
		return row, min(end, len(w)), false, errors.Join(format, h.err)
	}
	if !gtpv2EPCType(typ) {
		// S101/S121 and Sv select their own interface specifications. A shared
		// v2 header does not establish the EPC IE namespace or P/MP semantics.
		// Retain the exact body without inventing an empty/decoded IE list.
		stop := min(end, len(w))
		row["Unparsed Body"] = bytes.Clone(w[h.at:stop])
		row["Wire Field Ranges"] = map[string]any{"Unparsed Body": []any{uint64(h.at), uint64(stop)}}
		row["Interface Scope"] = "unselected interface/message type; body and EPC-specific T/P/MP rules unavailable"
		context := fmt.Errorf("%w: unselected GTPv2 interface/message type", ErrObservedWireContext)
		if end > len(w) {
			h.need(false, "GTPv2 declared message extent exceeds captured payload")
		}
		return row, stop, false, errors.Join(format, context, h.err)
	}
	body := &controlReader{w: w[:min(end, len(w))], at: h.at, info: map[string]any{}}
	row["IEs"] = gtpv2IEs(body, budget, 0)
	if body.err != nil && end <= len(w) {
		body.err = gtpv2CompleteError(body.err)
	}
	if end > len(w) {
		body.need(false, "GTPv2 declared message extent exceeds captured payload")
	}
	if body.at < len(body.w) {
		row["Unparsed Body"] = bytes.Clone(body.w[body.at:])
	}
	return row, end, flags&0x10 != 0, errors.Join(format, body.err)
}

func gtpv2IEs(r *controlReader, budget *int, depth int) []map[string]any {
	var rows []map[string]any
	if depth > 8 {
		r.err = fmt.Errorf("%w: GTPv2 grouped IE depth exceeds eight", ErrObservedWireBudget)
		return rows
	}
	for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
		c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
		typ := c.num("IE Type", 1, false)
		length := c.num("IE Length", 2, false)
		instance := c.num("Spare and Instance", 1, false)
		if c.err == nil {
			c.info["Instance"], c.info["Ignored IE Spare"] = instance&15, instance>>4
			c.info["IE Name"] = GTPv2IEName(typ)
			start, end := c.at, c.at+int(length)
			if end > len(c.w) {
				// An owned partial value is not a complete semantic IE value.
				c.rest("Captured Value")
				c.need(false, "GTPv2 IE value exceeds its parent message/group")
			} else {
				value := &controlReader{w: c.w[:end], at: start, info: map[string]any{}}
				if typ == 93 {
					c.info["IEs"] = gtpv2IEs(value, budget, depth+1)
				} else {
					gtpv2IEValue(value, typ, int(length))
					c.info["Body"] = gtpv2Ranges(value)
				}
				c.at, c.err = value.at, value.err
				if c.err == nil && c.at != end {
					c.need(false, "GTPv2 IE value was not consumed within its own boundary")
				}
			}
		}
		r.at = c.at
		if c.err != nil {
			r.err = c.err
		}
		if len(c.info) != 0 {
			rows = append(rows, gtpv2Ranges(c))
		}
	}
	return rows
}

func gtpv2IEValue(r *controlReader, typ uint64, n int) {
	switch {
	case typ == 3:
		r.num("Recovery Counter", 1, false)
		if r.err == nil && n > 1 {
			// Table8.1 calls Recovery variable-length while 8.5 specifies a
			// one-octet counter. Do not invent semantics for the remaining bytes.
			r.rest("Unselected Value Tail")
			r.err = fmt.Errorf("%w: Recovery counter observed; additional variable-length octets unselected", ErrObservedWireContext)
		}
	case typ == 73:
		v := r.num("Bearer ID Octet", 1, false)
		if r.err == nil {
			r.info["EPS Bearer ID"], r.info["Ignored Value Spare"] = v&15, v>>4
			if r.at < len(r.w) {
				r.rest("Ignored Extension Bytes")
			}
		}
	case typ == 2:
		r.num("Cause", 1, false)
		r.num("Cause Flags", 1, false)
		if n == 6 {
			r.num("Offending IE Type", 1, false)
			r.num("Offending IE Length", 2, false)
			v := r.num("Offending IE Instance Octet", 1, false)
			if r.err == nil {
				r.info["Offending IE Instance"] = v & 15
			}
		}
		if n != 2 && n != 6 {
			if r.err == nil && r.at < len(r.w) {
				r.rest("Unselected Value Tail")
			}
			r.err = gtpv2Format(r.err, "Cause value length must be two or six")
		}
	case typ == 87:
		flags := r.num("F-TEID Flags", 1, false)
		if r.err == nil {
			r.info["Interface Type"] = flags & 63
		}
		r.num("TEID", 4, false)
		expected := 5
		if flags&128 != 0 {
			expected += 4
			routingAddress(r, "IPv4 Address", 4)
		}
		if flags&64 != 0 {
			expected += 16
			routingAddress(r, "IPv6 Address", 16)
		}
		if _, present := r.info["F-TEID Flags"]; present && flags&0xc0 == 0 {
			r.err = gtpv2Format(r.err, "F-TEID must indicate IPv4, IPv6 or both")
		}
		if n < expected {
			r.err = gtpv2Format(r.err, "F-TEID address flags require more value octets")
		}
		if r.err == nil && r.at < len(r.w) {
			// TS29.274 7.7.7 and Table8.1: receive-compatible extension
			// octets do not invalidate the observed fixed address prefix.
			r.rest("Ignored Extension Bytes")
		}
	default:
		r.rest("Opaque Value")
		r.info["Value Semantics"] = "unselected IE value layout; NAS, subscriber and bearer semantics unavailable"
	}
}
