package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

// ErrOSPFv2WireIntegrity identifies a proven checksum failure separately from
// unavailable authentication/extension context and a local collection budget.
var ErrOSPFv2WireIntegrity = errors.New("OSPFv2 wire integrity failure")

// ErrOSPFv2WireFormat marks a contradiction within a completely captured
// declared extent. A later missing trailer cannot turn that proof into NeedMore.
var ErrOSPFv2WireFormat = errors.New("OSPFv2 wire format failure")

func OSPFv2MessageName(typ uint64) string { return OSPFv3MessageName(typ) }

func ospfv2Ranges(r *controlReader) map[string]any { return ospfv3Ranges(r) }

func ospfv2Child(r *controlReader, end int, fn func(*controlReader)) map[string]any {
	if r.err != nil {
		return nil
	}
	if end < r.at || end > len(r.w) {
		r.need(false, "OSPFv2 child exceeds captured boundary")
		return nil
	}
	c := &controlReader{w: r.w[:end], at: r.at, info: map[string]any{}}
	fn(c)
	c.need(c.at == end, "OSPFv2 child has trailing bytes")
	r.at = c.at
	if c.err != nil {
		r.err = c.err
	}
	if len(c.info) == 0 && len(c.fields) == 0 {
		return nil
	}
	return ospfv2Ranges(c)
}

func ospfv2KeepRow(rows []map[string]any, row map[string]any) []map[string]any {
	if row != nil {
		return append(rows, row)
	}
	return rows
}

