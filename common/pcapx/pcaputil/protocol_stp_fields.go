package pcaputil

import (
	"fmt"
	"strings"
)

func stpDisplayBindings() []displayFieldBinding {
	var out []displayFieldBinding
	for _, f := range []struct {
		name, key string
		bits      int
	}{
		{"stp.protocol", "Protocol ID", 16}, {"stp.version", "Version", 8}, {"stp.type", "BPDU Type", 8}, {"stp.flags", "Flags", 8},
		{"stp.root.prio", "Root Priority", 16}, {"stp.root.ext", "Root System ID Extension", 12}, {"stp.root.cost", "Root Path Cost", 32},
		{"stp.bridge.prio", "Bridge Priority", 16}, {"stp.bridge.ext", "Bridge System ID Extension", 12}, {"stp.port", "Port ID", 16},
		{"stp.flags.port_role", "Port Role", 2}, {"stp.version_1_length", "Version 1 Length", 8},
		{"stp.msg_age.ticks", "Message Age Ticks", 16}, {"stp.max_age.ticks", "Max Age Ticks", 16}, {"stp.hello.ticks", "Hello Time Ticks", 16}, {"stp.forward.ticks", "Forward Delay Ticks", 16},
		{"mstp.version_3_length", "Version 3 Length", 16}, {"mstp.config_format_selector", "MST Configuration Format Selector", 8},
		{"mstp.config_revision_level", "MST Configuration Revision", 16}, {"mstp.cist_internal_root_path_cost", "CIST Internal Root Path Cost", 32},
		{"mstp.cist_bridge.prio", "CIST Bridge Priority", 16}, {"mstp.cist_bridge.ext", "CIST Bridge System ID Extension", 12}, {"mstp.cist_remaining_hops", "CIST Remaining Hops", 8},
	} {
		b := displayRoot("stp", f.name, f.key, DisplayUnsigned, f.bits, "pdu", f.key)
		switch f.key {
		case "Root Priority", "Root System ID Extension":
			b.rangeKey = "Root Priority and Extension"
		case "Bridge Priority", "Bridge System ID Extension":
			b.rangeKey = "Bridge Priority and Extension"
		case "CIST Bridge Priority", "CIST Bridge System ID Extension":
			b.rangeKey = "CIST Bridge Priority and Extension"
		case "Port Role":
			b.rangeKey = "Flags"
		}
		out = append(out, b)
	}
	for _, f := range []struct{ name, key string }{
		{"stp.root.hw", "Root MAC"}, {"stp.bridge.hw", "Bridge MAC"}, {"mstp.cist_bridge.hw", "CIST Bridge MAC"}, {"mstp.config_name", "MST Configuration Name"},
		{"stp.msg_age", "Message Age Seconds"}, {"stp.max_age", "Max Age Seconds"}, {"stp.hello", "Hello Time Seconds"}, {"stp.forward", "Forward Delay Seconds"},
	} {
		b := displayRoot("stp", f.name, f.key, DisplayString, 0, "pdu", f.key)
		if strings.HasSuffix(f.key, " Seconds") {
			key := f.key
			b.custom = func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
				if v, ok := e.Session[key].(float64); ok {
					emit(fmt.Sprintf("%g", v))
				}
			}
			b.path = nil
			b.descriptor.Origin = "derived"
			b.rangeKey = strings.TrimSuffix(key, " Seconds") + " Ticks"
		}
		if f.name == "mstp.config_name" {
			b.rangeKey = "MST Configuration Name Bytes"
			b.descriptor.AllowEmpty = true
		}
		// Seconds are exact finite decimal strings; .ticks is the unsigned
		// numeric filtering contract, avoiding lossy truncation to seconds.
		out = append(out, b)
	}
	for _, f := range []struct{ name, key string }{
		{"tc", "Topology Change"}, {"tcack", "Topology Change Acknowledgement"}, {"proposal", "Proposal"}, {"learning", "Learning"}, {"forwarding", "Forwarding"}, {"agreement", "Agreement"},
	} {
		b := displayRoot("stp", "stp.flags."+f.name, f.key, DisplayBoolean, 0, "pdu", f.key)
		b.rangeKey = "Flags"
		out = append(out, b)
	}
	digest := displayCustom("stp", "mstp.config_digest", "MST configuration digest (unverified)", DisplayString, 0, "pdu", func(e *ProtocolEvent, emit func(any), _ *displayProjection) {
		if v, ok := e.Session["MST Configuration Digest"].([]byte); ok {
			emit(fmt.Sprintf("%x", v))
		}
	})
	digest.rangeKey = "MST Configuration Digest"
	out = append(out, digest)
	for _, f := range []struct {
		name, key, list string
		bits            int
	}{
		{"stp.pvst.tlvtype", "Type", "PVST TLVs", 16}, {"stp.pvst.tlvlen", "Length", "PVST TLVs", 16}, {"stp.pvst.origvlan", "Originating VLAN", "PVST TLVs", 16},
		{"mstp.msti.flags", "Flags", "MST Instances", 8}, {"mstp.msti.priority", "Regional Root Priority Nibble", "MST Instances", 4}, {"mstp.msti.msti_id", "Instance ID", "MST Instances", 12},
		{"mstp.msti.root_cost", "Internal Root Path Cost", "MST Instances", 32}, {"mstp.msti.bridge_priority", "Bridge Priority", "MST Instances", 4},
		{"mstp.msti.port_priority", "Port Priority", "MST Instances", 4}, {"mstp.msti.remaining_hops", "Remaining Hops", "MST Instances", 8},
	} {
		b := displayRoot("stp", f.name, f.key, DisplayUnsigned, f.bits, "pdu", f.key)
		b.path = []string{f.list, "*", f.key}
		b.rangeKey = ""
		out = append(out, b)
	}
	b := displayRoot("stp", "mstp.msti.root.hw", "MSTI regional root MAC", DisplayString, 0, "pdu", "Regional Root MAC")
	b.path = []string{"MST Instances", "*", "Regional Root MAC"}
	b.rangeKey = ""
	out = append(out, b)
	return out
}
