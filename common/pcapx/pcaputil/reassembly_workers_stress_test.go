package pcaputil

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/stretchr/testify/require"
)

func TestTCPWorkersLargeStream(t *testing.T) {
	total := uint64(64<<20) + 17
	if os.Getenv("PCAPX_LARGE_TEST") == "1" {
		total = 5<<30 + 17
	}
	p := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: 4, Stream: true, MaxFrameBytes: 64 << 10})
	defer p.Close()
	var want, got [4]hash.Hash
	var sent [4]uint64
	for i := range want {
		want[i] = sha256.New()
		got[i] = sha256.New()
	}
	p.onFlowFrameDataFrameReassembled = append(p.onFlowFrameDataFrameReassembled, func(_ *TrafficFlow, c *TrafficConnection, f *TrafficFrame) { got[c.LocalPort()-1000].Write(f.Payload) })
	p.startWorkers()
	ip := &layers.IPv4{SrcIP: net.IP{10, 0, 0, 1}, DstIP: net.IP{10, 0, 0, 2}}
	for port := 1000; port < 1004; port++ {
		p.Feed(nil, ip, &layers.TCP{SrcPort: layers.TCPPort(port), DstPort: 80, SYN: true})
	}
	data := make([]byte, 32<<10)
	feed := func(id int, n int) {
		binary.BigEndian.PutUint64(data, sent[id])
		data[8] = byte(id)
		want[id].Write(data[:n])
		tcp := &layers.TCP{SrcPort: layers.TCPPort(1000 + id), DstPort: 80, Seq: 1 + uint32(sent[id])}
		tcp.Payload = data[:n]
		p.Feed(nil, ip, tcp)
		sent[id] += uint64(n)
	}
	var first, peak uint64
	for sent[0] < total {
		n := len(data)
		if uint64(n) > total-sent[0] {
			n = int(total - sent[0])
		}
		feed(0, n)
		if sent[0]%(4<<20) == 0 {
			for id := 1; id < 4; id++ {
				feed(id, len(data))
			}
		}
		if sent[0]%(1<<30) == 0 {
			runtime.GC()
			var m runtime.MemStats
			runtime.ReadMemStats(&m)
			if first == 0 {
				first = m.HeapAlloc
			}
			if m.HeapAlloc > peak {
				peak = m.HeapAlloc
			}
			t.Logf("primary_payload=%d heap_live=%d", sent[0], m.HeapAlloc)
		}
	}
	p.Close()
	require.NoError(t, p.Err())
	var bytes uint64
	for id := range want {
		require.Equal(t, want[id].Sum(nil), got[id].Sum(nil))
		bytes += sent[id]
		t.Logf("stream=%d bytes=%d SHA256=%x", id, sent[id], got[id].Sum(nil))
	}
	s := p.Stats()
	require.Equal(t, s.AcceptedPackets, s.ProcessedPackets)
	require.Equal(t, bytes, s.DeliveredBytes)
	require.Zero(t, s.RejectedPackets)
	if first != 0 {
		require.Less(t, peak-first, uint64(16<<20))
		t.Logf("live_heap_growth=%d", peak-first)
	}
	t.Logf("stats=%+v", s)
}

func TestTCPWorkersHTTP(t *testing.T) {
	for _, file := range []string{"image.pcapng", "aes_wtih_magic.pcapng"} {
		var counts [2]int64
		for i, workers := range []int{1, 4} {
			var count atomic.Int64
			err := OpenPcapFile(filepath.Join("tests", file), WithTCPReassemblyWorkers(workers), WithHTTPFlow(func(_ *TrafficFlow, r *http.Request, s *http.Response) {
				if r != nil && s != nil {
					count.Add(1)
				}
			}))
			require.NoError(t, err)
			counts[i] = count.Load()
		}
		require.Positive(t, counts[0])
		require.Equal(t, counts[0], counts[1])
	}
}

func TestTCPWorkersHTTPCallbackPanic(t *testing.T) {
	var stats TCPReassemblyStats
	err := OpenPcapFile(filepath.Join("tests", "image.pcapng"), WithTCPReassemblyWorkers(4), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithHTTPFlow(func(*TrafficFlow, *http.Request, *http.Response) {
		panic("HTTP consumer failed")
	}))
	require.ErrorContains(t, err, "panic")
	require.Positive(t, stats.CallbackPanics)
	require.Equal(t, stats.AcceptedPackets, stats.ProcessedPackets)
}

