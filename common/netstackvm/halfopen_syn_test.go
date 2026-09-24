package netstackvm

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestOpenHalfOpenSYNRejectsMissingInterface(t *testing.T) {
	_, err := OpenHalfOpenSYN(context.Background(), HalfOpenSYNConfig{})
	require.Error(t, err)
}

// Real gVisor TCP endpoint + real final injection gate, only the device writer
// is mocked. The fixture exercises queue draining, port reservation and Close.
func mockHalfOpen(t *testing.T, writer func(*HalfOpenSYN, []byte) error) *HalfOpenSYN {
	t.Helper()
	vm, err := NewChannelNetStackVirtualMachineEntry("192.0.2.10")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	h := &HalfOpenSYN{ctx: ctx, cancel: cancel, stack: vm.stack, main: vm,
		slots: make(chan struct{}, 16), limiter: rate.NewLimiter(rate.Inf, 1), done: make(chan struct{}),
		retry: SYNRetryPolicy{3, time.Second, 10 * time.Millisecond, 40 * time.Millisecond}, flights: make(map[flightKey]*synFlight)}
	drv := &PCAPEndpoint{Endpoint: vm.link, ctx: ctx, cancel: cancel, wg: new(sync.WaitGroup),
		adaptor: &pcapAdaptor{linkType: layers.LinkTypeRaw, writer: func(b []byte) error { return writer(h, b) }}}
	drv.SetPCAPOutboundFilter(func(pkt gopacket.Packet) bool { return h.allowOutbound(vm.MainNICID(), pkt) })
	drv.stackFrame = func(b []byte, lt gopacket.LayerType) error { return h.captureStackFrame(vm, b, lt) }
	vm.driver = drv
	vm.config.stack = vm.stack
	drv.wg.Add(1)
	go func() {
		defer drv.wg.Done()
		for {
			pkt := vm.link.ReadContext(ctx)
			if pkt == nil {
				return
			}
			_ = drv.writePacket(pkt)
		}
	}()
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	return h
}

func synReply(t *testing.T, raw []byte, change func(*layers.IPv4, *layers.TCP)) gopacket.Packet {
	t.Helper()
	packet := gopacket.NewPacket(raw, layers.LayerTypeIPv4, gopacket.Default)
	ip, syn, ok := ipv4TCP(packet)
	require.True(t, ok)
	replyIP := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: ip.DstIP, DstIP: ip.SrcIP}
	replyTCP := &layers.TCP{SrcPort: syn.DstPort, DstPort: syn.SrcPort, SYN: true, ACK: true, Seq: 42, Ack: syn.Seq + 1, Window: 1024}
	if change != nil {
		change(replyIP, replyTCP)
	}
	return mustPacket(t, layers.LinkTypeRaw, replyIP, replyTCP)
}

func startMockProbe(t *testing.T, h *HalfOpenSYN) *TCPProbe {
	t.Helper()
	p, err := h.StartTCPProbe(context.Background(), "192.0.2.20:80")
	require.NoError(t, err)
	t.Cleanup(func() { p.Close() })
	return p
}

func TestHalfOpenRetriesDroppedSYNWithoutChangingTupleOrCompletingHandshake(t *testing.T) {
	var writes atomic.Int32
	var first []byte
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		n := writes.Add(1)
		if n == 1 {
			first = append([]byte(nil), b...)
		} else {
			require.Equal(t, first, b, "retry must replay the same SYN")
		}
		if n == 3 {
			require.False(t, h.observeInbound(h.main.MainNICID(), synReply(t, b, nil)))
		}
		return nil
	})
	p := startMockProbe(t, h)
	_, err := p.ProbeSYNContext(context.Background())
	require.NoError(t, err)
	ack, err := p.ReceiveSYNACKContext(context.Background())
	require.NoError(t, err)
	require.True(t, ack.SYN && ack.ACK)
	require.EqualValues(t, 3, writes.Load())
	require.False(t, p.Established())
	_, err = p.ProbeACK()
	require.ErrorIs(t, err, ErrHalfOpenACK)
	p.Close()
	h.mu.Lock()
	require.Empty(t, h.flights)
	h.mu.Unlock()
	require.NoError(t, h.Close())
	require.EqualValues(t, 3, writes.Load(), "Close must not inject RST/ACK")
}

