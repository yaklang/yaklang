package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
)

// OSPFv3WireContext supplies the observed final IPv6 pseudo-header addresses.
// It is not an adjacency, authentication-trailer negotiation or IPsec SA.
type OSPFv3WireContext struct {
	Source, Destination     netip.Addr
	PseudoHeaderUnavailable bool
}

func OSPFv3MessageName(typ uint64) string {
	if typ >= 1 && typ <= 5 {
		return []string{"Hello", "Database Description", "Link State Request", "Link State Update", "Link State Acknowledgment"}[typ-1]
	}
	return "Unknown"
}

func ospfv3Ranges(r *controlReader) map[string]any {
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	return r.info
}

func ospfv3Charge(r *controlReader, budget *int) bool {
	if r.err != nil {
		return false
	}
	if *budget <= 0 {
		r.err = ErrObservedWireBudget
		return false
	}
	*budget--
	return true
}

// Children retain only completely read fields, with absolute PDU offsets.
// A later malformed sibling never licenses scanning for another plausible LSA.
func ospfv3Child(r *controlReader, end int, fn func(*controlReader)) map[string]any {
	if r.err != nil || end < r.at || end > len(r.w) {
		r.need(false, "OSPFv3 child boundary exceeds packet")
		return nil
	}
	c := &controlReader{w: r.w[:end], at: r.at, info: map[string]any{}}
	fn(c)
	c.need(c.at == end, "OSPFv3 child has trailing bytes")
	r.at = c.at
	if c.err != nil {
		r.err = c.err
	}
	return ospfv3Ranges(c)
}

// OSPFv3Checksum covers the OSPF packet, excluding any LLS/authentication
// trailer and enclosing AH. A valid sum is integrity evidence, not identity.
func OSPFv3Checksum(w []byte, context OSPFv3WireContext) uint16 {
	var sum uint32
	add := func(b []byte) {
		for len(b) >= 2 {
			sum += uint32(binary.BigEndian.Uint16(b))
			b = b[2:]
		}
		if len(b) != 0 {
			sum += uint32(b[0]) << 8
		}
	}
	add(context.Source.AsSlice())
	add(context.Destination.AsSlice())
	var pseudo [8]byte
	binary.BigEndian.PutUint32(pseudo[:4], uint32(len(w)))
	pseudo[7] = 89
	add(pseudo[:])
	add(w)
	for sum>>16 != 0 {
		sum = sum&0xffff + sum>>16
	}
	return ^uint16(sum)
}

func ospfv3LSAChecksum(w []byte) bool {
	if len(w) < 20 {
		return false
	}
	var c0, c1 uint32
	for _, b := range w[2:] { // LS Age is excluded from the Fletcher checksum.
		c0 = (c0 + uint32(b)) % 255
		c1 = (c1 + c0) % 255
	}
	return c0 == 0 && c1 == 0
}

