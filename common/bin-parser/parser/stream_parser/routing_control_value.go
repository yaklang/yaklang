package stream_parser

import (
	"fmt"
	"net"
	"net/netip"
)

// These are observed advertisements/control messages, not router election,
// forwarding-plane health or authenticated peer identity.
func DecodeRoutingControl(w []byte, profile string, limit int) (map[string]any, error) {
	if len(w) == 0 || len(w) > 65507 {
		return nil, fmt.Errorf("routing-control: complete UDP message outside byte budget")
	}
	if limit <= 0 {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{"Authentication Verified": false, "Observation Scope": "complete wire advertisement; peer and operational state unverified"}}
	switch profile {
	case "hsrp":
		decodeRoutingHSRP(r, limit)
	case "glbp":
		decodeRoutingGLBP(r, limit)
	case "bfd":
		decodeRoutingBFD(r)
	default:
		return nil, fmt.Errorf("routing-control: unknown profile")
	}
	r.need(r.at == len(w), "routing-control trailing bytes")
	if r.err != nil {
		return nil, r.err
	}
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	return r.info, nil
}
func routingFieldRanges(fields []tlsCertificateField) map[string]any {
	ranges := map[string]any{}
	seen := map[string]int{}
	var collect func([]tlsCertificateField)
	collect = func(fields []tlsCertificateField) {
		for _, f := range fields {
			if f.Type != "" {
				seen[f.Name]++
				if seen[f.Name] == 1 {
					ranges[f.Name] = []any{uint64(f.Start), uint64(f.End)}
				} else {
					delete(ranges, f.Name)
				}
			}
			collect(f.Children)
		}
	}
	collect(fields)
	return ranges
}
func routingTLVRanges(row map[string]any, fields []tlsCertificateField, start int) {
	ranges := routingFieldRanges(fields)
	ranges["Type"], ranges["Length"] = []any{uint64(start), uint64(start + 1)}, []any{uint64(start + 1), uint64(start + 2)}
	row["Wire Field Ranges"] = ranges
}
func routingAddress(r *controlReader, key string, n int) {
	b := r.take(key, n)
	if r.err != nil {
		return
	}
	if n == 4 {
		r.info[key] = netip.AddrFrom4([4]byte(b)).String()
	} else {
		r.info[key] = netip.AddrFrom16([16]byte(b)).String()
	}
}
func routingStateValid(s uint64) bool {
	return s == 0 || s == 1 || s == 2 || s == 4 || s == 8 || s == 16
}
func routingMD5(r *controlReader) {
	r.num("MD5 Algorithm", 1, false)
	r.num("MD5 Padding", 1, false)
	r.num("MD5 Flags", 2, false)
	routingAddress(r, "MD5 Address", 4)
	r.num("MD5 Key ID", 4, false)
	r.take("MD5 Digest", 16)
}

