package netstackvm

import (
	"context"
	"errors"
	"fmt"
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
	require.NoError(t, h.Wait(context.Background()))
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
	require.NoError(t, h.Wait(context.Background()))
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
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	p, err := h.StartTCPProbe(ctx, "192.0.2.20:80", WithSYNRetry(SYNRetryPolicy{3, time.Second, time.Second, time.Second}))
	require.NoError(t, err)
	defer p.Close()
	_, err = p.ProbeSYNContext(context.Background())
	require.NoError(t, err)
	_, err = p.ReceiveSYNACKContext(context.Background())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, h.Wait(context.Background()))
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

func TestHalfOpenConcurrentEmitDrainsAllResults(t *testing.T) {
	var opens, results atomic.Int32
	h := mockHalfOpen(t, func(h *HalfOpenSYN, b []byte) error {
		h.observeInbound(h.main.MainNICID(), synReply(t, b, nil))
		return nil
	})
	h.onOpen = func(net.IP, int) { opens.Add(1) }
	h.onResult = func(_ string, _ TCPSegment, err error) {
		if err != nil {
			t.Errorf("probe failed: %v", err)
		}
		results.Add(1)
	}
	var wg sync.WaitGroup
	for i := 0; i < 80; i++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			if err := h.Emit(context.Background(), "192.0.2.20", port); err != nil {
				t.Error(err)
			}
		}(10000 + i)
	}
	wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, h.Wait(ctx))
	require.EqualValues(t, 80, opens.Load())
	require.EqualValues(t, 80, results.Load())
	h.mu.Lock()
	require.Empty(t, h.flights)
	h.mu.Unlock()
	require.Empty(t, h.slots)
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
