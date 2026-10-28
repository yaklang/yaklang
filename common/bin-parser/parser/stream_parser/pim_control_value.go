package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
)

// PIMWireContext contains the actual enclosing IP addresses, never dummy
// addresses inside a Null-Register. A stateless canonical entry has no IPv6
// pseudo-header and explicitly leaves that checksum unverified.
type PIMWireContext struct {
	Source, Destination netip.Addr
	// Zero selects an explicitly bounded standalone PIM input. Native callers
	// must supply the literal enclosing IP payload length, or -1 when unknown.
	// A decoder's replacement of an IP zero length is not a declared boundary.
	DeclaredBytes int
}

const pimPrefixScope = "captured field prefixes only; control message not fully validated; no installed routing or verified neighbor state"

func PIMMessageName(typ uint64) string {
	if typ < 10 {
		return []string{"Hello", "Register", "Register-Stop", "Join/Prune", "Bootstrap", "Assert", "Graft", "Graft-Ack", "Candidate-RP-Advertisement", "State Refresh"}[typ]
	}
	return "Unknown"
}

func pimCharge(r *controlReader, budget *int) bool {
	if *budget <= 0 {
		r.err = ErrObservedWireBudget
		return false
	}
	*budget--
	return true
}

func pimRanges(r *controlReader) map[string]any {
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	return r.info
}

