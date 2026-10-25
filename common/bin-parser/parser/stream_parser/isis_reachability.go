package stream_parser

import (
	"fmt"
	"net"
	"net/netip"
)

func isisLegacyMetrics(r *controlReader, ipReachability bool) {
	for _, name := range []string{"Default", "Delay", "Expense", "Error"} {
		v := r.num(name+" Metric Octet", 1, false)
		r.info[name+" Metric"] = v & 63
		r.info[name+" Metric External"] = v&64 != 0
		if name != "Default" {
			r.info[name+" Metric Supported"] = v&128 == 0
		} else if ipReachability {
			r.info["Up Down"] = v&128 != 0
		} else {
			r.info["Default Metric Reserved Bit"] = v&128 != 0
		}
	}
}

func isisLegacyNeighbors(r *controlReader, typ, pdu uint64, limit *int) {
	var rows []map[string]any
	if typ == 2 {
		r.info["TLV Name"] = "Advertised Legacy IS Reachability"
		v := r.num("Legacy Neighbor Header Octet", 1, false)
		if pdu == 18 {
			r.info["Virtual Flag"] = v&1 != 0
		} else {
			r.info["Reserved Neighbor Octet"] = v
		}
		r.need((len(r.w)-r.at)%11 == 0, "legacy IS reachability requires complete eleven-octet records")
		for r.at < len(r.w) && r.err == nil {
			if !pimCharge(r, limit) {
				return
			}
			row := r.child("IS Reachability", r.at+11, func(c *controlReader) {
				isisLegacyMetrics(c, false)
				isisID(c, "Neighbor Node ID", 7)
				c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
			})
			rows = append(rows, row)
		}
	} else {
		r.info["TLV Name"] = "Advertised Legacy ES Neighbors"
		// ISO10589 (public format in RFC1142 section9.8) uses ONE metric
		// block followed by zero or more System IDs sharing that cost. This
		// deliberately differs from Wireshark/tcpdump's per-neighbor layout.
		r.need(len(r.w)-r.at >= 4 && (len(r.w)-r.at-4)%6 == 0, "legacy ES neighbors require one four-octet metric block and complete six-octet IDs")
		if r.err != nil {
			return
		}
		shared := r.child("Shared Metrics", r.at+4, func(c *controlReader) {
			isisLegacyMetrics(c, false)
			c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
		})
		r.info["Shared Metrics"] = shared
		for r.at < len(r.w) && r.err == nil {
			if !pimCharge(r, limit) {
				return
			}
			row := r.child("ES Neighbor", r.at+6, func(c *controlReader) {
				isisID(c, "System ID", 6)
				c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
			})
			rows = append(rows, row)
		}
	}
	r.info["Neighbors"] = rows
	if typ == 3 {
		r.info["ES Neighbor Count"] = uint64(len(rows))
		r.info["Metric Scope"] = "one shared metric block for every ES ID in this TLV; an empty list is valid"
	}
}

func isisLegacyIPReachability(r *controlReader, typ uint64, limit *int) {
	r.info["TLV Name"] = "Advertised Legacy IPv4 Internal Reachability"
	if typ == 130 {
		r.info["TLV Name"] = "Advertised Legacy IPv4 External Reachability"
	}
	r.need((len(r.w)-r.at)%12 == 0, "legacy IPv4 reachability requires complete twelve-octet records")
	var rows []map[string]any
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, limit) {
			return
		}
		row := r.child("IPv4 Reachability", r.at+12, func(c *controlReader) {
			isisLegacyMetrics(c, true)
			address := c.take("IPv4 Prefix", 4)
			mask := c.take("IPv4 Mask", 4)
			if c.err == nil {
				a, m := netip.AddrFrom4([4]byte(address)), netip.AddrFrom4([4]byte(mask))
				c.info["IPv4 Prefix"], c.info["IPv4 Mask"] = a.String(), m.String()
				bits, width := net.IPMask(mask).Size()
				if width == 32 {
					c.info["Prefix Length"] = uint64(bits)
					c.info["Canonical Prefix"] = netip.PrefixFrom(a, bits).Masked().String()
				} else {
					c.info["Conformance Warning"] = "noncontiguous IPv4 mask; literal advertisement retained, no usable route inferred"
				}
			}
			c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
		})
		rows = append(rows, row)
	}
	r.info["Reachability Entries"] = rows
}

func isisExtendedNeighbors(r *controlReader, limit *int) {
	r.info["TLV Name"] = "Advertised Extended IS Reachability"
	var rows []map[string]any
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, limit) {
			return
		}
		r.need(len(r.w)-r.at >= 11, "extended IS neighbor header truncated")
		if r.err != nil {
			return
		}
		end := r.at + 11 + int(r.w[r.at+10])
		row := r.child("Extended IS Reachability", end, func(c *controlReader) {
			isisID(c, "Neighbor Node ID", 7)
			c.num("Metric", 3, false)
			n := c.num("Sub-TLV Length", 1, false)
			isisSubTLVs(c, c.at+int(n), "te-link", limit)
			c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
		})
		if r.err == nil && row["Unselected Extensions"] == true {
			r.info["Unselected Extensions"] = true
		}
		rows = append(rows, row)
	}
	r.info["Neighbors"] = rows
}