func ospfv2Sum(w []byte) uint16 {
	var sum uint32
	for len(w) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(w))
		w = w[2:]
	}
	if len(w) != 0 {
		sum += uint32(w[0]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

// OSPFv2Checksum covers exactly the declared packet, excluding the eight-byte
// authentication field. An enclosing IP header, appended digest and LLS block
// are outside this extent. A mathematically valid field value zero is allowed.
func OSPFv2Checksum(w []byte) uint16 {
	if len(w) < 24 {
		return 0xffff
	}
	b := make([]byte, 0, len(w)-8)
	b = append(b, w[:16]...)
	b = append(b, w[24:]...)
	return ospfv2Sum(b)
}

func ospfv2IntegrityFailure(prior error, message string) error {
	err := fmt.Errorf("ospfv2: %s: %w", message, ErrOSPFv2WireIntegrity)
	if prior == nil {
		return err
	}
	return errors.Join(prior, err)
}

func ospfv2CompleteFormatError(err error) error {
	if err == nil || errors.Is(err, ErrObservedWireBudget) || errors.Is(err, ErrObservedWireContext) || errors.Is(err, ErrOSPFv2WireIntegrity) || errors.Is(err, ErrOSPFv2WireFormat) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrOSPFv2WireFormat, err)
}

func ospfv2AuthDeclarations(info map[string]any, prior error) error {
	h, _ := info["Header"].(map[string]any)
	if auth, ok := h["Authentication Type"].(uint64); !ok || auth != 2 {
		return prior
	}
	info["Checksum Verified"], info["Checksum Scope"] = false, "omitted: cryptographic authentication; MAC, key and SA unavailable"
	if checksum, present := h["Checksum"].(uint64); present && checksum != 0 {
		prior = errors.Join(prior, fmt.Errorf("%w: cryptographic packet checksum field must be zero", ErrOSPFv2WireFormat))
	}
	a, _ := info["Authentication"].(map[string]any)
	if length, present := a["Authentication Data Length"].(uint64); present && length == 0 {
		prior = errors.Join(prior, fmt.Errorf("%w: cryptographic authentication digest length is zero", ErrOSPFv2WireFormat))
	}
	return prior
}

func ospfv2PacketIntegrity(info map[string]any, w []byte, prior error) error {
	h, _ := info["Header"].(map[string]any)
	n, ok := h["Packet Length"].(uint64)
	if !ok || n < 24 || n > uint64(len(w)) {
		info["Checksum Scope"] = "unverified: complete declared OSPFv2 packet unavailable"
		return prior
	}
	auth, ok := h["Authentication Type"].(uint64)
	if !ok {
		return prior
	}
	switch auth {
	case 0, 1:
		valid := OSPFv2Checksum(w[:n]) == 0
		info["Checksum Verified"], info["Checksum Scope"] = valid, "declared OSPFv2 packet excluding eight authentication bytes; no authentication verdict"
		if !valid {
			return ospfv2IntegrityFailure(prior, "packet checksum failed")
		}
	case 2:
		info["Checksum Scope"] = "omitted: cryptographic authentication; MAC, key and SA unavailable"
	default:
		info["Checksum Scope"] = "unverified: unselected authentication type"
	}
	return prior
}

// DecodeOSPFv2Wire observes the five RFC2328 packet envelopes, RFC6549 instance
// IDs, base LSAs and bounded RFC5613 LLS. Digest bytes and matching sequences do
// not certify an SA, replay protection, neighbor, LSDB or installed route.
func DecodeOSPFv2Wire(w []byte, budget int) (observed map[string]any, resultErr error) {
	if len(w) > 65535 || budget <= 0 {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{}}
	h := &controlReader{w: w, info: map[string]any{}}
	version := h.num("Version", 1, false)
	h.need(version == 2, "OSPFv2 version must be two")
	typ := h.num("Type", 1, false)
	n := h.num("Packet Length", 2, false)
	routingAddress(h, "Router ID", 4)
	routingAddress(h, "Area ID", 4)
	h.num("Checksum", 2, false)
	h.num("Instance ID", 1, false) // RFC6549, formerly the high AuType octet.
	auth := h.num("Authentication Type", 1, false)
	r.info["Header"] = ospfv2Ranges(h)
	r.info["Packet Name"] = OSPFv2MessageName(typ)
	r.info["Authentication Verified"] = false
	r.info["Required Captured Bytes"] = uint64(24)
	r.info["Observation Scope"] = "bounded wire advertisements only; no SA, replay protection, verified neighbor, LSDB or installed route"
	r.info["Checksum Verified"], r.info["Checksum Scope"] = false, "unverified: OSPFv2 header incomplete or invalid"
	defer func() { resultErr = ospfv2AuthDeclarations(r.info, resultErr) }()
	if h.err != nil {
		if length, present := h.info["Packet Length"].(uint64); present && length < 24 {
			return r.info, errors.Join(h.err, fmt.Errorf("%w: declared packet length below header", ErrOSPFv2WireFormat))
		}
		return r.info, h.err
	}
	var badLength error
	if n < 24 {
		badLength = fmt.Errorf("%w: declared packet length is below header", ErrOSPFv2WireFormat)
	}
	h.at = 16
	var authentication map[string]any
	if auth == 2 {
		authentication = ospfv2Child(h, min(24, len(w)), func(c *controlReader) {
			c.num("Reserved", 2, false)
			c.num("Key ID", 1, false)
			c.num("Authentication Data Length", 1, false)
			c.num("Sequence", 4, false)
			c.info["Algorithm"] = "not selected without configured SA"
		})
	} else {
		authentication = ospfv2Child(h, min(24, len(w)), func(c *controlReader) { c.take("Observed Authentication Field", 8) })
	}
	if authentication != nil {
		authentication["Authentication Verified"] = false
	}
	r.info["Authentication"] = authentication
	if h.err != nil {
		return r.info, errors.Join(badLength, h.err)
	}
	if badLength != nil {
		return r.info, badLength
	}
	r.info["Required Captured Bytes"] = n
	r.w, r.at = w[:min(int(n), len(w))], 24
	defer func() { resultErr = ospfv2PacketIntegrity(r.info, w, resultErr) }()
	var unsupported error
	if auth > 2 {
		unsupported = fmt.Errorf("%w: OSPFv2 authentication type %d", ErrObservedWireContext, auth)
	}
	var options uint64
	var optionsObserved bool
	switch typ {
	case 1:
		r.info["Hello"] = ospfv2Child(r, len(r.w), func(c *controlReader) {
			routingAddress(c, "Network Mask", 4)
			c.num("Hello Interval", 2, false)
			options = c.num("Options", 1, false)
			optionsObserved = c.err == nil
			c.num("Router Priority", 1, false)
			c.num("Router Dead Interval", 4, false)
			routingAddress(c, "Designated Router", 4)
			routingAddress(c, "Backup Designated Router", 4)
			var neighbors []map[string]any
			for c.err == nil && c.at < len(c.w) && ospfv3Charge(c, &budget) {
				neighbors = ospfv2KeepRow(neighbors, ospfv2Child(c, min(c.at+4, len(c.w)), func(row *controlReader) { routingAddress(row, "Neighbor Router ID", 4) }))
			}
			c.info["Neighbors"] = neighbors
		})
	case 2:
		r.info["Database Description"] = ospfv2Child(r, len(r.w), func(c *controlReader) {
			c.num("Interface MTU", 2, false)
			options = c.num("Options", 1, false)
			optionsObserved = c.err == nil
			c.num("Flags", 1, false)
			c.num("DD Sequence", 4, false)
			c.info["LSA Headers"] = ospfv2Headers(c, &budget)
		})
	case 3:
		var requests []map[string]any
		for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, &budget) {
			requests = ospfv2KeepRow(requests, ospfv2Child(r, min(r.at+12, len(r.w)), func(c *controlReader) {
				c.num("LS Type", 4, false)
				routingAddress(c, "Link State ID", 4)
				routingAddress(c, "Advertising Router", 4)
			}))
		}
		r.info["Requests"] = requests
	case 4:
		count := r.num("LSA Count", 4, false)
		var lsas []map[string]any
		for i := uint64(0); i < count && r.err == nil && ospfv3Charge(r, &budget); i++ {
			row, soft := ospfv2LSA(r, &budget)
			lsas = ospfv2KeepRow(lsas, row)
			if unsupported == nil {
				unsupported = soft
			}
		}
		r.info["LSAs"] = lsas
	case 5:
		r.info["LSA Headers"] = ospfv2Headers(r, &budget)
	default:
		r.rest("Opaque Body")
		unsupported = fmt.Errorf("%w: OSPFv2 message type %d", ErrObservedWireContext, typ)
	}
	r.need(r.at == len(r.w), "OSPFv2 packet has trailing body bytes")
	ospfv2Ranges(r)
	if n > uint64(len(w)) {
		r.need(false, "OSPFv2 declared packet exceeds captured message")
	}
	bodyErr := r.err
	if bodyErr != nil {
		if r.at < len(r.w) {
			r.info["Unparsed Body"] = bytes.Clone(r.w[r.at:])
		}
		if n <= uint64(len(w)) {
			bodyErr = ospfv2CompleteFormatError(bodyErr)
		}
	}
	if n > uint64(len(w)) {
		return r.info, bodyErr
	}
	if optionsObserved {
		r.info["L Bit"] = options&0x10 != 0
	}
	if auth > 2 {
		// The digest length/placement of an unselected authentication scheme
		// is unknown. L-bit cannot authorize treating that opaque tail as LLS.
		if int(n) < len(w) {
			r.info["Trailing Bytes"] = bytes.Clone(w[n:])
		}
		r.info["Trailer Scope"] = "unselected authentication scheme; LLS position and integrity unavailable"
		return r.info, errors.Join(bodyErr, unsupported)
	}
	// Authentication and LLS are outside the declared OSPF packet. The digest
	// precedes LLS (RFC5613), unlike an assumed generic trailing-byte scan.
	tail := &controlReader{w: w, at: int(n), info: map[string]any{}}
	if auth == 2 {
		size, _ := authentication["Authentication Data Length"].(uint64)
		r.info["Required Captured Bytes"] = n + size
		b := tail.take("Authentication Digest", int(size))
		if tail.err == nil {
			authentication["Authentication Digest"] = b
			authentication["Wire Field Ranges"].(map[string]any)["Authentication Digest"] = []any{n, n + size}
		}
	}
	if tail.err == nil && optionsObserved && options&0x10 != 0 {
		start := tail.at
		r.info["Required Captured Bytes"] = uint64(start + 4)
		lls := ospfv2LLS(tail, auth, authentication, &budget)
		r.info["LLS"] = lls
		if words, ok := lls["Length Words"].(uint64); ok && words != 0 {
			r.info["Required Captured Bytes"] = uint64(start) + words*4
		}
	}
	if tail.err != nil {
		return r.info, errors.Join(bodyErr, tail.err, unsupported)
	}
	if tail.at != len(w) {
		r.info["Trailing Bytes"] = bytes.Clone(w[tail.at:])
		if unsupported == nil {
			unsupported = fmt.Errorf("%w: unexplained OSPFv2 trailer", ErrObservedWireContext)
		}
	}
	return r.info, errors.Join(bodyErr, unsupported)
}
