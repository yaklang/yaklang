package pcaputil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type tcpStep struct {
	seq                    uint32
	data                   string
	syn, fin, rst, reverse bool
}

func runTCP(t *testing.T, steps []tcpStep, ipv6 bool, port layers.TCPPort) (*TrafficPool, *TrafficFlow, *[]*TrafficFrame, *[]*TrafficFrame) {
	t.Helper()
	p := NewTrafficPool(context.Background())
	t.Cleanup(p.Close)
	var flow *TrafficFlow
	var arrived, assembled []*TrafficFrame
	p.onFlowCreated = func(f *TrafficFlow) { flow = f }
	p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { arrived = append(arrived, f) })
	p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { assembled = append(assembled, f) })
	for i, step := range steps {
		src, dst := net.ParseIP("192.0.2.1"), net.ParseIP("192.0.2.2")
		if ipv6 {
			src, dst = net.ParseIP("2001:db8::1"), net.ParseIP("2001:db8::2")
		}
		sport, dport := layers.TCPPort(12345), port
		if step.reverse {
			src, dst, sport, dport = dst, src, dport, sport
		}
		var network gopacket.SerializableLayer = &layers.IPv4{SrcIP: src, DstIP: dst}
		if ipv6 {
			network = &layers.IPv6{SrcIP: src, DstIP: dst}
		}
		tcp := &layers.TCP{SrcPort: sport, DstPort: dport, Seq: step.seq, SYN: step.syn, FIN: step.fin, RST: step.rst, ACK: !step.syn}
		tcp.Payload = []byte(step.data)
		p.Feed(nil, network, tcp, time.Unix(1700000000+int64(i), 0))
		// Feed does not take ownership of caller buffers, even for queued data.
		for j := range tcp.Payload {
			tcp.Payload[j] = '!'
		}
	}
	return p, flow, &arrived, &assembled
}

func frameData(frames []*TrafficFrame) string {
	var out bytes.Buffer
	for _, f := range frames {
		out.Write(f.Payload)
	}
	return out.String()
}

func TestTCPReassemblyEdges(t *testing.T) {
	cases := []struct {
		name  string
		steps []tcpStep
		want  string
	}{
		{"in_order", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 103, data: "def"}}, "abcdef"},
		{"midstream_without_psh", []tcpStep{{seq: 100, data: "abc"}, {seq: 103, data: "def"}}, "abcdef"},
		{"reordered_duplicate", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "def"}, {seq: 103, data: "def"}, {seq: 100, data: "abc"}, {seq: 100, data: "abc"}, {seq: 106, data: "ghi"}}, "abcdefghi"},
		{"overlap_suffix", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abcd"}, {seq: 102, data: "cdef"}, {seq: 102, data: "cd"}}, "abcdef"},
		{"queued_overlap", []tcpStep{{seq: 99, syn: true}, {seq: 102, data: "cdef"}, {seq: 104, data: "efgh"}, {seq: 100, data: "abcd"}}, "abcdefgh"},
		{"longer_duplicate", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "de"}, {seq: 103, data: "XYZg"}, {seq: 100, data: "abc"}}, "abcdeZg"},
		{"wrap", []tcpStep{{seq: math.MaxUint32 - 2, syn: true}, {seq: 1, data: "def"}, {seq: math.MaxUint32 - 1, data: "abc"}, {seq: math.MaxUint32 - 1, data: "abc"}}, "abcdef"},
		{"syn_data_and_retransmit", []tcpStep{{seq: 99, syn: true, data: "abc"}, {seq: 99, syn: true, data: "abc"}, {seq: 103, data: "def"}}, "abcdef"},
		{"data_fin", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc", fin: true}, {seq: 104, data: "ignored"}}, "abc"},
		{"cached_fin", []tcpStep{{seq: 99, syn: true}, {seq: 106, fin: true}, {seq: 103, data: "def"}, {seq: 100, data: "abc"}, {seq: 107, data: "ignored"}}, "abcdef"},
		{"cached_data_fin", []tcpStep{{seq: 99, syn: true}, {seq: 103, data: "def", fin: true}, {seq: 100, data: "abc"}, {seq: 107, data: "ignored"}}, "abcdef"},
		{"rst", []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "abc"}, {seq: 103, rst: true}, {seq: 103, data: "ignored"}}, "abc"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, f, arrived, assembled := runTCP(t, tc.steps, false, 80)
			require.NotNil(t, f)
			p.Close()
			require.Equal(t, tc.want, frameData(*arrived))
			require.Equal(t, tc.want, frameData(*assembled))
			data, err := io.ReadAll(f.ClientConn.GetBuffer())
			require.NoError(t, err)
			require.Equal(t, tc.want, string(data))
		})
	}
}