func decodeRoutingHSRP(r *controlReader, limit int) {
	if r.w[0] == 0 {
		r.num("Version", 1, false)
		op := r.num("Opcode", 1, false)
		r.need(op <= 3, "HSRP opcode")
		r.info["Protocol Version"] = uint64(1)
		r.info["Strong Identity"] = true
		if op == 3 {
			r.info["Packet Name"] = "Interface State Advertisement"
			r.need(r.num("Advertisement Type", 2, false) == 1, "HSRP advertisement type")
			// The advertisement TLV length includes its four-byte type/length
			// header: 14 bytes after the two version/opcode octets.
			r.need(r.num("Advertisement Length", 2, false) == 14, "HSRP advertisement length")
			state := r.num("Advertisement State", 1, false)
			r.need(state >= 1 && state <= 3, "HSRP advertisement state")
			r.num("Advertisement Reserved", 1, false)
			r.num("Active Groups", 2, false)
			r.num("Passive Groups", 2, false)
			r.num("Advertisement Reserved Tail", 4, false)
			return
		}
		r.info["Packet Name"] = map[uint64]string{0: "Hello", 1: "Coup", 2: "Resign"}[op]
		r.need(routingStateValid(r.num("State", 1, false)), "HSRP advertised state")
		r.num("Hello Time", 1, false)
		r.num("Hold Time", 1, false)
		r.num("Priority", 1, false)
		r.num("Group", 1, false)
		r.num("Reserved", 1, false)
		r.take("Text Authentication", 8)
		routingAddress(r, "Virtual IP", 4)
		if r.err == nil && r.at < len(r.w) {
			r.need(len(r.w)-r.at == 30, "HSRP MD5 trailer boundary")
			r.need(r.num("Authentication TLV", 1, false) == 4 && r.num("Authentication Length", 1, false) == 28, "HSRP MD5 TLV")
			routingMD5(r)
		}
		return
	}
	r.info["Protocol Version"] = uint64(2)
	r.info["Packet Name"] = "Interface State"
	var tlvs []map[string]any
	strong := false
	groups := 0
	auth := false
	for r.at < len(r.w) && r.err == nil {
		if len(tlvs) >= limit {
			r.err = ErrObservedWireBudget
			return
		}
		start := r.at
		t := r.num(fmt.Sprintf("TLV %d Type", len(tlvs)), 1, false)
		n := r.num(fmt.Sprintf("TLV %d Length", len(tlvs)), 1, false)
		end := r.at + int(n)
		r.need(end <= len(r.w), "HSRPv2 TLV exceeds datagram")
		if r.err != nil {
			return
		}
		var row map[string]any
		switch t {
		case 1:
			groups++
			r.need(groups == 1 && n == 40, "HSRPv2 group-state count/length")
			if r.err != nil {
				return
			}
			row = r.child("Group State", end, func(c *controlReader) {
				c.need(c.num("Version", 1, false) == 2, "HSRPv2 version")
				op := c.num("Opcode", 1, false)
				c.need(op <= 2, "HSRPv2 opcode")
				c.need(c.num("State", 1, false) <= 6, "HSRPv2 advertised state")
				ip := c.num("IP Version", 1, false)
				c.need(ip == 4 || ip == 6, "HSRPv2 address family")
				c.need(c.num("Group", 2, false) <= 4095, "HSRPv2 group range")
				b := c.take("Identifier", 6)
				c.info["Identifier"] = net.HardwareAddr(b).String()
				c.num("Priority", 4, false)
				c.num("Hello Time", 4, false)
				c.num("Hold Time", 4, false)
				if ip == 4 {
					routingAddress(c, "Virtual IP", 4)
					c.take("Address Padding", 12)
				} else {
					routingAddress(c, "Virtual IP", 16)
				}
				c.info["Packet Name"] = map[uint64]string{0: "Hello", 1: "Coup", 2: "Resign"}[op]
			})
			for k, v := range row {
				r.info[k] = v
			}
			strong = true
		case 2:
			r.need(n == 4, "HSRPv2 interface-state length")
			row = r.child("Interface State", end, func(c *controlReader) { c.num("Active Groups", 2, false); c.num("Passive Groups", 2, false) })
		case 3:
			r.need(!auth && n == 8, "HSRPv2 text authentication length/duplicate")
			auth = true
			row = r.child("Text Authentication", end, func(c *controlReader) { c.take("Authentication Data", 8) })
		case 4:
			r.need(!auth && n == 28, "HSRPv2 MD5 authentication length/duplicate")
			auth = true
			row = r.child("MD5 Authentication", end, routingMD5)
		default:
			row = r.child("Unknown TLV", end, func(c *controlReader) { c.rest("Value") })
		}
		if r.err != nil {
			return
		}
		row["Type"], row["Length"], row["Byte Range"] = t, n, []int{start, end}
		routingTLVRanges(row, r.fields[len(r.fields)-1].Children, start)
		tlvs = append(tlvs, row)
	}
	r.need(len(tlvs) > 0, "HSRPv2 empty TLV list")
	r.info["TLVs"], r.info["Strong Identity"] = tlvs, strong
}

