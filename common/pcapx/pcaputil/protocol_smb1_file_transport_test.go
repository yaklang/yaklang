package pcaputil

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestSMB1FileCommandsTCPRecoveryAndGeneration(t *testing.T) {
	for _, cmd := range []byte{0x75, 0x06} {
		tid := uint16(2048)
		if cmd == 0x75 {
			tid = 0
		}
		req, rsp := smb1FileWire(cmd, false, 4, 2048, tid), smb1FileWire(cmd, true, 4, 2048, 2048)
		nextReq, nextRsp := smb1FileWire(cmd, false, 5, 2048, tid), smb1FileWire(cmd, true, 5, 2048, 2048)
		steps := []tcpStep{{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true},
			{seq: 100, data: string(req[:8])}, {seq: 140, data: string(req[40:])}, {seq: 108, data: string(req[8:40])},
			{seq: 1000, reverse: true, data: string(rsp[:8])}, {seq: 1036, reverse: true, data: string(rsp[36:])}, {seq: 1008, reverse: true, data: string(rsp[8:36])},
			{seq: 1000, reverse: true, data: string(rsp)},
			{seq: 100 + uint32(len(req)), rst: true},
			{seq: 9000, syn: true}, {seq: 12000, syn: true, reverse: true},
			{seq: 12001, reverse: true, data: string(rsp)},
			{seq: 9001, data: string(req)}, {seq: 12001 + uint32(len(rsp)), reverse: true, data: string(rsp)},
			{seq: 9001 + uint32(len(req)), data: string(nextReq)}, {seq: 12001 + uint32(2*len(rsp)), reverse: true, data: string(nextRsp)}}
		for _, workers := range []int{1, 2, 4} {
			for _, deferred := range []bool{false, true} {
				for _, ipv6 := range []bool{false, true} {
					t.Run(fmt.Sprintf("command%02x/workers%d/deferred%v/ipv6%v", cmd, workers, deferred, ipv6), func(t *testing.T) {
						events, stats, err := binReplay(t, binTestPcap(t, steps, 445, ipv6, true), workers, WithBinParserDeferred(deferred))
						require.NoError(t, err)
						require.Len(t, events, 7, "a TCP retransmission is not a duplicate application request/response")
						require.Equal(t, events[0].ID, events[1].ResponseTo)
						for _, i := range []int{2, 3, 4} {
							require.Zero(t, events[i].ResponseTo)
							require.NotContains(t, events[i].Session, "Request Event ID")
						}
						require.Equal(t, events[5].ID, events[6].ResponseTo)
						require.NotEqual(t, events[0].FlowID, events[2].FlowID)
						require.NotEqual(t, events[0].TCPFlowID, events[2].TCPFlowID)
						require.Len(t, events[0].SourceBytes.PacketRefs, 3)
						require.Len(t, events[1].SourceBytes.PacketRefs, 3)
						wantStatus := "decoded"
						if deferred {
							wantStatus = "deferred"
						}
						for _, e := range events {
							fields, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, wantStatus, e.Status)
							require.Equal(t, false, fields["Authentication Verified"])
						}
						require.Zero(t, stats.BufferedBytes)
					})
				}
			}
		}
		missing := []tcpStep{{seq: 99, syn: true}, {seq: 999, syn: true, reverse: true}, {seq: 100, data: string(req)},
			{seq: 1000, reverse: true, data: string(rsp[:8])}, {seq: 1036, reverse: true, data: string(rsp[36:])}, {seq: 1000 + uint32(len(rsp)), reverse: true, fin: true}}
		for _, allow := range []bool{false, true} {
			t.Run(fmt.Sprintf("command%02x/permanent-gap/allow%v", cmd, allow), func(t *testing.T) {
				events, stats, err := binReplay(t, binTestPcap(t, missing, 445, false, false), 1, WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: allow}))
				if allow {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "unfilled sequence gap")
				}
				require.GreaterOrEqual(t, len(events), 2)
				for _, e := range events[1:] {
					require.NotEqual(t, "decoded", e.Status)
					require.Zero(t, e.ResponseTo)
					require.Nil(t, e.SourceBytes.CompletedIn)
					_, fieldErr := e.GetFields()
					require.Error(t, fieldErr)
				}
				require.Zero(t, stats.BufferedBytes)
			})
		}
	}
}