func TestHalfOpenWriteFailuresAreNotNoResponse(t *testing.T) {
	failure := errors.New("pcap write failed")
	var writes atomic.Int32
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		writes.Add(1)
		// Even a response racing a failed first write cannot report open.
		h.observeInbound(h.main.MainNICID(), synReply(t, b, nil))
		return failure
	})
	p := startMockProbe(t, h)
	_, err := p.ProbeSYNContext(context.Background())
	require.ErrorIs(t, err, ErrProbeSend)
	require.ErrorIs(t, err, failure)
	require.NotErrorIs(t, err, ErrProbeNoResponse)
	require.EqualValues(t, 3, writes.Load())
	_, err = p.ReceiveSYNACKContext(context.Background())
	require.Error(t, err)
}

func TestHalfOpenNoResponseIsInconclusiveAndBudgetIsShared(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error {
		if writes.Add(1) == 1 {
			return errors.New("busy writer")
		}
		return nil
	})
	p := startMockProbe(t, h)
	_, err := p.ProbeSYN()
	require.NoError(t, err)
	_, err = p.ReceiveSYNACK()
	require.ErrorIs(t, err, ErrProbeNoResponse)
	require.NotErrorIs(t, err, ErrProbeRefused)
	require.EqualValues(t, 3, writes.Load(), "send failure and response timeout use one attempt budget")
}

func TestHalfOpenRejectsWrongResponses(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		writes.Add(1)
		mutations := []func(*layers.IPv4, *layers.TCP){
			func(_ *layers.IPv4, tcp *layers.TCP) { tcp.Ack++ },
			func(_ *layers.IPv4, tcp *layers.TCP) { tcp.DstPort++ },
			func(_ *layers.IPv4, tcp *layers.TCP) { tcp.SrcPort++ },
			func(ip *layers.IPv4, _ *layers.TCP) { ip.SrcIP = net.ParseIP("192.0.2.21") },
			func(ip *layers.IPv4, _ *layers.TCP) { ip.DstIP = net.ParseIP("192.0.2.11") },
			func(_ *layers.IPv4, tcp *layers.TCP) { tcp.FIN = true },
			func(_ *layers.IPv4, tcp *layers.TCP) { tcp.RST = true },
			func(ip *layers.IPv4, _ *layers.TCP) { ip.Flags = layers.IPv4MoreFragments },
		}
		for _, mutate := range mutations {
			h.observeInbound(h.main.MainNICID(), synReply(t, b, mutate))
		}
		h.observeInbound(h.main.MainNICID()+1, synReply(t, b, nil))
		h.mu.Lock()
		for _, f := range h.flights {
			require.Empty(t, f.replies)
		}
		h.mu.Unlock()
		h.observeInbound(h.main.MainNICID(), synReply(t, b, nil))
		h.observeInbound(h.main.MainNICID(), synReply(t, b, nil))
		return nil
	})
	p := startMockProbe(t, h)
	_, err := p.ProbeSYN()
	require.NoError(t, err)
	_, err = p.ReceiveSYNACK()
	require.NoError(t, err)
	require.EqualValues(t, 1, writes.Load())
}

func TestHalfOpenMatchingResetIsClosedWithoutRetry(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		writes.Add(1)
		h.observeInbound(h.main.MainNICID(), synReply(t, b, func(_ *layers.IPv4, tcp *layers.TCP) { tcp.SYN = false; tcp.RST = true }))
		return nil
	})
	p := startMockProbe(t, h)
	_, err := p.ProbeSYN()
	require.NoError(t, err)
	_, err = p.ReceiveSYNACK()
	require.ErrorIs(t, err, ErrProbeRefused)
	require.EqualValues(t, 1, writes.Load())
}

func TestHalfOpenContextCancelsReceiveAndAdmission(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error { writes.Add(1); return nil })
	h.slots = make(chan struct{}, 1)
	h.retry = SYNRetryPolicy{3, time.Second, time.Second, 2 * time.Second}
	p := startMockProbe(t, h)
	_, err := p.ProbeSYN()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = h.StartTCPProbe(ctx, "192.0.2.21:80")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	ctx2, cancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel2()
	_, err = p.ReceiveSYNACKContext(ctx2)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualValues(t, 1, writes.Load())
	p.Close()
	require.NoError(t, h.Close())
}

func TestHalfOpenContextCancelsRateWaitBeforeWrite(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error { writes.Add(1); return nil })
	h.limiter = rate.NewLimiter(1, 1)
	require.True(t, h.limiter.Allow())
	p := startMockProbe(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := p.ProbeSYNContext(ctx); done <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
	require.Zero(t, writes.Load())
}

