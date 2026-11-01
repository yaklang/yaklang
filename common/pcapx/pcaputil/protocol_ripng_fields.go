package pcaputil

import "fmt"

func ripngDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, f := range []struct {
		name, key string
		bits      int
	}{
		{"ripng.cmd", "Command", 8}, {"ripng.version", "Version", 8},
		{"ripng.rte.route_tag", "Route Tag", 16}, {"ripng.rte.prefix_length", "Prefix Length", 8}, {"ripng.rte.metric", "Metric", 8},
	} {
		b := displayRoot("ripng", f.name, f.key, DisplayUnsigned, f.bits, "pdu", f.key)
		if f.name != "ripng.cmd" && f.name != "ripng.version" {
			b.path, b.rangeKey = []string{"Entries", "*", f.key}, ""
		}
		out = append(out, b)
	}
	b := displayRoot("ripng", "ripng.rte.ipv6_prefix", "Literal IPv6 prefix/next-hop octets", DisplayAddress, 128, "pdu", "IPv6 Prefix")
	b.path, b.rangeKey = []string{"Entries", "*", "IPv6 Prefix"}, ""
	out = append(out, b)
	b = displayCustom("ripng", "ripng.reserved", "Reserved header octets", DisplayString, 0, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		if v, ok := displayUint(e.Session["Reserved"]); ok {
			emit(fmt.Sprintf("%04x", v))
		}
	})
	b.rangeKey = "Reserved"
	return append(out, b)
}