func decodeRoutingGLBP(r *controlReader, limit int) {
	r.need(r.num("Version", 1, false) == 1, "GLBP version")
	r.num("Reserved", 1, false)
	r.need(r.num("Group", 2, false) <= 1023, "GLBP group range")
	r.take("Header Reserved", 2)
	b := r.take("Owner ID", 6)
	r.info["Owner ID"] = net.HardwareAddr(b).String()
	r.info["Packet Name"] = "Advertisement"
	var tlvs []map[string]any
	strong := false
	auth := false
	hello := false
	for r.at < len(r.w) && r.err == nil {
		if len(tlvs) >= limit {
			r.err = ErrObservedWireBudget
			return
		}
		start := r.at
		t := r.num(fmt.Sprintf("TLV %d Type", len(tlvs)), 1, false)
		n := r.num(fmt.Sprintf("TLV %d Length", len(tlvs)), 1, false)
		r.need(n >= 2 && int(n) <= len(r.w)-start, "GLBP TLV exact boundary")
		if r.err != nil {
			return
		}
		end := start + int(n)
		var row map[string]any
		switch t {
		case 1:
			r.need(!hello, "GLBP duplicate Hello TLV")
			hello = true
			row = r.child("Hello", end, func(c *controlReader) {
				c.num("Unknown 10", 1, false)
				state := c.num("Gateway State", 1, false)
				c.need(state == 0 || state == 4 || state == 8 || state == 16 || state == 32, "GLBP gateway state")
				c.num("Unknown 11", 1, false)
				c.num("Priority", 1, false)
				c.take("Unknown 12", 2)
				c.num("Hello Interval", 4, false)
				c.num("Hold Interval", 4, false)
				c.num("Redirect Interval", 2, false)
				c.num("Timeout Interval", 2, false)
				c.take("Unknown 13", 2)
				family := c.num("Address Type", 1, false)
				size := c.num("Address Length", 1, false)
				c.need(family == 1 && size == 4 || family == 2 && size == 16, "GLBP address type/length")
				if family == 1 {
					routingAddress(c, "Virtual IP", 4)
				} else {
					routingAddress(c, "Virtual IP", 16)
				}
			})
			strong = true
		case 2:
			r.need(n == 20, "GLBP forwarder TLV length")
			row = r.child("Forwarder", end, func(c *controlReader) {
				c.num("Forwarder", 1, false)
				state := c.num("Forwarder State", 1, false)
				c.need(state == 0 || state == 4 || state == 32, "GLBP forwarder state")
				c.num("Unknown 21", 1, false)
				c.num("Priority", 1, false)
				c.num("Weight", 1, false)
				c.take("Unknown 22", 7)
				b := c.take("Virtual MAC", 6)
				c.info["Virtual MAC"] = net.HardwareAddr(b).String()
			})
			strong = true
		case 3:
			r.need(!auth, "GLBP duplicate authentication TLV")
			auth = true
			row = r.child("Authentication", end, func(c *controlReader) {
				t := c.num("Authentication Type", 1, false)
				n := c.num("Authentication Length", 1, false)
				c.need(int(n) == end-c.at, "GLBP authentication exact body length")
				if t == 2 {
					c.need(n == 16, "GLBP MD5 digest length")
				}
				if t == 3 {
					c.need(n == 20, "GLBP MD5 chain index and digest length")
					c.num("Chain Index", 4, false)
					c.take("Authentication Data", 16)
				} else {
					c.take("Authentication Data", int(n))
				}
			})
		default:
			row = r.child("Unknown TLV", end, func(c *controlReader) { c.rest("Value") })
		}
		if r.err != nil {
			return
		}
		row["Type"], row["Length"], row["Byte Range"] = t, n, []int{start, end}
		routingTLVRanges(row, r.fields[len(r.fields)-1].Children, start)
		tlvs = append(tlvs, row)
	}
	r.need(len(tlvs) > 0, "GLBP empty TLV list")
	r.info["TLVs"], r.info["Strong Identity"] = tlvs, strong
}

func decodeRoutingBFD(r *controlReader) {
	a := r.num("Version and Diagnostic", 1, false)
	flags := r.num("State and Flags", 1, false)
	mult := r.num("Detect Multiplier", 1, false)
	n := r.num("Length", 1, false)
	r.need(a>>5 == 1 && n >= 24 && int(n) <= len(r.w) && mult != 0, "BFD version/length/detect multiplier")
	r.info["Version"], r.info["Diagnostic"], r.info["State"] = a>>5, a&31, flags>>6
	for key, bit := range map[string]uint64{"Poll": 32, "Final": 16, "Control Plane Independent": 8, "Authentication Present": 4, "Demand": 2, "Multipoint": 1} {
		r.info[key] = flags&bit != 0
	}
	r.need(flags&0x30 != 0x30, "BFD Poll and Final cannot both be set")
	my, peer := r.num("My Discriminator", 4, false), r.num("Your Discriminator", 4, false)
	r.need(my != 0 && (peer != 0 || flags>>6 <= 1), "BFD discriminator/state")
	r.num("Desired Min TX", 4, false)
	r.num("Required Min RX", 4, false)
	r.num("Required Min Echo RX", 4, false)
	r.info["Packet Name"], r.info["Strong Identity"] = "Control", true
	if flags&4 != 0 {
		r.need(n >= 26, "BFD authenticated minimum length")
		end := int(n)
		r.info["Authentication"] = r.child("Authentication", end, func(c *controlReader) {
			t := c.num("Authentication Type", 1, false)
			length := c.num("Authentication Length", 1, false)
			c.need(length >= 2 && int(length) == end-24, "BFD authentication exact length")
			switch t {
			case 1:
				c.need(length >= 4 && length <= 19, "BFD simple password length")
				c.num("Key ID", 1, false)
				c.take("Password", int(length)-3)
			case 2, 3, 4, 5:
				want := uint64(24)
				if t >= 4 {
					want = 28
				}
				c.need(length == want, "BFD digest authentication length")
				c.num("Key ID", 1, false)
				c.num("Reserved", 1, false)
				c.num("Sequence", 4, false)
				c.take("Digest", int(want)-8)
			default:
				c.need(t != 0, "BFD authentication type zero with A bit")
				c.rest("Unselected Authentication Data")
				c.info["Interpretation"] = "unselected authentication type"
			}
		})
	} else {
		r.need(n == 24, "BFD unauthenticated control length")
	}
	if r.err == nil && r.at < len(r.w) {
		r.rest("Datagram Padding")
	}
}

// Validate leading layout only as a routing hint; complete decoding is always
// required before admission. A six-byte interface TLV is not a new identity.
func RoutingControlStrong(fields map[string]any) bool {
	v, _ := fields["Strong Identity"].(bool)
	return v
}
