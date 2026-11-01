package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestTCPOpeningObservationsKeepEveryPacketAndOriginalOpener(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, ipv6 := range []bool{false, true} {
			for _, reversed := range []bool{false, true} {
				for _, close := range []string{"absent", "late-RST", "late-FINs", "sender-FIN-retry"} {
					t.Run(fmt.Sprintf("w%d/v6%t/reverse%t/%s", workers, ipv6, reversed, close), func(t *testing.T) {
						var steps []tcpStep
						var owners []int
						add := func(owner int, step tcpStep) { steps = append(steps, step); owners = append(owners, owner) }
						add(0, tcpStep{seq: 99, syn: true})
						add(0, tcpStep{seq: 499, ack: 100, syn: true, synack: true, reverse: true})
						add(0, tcpStep{seq: 100, ack: 500, data: "old"})
						for i := 0; i < 3; i++ {
							add(1, tcpStep{seq: 999, syn: true, reverse: reversed})
						}
						switch close {
						case "late-RST":
							add(0, tcpStep{seq: 103, ack: 500, rst: true})
						case "late-FINs":
							add(0, tcpStep{seq: 103, ack: 500, fin: true})
							add(0, tcpStep{seq: 500, ack: 104, fin: true, reverse: true})
						case "sender-FIN-retry":
							add(0, tcpStep{seq: 103, ack: 500, fin: true})
							add(1, tcpStep{seq: 999, syn: true, reverse: reversed})
						}
						add(1, tcpStep{seq: 1999, ack: 1000, syn: true, synack: true, reverse: !reversed})
						add(1, tcpStep{seq: 1000, ack: 2000, data: "new", reverse: reversed})
						add(1, tcpStep{seq: 2000, ack: 1003, data: "reply", reverse: !reversed})
						wire := binTestPcap(t, steps, 135, ipv6, true)
						reader, err := pcapgo.NewNgReader(bytes.NewReader(wire), pcapgo.DefaultNgReaderOptions)
						require.NoError(t, err)
						var times []time.Time
						var openerMAC string
						for i := range steps {
							data, ci, err := reader.ReadPacketData()
							require.NoError(t, err)
							if i == 3 {
								packet := gopacket.NewPacket(data, layers.LayerTypeEthernet, gopacket.Default)
								openerMAC = packet.Layer(layers.LayerTypeEthernet).(*layers.Ethernet).SrcMAC.String()
							}
							times = append(times, ci.Timestamp)
						}
						var mu sync.Mutex
						var ids []uint64
						var sources, macs []string
						observed := map[uint64]uint64{}
						observedTimes := map[uint64]time.Time{}
						pools := map[*TrafficPool]bool{}
						err = ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyWorkers(workers), WithTCPReassemblyStream(64),
							WithOnTrafficFlowCreated(func(f *TrafficFlow) {
								mu.Lock()
								defer mu.Unlock()
								ids = append(ids, f.Index)
								sources = append(sources, f.ClientConn.LocalAddr().String())
								macs = append(macs, f.HardwareSrcMac)
								pools[f.pool] = true
							}), WithOnTrafficFlowPacket(func(f *TrafficFlow, _ *TrafficConnection, refs []PacketReference, ts time.Time) {
								mu.Lock()
								defer mu.Unlock()
								for _, ref := range refs {
									require.NotContains(t, observed, ref.Number)
									observed[ref.Number], observedTimes[ref.Number] = f.Index, ts
								}
							}))
						require.NoError(t, err)
						require.Len(t, ids, 2)
						require.Len(t, observed, len(steps), "every confirmed opening and retransmission must retain its own packet")
						for i, owner := range owners {
							require.Equal(t, ids[owner], observed[uint64(i+1)], "frame %d belongs to its original generation", i+1)
							require.Equal(t, times[i], observedTimes[uint64(i+1)], "frame %d keeps its original capture time", i+1)
						}
						wantSource := "192.0.2.1:12345"
						if reversed {
							wantSource = "192.0.2.2:135"
						}
						if ipv6 {
							wantSource = "[2001:db8::1]:12345"
							if reversed {
								wantSource = "[2001:db8::2]:135"
							}
						}
						require.Equal(t, wantSource, sources[1])
						require.Equal(t, openerMAC, macs[1], "opening hardware source is captured, not inferred from the confirmation")
						generationPoolsReleased(t, pools)
					})
				}
			}
		}
	}
}

