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

func TestReplayTCPIdleClockDoesNotUseProcessingDelay(t *testing.T) {
	wire := binTestPcap(t, []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "a"}, {seq: 101, data: "b"}}, 65001, false, false)
	for _, workers := range []int{1, 2, 4} {
		for _, delay := range []time.Duration{0, 25 * time.Millisecond} {
			t.Run(fmt.Sprintf("w%d/%v", workers, delay), func(t *testing.T) {
				var mu sync.Mutex
				var observed []uint64
				var payload bytes.Buffer
				packets := 0
				err := ReplayPcap(bytes.NewReader(wire),
					WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, Stream: true, IdleTimeout: 5 * time.Millisecond, Workers: workers, WorkerBatchPackets: 1}),
					WithBeforeTransportPacket(func(gopacket.Packet) {
						packets++
						if packets == 2 {
							time.Sleep(delay) // captured timestamps remain 1.234 us apart
						}
					}),
					WithOnTrafficFlowPacket(func(flow *TrafficFlow, _ *TrafficConnection, _ []PacketReference, _ time.Time) {
						mu.Lock()
						observed = append(observed, flow.Index)
						mu.Unlock()
					}),
					WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, _ *TrafficConnection, frame *TrafficFrame) {
						mu.Lock()
						_, _ = payload.Write(frame.Payload)
						mu.Unlock()
					}))
				require.NoError(t, err)
				require.Equal(t, "ab", payload.String())
				require.Len(t, observed, 3)
				require.Equal(t, observed[0], observed[1], "decoder scheduling delay must not expire captured TCP identity")
				require.Equal(t, observed[0], observed[2])
			})
		}
	}
}

func replayClockWire(t *testing.T, times []time.Time) []byte {
	t.Helper()
	input := binTestPcap(t, []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "a"}, {seq: 101, data: "b"}}, 65001, false, false)
	r, err := pcapgo.NewReader(bytes.NewReader(input))
	require.NoError(t, err)
	var out bytes.Buffer
	w := pcapgo.NewWriterNanos(&out)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	for _, ts := range times {
		wire, ci, err := r.ReadPacketData()
		require.NoError(t, err)
		ci.Timestamp = ts
		require.NoError(t, w.WritePacket(ci, wire))
	}
	return out.Bytes()
}

func TestReplayTCPDefaultAndExplicitCapturedIdle(t *testing.T) {
	wire := replayClockWire(t, []time.Time{time.Unix(100, 0), time.Unix(160, 0), time.Unix(220, 0)})
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprint(explicit), func(t *testing.T) {
			options := TCPReassemblyOptions{Stream: true, AllowIncomplete: true}
			if explicit {
				options.IdleTimeout = 30 * time.Second
			}
			var ids []uint64
			err := ReplayPcap(bytes.NewReader(wire), WithTCPReassemblyOptions(options), WithOnTrafficFlowPacket(func(flow *TrafficFlow, _ *TrafficConnection, _ []PacketReference, _ time.Time) {
				ids = append(ids, flow.Index)
			}))
			require.NoError(t, err)
			require.Len(t, ids, 3)
			if explicit {
				require.NotEqual(t, ids[0], ids[1])
				require.NotEqual(t, ids[1], ids[2])
			} else {
				require.Equal(t, ids[0], ids[1], "a valid captured keepalive gap is not an implicit peer close")
				require.Equal(t, ids[0], ids[2])
			}
		})
	}
}

func TestPacketAnalyzerReplayClockMatchesReader(t *testing.T) {
	wire := replayClockWire(t, []time.Time{time.Unix(100, 0), time.Unix(100, 1000), time.Unix(100, 2000)})
	var ids []uint64
	analyzer, err := NewPacketAnalyzer(WithCaptureReplayClock(), WithTCPReassemblyOptions(TCPReassemblyOptions{AllowIncomplete: true, IdleTimeout: 5 * time.Millisecond}),
		WithOnProtocolMessage(func(*ProtocolEvent) {}), WithOnTrafficFlowPacket(func(flow *TrafficFlow, _ *TrafficConnection, _ []PacketReference, _ time.Time) {
			ids = append(ids, flow.Index)
		}))
	require.NoError(t, err)
	defer analyzer.Close()
	r, err := pcapgo.NewReader(bytes.NewReader(wire))
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		raw, ci, err := r.ReadPacketData()
		require.NoError(t, err)
		if i == 1 {
			time.Sleep(25 * time.Millisecond)
		}
		require.NoError(t, analyzer.Feed(raw, ci, layers.LinkTypeEthernet))
	}
	require.Len(t, ids, 3)
	require.Equal(t, ids[0], ids[1])
	require.Equal(t, ids[0], ids[2])
	require.NoError(t, analyzer.Close())
}

