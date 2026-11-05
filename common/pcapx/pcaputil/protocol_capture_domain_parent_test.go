package pcaputil

import (
	"bytes"
	"fmt"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// These controls are authored bytes, not copied public capture traffic.
func captureDomainParentAuditWire(t *testing.T, inner []byte, options []byte, first, flags byte, protocol uint16, etherIP bool) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 9, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 9, 2}, EthernetType: layers.EthernetTypeIPv4}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2")}
	var parts []gopacket.SerializableLayer
	if etherIP {
		ip.Protocol = layers.IPProtocol(97)
		parts = []gopacket.SerializableLayer{eth, ip, gopacket.Payload(append([]byte{0x30, 0}, inner...))}
	} else {
		u := &layers.UDP{SrcPort: 45000, DstPort: 6081}
		require.NoError(t, u.SetNetworkLayerForChecksum(ip))
		h := []byte{first, flags, byte(protocol >> 8), byte(protocol), 0, 0x13, 0x95, 0}
		h = append(h, options...)
		h = append(h, inner...)
		parts = []gopacket.SerializableLayer{eth, ip, u, gopacket.Payload(h)}
	}
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, parts...))
	return bytes.Clone(b.Bytes())
}

func TestCaptureDomainStopsAtUnselectedParent(t *testing.T) {
	base := tunnelAdmissionWire(t, "geneve", 5013, "198.51.100.1", false, 1, false)
	inner := n14AuditPPPoE(t, base[50:], 1, false, 7)
	for _, c := range []struct {
		name         string
		first, flags byte
		protocol     uint16
		options      []byte
		etherIP      bool
	}{
		{name: "version-two", first: 0x80, protocol: 0x6558},
		{name: "version-one", first: 0x40, protocol: 0x6558},
		{name: "oam", flags: 0x80, protocol: 0x6558},
		{name: "critical", flags: 0x40, protocol: 0x6558},
		{name: "reserved-flags", flags: 1, protocol: 0x6558},
		{name: "unselected-options", first: 1, protocol: 0x6558, options: []byte{0, 0, 0, 0}},
		{name: "options-overrun", first: 1, protocol: 0x6558, options: []byte{0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 0}},
		{name: "EtherIP-unselected", etherIP: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			raw := captureDomainParentAuditWire(t, inner, c.options, c.first, c.flags, c.protocol, c.etherIP)
			want := "/geneve:198.51.100.1~198.51.100.2:5013"
			if c.etherIP {
				want = ""
			}
			events := tunnelAdmissionReplay(t, raw)
			require.Len(t, events, 1)
			require.Equal(t, "limited", events[0].Status)
			require.Zero(t, events[0].FlowID)
			require.Equal(t, "unconfirmed", events[0].Session["Child Header Identity"])
			require.Equal(t, "unselected profile", events[0].Session["Encapsulation Admission"])
			// None of the unselected parent's apparent child VLAN/SID is an observed
			// capture-session identity. Preserve the selected outer domain only.
			legacy := gopacket.NewPacket(raw, captureLinkDecoder(layers.LinkTypeEthernet), gopacket.Default)
			legacy.Metadata().CaptureInfo = gopacket.CaptureInfo{InterfaceIndex: 7, CaptureLength: len(raw), Length: len(raw)}
			require.Equal(t, want, PacketDomain(raw, legacy.Metadata().CaptureInfo, layers.LinkTypeEthernet).Encapsulation)
			require.Equal(t, want, CapturePacketEvidence(legacy).Domain.Encapsulation)
			require.Equal(t, 7, CapturePacketEvidence(legacy).Domain.Interface)
		})
	}
}