func openingBudgetFeed(p *TrafficPool, reverse bool, seq, ack uint32, syn, synack, rst bool, payload string, number uint64) {
	src, dst, sport, dport := net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2"), layers.TCPPort(12345), layers.TCPPort(135)
	if reverse {
		src, dst, sport, dport = dst, src, dport, sport
	}
	tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: seq, Ack: ack, SYN: syn, ACK: !syn || synack, RST: rst}
	tcp.Payload = []byte(payload)
	p.feedEvidence(nil, &layers.IPv4{SrcIP: src, DstIP: dst}, tcp, captureEvidence{Ref: PacketReference{Number: number}}, time.Unix(1, int64(number)))
}

func TestTCPOpeningObservationBudgetAndLifetime(t *testing.T) {
	for _, ending := range []string{"confirm", "replace", "RST-before-confirm", "unsupported-opening", "per-direction-limit", "global-limit", "release", "cancel"} {
		t.Run(ending, func(t *testing.T) {
			options := TCPReassemblyOptions{MaxPendingSegments: 8, MaxTotalPendingSegments: 8}
			if ending == "per-direction-limit" {
				options.MaxPendingSegments = 1
			}
			if ending == "global-limit" {
				options.MaxTotalPendingSegments = 1
			}
			p := newTrafficPool(context.Background(), options)
			defer p.Close()
			var old *TrafficFlow
			p.onFlowCreated = func(f *TrafficFlow) {
				if old == nil {
					old = f
				}
			}
			openingBudgetFeed(p, false, 99, 0, true, false, false, "", 1)
			openingBudgetFeed(p, true, 499, 100, true, true, false, "", 2)
			openingBudgetFeed(p, false, 999, 0, true, false, false, "", 3)
			require.Equal(t, 1, p.pendingSegments)
			openingBudgetFeed(p, false, 999, 0, true, false, false, "", 4)
			if ending == "per-direction-limit" || ending == "global-limit" {
				require.Zero(t, p.pendingSegments, "a refused candidate cannot keep an incomplete ownership list")
				require.False(t, old.IsClosed(), "untrusted opening metadata cannot evict a valid parser")
				require.False(t, old.reassemblyIncomplete)
				require.Positive(t, p.singleDiagnostics.limits.Load())
				return
			}
			require.Equal(t, 2, p.pendingSegments)
			switch ending {
			case "confirm":
				openingBudgetFeed(p, true, 1999, 1000, true, true, false, "", 5)
			case "replace":
				openingBudgetFeed(p, false, 2999, 0, true, false, false, "", 5)
				require.Equal(t, 1, p.pendingSegments)
				openingBudgetFeed(p, true, 3999, 3000, true, true, false, "", 6)
			case "RST-before-confirm":
				openingBudgetFeed(p, false, 100, 500, false, false, true, "", 5)
				require.True(t, old.IsClosed())
				require.Equal(t, 2, p.pendingSegments, "a retained closed tuple still owns the awaiting opening")
				openingBudgetFeed(p, true, 1999, 1000, true, true, false, "", 6)
			case "unsupported-opening":
				openingBudgetFeed(p, false, 2999, 0, true, false, false, "unsupported TFO", 5)
				require.Zero(t, p.pendingSegments)
				openingBudgetFeed(p, true, 1999, 1000, true, true, false, "", 6)
				require.False(t, old.IsClosed())
				require.False(t, old.reassemblyIncomplete)
			case "release":
				old.Release()
			case "cancel":
				p.Close()
			}
			require.Zero(t, p.pendingSegments)
			require.Zero(t, p.pendingBytes)
			p.Close()
			require.Zero(t, p.pendingSegments, "close/release is idempotent")
		})
	}
}