// DecodeOSPFv3Wire observes RFC5340's five bounded envelopes and base LSAs.
// Every advertisement is independent: no LSDB, elected router, verified
// neighbor, replay verdict or installed route is inferred from an ID/sequence.
func DecodeOSPFv3Wire(w []byte, context OSPFv3WireContext, budget int) (map[string]any, error) {
	if len(w) > 65535 {
		return nil, ErrObservedWireBudget
	}
	if budget <= 0 {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{}}
	h := &controlReader{w: w, info: map[string]any{}}
	v := h.num("Version", 1, false)
	h.need(v == 3, "OSPFv3 version must be three")
	typ := h.num("Type", 1, false)
	n := h.num("Packet Length", 2, false)
	routingAddress(h, "Router ID", 4)
	routingAddress(h, "Area ID", 4)
	h.num("Checksum", 2, false)
	h.num("Instance ID", 1, false)
	h.num("Reserved", 1, false) // Ignored on receipt; not a protocol signature.
	r.info["Header"] = ospfv3Ranges(h)
	r.info["Packet Name"] = OSPFv3MessageName(typ)
	r.info["Authentication Verified"] = false
	r.info["Observation Scope"] = "bounded wire advertisements only; no LSDB, verified neighbor or installed route"
	r.info["Checksum Verified"] = false
	r.info["Checksum Scope"] = "unverified: enclosing IPv6 pseudo-header not supplied"
	if h.err != nil {
		return r.info, h.err
	}
	if n < 16 {
		return r.info, fmt.Errorf("ospfv3: declared packet length is below header")
	}
	truncated := n > uint64(len(w))
	r.w, r.at = w[:min(int(n), len(w))], 16
	if h.info["Instance ID"].(uint64) >= 64 {
		// RFC5838 assigns these instances other/unassigned address families;
		// old RFC5340 configurations can also use them for base IPv6. Without
		// explicit AF context, do not silently render IPv4 prefixes as IPv6.
		r.info["Unparsed Body"] = bytes.Clone(r.w[16:])
		return r.info, fmt.Errorf("%w: OSPFv3 non-base instance needs address-family context", ErrObservedWireContext)
	}
	var options uint64
	switch typ {
	case 1:
		r.info["Hello"] = ospfv3Child(r, len(r.w), func(c *controlReader) {
			c.num("Interface ID", 4, false)
			c.num("Router Priority", 1, false)
			options = c.num("Options", 3, false)
			c.num("Hello Interval", 2, false)
			c.num("Router Dead Interval", 2, false)
			routingAddress(c, "Designated Router", 4)
			routingAddress(c, "Backup Designated Router", 4)
			var neighbors []map[string]any
			for c.err == nil && c.at < len(c.w) && ospfv3Charge(c, &budget) {
				neighbors = append(neighbors, ospfv3Child(c, min(c.at+4, len(c.w)), func(row *controlReader) { routingAddress(row, "Neighbor Router ID", 4) }))
			}
			c.info["Neighbors"] = neighbors
		})
	case 2:
		r.info["Database Description"] = ospfv3Child(r, len(r.w), func(c *controlReader) {
			c.num("Reserved 1", 1, false)
			options = c.num("Options", 3, false)
			c.num("Interface MTU", 2, false)
			c.num("Reserved 2", 1, false)
			c.num("Flags", 1, false)
			c.num("DD Sequence", 4, false)
			c.info["LSA Headers"] = ospfv3Headers(c, &budget)
		})
	case 3:
		var requests []map[string]any
		for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, &budget) {
			requests = append(requests, ospfv3Child(r, min(r.at+12, len(r.w)), func(c *controlReader) {
				c.num("Reserved", 2, false)
				c.num("LS Type", 2, false)
				routingAddress(c, "Link State ID", 4)
				routingAddress(c, "Advertising Router", 4)
			}))
		}
		r.info["Requests"] = requests
	case 4:
		count := r.num("LSA Count", 4, false)
		var lsas []map[string]any
		for i := uint64(0); i < count && r.err == nil && ospfv3Charge(r, &budget); i++ {
			lsas = append(lsas, ospfv3LSA(r, &budget))
		}
		r.info["LSAs"] = lsas
	case 5:
		r.info["LSA Headers"] = ospfv3Headers(r, &budget)
	default:
		r.err = fmt.Errorf("%w: unselected OSPFv3 message type %d", ErrObservedWireContext, typ)
	}
	r.need(r.at == len(r.w), "OSPFv3 packet has trailing body bytes")
	ospfv3Ranges(r)
	if truncated {
		r.need(false, "OSPFv3 declared packet exceeds captured message")
	}
	if r.err != nil {
		if r.at <= len(r.w) {
			r.info["Unparsed Body"] = bytes.Clone(r.w[r.at:])
		}
		return r.info, r.err
	}
	// RFC7166 suppresses this checksum when AT is negotiated. A stateless
	// observer cannot infer the AT state for the other three packet types.
	if options&0x400 != 0 || len(w) != int(n) {
		r.info["Authentication Trailer Bit"] = options&0x400 != 0
		r.info["LLS Bit"] = options&0x200 != 0
		r.info["Checksum Scope"] = "unverified: trailer/LLS or neighbor authentication context required"
		r.info["Trailing Bytes"] = bytes.Clone(w[int(n):])
		return r.info, fmt.Errorf("%w: OSPFv3 trailer/LLS authentication context not supplied", ErrObservedWireContext)
	}
	if context.PseudoHeaderUnavailable {
		r.info["Checksum Scope"] = "unverified: final IPv6 pseudo-header destination unavailable"
		return r.info, fmt.Errorf("%w: OSPFv3 final IPv6 pseudo-header destination unavailable", ErrObservedWireContext)
	}
	if context.Source.Is6() && context.Destination.Is6() {
		valid := OSPFv3Checksum(r.w, context) == 0
		r.info["Checksum Verified"], r.info["Checksum Scope"] = valid, "OSPF packet and IPv6 pseudo-header; authentication unverified"
		if !valid {
			return r.info, fmt.Errorf("ospfv3: checksum failed")
		}
	}
	return r.info, nil
}

