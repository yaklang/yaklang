package stream_parser

import (
	"encoding/binary"
	"fmt"
)

func ospfv2LSAHeader(r *controlReader) {
	r.num("LS Age", 2, false)
	r.num("Options", 1, false)
	r.num("LS Type", 1, false)
	routingAddress(r, "Link State ID", 4)
	routingAddress(r, "Advertising Router", 4)
	r.num("Sequence", 4, false)
	r.num("Checksum", 2, false)
	r.num("Length", 2, false)
}

func ospfv2Headers(r *controlReader, budget *int) []map[string]any {
	var rows []map[string]any
	for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
		row := ospfv2Child(r, min(r.at+20, len(r.w)), ospfv2LSAHeader)
		if row != nil {
			row["Checksum Verified"] = false
			row["Checksum Scope"] = "unverified: advertised LSA header without body"
			rows = append(rows, row)
		}
	}
	return rows
}

func ospfv2LSA(r *controlReader, budget *int) (map[string]any, error) {
	start := r.at
	if len(r.w)-start < 20 {
		row := ospfv2Child(r, len(r.w), ospfv2LSAHeader)
		if row != nil {
			row["Checksum Verified"] = false
			row["Checksum Scope"] = "unverified: complete LSA header/body unavailable"
		}
		return row, nil
	}
	n := int(binary.BigEndian.Uint16(r.w[start+18:]))
	end := min(start+max(20, n), len(r.w))
	var unsupported error
	row := ospfv2Child(r, end, func(c *controlReader) {
		ospfv2LSAHeader(c)
		c.need(n >= 20, "OSPFv2 LSA length is below header")
		if c.err != nil {
			return
		}
		typ := c.info["LS Type"].(uint64)
		c.info["Body"] = ospfv2Child(c, end, func(body *controlReader) {
			if ospfv2LSABody(body, typ, budget) {
				unsupported = fmt.Errorf("%w: unselected OSPFv2 LSA type %d", ErrObservedWireContext, typ)
			}
		})
	})
	if row != nil {
		row["Checksum Verified"] = false
		row["Checksum Scope"] = "unverified: complete declared LSA unavailable"
		if n >= 20 && start+n <= len(r.w) {
			// Both versions use the same Fletcher layout, exclude LS Age and
			// forbid checksum field0000 even when both residues are zero.
			valid := ospfv3LSAChecksum(r.w[start : start+n])
			row["Checksum Verified"], row["Checksum Scope"] = valid, "complete LSA excluding LS Age; no authentication verdict"
			if !valid {
				r.err = ospfv2IntegrityFailure(r.err, "LSA Fletcher checksum failed")
			}
		}
	}
	r.need(start+n <= len(r.w), "OSPFv2 LSA length exceeds packet")
	return row, unsupported
}

func ospfv2LSABody(r *controlReader, typ uint64, budget *int) (unsupported bool) {
	switch typ {
	case 1:
		r.num("Flags", 1, false)
		r.num("Reserved", 1, false)
		count := r.num("Link Count", 2, false)
		var links []map[string]any
		for i := uint64(0); i < count && r.err == nil && ospfv3Charge(r, budget); i++ {
			// A Router-LSA link contains variable selector/metric entries.
			// Its own count determines the next boundary; never scan for one.
			c := &controlReader{w: r.w, at: r.at, info: map[string]any{}}
			routingAddress(c, "Link ID", 4)
			routingAddress(c, "Link Data", 4)
			linkType := c.num("Link Type", 1, false)
			n := c.num("TOS/MT-ID Count", 1, false)
			c.num("Metric", 2, false)
			var metrics []map[string]any
			for j := uint64(0); j < n && c.err == nil && ospfv3Charge(c, budget); j++ {
				metrics = ospfv2KeepRow(metrics, ospfv2Child(c, min(c.at+4, len(c.w)), func(m *controlReader) {
					m.num("TOS/MT-ID Selector", 1, false)
					m.num("Reserved", 1, false)
					m.num("Metric", 2, false)
				}))
			}
			if c.err != nil {
				r.err = c.err
			}
			c.info["TOS/MT-ID Metrics"] = metrics
			c.info["Selector Scope"] = "observed legacy TOS/MT-ID selector; topology semantics not selected"
			if linkType == 3 {
				c.info["Link Data Meaning"] = "stub network mask bytes; not installed routing state"
			} else {
				c.info["Link Data Meaning"] = "raw interface address or unnumbered ifIndex; interpretation not certified"
			}
			r.at = c.at
			if len(c.fields) > 0 {
				links = append(links, ospfv2Ranges(c))
			}
		}
		r.info["Links"] = links
	case 2:
		routingAddress(r, "Network Mask", 4)
		var routers []map[string]any
		for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
			routers = ospfv2KeepRow(routers, ospfv2Child(r, min(r.at+4, len(r.w)), func(c *controlReader) { routingAddress(c, "Attached Router", 4) }))
		}
		r.info["Attached Routers"] = routers
		r.need(len(routers) != 0, "OSPFv2 Network-LSA lacks an attached router")
	case 3, 4:
		routingAddress(r, "Network Mask", 4)
		if typ == 4 {
			r.info["Network Mask Meaning"] = "not meaningful for ASBR summary; no network prefix inferred"
		}
		var metrics []map[string]any
		for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
			metrics = ospfv2KeepRow(metrics, ospfv2Child(r, min(r.at+4, len(r.w)), func(c *controlReader) {
				c.num("TOS/MT-ID Selector", 1, false)
				c.num("Metric", 3, false)
			}))
		}
		r.need(len(metrics) != 0, "OSPFv2 summary LSA lacks base metric")
		r.info["TOS/MT-ID Metrics"] = metrics
		r.info["Selector Scope"] = "first selector is reserved in base RFC2328; further selectors may represent TOS or MT-ID; topology semantics not selected"
	case 5, 7:
		routingAddress(r, "Network Mask", 4)
		var metrics []map[string]any
		for r.err == nil && r.at < len(r.w) && ospfv3Charge(r, budget) {
			metrics = ospfv2KeepRow(metrics, ospfv2Child(r, min(r.at+12, len(r.w)), func(c *controlReader) {
				flags := c.num("Flags and TOS/MT-ID", 1, false)
				if c.err == nil {
					c.info["External Metric Type"] = uint64(1 + (flags >> 7))
					c.info["TOS/MT-ID Selector"] = flags & 0x7f
				}
				c.num("Metric", 3, false)
				routingAddress(c, "Forwarding Address", 4)
				c.num("External Route Tag", 4, false)
			}))
		}
		r.need(len(metrics) != 0, "OSPFv2 external LSA lacks base metric")
		r.info["TOS/MT-ID Metrics"] = metrics
		r.info["Selector Scope"] = "observed legacy TOS/MT-ID selector; topology semantics not selected"
	default:
		r.rest("Opaque Body")
		return true
	}
	return false
}
