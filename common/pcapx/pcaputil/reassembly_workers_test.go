package pcaputil

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestTCPWorkersOrderedIntegrity(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		for _, workers := range []int{2, 4, 8} {
			t.Run(fmt.Sprintf("ipv6=%v/workers=%d", ipv6, workers), func(t *testing.T) {
				const flows, parts = 32, 64
				p := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: workers, Stream: true, MaxFrameBytes: 113, WorkerBatchPackets: 7, WorkerBatchBytes: 1024})
				defer p.Close()
				var got [flows][2]bytes.Buffer
				var indices sync.Map
				var created atomic.Int64
				p.onFlowCreated = func(f *TrafficFlow) {
					if _, exists := indices.LoadOrStore(f.Index, true); exists {
						t.Errorf("duplicate stream index %d", f.Index)
					}
					created.Add(1)
				}
				p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, c *TrafficConnection, f *TrafficFrame) {
					ip := c.LocalIP()
					if !ipv6 {
						ip = ip.To4()
					}
					id, dir := int(ip[len(ip)-2]), int(ip[len(ip)-1])-1
					got[id][dir].Write(f.Payload)
				})
				p.startWorkers()
				var wg sync.WaitGroup
				for id := 0; id < flows; id++ {
					wg.Add(1)
					go func(id int) {
						defer wg.Done()
						ips := [2]net.IP{net.IP{10, 0, byte(id), 1}, net.IP{10, 0, byte(id), 2}}
						if ipv6 {
							ips = [2]net.IP{net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2")}
							ips[0][14], ips[1][14] = byte(id), byte(id)
						}
						feed := func(dir int, seq uint32, data []byte, syn bool) {
							var ip gopacket.SerializableLayer = &layers.IPv4{SrcIP: ips[dir], DstIP: ips[1-dir]}
							if ipv6 {
								ip = &layers.IPv6{SrcIP: ips[dir], DstIP: ips[1-dir]}
							}
							tcp := &layers.TCP{SrcPort: 4000, DstPort: 4000, Seq: seq, SYN: syn, ACK: !syn}
							tcp.Payload = data
							p.Feed(nil, ip, tcp, time.Unix(1700000000, int64(seq)))
						}
						feed(0, 99, nil, true)
						feed(1, 99, nil, true)
						for _, part := range rand.New(rand.NewSource(int64(id))).Perm(parts) {
							for dir := 0; dir < 2; dir++ {
								data := bytes.Repeat([]byte{byte(part)}, 17)
								data[0], data[1] = byte(id), byte(dir)
								feed(dir, 100+uint32(part*17), data, false)
								if part%5 == 0 {
									feed(dir, 100+uint32(part*17), data, false)
								}
								for i := range data {
									data[i] = 0xff
								}
							}
						}
					}(id)
				}
				wg.Wait()
				p.Close()
				require.NoError(t, p.Err())
				require.Equal(t, int64(flows), created.Load())
				for id := range got {
					for dir := range got[id] {
						var want []byte
						for part := 0; part < parts; part++ {
							data := bytes.Repeat([]byte{byte(part)}, 17)
							data[0], data[1] = byte(id), byte(dir)
							want = append(want, data...)
						}
						require.Equal(t, want, got[id][dir].Bytes())
					}
				}
				s := p.Stats()
				require.Equal(t, s.AcceptedPackets, s.ProcessedPackets)
				require.Zero(t, s.RejectedPackets)
				require.Equal(t, uint64(flows*2*parts*17), s.DeliveredBytes)
				require.Zero(t, p.parallel.budget.bytes)
				require.Zero(t, p.parallel.budget.segments)
				require.Zero(t, p.parallel.budget.flows)
			})
		}
	}
}

func TestTCPWorkersFileParity(t *testing.T) {
	for _, kind := range []string{"ipv4", "ipv6", "vlan", "raw"} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/full=%v", kind, full), func(t *testing.T) {
				var got bytes.Buffer
				var stats TCPReassemblyStats
				opts := []CaptureOption{WithTCPReassemblyWorkers(3), WithTCPReassemblyStream(2), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowOnDataFrameReassembled(func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { got.Write(f.Payload) })}
				if full {
					opts = append(opts, WithEveryPacket(func(gopacket.Packet) {}))
				}
				require.NoError(t, OpenPcapFile(makeTestCapture(t, kind), opts...))
				require.Equal(t, "abcdefghi", got.String())
				require.Equal(t, uint64(4), stats.AcceptedPackets)
				require.Equal(t, stats.AcceptedPackets, stats.ProcessedPackets)
			})
		}
	}
}