func isisExtendedIPReachability(r *controlReader, ipv6 bool, limit *int) {
	r.info["TLV Name"] = "Advertised Extended IPv4 Reachability"
	fixed, maxBits, subFlag, namespace := 5, 32, byte(64), "ipv4-prefix"
	if ipv6 {
		r.info["TLV Name"] = "Advertised IPv6 Reachability"
		fixed, maxBits, subFlag, namespace = 6, 128, 32, "ipv6-prefix"
	}
	var rows []map[string]any
	for r.at < len(r.w) && r.err == nil {
		if !pimCharge(r, limit) {
			return
		}
		r.need(len(r.w)-r.at >= fixed, "IS-IS reachability header truncated")
		if r.err != nil {
			return
		}
		control := r.w[r.at+4]
		bits := int(control & 63)
		if ipv6 {
			bits = int(r.w[r.at+5])
		}
		r.need(bits <= maxBits, "IS-IS reachability prefix length out of range")
		count := (bits + 7) / 8
		end := r.at + fixed + count
		r.need(end <= len(r.w), "IS-IS encoded prefix truncated")
		if r.err != nil {
			return
		}
		if control&subFlag != 0 {
			r.need(end < len(r.w), "IS-IS prefix sub-TLV length missing")
			if r.err != nil {
				return
			}
			end += 1 + int(r.w[end])
		}
		row := r.child("Reachability Entry", end, func(c *controlReader) {
			c.num("Metric", 4, false)
			flags := c.num("Control Octet", 1, false)
			c.info["Up Down"], c.info["Sub-TLVs Present"] = flags&128 != 0, byte(flags)&subFlag != 0
			if ipv6 {
				c.info["External"], c.info["Reserved Reachability Flags"] = flags&64 != 0, flags&31
				c.num("Prefix Length", 1, false)
			} else {
				c.info["Prefix Length"] = uint64(bits)
			}
			encoded := c.take("Prefix Bytes", count)
			if c.err != nil {
				return
			}
			var address netip.Addr
			if ipv6 {
				var a [16]byte
				copy(a[:], encoded)
				address = netip.AddrFrom16(a)
			} else {
				var a [4]byte
				copy(a[:], encoded)
				address = netip.AddrFrom4(a)
			}
			prefix := netip.PrefixFrom(address, bits).Masked()
			c.info["Prefix Address"], c.info["Canonical Prefix"], c.info["Canonical Prefix Address"] = address.String(), prefix.String(), prefix.Addr().String()
			if prefix.Addr() != address {
				c.info["Conformance Warning"] = "nonzero unused prefix bits retained literally; canonical prefix masks them separately"
			}
			if control&subFlag != 0 {
				n := c.num("Sub-TLV Length", 1, false)
				isisSubTLVs(c, c.at+int(n), namespace, limit)
			}
			c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
		})
		if r.err == nil && row["Unselected Extensions"] == true {
			r.info["Unselected Extensions"] = true
		}
		rows = append(rows, row)
	}
	r.info["Reachability Entries"] = rows
}

func isisSubTLVs(r *controlReader, end int, namespace string, limit *int) {
	r.need(end <= len(r.w) && end >= r.at, "IS-IS sub-TLV block exceeds its entry")
	var rows []map[string]any
	for r.at < end && r.err == nil {
		if !pimCharge(r, limit) {
			return
		}
		r.need(end-r.at >= 2, "IS-IS sub-TLV header truncated")
		if r.err != nil {
			return
		}
		start := r.at
		typ := r.num("Sub-TLV Type", 1, false)
		n := r.num("Sub-TLV Value Length", 1, false)
		r.need(n <= uint64(end-r.at), "IS-IS sub-TLV exceeds its declared block")
		if r.err != nil {
			return
		}
		known := namespace == "te-link" && (typ == 3 || typ == 6 || typ == 8 || typ == 18)
		row := r.child("Sub-TLV", r.at+int(n), func(c *controlReader) {
			if known {
				switch typ {
				case 3:
					c.need(n == 4, "TE administrative group requires four octets")
					c.num("Administrative Group", 4, false)
				case 6, 8:
					c.need(n == 4, "TE IPv4 address requires four octets")
					routingAddress(c, "Address", 4)
				case 18:
					c.need(n == 3, "TE metric requires three octets")
					c.num("TE Metric", 3, false)
				}
			} else {
				c.rest("Opaque Value")
			}
			c.info["Wire Field Ranges"] = isisFieldRanges(c.fields)
		})
		if r.err != nil {
			return
		}
		row["Type"], row["Length"], row["Namespace"] = typ, n, namespace
		spans := row["Wire Field Ranges"].(map[string]any)
		spans["Type"], spans["Length"] = []any{uint64(start), uint64(start + 1)}, []any{uint64(start + 1), uint64(start + 2)}
		if !known {
			r.info["Unselected Extensions"] = true
			row["Selection"] = fmt.Sprintf("unselected %s sub-TLV%d", namespace, typ)
		}
		rows = append(rows, row)
	}
	delete(r.info, "Sub-TLV Type")
	delete(r.info, "Sub-TLV Value Length")
	r.info["Sub-TLVs"] = rows
}