// PIMChecksum checks one's-complement integrity, not authentication. Register
// messages always use an eight-byte IPv6 pseudo-header length, including the
// interoperable whole-message fallback (RFC7761 sections 4.9 and 4.9.3).
func PIMChecksum(w []byte, context PIMWireContext, pseudoLength int) uint16 {
	var sum uint32
	add := func(b []byte) {
		for len(b) >= 2 {
			sum += uint32(binary.BigEndian.Uint16(b))
			b = b[2:]
		}
		if len(b) > 0 {
			sum += uint32(b[0]) << 8
		}
	}
	if context.Source.Is6() && context.Destination.Is6() {
		add(context.Source.AsSlice())
		add(context.Destination.AsSlice())
		var pseudo [8]byte
		binary.BigEndian.PutUint32(pseudo[:4], uint32(pseudoLength))
		pseudo[7] = 103
		add(pseudo[:])
	}
	add(w)
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

// DecodePIMControl validates an entire selected v2 message. It describes
// advertised control information, not adjacency, installed multicast routes,
// elected DR/BSR, authentication, or reassembled Bootstrap RP-sets.
func DecodePIMControl(w []byte, context PIMWireContext, budget int) (map[string]any, error) {
	if len(w) < 4 || len(w) > 65535 {
		return nil, fmt.Errorf("pim: complete common header and bounded datagram required")
	}
	if budget <= 0 {
		return nil, ErrObservedWireBudget
	}
	if context.DeclaredBytes > 0 && len(w) > context.DeclaredBytes {
		return nil, fmt.Errorf("%w: PIM input exceeds the enclosing IP message", ErrObservedWireContext)
	}
	r := &controlReader{w: w, info: map[string]any{}}
	v := r.num("Version and Type", 1, false)
	r.need(v>>4 == 2, "unselected PIM version")
	r.info["Version"], r.info["Type"] = v>>4, v&15
	r.num("Reserved", 1, false) // ignored on receipt, not an identity signature
	r.num("Checksum", 2, false)
	if r.err != nil {
		return nil, r.err
	}
	typ := v & 15
	r.info["Packet Name"], r.info["Authentication Verified"] = PIMMessageName(typ), false
	if typ == 4 {
		r.info["No Forward"] = w[1]&128 != 0
	}
	if typ == 2 {
		r.info["Packed Register Stop Capable"] = w[1]&1 != 0
	}
	r.info["Observation Scope"] = "complete selected wire fields; advertisements only, no installed routing or verified neighbor state"
	var err error
	switch typ {
	case 0:
		err = decodePIMHello(r, &budget)
	case 1:
		err = decodePIMRegister(r, context)
	case 2, 5, 9:
		err = decodePIMGroupSource(r, typ, &budget)
	case 3, 6, 7:
		err = decodePIMJoinPrune(r, &budget)
	case 4:
		err = decodePIMBootstrap(r, &budget)
	case 8:
		err = decodePIMCandidateRP(r, &budget)
	default:
		err = fmt.Errorf("%w: unselected PIM message type %d", ErrObservedWireContext, typ)
	}
	if err != nil && r.err == nil {
		r.err = err
	}
	if r.err != nil {
		// Header bytes alone never become a decoded control message. Opaque
		// extensions stay inspectable without guessing subsequent boundaries.
		if r.at >= 0 && r.at <= len(w) {
			r.info["Unparsed Body"] = bytes.Clone(w[r.at:])
		}
		r.info["Observation Scope"] = pimPrefixScope
		return pimRanges(r), r.err
	}
	r.need(r.at == len(w), "PIM message trailing bytes")
	if r.err != nil {
		return nil, r.err
	}
	ranges := routingFieldRanges(r.fields)
	ranges["Version"], ranges["Type"] = []any{uint64(0), uint64(1)}, []any{uint64(0), uint64(1)}
	if typ == 1 {
		ranges["Border"], ranges["Null Register"] = ranges["Register Flags"], ranges["Register Flags"]
	}
	if typ == 5 || typ == 9 {
		ranges["RPT"], ranges["Metric Preference"] = ranges["Metric Preference Word"], ranges["Metric Preference Word"]
	}
	if typ == 9 {
		for _, key := range []string{"Prune Indicator", "Prune Now", "Assert Override"} {
			ranges[key] = ranges["Refresh Flags"]
		}
	}
	r.info["Wire Field Ranges"] = ranges
	r.info["Checksum Verified"] = false
	r.info["Checksum Scope"] = "unverified: enclosing IP addresses not supplied"
	completeExtent := context.DeclaredBytes == 0 || context.DeclaredBytes == len(w)
	if !completeExtent {
		r.info["Observation Scope"] = pimPrefixScope
		r.info["Checksum Scope"] = "unverified: enclosing IP message extent unavailable or incomplete"
	}
	if context.Source.IsValid() && context.Destination.IsValid() {
		if context.Source.Is4() != context.Destination.Is4() {
			r.info["Observation Scope"] = pimPrefixScope
			return r.info, fmt.Errorf("pim: enclosing address families differ")
		}
		if !completeExtent && typ != 1 {
			return r.info, fmt.Errorf("%w: enclosing IP message extent unavailable or incomplete", ErrObservedWireContext)
		}
		n := len(w)
		if typ == 1 {
			n = 8
		}
		verified := PIMChecksum(w[:n], context, n) == 0
		scope := "message"
		if typ == 1 {
			scope = "register-header"
			if !verified && completeExtent && PIMChecksum(w, context, 8) == 0 {
				verified, scope = true, "register-whole-message-interop"
			}
		}
		r.info["Checksum Verified"], r.info["Checksum Scope"] = verified, scope
		if !verified {
			r.info["Observation Scope"] = pimPrefixScope
			if !completeExtent {
				r.info["Checksum Scope"] = "unverified: enclosing IP message extent unavailable or incomplete"
				return r.info, fmt.Errorf("%w: enclosing IP message extent unavailable or incomplete", ErrObservedWireContext)
			}
			return r.info, fmt.Errorf("pim: checksum failed")
		}
	}
	if !completeExtent {
		return r.info, fmt.Errorf("%w: enclosing IP message extent unavailable or incomplete", ErrObservedWireContext)
	}
	return r.info, nil
}

// Every address is nested with absolute PDU spans so repeated group/source
// fields highlight their own bytes. Unsupported encoding cannot be skipped
// with an invented width.
func pimAddress(r *controlReader, kind string, budget *int) map[string]any {
	if r.err != nil {
		return nil
	}
	if !pimCharge(r, budget) {
		return nil
	}
	c := &controlReader{w: r.w, at: r.at, info: map[string]any{"Kind": kind}}
	af := c.num("Address Family", 1, false)
	enc := c.num("Encoding", 1, false)
	if c.err == nil && (af != 1 && af != 2 || enc != 0) {
		r.err = fmt.Errorf("%w: unselected PIM address family/encoding %d/%d", ErrObservedWireContext, af, enc)
		return nil
	}
	n := 4
	if af == 2 {
		n = 16
	}
	if kind == "group" || kind == "source" {
		flags := c.num("Flags", 1, false)
		mask := c.num("Mask Length", 1, false)
		c.need(mask <= uint64(n*8), "PIM mask exceeds address width")
		if kind == "group" {
			c.info["Bidirectional"], c.info["Admin Scope Zone"] = flags&128 != 0, flags&1 != 0
		} else {
			c.info["Sparse"], c.info["Wildcard"], c.info["RPT"] = flags&4 != 0, flags&2 != 0, flags&1 != 0
			// Preserve received prefix/flag values. Non-host source masks and
			// WC without RPT are receive-semantic issues, never routes.
			c.info["Source Semantics Valid"] = mask == uint64(n*8) && (flags&2 == 0 || flags&1 != 0)
		}
	}
	routingAddress(c, "Address", n)
	if c.err != nil {
		r.err = c.err
		return nil
	}
	if kind == "group" {
		address, _ := netip.ParseAddr(c.info["Address"].(string))
		if !address.IsMulticast() {
			r.err = fmt.Errorf("pim: encoded group is not multicast")
			return nil
		}
	}
	r.at = c.at
	pimRanges(c)
	ranges := c.info["Wire Field Ranges"].(map[string]any)
	for _, key := range []string{"Bidirectional", "Admin Scope Zone", "Sparse", "Wildcard", "RPT"} {
		if _, ok := c.info[key]; ok {
			ranges[key] = ranges["Flags"]
		}
	}
	return c.info
}

func decodePIMHello(r *controlReader, budget *int) error {
	var options []map[string]any
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, budget) {
			break
		}
		c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
		typ := c.num("Option Type", 2, false)
		n := c.num("Option Length", 2, false)
		c.need(n <= uint64(len(c.w)-c.at), "PIM Hello option crosses datagram")
		if c.err != nil {
			return c.err
		}
		end := c.at + int(n)
		c.w = c.w[:end]
		switch typ {
		case 1:
			c.num("Hold Time", 2, false)
		case 2:
			v := c.num("Propagation Flags", 2, false)
			c.info["Tracking Support"], c.info["Propagation Delay"] = v&0x8000 != 0, v&0x7fff
			c.num("Override Interval", 2, false)
		case 19:
			c.num("DR Priority", 4, false)
		case 20:
			c.num("Generation ID", 4, false)
		case 21:
			c.num("State Refresh Version", 1, false)
			c.num("State Refresh Interval", 1, false)
			c.num("State Refresh Reserved", 2, false)
		case 24, 65001:
			var addresses []map[string]any
			var family any
			for c.at < end && c.err == nil {
				a := pimAddress(c, "unicast", budget)
				if a == nil {
					break
				}
				if family != nil {
					c.need(family == a["Address Family"], "PIM address list mixes families")
				}
				family = a["Address Family"]
				addresses = append(addresses, a)
			}
			c.info["Addresses"] = addresses
		default:
			c.rest("Unknown Option Value")
		}
		c.need(c.at == end, "PIM Hello option has trailing bytes")
		if c.err != nil {
			return c.err
		}
		pimRanges(c)
		if typ == 2 {
			ranges := c.info["Wire Field Ranges"].(map[string]any)
			ranges["Tracking Support"], ranges["Propagation Delay"] = ranges["Propagation Flags"], ranges["Propagation Flags"]
		}
		options = append(options, c.info)
		r.at = end
	}
	r.info["Options"] = options
	return r.err
}