func TestTCPReassemblyDirectionsAndMetadata(t *testing.T) {
	for _, ipv6 := range []bool{false, true} {
		t.Run(fmt.Sprint(ipv6), func(t *testing.T) {
			p, f, arrived, assembled := runTCP(t, []tcpStep{{seq: 99, syn: true}, {seq: 199, syn: true, reverse: true}, {seq: 103, data: "def"}, {seq: 100, data: "abc"}, {seq: 200, data: "reply", reverse: true}, {seq: 106, fin: true}, {seq: 205, fin: true, reverse: true}}, ipv6, 12345)
			require.True(t, f.IsClosed())
			p.Close()
			require.Len(t, *arrived, 3)
			require.Len(t, *assembled, 2)
			require.Equal(t, uint32(103), (*arrived)[1].Seq)
			require.Equal(t, time.Unix(1700000002, 0), (*arrived)[1].Timestamp)
			require.Equal(t, "abcdef", string((*assembled)[0].Payload))
			require.Equal(t, "reply", string((*assembled)[1].Payload))
			require.Equal(t, f.ClientConn, (*assembled)[0].Connection)
			require.Equal(t, f.ServerConn, (*assembled)[1].Connection)
			require.Equal(t, f.ClientConn.RemoteIP(), f.ServerConn.LocalIP())
			require.Equal(t, f.ClientConn.RemotePort(), f.ServerConn.LocalPort())
			require.Equal(t, f.ClientConn.Hash(), (*arrived)[0].ConnHash)
			require.Equal(t, time.Unix(1700000003, 0), f.ClientConn.timestamps.at(3))
			require.Equal(t, time.Unix(1700000002, 0), f.ClientConn.timestamps.at(4))
		})
	}
}

func TestTCPReassemblyRandomized(t *testing.T) {
	r := rand.New(rand.NewSource(20260912))
	for trial := 0; trial < 50; trial++ {
		const parts = 128
		data := make([]byte, parts*31)
		_, _ = r.Read(data)
		base := r.Uint32()
		steps := []tcpStep{{seq: base - 1, syn: true}}
		for _, index := range r.Perm(parts) {
			step := tcpStep{seq: base + uint32(index*31), data: string(data[index*31 : (index+1)*31])}
			steps = append(steps, step)
			if index%3 == 0 {
				steps = append(steps, step)
			}
		}
		p, _, arrived, assembled := runTCP(t, steps, false, 80)
		p.Close()
		require.Equal(t, data, []byte(frameData(*arrived)), "trial %d", trial)
		require.Equal(t, data, []byte(frameData(*assembled)), "trial %d", trial)
	}
}