func TestHalfOpenFinalGateDropsUnsolicitedAndNonSYNTraffic(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error { writes.Add(1); return nil })
	for _, tcp := range []*layers.TCP{
		{SrcPort: 40000, DstPort: 80, SYN: true, Seq: 1},
		{SrcPort: 40000, DstPort: 80, ACK: true},
		{SrcPort: 40000, DstPort: 80, RST: true},
		{SrcPort: 40000, DstPort: 80, FIN: true, ACK: true},
	} {
		pkt := mustPacket(t, layers.LinkTypeRaw, &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.ParseIP("192.0.2.10"), DstIP: net.ParseIP("192.0.2.20")}, tcp)
		require.NoError(t, h.main.driver.writeFrame(pkt.Data(), layers.LayerTypeIPv4))
	}
	require.Zero(t, writes.Load())
}

func TestHalfOpenFlightKeyPreservesDelayedRepliesAcrossPortReuse(t *testing.T) {
	h := &HalfOpenSYN{ctx: context.Background(), flights: make(map[flightKey]*synFlight)}
	for i := 0; i < 2; i++ {
		key := flightKey{nic: 1, local: ip4key(net.ParseIP("192.0.2.10")), port: 40000, remote: ip4key(net.ParseIP(fmt.Sprintf("192.0.2.%d", 20+i))), remotePort: 80}
		syn := TCPSegment{LocalIP: net.IP(key.local[:]), RemoteIP: net.IP(key.remote[:]), LocalPort: key.port, RemotePort: 80, Seq: uint32(1000 + i)}
		h.flights[key] = &synFlight{ctx: context.Background(), key: key, frame: &synFrame{segment: syn}, sent: true, replies: make(chan probeReply, 1)}
	}
	for _, f := range h.flights {
		syn := f.frame.segment
		pkt := mustPacket(t, layers.LinkTypeRaw, &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: syn.RemoteIP, DstIP: syn.LocalIP}, &layers.TCP{SrcPort: 80, DstPort: 40000, SYN: true, ACK: true, Ack: syn.Seq + 1})
		h.observeInbound(1, pkt)
	}
	for _, f := range h.flights {
		require.Len(t, f.replies, 1)
	}
}

func TestHalfOpenPCAPLoopbackDoesNotAccept(t *testing.T) {
	device := os.Getenv("NETSTACKVM_PCAP_DEVICE")
	if device == "" {
		t.Skip("set NETSTACKVM_PCAP_DEVICE to a loopback interface for live validation")
	}
	iface, err := net.InterfaceByName(device)
	require.NoError(t, err)
	if iface.Flags&net.FlagLoopback == 0 {
		t.Skip("live test only uses loopback")
	}
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer h.Close()
	p, err := h.StartTCPProbe(ctx, listener.Addr().String())
	require.NoError(t, err)
	defer p.Close()
	_, err = p.ProbeSYNContext(ctx)
	require.NoError(t, err)
	_, err = p.ReceiveSYNACKContext(ctx)
	require.NoError(t, err)
	require.NoError(t, listener.SetDeadline(time.Now().Add(100*time.Millisecond)))
	conn, err := listener.Accept()
	if conn != nil {
		conn.Close()
	}
	require.Error(t, err, "half-open scan must not establish a connection")
}

func mustPacket(t *testing.T, link layers.LinkType, parts ...gopacket.SerializableLayer) gopacket.Packet {
	t.Helper()
	var network gopacket.NetworkLayer
	for _, part := range parts {
		if n, ok := part.(gopacket.NetworkLayer); ok {
			network = n
		}
	}
	for _, part := range parts {
		tcpLayer, ok := part.(*layers.TCP)
		if ok && network != nil {
			if err := tcpLayer.SetNetworkLayerForChecksum(network); err != nil {
				t.Fatal(err)
			}
		}
	}
	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, parts...); err != nil {
		t.Fatal(err)
	}
	var decode gopacket.Decoder = link
	if rawIPLink(link) {
		decode = layers.LayerTypeIPv4
	}
	packet := gopacket.NewPacket(buf.Bytes(), decode, gopacket.Default)
	require.Nil(t, packet.ErrorLayer(), "fixture must decode successfully")
	return packet
}