func TestCaptureDomainKeepsSelectedGenevePPPoEAndOuterVLAN(t *testing.T) {
	base := tunnelAdmissionWire(t, "geneve", 5013, "198.51.100.1", false, 1, false)
	for _, peer := range []byte{1, 2} {
		t.Run(fmt.Sprintf("peer%d", peer), func(t *testing.T) {
			inner := n14AuditPPPoE(t, base[50:], peer, false, 7)
			raw := captureDomainParentAuditWire(t, inner, nil, 0, 0, 0x6558, false)
			want := fmt.Sprintf("/geneve:198.51.100.1~198.51.100.2:5013/vlan:7/pppoe:02:00:00:00:%02x:01~02:00:00:00:%02x:02:30", peer, peer)
			ci := gopacket.CaptureInfo{InterfaceIndex: 9, CaptureLength: len(raw), Length: len(raw)}
			require.Equal(t, CaptureDomain{Interface: 9, Encapsulation: want}, PacketDomain(raw, ci, layers.LinkTypeEthernet))
			changed := bytes.Clone(raw)
			changed[11] ^= 0x20
			require.Equal(t, PacketDomain(raw, ci, layers.LinkTypeEthernet), PacketDomain(changed, ci, layers.LinkTypeEthernet))
			events := tunnelAdmissionReplay(t, raw)
			require.Len(t, events, 1)
			require.Equal(t, "dns", events[0].Protocol)
			require.Equal(t, "decoded", events[0].Status)
			require.Equal(t, want, events[0].Domain.Encapsulation)
			vlan := n14AuditVLAN(raw, 81)
			require.Equal(t, "/vlan:81"+want, PacketDomain(vlan, ci, layers.LinkTypeEthernet).Encapsulation)
		})
	}
}

func FuzzCaptureDomainUnselectedParent(f *testing.F) {
	for _, seed := range []struct {
		first, flags, option, peer byte
		cut                        uint16
	}{
		{0, 0, 0, 1, 0xffff},
		{0x40, 0, 0, 1, 0xffff},
		{0x80, 0, 0, 2, 0xffff},
		{0, 0x80, 0, 1, 0xffff},
		{0, 0x40, 0, 1, 0xffff},
		{1, 0, 2, 2, 0xffff},
		{63, 1, 31, 1, 0xffff},
		{1, 0, 0, 1, 49},
	} {
		f.Add(seed.first, seed.flags, seed.option, seed.peer, seed.cut)
	}
	f.Fuzz(func(t *testing.T, first, flags, option, peer byte, cut uint16) {
		base := tunnelAdmissionWire(t, "geneve", 5013, "198.51.100.1", false, 1, false)
		inner := n14AuditPPPoE(t, base[50:], peer, false, 7)
		options := make([]byte, int(first&63)*4)
		if len(options) >= 4 {
			options[3] = option & 31 // Includes options exceeding Opt Len.
		}
		raw := captureDomainParentAuditWire(t, inner, options, first, flags, 0x6558, false)
		full := len(raw)
		if cut != 0xffff {
			n := int(cut) % (full + 1)
			raw = raw[:n:n]
		}
		ci := gopacket.CaptureInfo{InterfaceIndex: 9, CaptureLength: len(raw), Length: full}
		domain := PacketDomain(raw, ci, layers.LinkTypeEthernet)
		require.Equal(t, 9, domain.Interface)
		if first != 0 || flags != 0 {
			// The literal parent is outside the selected v0 option-free data
			// profile, even if a generic decoder finds apparent inner headers.
			require.NotContains(t, domain.Encapsulation, "/pppoe:")
			require.NotContains(t, domain.Encapsulation, "/vlan:")
		} else if len(raw) == full {
			want := fmt.Sprintf("/geneve:198.51.100.1~198.51.100.2:5013/vlan:7/pppoe:02:00:00:00:%02x:01~02:00:00:00:%02x:02:30", peer, peer)
			require.Equal(t, want, domain.Encapsulation)
		}
		if len(raw) >= 14 {
			changed := bytes.Clone(raw)
			changed[11] ^= 0x20
			require.Equal(t, domain, PacketDomain(changed, ci, layers.LinkTypeEthernet), "an outer Ethernet address cannot rename an inner PPPoE session")
		}
	})
}
