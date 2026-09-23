package netstackvm

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/buffer"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/stack"
)

func TestPCAPEndpointIPLinkTypes(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		buf := gopacket.NewSerializeBuffer()
		var network gopacket.SerializableLayer = &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
		if ipv6 {
			network = &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolICMPv6, SrcIP: net.ParseIP("2001:db8::1"), DstIP: net.ParseIP("2001:db8::2")}
		}
		if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, network, gopacket.Payload(make([]byte, 8))); err != nil {
			t.Fatal(err)
		}
		payload := bytes.Clone(buf.Bytes())
		for _, link := range []layers.LinkType{layers.LinkTypeEthernet, layers.LinkTypeRaw, layers.LinkTypeNull, layers.LinkTypeLoop} {
			t.Run(fmt.Sprintf("%s/ipv6=%t", link, ipv6), func(t *testing.T) {
				var written []byte
				p := &PCAPEndpoint{adaptor: &pcapAdaptor{linkType: link, writer: func(b []byte) error { written = bytes.Clone(b); return nil }}}
				p.Endpoint = channel.New(1, 1500, tcpip.LinkAddress("\x02\x00\x00\x00\x00\x01"))
				p.netBridge = &pcapBridge{external: net.HardwareAddr{2, 0, 0, 0, 0, 1}}
				p.ipToMac = new(sync.Map)
				p.gatewayFound = utils.NewAtomicBool()
				p.SetGatewayHardwareAddr(net.HardwareAddr{2, 0, 0, 0, 0, 2})
				pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{Payload: buffer.MakeWithData(payload)})
				if err := p.writePacket(pkt); err != nil {
					t.Fatal(err)
				}
				decoded := gopacket.NewPacket(written, link, gopacket.Default)
				if decoded.NetworkLayer() == nil || !bytes.Equal(decoded.NetworkLayer().LayerContents(), payload[:len(decoded.NetworkLayer().LayerContents())]) {
					t.Fatalf("invalid network payload for %v", link)
				}
				offset := 0
				if link == layers.LinkTypeEthernet {
					offset = 14
				} else if link != layers.LinkTypeRaw {
					offset = 4
				}
				if len(written) < offset+len(payload) || !bytes.Equal(written[offset:offset+len(payload)], payload) {
					t.Fatal("IP payload was modified")
				}
			})
		}
	}
}
