package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestUnselectedEtherIPCannotAdmitAnARPChild(t *testing.T) {
	for _, family := range []int{4, 6} {
		for _, word := range []uint16{0, 0x3000, 0x3001, 0xf000} {
			t.Run(fmt.Sprintf("IPv%d/header-%04x", family, word), func(t *testing.T) {
				innerEth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 3}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 4}, EthernetType: layers.EthernetTypeARP}
				arp := &layers.ARP{AddrType: layers.LinkTypeEthernet, Protocol: layers.EthernetTypeIPv4, HwAddressSize: 6, ProtAddressSize: 4, Operation: 1, SourceHwAddress: innerEth.SrcMAC, DstHwAddress: make([]byte, 6), SourceProtAddress: []byte{192, 0, 2, 1}, DstProtAddress: []byte{192, 0, 2, 2}}
				inner := gopacket.NewSerializeBuffer()
				require.NoError(t, gopacket.SerializeLayers(inner, gopacket.SerializeOptions{FixLengths: true}, innerEth, arp))
				payload := make([]byte, 2)
				binary.BigEndian.PutUint16(payload, word)
				payload = append(payload, inner.Bytes()...)
				eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
				var ip gopacket.SerializableLayer = &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocol(97), SrcIP: net.ParseIP("198.51.100.1"), DstIP: net.ParseIP("198.51.100.2")}
				prefix := []string{"Ethernet", "IPv4"}
				if family == 6 {
					eth.EthernetType = layers.EthernetTypeIPv6
					ip = &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocol(97), SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
					prefix[1] = "IPv6"
				}
				wire := gopacket.NewSerializeBuffer()
				require.NoError(t, gopacket.SerializeLayers(wire, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, gopacket.Payload(payload)))
				p := gopacket.NewPacket(wire.Bytes(), protocolPacketDecoder{decoder: layers.LinkTypeEthernet}, gopacket.Default)
				require.NotNil(t, p.Layer(layers.LayerTypeARP), "a decoded link-layer child must still pass the parent admission gate")
				domain := CaptureDomain{Interface: 17}
				p.Metadata().CaptureInfo = withEvidence(gopacket.CaptureInfo{Timestamp: time.Unix(100, 0), CaptureLength: len(wire.Bytes()), Length: len(wire.Bytes())}, captureEvidence{Ref: PacketReference{Number: 1, Domain: domain}})
				var events []*ProtocolEvent
				a := coreFragmentParser(&events, 1<<20, 16)
				t.Cleanup(func() { a.closeFragments(); require.Zero(t, a.buffered.Load()) })
				_, consumed := a.networkPacket(p)
				require.True(t, consumed)
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "ip", e.Protocol)
				require.Equal(t, "network-encapsulation-refusal", e.Profile)
				require.Equal(t, "limited", e.Status)
				require.Equal(t, "EncapsulationDepthOrProfile", e.ExpertCode)
				require.Equal(t, prefix, e.Session["Observed Header Layers"])
				require.Equal(t, "unconfirmed", e.Session["Child Header Identity"])
				require.Empty(t, networkAliasValues(e.DisplayFieldValues()))
				require.Equal(t, domain, e.Domain)
				require.Zero(t, e.FlowID)
				require.Nil(t, a.udpSessions)
			})
		}
	}
}