func TestHalfOpenQueuedOldSYNDoesNotChooseNewProbeISN(t *testing.T) {
	var written []byte
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		written = append([]byte(nil), b...)
		h.observeInbound(h.main.MainNICID(), synReply(t, b, nil))
		return nil
	})
	p := startMockProbe(t, h)
	transport := p.transport.(*pcapProbeTransport)
	key := transport.flight.key
	old := mustPacket(t, layers.LinkTypeRaw, &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IP(key.local[:]), DstIP: net.IP(key.remote[:])}, &layers.TCP{SrcPort: layers.TCPPort(key.port), DstPort: 80, SYN: true, Seq: 7})
	require.NoError(t, h.captureStackFrame(h.main, old.Data(), layers.LayerTypeIPv4))
	syn, err := p.ProbeSYN()
	require.NoError(t, err)
	require.NotEqualValues(t, 7, syn.Seq)
	require.NotEqual(t, old.Data(), written)
	_, err = p.ReceiveSYNACK()
	require.NoError(t, err)
}

func TestHalfOpenGenerationLossIsSendFailure(t *testing.T) {
	var writes atomic.Int32
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error { writes.Add(1); return nil })
	// Model output lost before the device boundary (e.g. a full channel queue).
	h.main.driver.filterMutex.Lock()
	h.main.driver.stackFrame = func([]byte, gopacket.LayerType) error { return nil }
	h.main.driver.filterMutex.Unlock()
	h.retry = SYNRetryPolicy{2, 10 * time.Millisecond, time.Millisecond, time.Millisecond}
	p := startMockProbe(t, h)
	_, err := p.ProbeSYN()
	require.ErrorIs(t, err, ErrProbeSend)
	require.NotErrorIs(t, err, ErrProbeNoResponse)
	require.Zero(t, writes.Load())
}

func TestHalfOpenLifetimeDeadlineCancelsBackgroundReceive(t *testing.T) {
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	p, err := h.StartTCPProbe(ctx, "192.0.2.20:80", WithSYNRetry(SYNRetryPolicy{3, time.Second, time.Second, time.Second}))
	require.NoError(t, err)
	defer p.Close()
	_, err = p.ProbeSYNContext(context.Background())
	require.NoError(t, err)
	_, err = p.ReceiveSYNACKContext(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, h.Close())
	h.mu.Lock()
	require.Empty(t, h.flights)
	h.mu.Unlock()
}

func TestHalfOpenCorruptChecksumsAndStaleACKCannotReportOpen(t *testing.T) {
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		valid := synReply(t, b, nil)
		corrupt := append([]byte(nil), valid.Data()...)
		corrupt[len(corrupt)-1] ^= 1
		h.observeInbound(h.main.MainNICID(), gopacket.NewPacket(corrupt, layers.LayerTypeIPv4, gopacket.Default))
		corrupt = append([]byte(nil), valid.Data()...)
		corrupt[8] ^= 1
		h.observeInbound(h.main.MainNICID(), gopacket.NewPacket(corrupt, layers.LayerTypeIPv4, gopacket.Default))
		h.observeInbound(h.main.MainNICID(), synReply(t, b, func(_ *layers.IPv4, tcp *layers.TCP) { tcp.Ack -= 100 }))
		h.mu.Lock()
		for _, f := range h.flights {
			require.Empty(t, f.replies)
		}
		h.mu.Unlock()
		h.observeInbound(h.main.MainNICID(), valid)
		return nil
	})
	p := startMockProbe(t, h)
	_, err := p.ProbeSYN()
	require.NoError(t, err)
	_, err = p.ReceiveSYNACK()
	require.NoError(t, err)
}

func TestHalfOpenConcurrentSynchronousProbesReleaseAllResources(t *testing.T) {
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		h.observeInbound(h.main.MainNICID(), synReply(t, b, nil))
		return nil
	})
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			ack, err := h.ProbeSYN(ctx, fmt.Sprintf("192.0.2.20:%d", port))
			if err != nil {
				t.Errorf("probe %d: %v", port, err)
				return
			}
			if int(ack.RemotePort) != port {
				t.Errorf("probe %d got response for %d", port, ack.RemotePort)
			}
		}(10000 + i)
	}
	wg.Wait()
	h.mu.Lock()
	require.Empty(t, h.flights)
	h.mu.Unlock()
	require.Empty(t, h.slots)
}