func TestTCPWorkersCaptureLossIsVisible(t *testing.T) {
	for _, s := range []TCPDeviceCaptureStats{{Available: true, Dropped: 3}, {Available: true, InterfaceDropped: 1}, {Error: "not supported"}} {
		p := newTrafficPool(context.Background(), TCPReassemblyOptions{Workers: 2})
		p.startWorkers()
		p.parallel.recordDeviceStats(s)
		p.Close()
		require.Error(t, p.Err())
		require.Len(t, p.Stats().Devices, 1)
	}
	name := makeTestCapture(t, "ipv4")
	raw, err := os.ReadFile(name)
	require.NoError(t, err)
	// The second packet has a complete TCP header but an impossible data offset.
	firstLen := int(binary.LittleEndian.Uint32(raw[32:36]))
	second := 24 + 16 + firstLen
	raw[second+16+14+20+12] = 0xf0
	require.NoError(t, os.WriteFile(name, raw, 0600))
	var stats TCPReassemblyStats
	err = OpenPcapFile(name, WithTCPReassemblyWorkers(2), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }))
	require.Error(t, err)
	require.Positive(t, stats.DecodeErrors)
	// Report snaplen truncation, even when the surviving TCP prefix decodes.
	valid, err := os.ReadFile(makeTestCapture(t, "ipv4"))
	require.NoError(t, err)
	r, err := pcapgo.NewReader(bytes.NewReader(valid))
	require.NoError(t, err)
	packet, ci, err := r.ReadPacketData()
	require.NoError(t, err)
	var truncated bytes.Buffer
	w := pcapgo.NewWriter(&truncated)
	require.NoError(t, w.WriteFileHeader(65535, layers.LinkTypeEthernet))
	ci.Length += 10
	require.NoError(t, w.WritePacket(ci, packet))
	require.NoError(t, os.WriteFile(name, truncated.Bytes(), 0600))
	err = OpenPcapFile(name, WithTCPReassemblyWorkers(2), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }))
	require.ErrorContains(t, err, "truncated")
	require.Equal(t, uint64(1), stats.TruncatedCaptures)
}

func TestTCPWorkersLiveIntegrity(t *testing.T) {
	if os.Getenv("PCAPX_LIVE_TEST_IFACE") != "lo0" {
		t.Skip("set PCAPX_LIVE_TEST_IFACE=lo0 for local TCP capture")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	serverDone := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_, err = io.Copy(io.Discard, conn)
		serverDone <- err
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	want := bytes.Repeat([]byte("local TCP worker integrity\n"), 8192)
	got := sha256.New()
	var received atomic.Uint64
	var stats TCPReassemblyStats
	go func() {
		done <- Start(WithContext(ctx), WithDeviceAdapter(&DeviceAdapter{DeviceName: "lo0", Snaplen: 65535, BPF: "tcp port " + strconv.Itoa(port)}), WithTCPReassemblyWorkers(4), WithTCPReassemblyStream(4096), WithNetInterfaceCreated(func(*PcapHandleWrapper) { close(ready) }), WithTCPReassemblyStats(func(s TCPReassemblyStats) { stats = s }), WithOnTrafficFlowOnDataFrameArrived(func(_ *TrafficFlow, c *TrafficConnection, f *TrafficFrame) {
			if c.RemotePort() == port {
				got.Write(f.Payload)
				received.Add(uint64(len(f.Payload)))
			}
		}))
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("capture failed to open: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not open")
	}
	conn, err := net.Dial("tcp4", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()
	for offset := 0; offset < len(want); {
		n := 8192
		if n > len(want)-offset {
			n = len(want) - offset
		}
		written, err := conn.Write(want[offset : offset+n])
		require.NoError(t, err)
		offset += written
		time.Sleep(time.Millisecond)
	}
	require.NoError(t, conn.Close())
	select {
	case err := <-serverDone:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("TCP server did not finish")
	}
	require.Eventually(t, func() bool { return received.Load() == uint64(len(want)) }, 3*time.Second, time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not drain")
	}
	expected := sha256.Sum256(want)
	require.Equal(t, expected[:], got.Sum(nil))
	require.Equal(t, stats.AcceptedPackets, stats.ProcessedPackets)
	require.Len(t, stats.Devices, 1)
	require.True(t, stats.Devices[0].Available)
	require.Zero(t, stats.Devices[0].Dropped)
	require.Zero(t, stats.Devices[0].InterfaceDropped)
	t.Logf("loopback bytes=%d SHA256=%x stats=%+v", len(want), expected, stats)
}
