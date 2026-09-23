package netstackvm

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/link/channel"
)

func TestNetworkVMBridgeDHCPExchange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	mac := net.HardwareAddr{2, 1, 2, 3, 4, 5}
	link := channel.New(128, 1500, tcpip.LinkAddress(mac))
	v, e := NewNetworkVM(ctx, NetworkVMConfig{Mode: NetworkBridged, Link: link, DHCP: true})
	require.NoError(t, e)
	defer v.Close()
	done := make(chan error, 1)
	go func() {
		for {
			pkt := link.ReadContext(ctx)
			if pkt == nil {
				done <- ctx.Err()
				return
			}
			raw, e := packetBytes(pkt)
			if e != nil {
				done <- e
				return
			}
			parsed := gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
			layer := parsed.Layer(layers.LayerTypeDHCPv4)
			if layer == nil {
				continue
			}
			request := layer.(*layers.DHCPv4)
			var kind layers.DHCPMsgType
			for _, opt := range request.Options {
				if opt.Type == layers.DHCPOptMessageType && len(opt.Data) == 1 {
					kind = layers.DHCPMsgType(opt.Data[0])
				}
			}
			replyType := layers.DHCPMsgTypeOffer
			if kind == layers.DHCPMsgTypeRequest {
				replyType = layers.DHCPMsgTypeAck
			} else if kind != layers.DHCPMsgTypeDiscover {
				continue
			}
			reply := &layers.DHCPv4{Operation: layers.DHCPOpReply, HardwareType: layers.LinkTypeEthernet, HardwareLen: 6, Xid: request.Xid, ClientHWAddr: request.ClientHWAddr, YourClientIP: net.IPv4(10, 76, 0, 8), Options: layers.DHCPOptions{
				layers.NewDHCPOption(layers.DHCPOptMessageType, []byte{byte(replyType)}),
				layers.NewDHCPOption(layers.DHCPOptServerID, []byte{10, 76, 0, 99}),
				layers.NewDHCPOption(layers.DHCPOptSubnetMask, []byte{255, 255, 255, 0}),
				layers.NewDHCPOption(layers.DHCPOptRouter, []byte{10, 76, 0, 1}),
				layers.NewDHCPOption(layers.DHCPOptDNS, []byte{10, 76, 0, 53}),
				layers.NewDHCPOption(layers.DHCPOptLeaseTime, []byte{0, 0, 14, 16}),
			}}
			ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.IPv4(10, 76, 0, 99), DstIP: net.IPv4bcast, Protocol: layers.IPProtocolUDP}
			udp := &layers.UDP{SrcPort: 67, DstPort: 68}
			udp.SetNetworkLayerForChecksum(ip)
			buf := gopacket.NewSerializeBuffer()
			e = gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, ip, udp, reply)
			if e != nil {
				done <- e
				return
			}
			injectIPv4(link, buf.Bytes())
			if replyType == layers.DHCPMsgTypeAck {
				done <- nil
				return
			}
		}
	}()
	require.NoError(t, v.WaitReady(ctx))
	require.NoError(t, <-done)
	require.Equal(t, "10.76.0.8/24", v.Address().String())
	require.Equal(t, "10.76.0.1", v.Gateway().String())
	require.Eventually(t, func() bool { return len(v.DNSServers()) == 1 }, time.Second, time.Millisecond)
	require.Equal(t, "10.76.0.53", v.DNSServers()[0].String())
	require.Equal(t, tcpip.LinkAddress(mac), link.LinkAddress())
}

func TestNetworkVMDHCPTimeoutAndCancellation(t *testing.T) {
	link := channel.New(128, 1500, tcpip.LinkAddress("\x02\x01\x02\x03\x04\x06"))
	v, e := NewNetworkVM(context.Background(), NetworkVMConfig{Mode: NetworkBridged, Link: link, DHCP: true})
	require.NoError(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, v.WaitReady(ctx), context.DeadlineExceeded)
	done := make(chan struct{})
	go func() { v.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("DHCP close blocked")
	}
}