func decodePIMRegister(r *controlReader, context PIMWireContext) error {
	flags := r.num("Register Flags", 4, false)
	if r.err != nil {
		return r.err // Missing flag bytes cannot establish false Border/Null values.
	}
	null := flags&0x40000000 != 0
	r.info["Null Register"], r.info["Border"] = null, flags&0x80000000 != 0
	start := r.at
	b := r.w[start:]
	if r.err != nil || len(b) == 0 {
		return fmt.Errorf("pim: missing Register IP header")
	}
	version := b[0] >> 4
	if null && version == 0 {
		if context.Source.Is4() {
			version = 4
		} else if context.Source.Is6() {
			version = 6
		} else {
			return fmt.Errorf("%w: old dummy header needs enclosing address family", ErrObservedWireContext)
		}
	}
	if context.Source.IsValid() && (version == 4) != context.Source.Is4() {
		return fmt.Errorf("pim: Register inner/outer address family mismatch")
	}
	minimum, src, dst := 0, 0, 0
	switch version {
	case 4:
		minimum, src, dst = 20, 12, 16
	case 6:
		minimum, src, dst = 40, 8, 24
	default:
		return fmt.Errorf("pim: unselected Register IP version")
	}
	if len(b) < minimum {
		return fmt.Errorf("pim: incomplete Register IP header")
	}
	if !null {
		if version == 4 {
			header := int(b[0]&15) * 4
			length := int(binary.BigEndian.Uint16(b[2:4]))
			if header < 20 || header > len(b) || length < header || length > len(b) {
				return fmt.Errorf("pim: invalid or truncated encapsulated IPv4 length")
			}
		} else if int(binary.BigEndian.Uint16(b[4:6]))+40 > len(b) {
			return fmt.Errorf("pim: truncated encapsulated IPv6 payload")
		}
	}
	c := &controlReader{w: r.w, at: start + src, info: map[string]any{"IP Version": uint64(b[0] >> 4), "Address Family Version": uint64(version), "Dummy Header": null}}
	n := 4
	if version == 6 {
		n = 16
	}
	routingAddress(c, "Source", n)
	c.at = start + dst
	routingAddress(c, "Group", n)
	group, _ := netip.ParseAddr(fmt.Sprint(c.info["Group"]))
	if !group.IsMulticast() {
		return fmt.Errorf("pim: Register group is not multicast")
	}
	if null {
		// RFC7761 permits older dummy headers. Never decode their apparent
		// protocol/ports as captured traffic, regardless of payload shape.
		c.info["Observation"] = "advertised (S,G), not an inner transport packet"
		if version == 4 {
			header := int(b[0]&15) * 4
			if header < 20 || header > len(b) {
				header = 20 // older dummy header fields are ignored on receipt
			}
			if binary.BigEndian.Uint16(b[10:12]) != 0 && PIMChecksum(b[:header], PIMWireContext{}, header) != 0 {
				c.info["Dummy Integrity"] = "invalid IPv4 dummy header checksum"
			} else {
				c.info["Dummy Integrity"] = "IPv4 dummy header checksum valid or zero (not supplied)"
			}
		} else {
			present := len(b) >= 44 && b[6] == 103
			c.info["Dummy PIM Header Present"] = present
			if present {
				s, _ := netip.ParseAddr(fmt.Sprint(c.info["Source"]))
				c.info["Dummy PIM Checksum Valid"] = PIMChecksum(b[40:44], PIMWireContext{Source: s, Destination: group}, 4) == 0
			}
		}
	} else {
		c.info["Observation"] = "encapsulated multicast data; transport normalization validated separately"
	}
	r.info["Register IP Header"] = pimRanges(c)
	r.rest("Encapsulated Bytes")
	return r.err
}

