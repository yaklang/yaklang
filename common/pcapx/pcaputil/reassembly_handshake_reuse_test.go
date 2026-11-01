package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestTCPConfirmedHandshakeReuseWithoutObservedCloseIsolatesRPC(t *testing.T) {
	var steps []tcpStep
	var cseq, sseq uint32
	open := func(c, s uint32) {
		steps = append(steps, tcpStep{seq: c, syn: true}, tcpStep{seq: s, ack: c + 1, syn: true, synack: true, reverse: true})
		cseq, sseq = c+1, s+1
	}
	send := func(reverse bool, wire []byte) {
		seq, ack := cseq, sseq
		if reverse {
			seq, ack = sseq, cseq
			sseq += uint32(len(wire))
		} else {
			cseq += uint32(len(wire))
		}
		steps = append(steps, tcpStep{seq: seq, ack: ack, reverse: reverse, data: string(wire)})
	}
	open(99, 499)
	send(false, dcerpcBind(dcerpcEPM, 0, 1))
	send(true, dcerpcBindAck(1))
	partial := dcerpcRequest(2, 0, 2, bytes.Repeat([]byte{0xa5}, 20))
	partial[3] = 1
	send(false, partial) // No closing RST and no last RPC fragment are observed.
	open(999, 1999)
	send(false, dcerpcRequest(2, 0, 2, nil))
	send(true, dcerpcPDU(13, 3, 2, []byte{4, 0, 1, 5, 0, 0, 0, 0}))
	for generation, ctx := range []uint16{1, 0} {
		open(uint32(2999+generation*2000), uint32(3999+generation*2000))
		send(false, dcerpcBind(dcerpcEPM, ctx, 1))
		send(true, dcerpcBindAck(1))
		send(false, dcerpcRequest(2, ctx, 2, nil))
		first := dcerpcResponse(2, ctx, bytes.Repeat([]byte{byte(ctx + 1)}, 16))
		last := bytes.Clone(first)
		first[3], last[3] = 1, 2
		send(true, first)
		send(true, last)
	}
	for _, workers := range []int{1, 2, 4} {
		for _, ipv6 := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers=%d/ipv6=%t", workers, ipv6), func(t *testing.T) {
				var mu sync.Mutex
				observed := map[uint64]uint64{}
				events, stats, err := binReplay(t, binTestPcap(t, steps, 135, ipv6, true), workers,
					WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, _ time.Time) {
						mu.Lock()
						defer mu.Unlock()
						for _, ref := range refs {
							require.NotContains(t, observed, ref.Number, "a held SYN must not be observed twice")
							observed[ref.Number] = f.Index
						}
					}))
				require.NoError(t, err)
				require.Zero(t, stats.BufferedBytes)
				require.Len(t, observed, len(steps), "confirmed held SYN keeps its original packet identity")
				flows := map[uint64][]*ProtocolEvent{}
				for _, e := range events {
					if e.Protocol == "dcerpc" && len(e.Raw) > 0 {
						flows[e.FlowID] = append(flows[e.FlowID], e)
					}
				}
				require.Len(t, flows, 4)
				var ids []uint64
				for id := range flows {
					ids = append(ids, id)
				}
				sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
				require.Len(t, flows[ids[0]], 3)
				unbound := flows[ids[1]]
				require.Len(t, unbound, 2)
				require.Equal(t, "Request", unbound[0].Session["Packet Name"])
				require.NotContains(t, unbound[0].Session, "Interface", "accepted context cannot cross a fresh TCP handshake")
				require.Equal(t, "partial", unbound[0].Session["Context Level"])
				require.Equal(t, "decoded", unbound[1].Status, unbound[1].Error)
				require.Zero(t, unbound[1].ResponseTo)
				for generation, ctx := range []uint16{1, 0} {
					values := flows[ids[generation+2]]
					require.Len(t, values, 5)
					require.Equal(t, "EPM.ept_lookup", values[2].Session["Packet Name"])
					require.Equal(t, ctx, values[2].Session["Context ID"])
					require.Equal(t, values[2].ID, values[4].ResponseTo)
					message := values[4].Session["Reassembled Message"].(map[string]any)
					require.Equal(t, bytes.Repeat([]byte{byte(ctx + 1)}, 32), message["Stub"])
				}
			})
		}
	}
}

func TestTCPUnconfirmedOpeningCannotEvictEstablishedFlow(t *testing.T) {
	for _, start := range []uint32{999, ^uint32(0)} {
		for _, wrong := range []string{"wrong-ack", "same-direction", "old-server-isn", "payload", "no-reply"} {
			t.Run(fmt.Sprintf("%s/isn=%d", wrong, start), func(t *testing.T) {
				p := NewTrafficPool(context.Background())
				defer p.Close()
				var ids []uint64
				var data []string
				p.onFlowCreated = func(f *TrafficFlow) { ids = append(ids, f.Index) }
				p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { data = append(data, string(f.Payload)) })
				feed := func(reverse bool, seq, ack uint32, syn, synack bool, payload string) {
					src, dst, sport, dport := net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), layers.TCPPort(12345), layers.TCPPort(135)
					if reverse {
						src, dst, sport, dport = dst, src, dport, sport
					}
					tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq, Ack: ack, SYN: syn, ACK: !syn || synack}
					tcp.Payload = []byte(payload)
					p.Feed(nil, &layers.IPv4{SrcIP: src, DstIP: dst}, tcp)
				}
				feed(false, 99, 0, true, false, "")
				feed(true, 499, 100, true, true, "")
				feed(false, 100, 500, false, false, "old")
				feed(false, start, 0, true, false, "")
				switch wrong {
				case "wrong-ack":
					feed(true, 1999, start+2, true, true, "")
				case "same-direction":
					feed(false, 1999, start+1, true, true, "")
				case "old-server-isn":
					feed(true, 499, start+1, true, true, "")
				case "payload":
					feed(true, 1999, start+1, true, true, "foreign")
				}
				feed(false, 103, 500, false, false, "tail")
				require.Len(t, ids, 1)
				require.Equal(t, []string{"old", "tail"}, data)
				if wrong == "no-reply" {
					return
				}
				feed(true, 1999, start+1, true, true, "")
				feed(false, start+1, 2000, false, false, "new")
				require.Len(t, ids, 2)
				require.NotEqual(t, ids[0], ids[1])
				require.Equal(t, []string{"old", "tail", "new"}, data)
			})
		}
	}
}
