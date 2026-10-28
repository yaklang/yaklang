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

func TestMembershipPartialCarrierPreservesInnerPeersAndPhysicalCapture(t *testing.T) {
	physical := []string{"Ethernet", "IPv4", "GRE", "IPv6", "ICMPv6", "MLDv2MulticastListenerReport"}
	domain := pcaputil.CaptureDomain{Encapsulation: "/gre:key1"}
	e := &pcaputil.ProtocolEvent{Protocol: "icmpv6", Profile: "icmpv6-mld", Transport: "network", Status: "incomplete", Completeness: "incomplete",
		Source: "fe80::1", Destination: "ff02::16", Domain: domain, CarrierLayers: []string{"GRE"}, Summary: "MLDv2 Multicast Listener Report; record source address unavailable",
		Session: map[string]any{"MLD": map[string]any{"Version": uint8(2)}, "Observed Header Layers": physical}}
	row := presentEvents([]*pcaputil.ProtocolEvent{e}).apply(packetSummary{Source: "192.0.2.1", Destination: "198.51.100.1", Layers: physical})
	require.Equal(t, "fe80::1", row.Source)
	require.Equal(t, "ff02::16", row.Destination)
	require.Equal(t, "192.0.2.1", row.PhysicalSource)
	require.Equal(t, "198.51.100.1", row.PhysicalDestination)
	require.Equal(t, &domain, row.MessageDomain)
	require.Equal(t, physical, row.Layers)
	require.Equal(t, []string{"Ethernet", "IPv4", "GRE", "IPv6", "ICMPv6", "MLDv2"}, row.ProtocolStack)
	require.Equal(t, "incomplete", row.ProtocolStatus)
}