func TestHalfOpenSynchronousProbeCancellationDoesNotCancelSibling(t *testing.T) {
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		_, tcp, _ := ipv4TCP(gopacket.NewPacket(b, layers.LayerTypeIPv4, gopacket.Default))
		if tcp.DstPort == 80 {
			h.observeInbound(h.main.MainNICID(), synReply(t, b, nil))
		}
		return nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := h.ProbeSYN(ctx, "192.0.2.20:81"); done <- err }()
	ack, err := h.ProbeSYN(context.Background(), "192.0.2.20:80")
	require.NoError(t, err)
	require.EqualValues(t, 80, ack.RemotePort)
	require.ErrorIs(t, <-done, context.DeadlineExceeded)
	// Cancellation releases its slot; the same session remains usable.
	_, err = h.ProbeSYN(context.Background(), "192.0.2.20:80")
	require.NoError(t, err)
	h.mu.Lock()
	require.Empty(t, h.flights)
	h.mu.Unlock()
	require.Empty(t, h.slots)
}

func TestHalfOpenSynchronousProbeErrorsReleaseResources(t *testing.T) {
	for _, kind := range []string{"send", "silence", "refused"} {
		t.Run(kind, func(t *testing.T) {
			h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
				if kind == "send" {
					return errors.New("device write failed")
				}
				if kind == "refused" {
					h.observeInbound(h.main.MainNICID(), synReply(t, b, func(_ *layers.IPv4, tcp *layers.TCP) { tcp.SYN = false; tcp.RST = true }))
				}
				return nil
			})
			_, err := h.ProbeSYN(context.Background(), "192.0.2.20:80")
			want := map[string]error{"send": ErrProbeSend, "silence": ErrProbeNoResponse, "refused": ErrProbeRefused}[kind]
			require.ErrorIs(t, err, want)
			h.mu.Lock()
			require.Empty(t, h.flights)
			h.mu.Unlock()
			require.Empty(t, h.slots)
		})
	}
}

func TestSYNRetryBackoffBounds(t *testing.T) {
	policy := DefaultSYNRetryPolicy()
	require.NoError(t, policy.validate())
	for i, want := range []time.Duration{500 * time.Millisecond, time.Second, 2 * time.Second, 2 * time.Second} {
		require.Equal(t, want, policy.delay(i+1, 0))
		require.GreaterOrEqual(t, policy.delay(i+1, 1), want)
		require.LessOrEqual(t, policy.delay(i+1, 1), policy.MaxResponseTimeout)
	}
	for _, bad := range []SYNRetryPolicy{{}, {-1, time.Second, time.Second, time.Second}, {11, time.Second, time.Second, time.Second}, {1, 0, time.Second, time.Second}, {1, time.Second, time.Second, time.Millisecond}} {
		require.Error(t, bad.validate())
	}
}

func TestPCAPWriteQueueWaitHonorsContext(t *testing.T) {
	fanout := &pcapFanOut{writeGate: make(chan struct{}, 1)}
	fanout.writeGate <- struct{}{} // another injection owns the device
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, fanout.WritePacketContext(ctx, []byte{1}), context.DeadlineExceeded)
	<-fanout.writeGate
	require.Empty(t, fanout.writeGate)
}

// These tests are opt-in and only contact listeners created by the test.
func liveHalfOpenInterface(t *testing.T) *net.Interface {
	t.Helper()
	device := os.Getenv("NETSTACKVM_PCAP_DEVICE")
	if device == "" {
		t.Skip("set NETSTACKVM_PCAP_DEVICE to enable real loopback tests")
	}
	iface, err := net.InterfaceByName(device)
	require.NoError(t, err)
	if iface.Flags&net.FlagLoopback == 0 {
		t.Skip("this live test only uses loopback")
	}
	return iface
}