func TestTCPReassemblyLimits(t *testing.T) {
	for _, o := range []TCPReassemblyOptions{{MaxPendingBytes: 4}, {MaxPendingSegments: 1}} {
		p := newTrafficPool(context.Background(), o)
		var reason TrafficFlowCloseReason
		p.onFlowClosed = func(r TrafficFlowCloseReason, _ *TrafficFlow) { reason = r }
		ip := &layers.IPv4{SrcIP: net.IPv4(1, 1, 1, 1), DstIP: net.IPv4(2, 2, 2, 2)}
		tcp := &layers.TCP{SrcPort: 1000, DstPort: 80, SYN: true, Seq: 0}
		p.Feed(nil, ip, tcp)
		tcp.SYN, tcp.Seq, tcp.Payload = false, 10, []byte("abc")
		p.Feed(nil, ip, tcp)
		tcp.Seq = 20
		p.Feed(nil, ip, tcp)
		require.Equal(t, TrafficFlowCloseReason_RESOURCE_LIMIT, reason)
		p.Close()
	}
	for _, capacity := range []bool{false, true} {
		t.Run(fmt.Sprint(capacity), func(t *testing.T) {
			opts := TCPReassemblyOptions{IdleTimeout: 20 * time.Millisecond}
			want := TrafficFlowCloseReason_INACTIVE
			if capacity {
				opts = TCPReassemblyOptions{MaxFlows: 1}
				want = TrafficFlowCloseReason_RESOURCE_LIMIT
			}
			p := newTrafficPool(context.Background(), opts)
			defer p.Close()
			closed := make(chan TrafficFlowCloseReason, 4)
			p.onFlowClosed = func(r TrafficFlowCloseReason, _ *TrafficFlow) { closed <- r }
			ip := &layers.IPv4{SrcIP: net.IPv4(1, 1, 1, 1), DstIP: net.IPv4(2, 2, 2, 2)}
			p.Feed(nil, ip, &layers.TCP{SrcPort: 1000, DstPort: 80, SYN: true})
			if capacity {
				p.Feed(nil, ip, &layers.TCP{SrcPort: 1001, DstPort: 80, SYN: true})
			}
			select {
			case reason := <-closed:
				require.Equal(t, want, reason)
			case <-time.After(time.Second):
				t.Fatal("flow did not expire")
			}
		})
	}
}

func TestTCPReassemblyConcurrentFeed(t *testing.T) {
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true})
	defer p.Close()
	var wg sync.WaitGroup
	var received int
	p.onFlowFrameDataFrameArrived = append(p.onFlowFrameDataFrameArrived, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { received += len(f.Payload) })
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			ip := &layers.IPv4{SrcIP: net.IPv4(1, 1, 1, 1), DstIP: net.IPv4(2, 2, 2, 2)}
			for i := 0; i < 100; i++ {
				tcp := &layers.TCP{SrcPort: layers.TCPPort(1000 + worker), DstPort: 80, Seq: uint32(i * 3)}
				tcp.Payload = []byte("abc")
				p.Feed(nil, ip, tcp)
			}
		}(worker)
	}
	wg.Wait()
	require.Equal(t, 2400, received)
}

func TestTCPReassemblyLargeStream(t *testing.T) {
	total := uint64(16 << 20)
	if os.Getenv("PCAPX_LARGE_TEST") == "1" {
		total = 5 << 30
	}
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true, MaxFrameBytes: 64 << 10})
	defer p.Close()
	var flow *TrafficFlow
	p.onFlowCreated = func(f *TrafficFlow) { flow = f }
	gotHash, wantHash := sha256.New(), sha256.New()
	var received uint64
	p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) {
		if len(f.Payload) > 64<<10 {
			t.Errorf("oversized chunk: %d", len(f.Payload))
		}
		gotHash.Write(f.Payload)
		received += uint64(len(f.Payload))
	})
	ip := &layers.IPv4{SrcIP: net.IPv4(1, 1, 1, 1), DstIP: net.IPv4(2, 2, 2, 2)}
	tcp := &layers.TCP{SrcPort: 1000, DstPort: 80, Seq: 0, SYN: true}
	p.Feed(nil, ip, tcp)
	tcp.SYN = false
	payload := bytes.Repeat([]byte{0xa5}, 32<<10)
	var peak, first uint64
	start := time.Now()
	for offset := uint64(0); offset < total; offset += uint64(len(payload)) {
		binary.BigEndian.PutUint64(payload, offset)
		wantHash.Write(payload)
		tcp.Seq, tcp.Payload = 1+uint32(offset), payload
		p.Feed(nil, ip, tcp)
		if offset > 0 && offset%(8<<20) == 0 && (total < 1<<30 || offset%(1<<30) == 0) {
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if first == 0 {
				first = m.HeapAlloc
			}
			if m.HeapAlloc > peak {
				peak = m.HeapAlloc
			}
			t.Logf("processed=%d heap_live=%d", offset, m.HeapAlloc)
		}
	}
	p.Close()
	require.Equal(t, total, received)
	require.Equal(t, wantHash.Sum(nil), gotHash.Sum(nil))
	require.Nil(t, flow.ClientConn.reader)
	require.Empty(t, flow.ClientConn.timestamps.items)
	require.Empty(t, flow.ClientConn.waitGroup)
	require.Empty(t, flow.frames)
	if first > 0 {
		require.Less(t, peak-first, uint64(16<<20))
	}
	t.Logf("bytes=%d duration=%s SHA256=%x live_heap_growth=%d", total, time.Since(start), gotHash.Sum(nil), peak-first)
}

