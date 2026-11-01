package sharkcli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestNativeICMPAliasesAppearAtTheirObservedLayerOnce(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, label string
		physical, want        []string
	}{
		{"IPv4", "icmp", "ICMP", []string{"Ethernet", "IPv4", "ICMPv4"}, []string{"Ethernet", "IPv4", "ICMP"}},
		{"IPv6", "icmpv6", "ICMPv6", []string{"Ethernet", "IPv6", "ICMPv6"}, []string{"Ethernet", "IPv6", "ICMPv6"}},
		{"preserve-MLD-child", "icmpv6", "ICMPv6", []string{"Ethernet", "IPv6", "IPv6HopByHop", "ICMPv6", "MLDv2MulticastListenerReport"}, []string{"Ethernet", "IPv6", "IPv6HopByHop", "ICMPv6", "MLDv2MulticastListenerReport"}},
		{"preserve-tunnel", "icmp", "ICMP", []string{"Ethernet", "IPv4", "GRE", "IPv4", "ICMPv4"}, []string{"Ethernet", "IPv4", "GRE", "IPv4", "ICMP"}},
		{"different-family-is-not-an-alias", "icmpv6", "ICMPv6", []string{"Ethernet", "IPv4", "ICMPv4"}, []string{"Ethernet", "IPv4", "ICMPv4", "ICMPv6"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &pcaputil.ProtocolEvent{ID: 1, Protocol: tc.protocol, Transport: "network", Status: "decoded", Completeness: "message", Session: map[string]any{"Type": uint8(8), "Code": uint8(0)}}
			before := append([]string(nil), tc.physical...)
			row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Layers: tc.physical})
			require.Equal(t, tc.label, row.Protocol)
			require.Equal(t, tc.want, row.ProtocolStack)
			require.Equal(t, before, tc.physical, "the observed layer inventory remains literal")
			require.Equal(t, before, row.Layers)
		})
	}
}