func ospfv3LSAHeader(r *controlReader) {
	r.num("LS Age", 2, false)
	r.num("LS Type", 2, false)
	routingAddress(r, "Link State ID", 4)
	routingAddress(r, "Advertising Router", 4)
	r.num("Sequence", 4, false)
	r.num("Checksum", 2, false)
	n := r.num("Length", 2, false)
	r.need(n >= 20, "OSPFv3 LSA length below header")
}

func ospfv3Headers(r *controlReader, budget *int) []map[string]any {
	var rows []map[string]any
	for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
		row := ospfv3Child(r, min(r.at+20, len(r.w)), ospfv3LSAHeader)
		if row != nil {
			row["Checksum Verified"] = false
			row["Checksum Scope"] = "unverified: advertised LSA header without body"
		}
		rows = append(rows, row)
	}
	return rows
}

func ospfv3LSA(r *controlReader, budget *int) map[string]any {
	start := r.at
	if len(r.w)-start < 20 {
		r.need(false, "OSPFv3 LSA header truncated")
		return nil
	}
	n := int(binary.BigEndian.Uint16(r.w[start+18 : start+20]))
	truncated := n > len(r.w)-start
	end := min(start+max(20, n), len(r.w))
	row := ospfv3Child(r, end, func(c *controlReader) {
		ospfv3LSAHeader(c)
		if c.err != nil {
			return
		}
		typ := uint16(c.info["LS Type"].(uint64))
		c.info["Body"] = ospfv3Child(c, len(c.w), func(b *controlReader) { ospfv3LSABody(b, typ, budget) })
	})
	if row != nil {
		row["Checksum Verified"] = false
		row["Checksum Scope"] = "unverified: complete LSA body unavailable"
		if !truncated && n >= 20 {
			valid := ospfv3LSAChecksum(r.w[start : start+n])
			row["Checksum Verified"], row["Checksum Scope"] = valid, "complete LSA excluding LS Age; authentication unverified"
			r.need(valid, "OSPFv3 LSA Fletcher checksum failed")
		}
	}
	r.need(!truncated, "OSPFv3 LSA length exceeds packet")
	return row
}

// The prefix representation occupies ceil(PrefixLength/32) words, not
// PrefixLength bytes or a truncated bit/8 count (RFC5340 Appendix A.4.1).
func ospfv3Prefix(r *controlReader, metric bool, budget *int) map[string]any {
	if !ospfv3Charge(r, budget) {
		return nil
	}
	c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
	n := c.num("Prefix Length", 1, false)
	c.num("Prefix Options", 1, false)
	if metric {
		c.num("Metric", 2, false)
	} else {
		c.num("Reserved", 2, false)
	}
	c.need(n <= 128, "OSPFv3 prefix exceeds IPv6 address width")
	width := int((n + 31) / 32 * 4)
	b := c.take("Encoded Prefix", width)
	if c.err == nil {
		var address [16]byte
		copy(address[:], b)
		c.info["Prefix"] = netip.PrefixFrom(netip.AddrFrom16(address), int(n)).Masked().String()
		c.info["Encoded Prefix Bytes"] = uint64(width)
		if width > 0 {
			c.fields = append(c.fields, tlsCertificateField{Name: "Prefix", Type: "raw", Start: c.at - width, End: c.at})
		}
	}
	r.at = c.at
	if c.err != nil {
		r.err = c.err
	}
	return ospfv3Ranges(c)
}

