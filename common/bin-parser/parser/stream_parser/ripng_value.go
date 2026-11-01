package stream_parser

import (
	"fmt"
	"net/netip"
)

// DecodeRIPngWire decodes one explicitly bounded RFC2080 message. Advisory
// next hops are message-local; this does not run a router, install routes,
// validate a neighbor/hop limit, or associate an unsolicited response to a query.
func DecodeRIPngWire(w []byte, limit int) (map[string]any, error) {
	if len(w) < 4 || len(w) > 65527 || (len(w)-4)%20 != 0 {
		return nil, fmt.Errorf("ripng: complete header and exact twenty-byte entries required")
	}
	if limit <= 0 || (len(w)-4)/20 > limit {
		return nil, ErrObservedWireBudget
	}
	r := &controlReader{w: w, info: map[string]any{
		"Observation Scope":       "independent IPv6 datagram advertisement; no inherited next hop, installed routing table, matched response or validated neighbor",
		"Authentication Verified": false,
	}}
	command := r.num("Command", 1, false)
	version := r.num("Version", 1, false)
	r.need(command == 1 || command == 2, "RIPng command must be request or response")
	if version != 1 && r.err == nil {
		return nil, fmt.Errorf("%w: unselected RIPng version %d", ErrObservedWireContext, version)
	}
	reserved := r.num("Reserved", 2, false)
	if r.err != nil {
		return nil, r.err
	}
	r.info["Packet Name"] = "RIPng " + []string{"", "Request", "Response"}[command]
	if reserved != 0 {
		r.info["Conformance Warning"] = "nonzero reserved header; literal value retained, not a verified conformant sender"
	}
	nextHop := "originator"
	var entries []map[string]any
	routes, hops := 0, 0
	for r.at < len(w) && r.err == nil {
		row := r.child("Entry", r.at+20, func(c *controlReader) {
			b := c.take("IPv6 Prefix", 16)
			if c.err != nil {
				return
			}
			addr := netip.AddrFrom16([16]byte(b))
			c.info["IPv6 Prefix"] = addr.String()
			tag := c.num("Route Tag", 2, false)
			bits := c.num("Prefix Length", 1, false)
			metric := c.num("Metric", 1, false)
			if metric == 255 {
				c.info["Entry Type"] = "Next Hop"
				hops++
				nextHop = "originator"
				if addr.IsLinkLocalUnicast() {
					nextHop = addr.String()
				} else if !addr.IsUnspecified() {
					c.info["Conformance Warning"] = "non-link-local next-hop advisory; RFC2080 treats it as originator"
				}
				// RFC2080 2.1.1: these octets must be ignored on reception.
				if tag != 0 || bits != 0 {
					c.info["Ignored Next Hop Parameters"] = "nonzero route-tag/prefix-length retained as literal octets; ignored for next-hop selection"
				}
				c.info["Effective Next Hop Advisory"] = nextHop
			} else {
				c.need(bits <= 128 && metric >= 1 && metric <= 16, "RIPng route prefix length or metric outside selected grammar")
				if c.err != nil {
					return
				}
				routes++
				c.info["Entry Type"] = "Route"
				c.info["Effective Next Hop Advisory"] = nextHop
				c.info["Canonical Prefix"] = netip.PrefixFrom(addr, int(bits)).Masked().String()
				if command == 2 {
					c.info["Advertised Unreachable"] = metric == 16
				} else {
					c.info["Entry Type"] = "Route Query"
				}
				if addr.IsMulticast() || addr.IsLinkLocalUnicast() {
					c.info["Conformance Warning"] = "RFC2080 receiver ignores multicast/link-local route destinations; literal advertisement is not an installed route"
				}
			}
			c.info["Wire Field Ranges"] = routingFieldRanges(c.fields)
		})
		if r.err == nil {
			entries = append(entries, row)
		}
	}
	if r.err != nil {
		return nil, r.err
	}
	r.info["Entries"], r.info["Route Count"], r.info["Next Hop Count"] = entries, uint64(routes), uint64(hops)
	r.info["Whole Table Request"] = command == 1 && len(entries) == 1 && entries[0]["IPv6 Prefix"] == "::" && entries[0]["Prefix Length"] == uint64(0) && entries[0]["Metric"] == uint64(16)
	r.info["Wire Field Ranges"] = routingFieldRanges(r.fields)
	r.info["PDU Length"] = uint64(len(w))
	return r.info, nil
}