func decodePIMGroupSource(r *controlReader, typ uint64, budget *int) error {
	r.info["Group"] = pimAddress(r, "group", budget)
	r.info["Source"] = pimAddress(r, "unicast", budget)
	if typ == 9 {
		r.info["Originator"] = pimAddress(r, "unicast", budget)
	}
	if typ == 5 || typ == 9 {
		v := r.num("Metric Preference Word", 4, false)
		if r.err == nil {
			r.info["RPT"], r.info["Metric Preference"] = v&0x80000000 != 0, v&0x7fffffff
		}
		r.num("Metric", 4, false)
	}
	if typ == 9 {
		r.num("Mask Length", 1, false)
		r.num("TTL", 1, false)
		v := r.num("Refresh Flags", 1, false)
		if r.err == nil {
			r.info["Prune Indicator"], r.info["Prune Now"], r.info["Assert Override"] = v&128 != 0, v&64 != 0, v&32 != 0
		}
		r.num("Interval", 1, false)
	}
	return r.err
}

func decodePIMJoinPrune(r *controlReader, budget *int) error {
	r.info["Upstream Neighbor"] = pimAddress(r, "unicast", budget)
	r.num("Join Reserved", 1, false)
	n := r.num("Group Count", 1, false)
	r.num("Hold Time", 2, false)
	var groups []map[string]any
	for i := uint64(0); i < n && r.err == nil; i++ {
		if !pimCharge(r, budget) {
			break
		}
		c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
		c.info["Group"] = pimAddress(c, "group", budget)
		joins, prunes := c.num("Join Count", 2, false), c.num("Prune Count", 2, false)
		for _, list := range []struct {
			name  string
			count uint64
		}{{"Joined Sources", joins}, {"Pruned Sources", prunes}} {
			var addresses []map[string]any
			for j := uint64(0); j < list.count && c.err == nil; j++ {
				addresses = append(addresses, pimAddress(c, "source", budget))
			}
			c.info[list.name] = addresses
		}
		if c.err != nil {
			return c.err
		}
		groups = append(groups, pimRanges(c))
		r.at = c.at
	}
	r.info["Groups"] = groups
	return r.err
}