func TestTCPWorkersBackpressureCancelDrain(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := newTrafficPool(ctx, TCPReassemblyOptions{Workers: 2, Stream: true, MaxFrameBytes: 1, WorkerQueueDepth: 1, WorkerBatchPackets: 1, WorkerBatchBytes: 64, IdleTimeout: time.Millisecond})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var received atomic.Int64
	p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
		once.Do(func() { close(entered); <-release })
		received.Add(int64(len(f.Payload)))
	})
	p.startWorkers()
	ip := &layers.IPv4{SrcIP: net.IP{10, 0, 0, 1}, DstIP: net.IP{10, 0, 0, 2}}
	feed := func(seq uint32) {
		tcp := &layers.TCP{SrcPort: 1000, DstPort: 80, Seq: seq}
		tcp.Payload = []byte("x")
		p.Feed(nil, ip, tcp)
	}
	feed(0)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not run")
	}
	fed := make(chan struct{})
	go func() { feed(1); feed(2); close(fed) }()
	require.Eventually(t, func() bool { return p.Stats().BackpressureEvents > 0 }, time.Second, time.Millisecond)
	cancel()
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("closed before draining accepted data")
	default:
	}
	close(release)
	select {
	case <-fed:
	case <-time.After(time.Second):
		t.Fatal("producer stuck")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("close stuck")
	}
	require.NoError(t, p.Err())
	require.Equal(t, int64(3), received.Load())
	s := p.Stats()
	require.Equal(t, uint64(3), s.AcceptedPackets)
	require.Equal(t, s.AcceptedPackets, s.ProcessedPackets)
	require.Zero(t, s.RejectedPackets)
}

func TestTCPWorkersFailuresAreVisible(t *testing.T) {
	t.Run("oversized", func(t *testing.T) {
		p := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: 2, WorkerBatchBytes: 32})
		p.startWorkers()
		ip := &layers.IPv4{SrcIP: net.IP{10, 0, 0, 1}, DstIP: net.IP{10, 0, 0, 2}}
		tcp := &layers.TCP{SrcPort: 1000, DstPort: 80}
		tcp.Payload = make([]byte, 33)
		p.Feed(nil, ip, tcp)
		p.Close()
		require.ErrorContains(t, p.Err(), "exceeds")
		require.Equal(t, uint64(1), p.Stats().RejectedPackets)
		require.Zero(t, p.Stats().AcceptedPackets)
	})
	t.Run("global-flows", func(t *testing.T) {
		p := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: 4, MaxFlows: 1})
		p.startWorkers()
		ip := &layers.IPv4{SrcIP: net.IP{10, 0, 0, 1}, DstIP: net.IP{10, 0, 0, 2}}
		for port := 1000; port < 1008; port++ {
			p.Feed(nil, ip, &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 80, SYN: true})
		}
		require.Eventually(t, func() bool { s := p.Stats(); return s.AcceptedPackets == s.ProcessedPackets }, time.Second, time.Millisecond)
		p.parallel.budget.mu.Lock()
		count := p.parallel.budget.flows
		p.parallel.budget.mu.Unlock()
		require.LessOrEqual(t, count, 1)
		p.Close()
		require.Error(t, p.Err())
		require.Positive(t, p.Stats().ResourceLimitEvents)
		require.Zero(t, p.parallel.budget.flows)
	})
	t.Run("callback", func(t *testing.T) {
		var stats TCPReassemblyStats
		err := OpenPcapFile(makeTestCapture(t, "ipv4"), WithTCPReassemblyWorkers(2), WithTCPReassemblyStream(2), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowOnDataFrameReassembled(func(*TrafficFlow, *TrafficConnection, *TrafficFrame) { panic("consumer failed") }))
		require.ErrorContains(t, err, "panic")
		require.Positive(t, stats.CallbackPanics)
		require.Equal(t, stats.AcceptedPackets, stats.ProcessedPackets)
	})
	t.Run("packet-observer", func(t *testing.T) {
		var stats TCPReassemblyStats
		err := OpenPcapFile(makeTestCapture(t, "ipv4"), WithTCPReassemblyWorkers(2), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithEveryPacket(func(gopacket.Packet) { panic("observer failed") }))
		require.ErrorContains(t, err, "panic")
		require.Positive(t, stats.CallbackPanics)
	})
	for _, limit := range []bool{false, true} {
		t.Run(fmt.Sprintf("gap/limit=%v", limit), func(t *testing.T) {
			opts := TCPReassemblyOptions{Workers: 2, Stream: true, MaxTotalPendingBytes: 10}
			p := newTrafficPool(context.Background(), opts)
			p.startWorkers()
			defer p.Close()
			ip := &layers.IPv4{SrcIP: net.IP{10, 0, 0, 1}, DstIP: net.IP{10, 0, 0, 2}}
			for port := 1000; port < 1002; port++ {
				p.Feed(nil, ip, &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 80, SYN: true})
				tcp := &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 80, Seq: 20}
				tcp.Payload = []byte("abcdef")
				p.Feed(nil, ip, tcp)
				if !limit {
					break
				}
			}
			require.Eventually(t, func() bool { s := p.Stats(); return s.AcceptedPackets == s.ProcessedPackets }, time.Second, time.Millisecond)
			p.Close()
			require.Error(t, p.Err())
			require.Positive(t, p.Stats().UnreassembledBytes)
			if limit {
				require.Positive(t, p.Stats().ResourceLimitEvents)
			}
			require.Zero(t, p.parallel.budget.bytes)
			require.Zero(t, p.parallel.budget.segments)
		})
	}
	require.Error(t, Start(WithTCPReassemblyWorkers(2), WithEnableCache(true)))
	require.Error(t, Start(WithTCPReassemblyWorkers(65)))
}