func TestTCPReassemblyGlobalBudgetAndChurn(t *testing.T) {
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true, MaxFlows: 64, MaxTotalPendingBytes: 4})
	defer p.Close()
	ip := &layers.IPv4{SrcIP: net.IPv4(1, 1, 1, 1), DstIP: net.IPv4(2, 2, 2, 2)}
	var closed int
	p.onFlowClosed = func(reason TrafficFlowCloseReason, f *TrafficFlow) {
		require.True(t, f.IsClosed()) // Close callbacks may inspect closure safely.
		f.Close()                     // Idempotent and reentrant.
		if reason == TrafficFlowCloseReason_RESOURCE_LIMIT {
			closed++
		}
	}
	for port := 1000; port < 1002; port++ {
		p.Feed(nil, ip, &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 80, SYN: true})
		tcp := &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 80, Seq: 10}
		tcp.Payload = []byte("abc")
		p.Feed(nil, ip, tcp)
	}
	require.Equal(t, 1, closed)
	require.Equal(t, 3, p.pendingBytes)
	for port := 2000; port < 22000; port++ {
		p.Feed(nil, ip, &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 80, SYN: true})
		require.LessOrEqual(t, p.flowCache.Count(), 64)
	}
	require.Empty(t, p.flowCache.retired)
	require.Zero(t, p.pendingBytes)
	require.Zero(t, p.pendingSegments)
	p.Close()
	require.Zero(t, p.flowCache.Count())
}

func TestTCPReassemblyReuseTuple(t *testing.T) {
	p, old, arrived, assembled := runTCP(t, []tcpStep{{seq: 99, syn: true}, {seq: 100, data: "old"}, {seq: 103, rst: true}}, false, 80)
	ip := &layers.IPv4{SrcIP: net.ParseIP("192.0.2.1"), DstIP: net.ParseIP("192.0.2.2")}
	p.Feed(nil, ip, &layers.TCP{SrcPort: 12345, DstPort: 80, SYN: true, Seq: 999})
	tcp := &layers.TCP{SrcPort: 12345, DstPort: 80, Seq: 1000}
	tcp.Payload = []byte("new")
	p.Feed(nil, ip, tcp)
	p.Close()
	require.True(t, old.IsClosed())
	require.Equal(t, "oldnew", frameData(*arrived))
	require.Equal(t, "oldnew", frameData(*assembled))
}

func TestTCPReassemblyCallbackClose(t *testing.T) {
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true, MaxFrameBytes: 2})
	defer p.Close()
	var count int
	var flow *TrafficFlow
	p.onFlowCreated = func(f *TrafficFlow) { flow = f }
	p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(f *TrafficFlow, _ *TrafficConnection, _ *TrafficFrame) { count++; f.Close() })
	ip := &layers.IPv4{SrcIP: net.IPv4(1, 1, 1, 1), DstIP: net.IPv4(2, 2, 2, 2)}
	tcp := &layers.TCP{SrcPort: 1000, DstPort: 80, Seq: 100}
	tcp.Payload = []byte("abcdef")
	p.Feed(nil, ip, tcp)
	p.Close()
	require.Equal(t, 1, count)
	require.Empty(t, flow.frames)
}

func TestTCPReassemblySharedCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := NewTrafficPool(ctx)
	defer p.Close()
	var closed int
	p.onFlowClosed = func(reason TrafficFlowCloseReason, _ *TrafficFlow) {
		require.Equal(t, TrafficFlowCloseReason_CTX_CANCEL, reason)
		closed++
	}
	ip := &layers.IPv4{SrcIP: net.IPv4(1, 1, 1, 1), DstIP: net.IPv4(2, 2, 2, 2)}
	var f *TrafficFlow
	p.onFlowCreated = func(flow *TrafficFlow) { f = flow }
	p.Feed(nil, ip, &layers.TCP{SrcPort: 1000, DstPort: 80, SYN: true})
	f.ClientConn.Close()
	require.False(t, f.IsClosed(), "one FIN must not close the other direction")
	require.False(t, f.ServerConn.IsClosed())
	cancel()
	require.True(t, f.ServerConn.IsClosed())
	require.True(t, f.IsClosed())
	require.True(t, f.IsClosed())
	require.Equal(t, 1, closed)
	p.Feed(nil, ip, &layers.TCP{SrcPort: 1001, DstPort: 80, SYN: true})
	require.Equal(t, 1, p.flowCache.Count())
	p.Close()
	require.Equal(t, 1, closed)
}

func TestTCPReassemblyAddressOwnershipAndHash(t *testing.T) {
	p := NewTrafficPool(context.Background())
	defer p.Close()
	f, err := p.NewFlow("tcp6", "[2001:0db8:0:0::1]:1234", "[2001:db8::2]:80")
	require.NoError(t, err)
	require.Equal(t, p.flowhash("tcp6", "[2001:0db8:0:0::1]:1234", "[2001:db8::2]:80"), f.Hash)
	require.Equal(t, codec.Sha256("[2001:db8::1]:1234 -> [2001:db8::2]:80"), f.ClientConn.Hash())
	var captured *TrafficFlow
	p.onFlowCreated = func(flow *TrafficFlow) { captured = flow }
	ip := &layers.IPv4{SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	p.Feed(nil, ip, &layers.TCP{SrcPort: 1234, DstPort: 80, SYN: true})
	ip.SrcIP[3], ip.DstIP[3] = 9, 10
	require.Equal(t, "192.0.2.1:1234", captured.ClientConn.LocalAddr().String())
	require.Equal(t, "192.0.2.2:80", captured.ServerConn.LocalAddr().String())
	require.Equal(t, codec.Sha256("192.0.2.1:1234 -> 192.0.2.2:80"), captured.ClientConn.Hash())
}

func TestTCPReassemblyStreamChunkOwnership(t *testing.T) {
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Stream: true, MaxFrameBytes: 64})
	defer p.Close()
	var chunks []*TrafficFrame
	p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, _ *TrafficConnection, f *TrafficFrame) { chunks = append(chunks, f) })
	ip := &layers.IPv4{SrcIP: net.IP{192, 0, 2, 1}, DstIP: net.IP{192, 0, 2, 2}}
	var want []byte
	for i := 0; i < 100; i++ {
		tcp := &layers.TCP{SrcPort: 1234, DstPort: 80, Seq: uint32(i * 17)}
		tcp.Payload = bytes.Repeat([]byte{byte(i)}, 17)
		want = append(want, tcp.Payload...)
		p.Feed(nil, ip, tcp)
		for j := range tcp.Payload {
			tcp.Payload[j] = 0xff
		}
	}
	p.Close()
	require.Equal(t, string(want), frameData(chunks))
	require.Equal(t, 64, len(chunks[1].Payload))
	chunks[0].Payload[0] ^= 0xff
	require.Equal(t, want[64], chunks[1].Payload[0])
}