func TestSMB1FileCommandsCaptureDomainsDoNotBorrowRequests(t *testing.T) {
	for _, cmd := range []byte{0x75, 0x06} {
		for _, vlan := range []bool{false, true} {
			for _, workers := range []int{1, 3} {
				for _, deferred := range []bool{false, true} {
					t.Run(fmt.Sprintf("command%02x/vlan%v/workers%d/deferred%v", cmd, vlan, workers, deferred), func(t *testing.T) {
						var mu sync.Mutex
						var events []*ProtocolEvent
						a, err := NewPacketAnalyzer(WithOnProtocolMessage(func(e *ProtocolEvent) { mu.Lock(); events = append(events, e); mu.Unlock() }), WithTCPReassemblyWorkers(workers), WithBinParserDeferred(deferred))
						require.NoError(t, err)
						defer a.Close()
						tid := uint16(2048)
						if cmd == 0x75 {
							tid = 0
						}
						req, rsp := smb1FileWire(cmd, false, 4, 2048, tid), smb1FileWire(cmd, true, 4, 2048, 2048)
						req5, rsp5 := smb1FileWire(cmd, false, 5, 2048, tid), smb1FileWire(cmd, true, 5, 2048, 2048)
						req6, rsp6 := smb1FileWire(cmd, false, 6, 2048, tid), smb1FileWire(cmd, true, 6, 2048, 2048)
						wires := [][]byte{req, rsp, rsp, req5, rsp5, rsp5, req6, rsp6}
						domains, dirs := []int{0, 1, 0, 0, 1, 0, 1, 1}, []int{0, 1, 1, 0, 1, 1, 0, 1}
						seq := [2][2]uint32{{1, 1}, {1, 1}}
						for i, w := range wires {
							domain, dir := domains[i], dirs[i]
							src, dst := net.IPv4(192, 0, 2, 1), net.IPv4(192, 0, 2, 2)
							sport, dport := layers.TCPPort(50000), layers.TCPPort(445)
							if dir == 1 {
								src, dst, sport, dport = dst, src, dport, sport
							}
							eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
							ip := &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: src, DstIP: dst}
							tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq[domain][dir], ACK: true, PSH: true, Window: 4096}
							seq[domain][dir] += uint32(len(w))
							require.NoError(t, tcp.SetNetworkLayerForChecksum(ip))
							serial := []gopacket.SerializableLayer{eth, ip, tcp, gopacket.Payload(w)}
							iface := domain
							if vlan {
								iface = 0
								eth.EthernetType = layers.EthernetTypeDot1Q
								serial = []gopacket.SerializableLayer{eth, &layers.Dot1Q{VLANIdentifier: uint16(80 + domain), Type: layers.EthernetTypeIPv4}, ip, tcp, gopacket.Payload(w)}
							}
							b := gopacket.NewSerializeBuffer()
							require.NoError(t, gopacket.SerializeLayers(b, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, serial...))
							raw := b.Bytes()
							require.NoError(t, a.Feed(raw, gopacket.CaptureInfo{Timestamp: time.Unix(int64(100+i), 0), CaptureLength: len(raw), Length: len(raw), InterfaceIndex: iface}, layers.LinkTypeEthernet))
							clear(raw)
						}
						require.NoError(t, a.Close())
						require.Len(t, events, 8)
						byFrame := map[uint64]*ProtocolEvent{}
						for _, e := range events {
							require.NotNil(t, e.SourceBytes.CompletedIn)
							n := e.SourceBytes.CompletedIn.Number
							require.NotContains(t, byFrame, n)
							byFrame[n] = e
							fields, err := e.GetFields()
							require.NoError(t, err)
							require.Equal(t, false, fields["Authentication Verified"])
						}
						for _, pair := range [][2]uint64{{1, 3}, {4, 6}, {7, 8}} {
							require.Equal(t, byFrame[pair[0]].ID, byFrame[pair[1]].ResponseTo)
						}
						for _, n := range []uint64{2, 5} {
							require.Zero(t, byFrame[n].ResponseTo)
							require.NotContains(t, byFrame[n].Session, "Request Event ID")
						}
						require.NotEqual(t, byFrame[1].FlowID, byFrame[2].FlowID)
						require.NotEqual(t, byFrame[1].SourceBytes.CompletedIn.Domain, byFrame[2].SourceBytes.CompletedIn.Domain)
					})
				}
			}
		}
	}
}