func decodePIMBootstrap(r *controlReader, budget *int) error {
	r.num("Fragment Tag", 2, false)
	r.num("Hash Mask Length", 1, false)
	r.num("BSR Priority", 1, false)
	r.info["BSR"] = pimAddress(r, "unicast", budget)
	var groups []map[string]any
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, budget) {
			break
		}
		c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
		c.info["Group"] = pimAddress(c, "group", budget)
		n := c.num("RP Count", 1, false)
		fragment := c.num("Fragment RP Count", 1, false)
		c.num("RP Reserved", 2, false)
		c.need(fragment <= n, "PIM Bootstrap fragment RP count exceeds total")
		var rps []map[string]any
		for i := uint64(0); i < fragment && c.err == nil; i++ {
			p := &controlReader{w: c.w, at: c.at, info: map[string]any{}}
			p.info["RP"] = pimAddress(p, "unicast", budget)
			p.num("Hold Time", 2, false)
			p.num("Priority", 1, false)
			p.num("Reserved", 1, false)
			if p.err != nil {
				return p.err
			}
			rps = append(rps, pimRanges(p))
			c.at = p.at
		}
		if c.err != nil {
			return c.err
		}
		c.info["RPs"] = rps
		c.info["RP Set Complete In Message"] = n == fragment
		groups = append(groups, pimRanges(c))
		r.at = c.at
	}
	r.info["Groups"] = groups
	r.info["Bootstrap Reassembly"] = "not attempted; fragment advertisements only"
	return r.err
}

func decodePIMCandidateRP(r *controlReader, budget *int) error {
	n := r.num("Prefix Count", 1, false)
	r.num("Priority", 1, false)
	r.num("Hold Time", 2, false)
	r.info["RP"] = pimAddress(r, "unicast", budget)
	var groups []map[string]any
	for i := uint64(0); i < n && r.err == nil; i++ {
		groups = append(groups, pimAddress(r, "group", budget))
	}
	r.info["Advertised Groups"] = groups
	return r.err
}