func TestHalfOpenPCAPBatchPreservesHostConnection(t *testing.T) {
	iface := liveHalfOpenInterface(t)
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer listener.Close()
	var accepted atomic.Int32
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			accepted.Add(1)
			go func() { defer conn.Close(); _, _ = io.Copy(conn, conn) }()
		}
	}()
	host, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	require.NoError(t, err)
	defer host.Close()
	echo := func() {
		require.NoError(t, host.SetDeadline(time.Now().Add(2*time.Second)))
		_, err := host.Write([]byte("host-health"))
		require.NoError(t, err)
		reply := make([]byte, len("host-health"))
		_, err = io.ReadFull(host, reply)
		require.NoError(t, err)
		require.Equal(t, "host-health", string(reply))
	}
	echo()
	const count = 64
	results := make(chan error, count)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.IPv4(127, 0, 0, 1), MaxInFlight: 16, PacketsPerSecond: 200})
	require.NoError(t, err)
	defer h.Close()
	for i := 0; i < count; i++ {
		go func() {
			probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
			defer probeCancel()
			_, err := h.ProbeSYN(probeCtx, listener.Addr().String())
			results <- err
		}()
		if i%8 == 0 {
			echo()
		}
	}
	for i := 0; i < count; i++ {
		require.NoError(t, <-results)
	}
	echo()
	require.NoError(t, h.Close())
	echo()
	// Only the ordinary host socket may have completed a handshake.
	require.EqualValues(t, 1, accepted.Load())
	host.Close()
	listener.Close()
	<-serverDone
	h.mu.Lock()
	require.Empty(t, h.flights)
	h.mu.Unlock()
	require.Empty(t, h.slots)
}

func TestHalfOpenPCAPRetryAfterCapturedReplyLoss(t *testing.T) {
	iface := liveHalfOpenInterface(t)
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.IPv4(127, 0, 0, 1), Retry: SYNRetryPolicy{4, time.Second, 200 * time.Millisecond, time.Second}})
	require.NoError(t, err)
	defer h.Close()
	var replies atomic.Int32
	h.main.driver.SetPCAPInboundFilter(func(packet gopacket.Packet) bool {
		_, tcp, ok := ipv4TCP(packet)
		if ok && tcp.SYN && tcp.ACK && int(tcp.SrcPort) == listener.Addr().(*net.TCPAddr).Port && replies.Add(1) <= 2 {
			return false
		}
		return h.observeInbound(h.main.MainNICID(), packet)
	})
	p, err := h.StartTCPProbe(ctx, listener.Addr().String())
	require.NoError(t, err)
	defer p.Close()
	_, err = p.ProbeSYNContext(ctx)
	require.NoError(t, err)
	_, err = p.ReceiveSYNACKContext(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, replies.Load(), int32(3))
	require.GreaterOrEqual(t, p.attempts, 2)
	require.LessOrEqual(t, p.attempts, 4)
	require.NoError(t, listener.SetDeadline(time.Now().Add(100*time.Millisecond)))
	conn, err := listener.Accept()
	if conn != nil {
		conn.Close()
	}
	require.Error(t, err, "retry must not complete a handshake")
	t.Logf("captured replies=%d SYN attempts=%d", replies.Load(), p.attempts)
}

func TestHalfOpenPCAPClosedPortAndCancellation(t *testing.T) {
	iface := liveHalfOpenInterface(t)
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	target := listener.Addr().String()
	listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	defer h.Close()
	p, err := h.StartTCPProbe(ctx, target)
	require.NoError(t, err)
	_, err = p.ProbeSYNContext(ctx)
	require.NoError(t, err)
	_, err = p.ReceiveSYNACKContext(ctx)
	require.ErrorIs(t, err, ErrProbeRefused)
	p.Close()
	// Discard captured responses to make cancellation deterministic on a real device.
	h.main.driver.SetPCAPInboundFilter(func(gopacket.Packet) bool { return false })
	p, err = h.StartTCPProbe(ctx, target)
	require.NoError(t, err)
	_, err = p.ProbeSYNContext(ctx)
	require.NoError(t, err)
	stepCtx, stepCancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer stepCancel()
	started := time.Now()
	_, err = p.ReceiveSYNACKContext(stepCtx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Less(t, time.Since(started), time.Second)
	p.Close()
	started = time.Now()
	require.NoError(t, h.Close())
	require.Less(t, time.Since(started), 2*time.Second)
	t.Logf("capture close took %s", time.Since(started))
}

// The opt-in peer must implement COUNT\n -> cumulative accepted TCP connections.
// Comparing counts distinguishes wire-level half-open probes from a TUN proxy
// that establishes a remote connection on behalf of a local SYN.
func TestHalfOpenPCAPControlledPeer(t *testing.T) {
	device, source, target := os.Getenv("NETSTACKVM_LIVE_DEVICE"), os.Getenv("NETSTACKVM_LIVE_SOURCE"), os.Getenv("NETSTACKVM_LIVE_TARGET")
	if device == "" || source == "" || target == "" {
		t.Skip("set NETSTACKVM_LIVE_DEVICE/SOURCE/TARGET for a controlled LAN peer")
	}
	iface, err := net.InterfaceByName(device)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	host, err := (&net.Dialer{}).DialContext(ctx, "tcp4", target)
	require.NoError(t, err)
	defer host.Close()
	reader := bufio.NewReader(host)
	count := func() int {
		require.NoError(t, host.SetDeadline(time.Now().Add(2*time.Second)))
		_, err := fmt.Fprintln(host, "COUNT")
		require.NoError(t, err)
		var n int
		_, err = fmt.Fscanln(reader, &n)
		require.NoError(t, err)
		return n
	}
	before := count()
	h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.ParseIP(source), Gateway: net.ParseIP(os.Getenv("NETSTACKVM_LIVE_GATEWAY"))})
	require.NoError(t, err)
	defer h.Close()
	t.Logf("device=%s link=%d source=%s target=%s", iface.Name, h.main.driver.adaptor.linkType, source, target)
	for i := 0; i < 16; i++ {
		probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
		_, err := h.ProbeSYN(probeCtx, target)
		probeCancel()
		require.NoError(t, err)
	}
	if closed := os.Getenv("NETSTACKVM_LIVE_CLOSED_TARGET"); closed != "" {
		_, err := h.ProbeSYN(ctx, closed)
		require.True(t, errors.Is(err, ErrProbeRefused) || errors.Is(err, ErrProbeNoResponse),
			"a non-listening peer must never be reported open: %v", err)
		t.Logf("non-listening peer result: %v", err)
	}
	require.NoError(t, h.Close())
	require.Equal(t, before, count(), "the capture transport completed remote TCP handshakes")
	t.Logf("16 open probes and existing host connection passed without remote Accept")
}

