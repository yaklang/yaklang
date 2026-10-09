package pcaputil

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"net"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

// Both inner directions travel through the same observed outer endpoints.
// Worker routing must pair the inner addresses with the inner TCP ports.
func tunnelWorkerWire(t *testing.T, profile string, reverse bool, seq uint32, syn, fin bool, body string) []byte {
	t.Helper()
	eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
	outer := &layers.IPv4{Version: 4, TTL: 64, SrcIP: net.ParseIP("198.51.100.11"), DstIP: net.ParseIP("198.51.100.12"), Protocol: layers.IPProtocolIPv4}
	src, dst := net.ParseIP("192.0.2.11"), net.ParseIP("192.0.2.12")
	sport, dport := layers.TCPPort(41001), layers.TCPPort(80)
	if reverse {
		src, dst = dst, src
		sport, dport = dport, sport
	}
	ip := &layers.IPv4{Version: 4, TTL: 64, SrcIP: src, DstIP: dst, Protocol: layers.IPProtocolTCP}
	var network gopacket.NetworkLayer = ip
	var inner gopacket.SerializableLayer = ip
	stack := []gopacket.SerializableLayer{eth, outer}
	switch profile {
	case "6in4":
		outer.Protocol = layers.IPProtocolIPv6
		src, dst = net.ParseIP("2001:db8::11"), net.ParseIP("2001:db8::12")
		if reverse {
			src, dst = dst, src
		}
		v6 := &layers.IPv6{Version: 6, HopLimit: 64, SrcIP: src, DstIP: dst, NextHeader: layers.IPProtocolTCP}
		network, inner = v6, v6
	case "gre":
		outer.Protocol = layers.IPProtocolGRE
		stack = append(stack, &layers.GRE{Protocol: layers.EthernetTypeIPv4})
	case "geneve", "vxlan":
		outer.Protocol = layers.IPProtocolUDP
		port := layers.UDPPort(6081)
		if profile == "vxlan" {
			port = 4789
		}
		udp := &layers.UDP{SrcPort: 45000, DstPort: port}
		require.NoError(t, udp.SetNetworkLayerForChecksum(outer))
		stack = append(stack, udp)
		if profile == "geneve" {
			stack = append(stack, &layers.Geneve{Protocol: layers.EthernetTypeTransparentEthernetBridging, VNI: 5013})
		} else {
			stack = append(stack, &layers.VXLAN{ValidIDFlag: true, VNI: 5013})
		}
		stack = append(stack, eth)
	}
	tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq, SYN: syn, ACK: reverse || !syn, FIN: fin, Window: 4096}
	require.NoError(t, tcp.SetNetworkLayerForChecksum(network))
	stack = append(stack, inner, tcp, gopacket.Payload(body))
	b := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, stack...))
	return bytes.Clone(b.Bytes())
}

func TestTunnelWorkerRoutingUsesInnerTuple(t *testing.T) {
	for _, profile := range []string{"ipip", "6in4", "gre", "vxlan", "geneve"} {
		t.Run(profile, func(t *testing.T) {
			a, ok, err := rawFlowKey(tunnelWorkerWire(t, profile, false, 100, false, false, ""), layers.LinkTypeEthernet)
			require.NoError(t, err)
			require.True(t, ok)
			b, ok, err := rawFlowKey(tunnelWorkerWire(t, profile, true, 200, false, false, ""), layers.LinkTypeEthernet)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, a, b, "inner request and response must share canonical worker tuple")
		})
	}
}

func TestTunnelHTTPResponseWorkerParity(t *testing.T) {
	request := "GET /worker HTTP/1.1\r\nHost: tunnel.test\r\n\r\n"
	response := "HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nhello"
	for _, profile := range []string{"ipip", "6in4", "gre", "vxlan", "geneve"} {
		t.Run(profile, func(t *testing.T) {
			capture, err := trafficfixture.ReadFile("industrial-link-core/carriers/captures/tunnel-" + profile + "-http-worker-transaction.pcap")
			require.NoError(t, err)
			var reference []string
			for _, workers := range []int{1, 2, 4} {
				for _, deferred := range []bool{false, true} {
					for _, observe := range []bool{false, true} {
						t.Run(fmt.Sprintf("w%d/d%v/o%v", workers, deferred, observe), func(t *testing.T) {
							var mu sync.Mutex
							var es []*ProtocolEvent
							var stats ProtocolStats
							opts := []CaptureOption{WithTCPReassemblyWorkers(workers), WithProtocolDeferred(deferred), WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); es = append(es, e); mu.Unlock() }), WithOnProtocolStats(func(s ProtocolStats) { stats = s })}
							if observe {
								opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
							}
							require.NoError(t, ReplayPcap(bytes.NewReader(capture), opts...))
							require.Zero(t, stats.BufferedBytes)
							require.Len(t, es, 2)
							require.Equal(t, "http", es[0].Protocol)
							require.Equal(t, "http", es[1].Protocol)
							require.Equal(t, es[0].FlowID, es[1].FlowID)
							require.Equal(t, es[0].Domain, es[1].Domain)
							require.Equal(t, 0, es[0].Direction)
							require.Equal(t, 1, es[1].Direction)
							require.Equal(t, request, string(es[0].Raw))
							require.Equal(t, response, string(es[1].Raw))
							require.Equal(t, es[0].ID, es[1].ResponseTo)
							require.Equal(t, es[0].ID, es[1].TransactionID)
							fields, err := es[1].GetFields()
							require.NoError(t, err)
							assertMVPJSONFields(t, map[string]any{"Message": map[string]any{"HTTP Response": map[string]any{"FirstLine": map[string]any{"Version": "HTTP/1.1", "Status": "200", "Message": "OK"}, "Headers": []string{"Content-Length: 5", ""}, "Body": map[string]any{"Octets": "hello"}}}}, fields)
							for _, e := range es {
								require.Contains(t, []string{"decoded", "deferred"}, e.Status)
								_, err := e.GetFields()
								require.NoError(t, err)
							}
							canonical := smallCanonicalEvents(t, es)
							if reference == nil {
								reference = canonical
							} else {
								require.Equal(t, reference, canonical)
							}
						})
					}
				}
			}
		})
	}
}