func ospfv3Prefixes(r *controlReader, n uint64, metric bool, budget *int) []map[string]any {
	var rows []map[string]any
	for i := uint64(0); i < n && r.err == nil; i++ {
		rows = append(rows, ospfv3Prefix(r, metric, budget))
	}
	return rows
}

func ospfv3LSABody(r *controlReader, typ uint16, budget *int) {
	switch typ {
	case 0x2001: // Router-LSA: one fixed16-byte record per described link.
		r.num("Flags", 1, false)
		r.num("Options", 3, false)
		var links []map[string]any
		for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
			links = append(links, ospfv3Child(r, min(r.at+16, len(r.w)), func(c *controlReader) {
				c.num("Link Type", 1, false)
				c.num("Reserved", 1, false)
				c.num("Metric", 2, false)
				c.num("Interface ID", 4, false)
				c.num("Neighbor Interface ID", 4, false)
				routingAddress(c, "Neighbor Router ID", 4)
			}))
		}
		r.info["Links"] = links
	case 0x2002: // Network-LSA.
		r.num("Reserved", 1, false)
		r.num("Options", 3, false)
		var routers []map[string]any
		for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
			routers = append(routers, ospfv3Child(r, min(r.at+4, len(r.w)), func(c *controlReader) { routingAddress(c, "Attached Router", 4) }))
		}
		r.info["Attached Routers"] = routers
	case 0x2003: // Inter-Area-Prefix-LSA.
		r.num("Reserved", 1, false)
		r.num("Metric", 3, false)
		r.info["Prefixes"] = []map[string]any{ospfv3Prefix(r, false, budget)}
	case 0x2004: // Inter-Area-Router-LSA.
		r.num("Reserved 1", 1, false)
		r.num("Options", 3, false)
		r.num("Reserved 2", 1, false)
		r.num("Metric", 3, false)
		routingAddress(r, "Destination Router", 4)
	case 0x4005, 0x2007: // AS-External or NSSA-LSA.
		flags := r.num("Flags", 1, false)
		r.num("Metric", 3, false)
		if !ospfv3Charge(r, budget) {
			return
		}
		// This prefix's last word is a referenced LS type, not reserved.
		c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
		bits := c.num("Prefix Length", 1, false)
		c.num("Prefix Options", 1, false)
		reference := c.num("Referenced LS Type", 2, false)
		c.need(bits <= 128, "OSPFv3 external prefix exceeds IPv6 address width")
		if c.err == nil {
			width := int((bits + 31) / 32 * 4)
			data := c.take("Encoded Prefix", width)
			if c.err == nil {
				var a [16]byte
				copy(a[:], data)
				c.info["Prefix"] = netip.PrefixFrom(netip.AddrFrom16(a), int(bits)).Masked().String()
				c.info["Encoded Prefix Bytes"] = uint64(width)
				if width > 0 {
					c.fields = append(c.fields, tlsCertificateField{Name: "Prefix", Type: "raw", Start: c.at - width, End: c.at})
				}
			}
		}
		r.info["Prefixes"] = []map[string]any{ospfv3Ranges(c)}
		r.at, r.err = c.at, c.err
		if flags&2 != 0 {
			routingAddress(r, "Forwarding Address", 16)
		}
		if flags&1 != 0 {
			r.num("External Route Tag", 4, false)
		}
		if reference != 0 {
			routingAddress(r, "Referenced Link State ID", 4)
		}
	case 0x0008: // Link-LSA.
		r.num("Router Priority", 1, false)
		r.num("Options", 3, false)
		routingAddress(r, "Link Local Address", 16)
		n := r.num("Prefix Count", 4, false)
		r.info["Prefixes"] = ospfv3Prefixes(r, n, false, budget)
	case 0x2009: // Intra-Area-Prefix-LSA.
		n := r.num("Prefix Count", 2, false)
		r.num("Referenced LS Type", 2, false)
		routingAddress(r, "Referenced Link State ID", 4)
		routingAddress(r, "Referenced Advertising Router", 4)
		r.info["Prefixes"] = ospfv3Prefixes(r, n, true, budget)
	default:
		r.rest("Opaque Body")
		r.err = fmt.Errorf("%w: unselected OSPFv3 LSA type 0x%04x", ErrObservedWireContext, typ)
	}
}