func TestHalfOpenTransportPolicy(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   net.Flags
		link    layers.LinkType
		allowed bool
	}{
		{"ethernet", net.FlagUp, layers.LinkTypeEthernet, true},
		{"npcap-loopback", net.FlagLoopback, layers.LinkTypeNull, true},
		{"loopback-raw", net.FlagLoopback, 12, true},
		{"tun", net.FlagUp, 12, false},
		{"raw", net.FlagUp, layers.LinkTypeRaw, false},
		{"point-to-point", net.FlagPointToPoint, layers.LinkTypeEthernet, false},
		{"unknown", net.FlagUp, 255, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			iface := &net.Interface{Name: tc.name, Flags: tc.flags}
			err := validateHalfOpenTransport(iface, tc.link, false)
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrUnverifiedSYNTransport)
			}
			require.NoError(t, validateHalfOpenTransport(iface, tc.link, true))
		})
	}
}

func TestHalfOpenPCAPUnverifiedTransportRejected(t *testing.T) {
	device, source := os.Getenv("NETSTACKVM_UNVERIFIED_DEVICE"), os.Getenv("NETSTACKVM_UNVERIFIED_SOURCE")
	if device == "" || source == "" {
		t.Skip("set NETSTACKVM_UNVERIFIED_DEVICE/SOURCE for a raw-IP proxy interface")
	}
	iface, err := net.InterfaceByName(device)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for i := 0; i < 3; i++ {
		h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.ParseIP(source)})
		if h != nil {
			h.Close()
		}
		require.Nil(t, h)
		require.ErrorIs(t, err, ErrUnverifiedSYNTransport)
	}
	// The low-level API still permits a deliberate opt-in; opening it sends no SYN.
	h, err := OpenHalfOpenSYN(ctx, HalfOpenSYNConfig{Iface: iface, SourceIP: net.ParseIP(source), AllowUnverifiedTransport: true})
	require.NoError(t, err)
	require.NoError(t, h.Close())
}

func TestHalfOpenSessionCloseCancelsSynchronousCall(t *testing.T) {
	sent := make(chan struct{})
	var once sync.Once
	h := mockHalfOpen(t, func(_ *HalfOpenSYN, _ []byte) error {
		once.Do(func() { close(sent) })
		return nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := h.ProbeSYN(context.Background(), "192.0.2.20:80", WithSYNRetry(SYNRetryPolicy{3, time.Second, time.Second, time.Second}))
		done <- err
	}()
	select {
	case <-sent:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not send")
	}
	require.NoError(t, h.Close())
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("session close did not interrupt synchronous call")
	}
	_, err := h.ProbeSYN(context.Background(), "192.0.2.20:80")
	require.ErrorIs(t, err, errHalfOpenClosed)
	require.Empty(t, h.slots)
}