func clockPoolFeed(p *TrafficPool, domain CaptureDomain, port uint16, seq uint32, syn bool, data string, ts time.Time) {
	ip := &layers.IPv4{Version: 4, SrcIP: net.IPv4(192, 0, 2, 1), DstIP: net.IPv4(192, 0, 2, 2)}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 65001, Seq: seq, SYN: syn, ACK: !syn}
	tcp.Payload = []byte(data)
	p.feedEvidence(nil, ip, tcp, captureEvidence{Ref: PacketReference{Number: uint64(seq) + 1, Domain: domain}}, ts)
}

func TestCaptureIdleDomainIsolationLateTimeAndBounds(t *testing.T) {
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true, AllowIncomplete: true, IdleTimeout: 10 * time.Second, MaxFlows: 3, captureReplay: true})
	defer p.Close()
	a, b := CaptureDomain{Interface: 0}, CaptureDomain{Interface: 1}
	clockPoolFeed(p, a, 50001, 99, true, "", time.Unix(100, 0))
	var first *flowEntry
	for _, entry := range p.flowCache.items {
		first = entry
	}
	require.NotNil(t, first)
	clockPoolFeed(p, b, 50001, 99, true, "", time.Unix(10000, 0))
	clockPoolFeed(p, a, 50001, 100, false, "abc", time.Unix(105, 0))
	require.Same(t, first, p.flowCache.items[first.key], "another interface's future clock cannot expire this observed sender")
	clockPoolFeed(p, a, 50002, 199, true, "", time.Unix(109, 0))
	clockPoolFeed(p, a, 50001, 103, false, "", time.Unix(101, 0))
	require.Equal(t, time.Unix(105, 0), first.seen, "a late timestamp must neither rewind nor refresh to the domain watermark")
	clockPoolFeed(p, a, 50002, 200, false, "", time.Unix(116, 0))
	require.Nil(t, p.flowCache.items[first.key], "expiry uses actual last captured time, not the late record's processing position")
	require.Len(t, p.flowCache.captureDomains, 2)
	clockPoolFeed(p, CaptureDomain{Interface: 2}, 50003, 299, true, "", time.Unix(100, 0))
	clockPoolFeed(p, CaptureDomain{Interface: 3}, 50004, 399, true, "", time.Unix(100, 0))
	require.LessOrEqual(t, len(p.flowCache.captureDomains), p.flowCache.Count())
	require.LessOrEqual(t, p.flowCache.Count(), 3, "offline clocks must not remove the global capacity bound")
	p.Close()
	require.Empty(t, p.flowCache.captureDomains)
}

func TestUnknownCaptureTimeDoesNotInventWallClock(t *testing.T) {
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true, captureReplay: true, IdleTimeout: time.Second})
	defer p.Close()
	var times []time.Time
	p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { times = append(times, f.Timestamp) })
	clockPoolFeed(p, CaptureDomain{}, 50001, 99, true, "", time.Time{})
	clockPoolFeed(p, CaptureDomain{}, 50001, 100, false, "abc", time.Time{})
	require.Equal(t, []time.Time{{}}, times)
	clockPoolFeed(p, CaptureDomain{}, 50002, 99, true, "", time.Unix(100, 0))
	require.Equal(t, 2, p.flowCache.Count(), "an unknown timestamp cannot be proven idle by wall-clock age")
	require.Len(t, p.flowCache.captureDomains, 1)
	require.Len(t, p.flowCache.captureDomains[CaptureDomain{}].expiry, 1, "only timestamped entries enter the expiry heap")
}
