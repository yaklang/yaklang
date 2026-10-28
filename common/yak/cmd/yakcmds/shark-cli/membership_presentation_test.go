package sharkcli

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func TestMembershipNativeVersionReplacesLegacyGuessOnlyInDisplayStack(t *testing.T) {
	for _, tc := range []struct {
		name, protocol, profile, status, label string
		session                                map[string]any
		physical, want                         []string
	}{
		{"MLD-confirmed", "icmpv6", "icmpv6-mld", "decoded", "MLDv2", map[string]any{"MLD": map[string]any{"Version": uint8(2)}}, []string{"Ethernet", "IPv6", "ICMPv6", "MLDv2MulticastListenerReport"}, []string{"Ethernet", "IPv6", "ICMPv6", "MLDv2"}},
		{"MLD-unconfirmed", "icmpv6", "icmpv6-mld", "limited", "MLD", map[string]any{"MLD": map[string]any{"Message": "Multicast Listener Query"}}, []string{"Ethernet", "IPv4", "ICMPv6", "MLDv2MulticastListenerQuery"}, []string{"Ethernet", "IPv4", "ICMPv6", "MLD"}},
		{"IGMP-confirmed", "igmp", "igmp-membership-wire", "decoded", "IGMPv3", map[string]any{"IGMP": map[string]any{"Version": uint8(3)}}, []string{"Ethernet", "IPv4", "IGMP"}, []string{"Ethernet", "IPv4", "IGMPv3"}},
		{"IGMP-unconfirmed", "igmp", "igmp-membership-wire", "limited", "IGMP", map[string]any{"IGMP": map[string]any{"Message": "Membership Query"}}, []string{"Ethernet", "IPv4", "IGMP"}, []string{"Ethernet", "IPv4", "IGMP"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			completeness := tc.status
			if tc.status == "decoded" {
				completeness = "message"
			}
			e := &pcaputil.ProtocolEvent{Protocol: tc.protocol, Profile: tc.profile, Transport: "network", Status: tc.status, Completeness: completeness, Summary: "Observed membership fields", Session: tc.session}
			before := append([]string(nil), tc.physical...)
			got := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Layers: tc.physical})
			require.Equal(t, tc.label, got.Protocol)
			require.Equal(t, tc.want, got.ProtocolStack)
			require.Equal(t, before, got.Layers)
			require.Equal(t, before, tc.physical)
		})
	}
}
