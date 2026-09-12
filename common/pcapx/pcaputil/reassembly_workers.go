package pcaputil

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// TCPReassemblyStats separates queue accounting from TCP stream completeness.
// ProcessedPackets counts attempted packets; success also requires Err()==nil.
type TCPReassemblyStats struct {
	CaptureAccountingAvailable                          bool
	CapturedPackets, CapturedBytes                      uint64 // all read packets, before protocol analysis; excludes file record headers
	AccountingAvailable                                 bool
	Workers                                             int
	AcceptedPackets, ProcessedPackets, RejectedPackets  uint64
	DeliveredBytes, CallbackPanics, ResourceLimitEvents uint64
	DecodeErrors, TruncatedCaptures                     uint64
	InvalidSegments                                     uint64
	UnreassembledBytes, UnreassembledSegments           uint64
	BackpressureEvents                                  uint64
	BackpressureTime                                    time.Duration
	QueueBufferBytes                                    int64 // configured payload arena capacity, excluding metadata
	QueuePacketCapacity                                 int
	Devices                                             []TCPDeviceCaptureStats
}

type TCPDeviceCaptureStats struct {
	Device                              string
	Received, Dropped, InterfaceDropped int
	Available                           bool
	Error                               string
}

type reassemblyCounters struct {
	accepted, processed, rejected             atomic.Uint64
	delivered, panics, limits                 atomic.Uint64
	decodeErrors, truncated                   atomic.Uint64
	invalid                                   atomic.Uint64
	unreassembledBytes, unreassembledSegments atomic.Uint64
	blocked                                   atomic.Uint64
	blockedNS                                 atomic.Int64
}

// Shared only for connection admission and out-of-order reservations. Ordered
// payload processing does not contend on this lock.
type reassemblyBudget struct {
	mu                     sync.Mutex
	flows, bytes, segments int
	options                TCPReassemblyOptions
}

func (b *reassemblyBudget) reserveFlow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.flows >= b.options.MaxFlows {
		return false
	}
	b.flows++
	return true
}
func (b *reassemblyBudget) releaseFlows(n int) { b.mu.Lock(); b.flows -= n; b.mu.Unlock() }
func (b *reassemblyBudget) reserve(bytes, segments int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if bytes > b.options.MaxTotalPendingBytes-b.bytes || segments > b.options.MaxTotalPendingSegments-b.segments {
		return false
	}
	b.bytes += bytes
	b.segments += segments
	return true
}
func (b *reassemblyBudget) release(bytes, segments int) {
	b.mu.Lock()
	b.bytes -= bytes
	b.segments -= segments
	b.mu.Unlock()
}

// Decoded submissions own addresses and just the TCP fields consumed by the
// reassembler. Raw private capture paths move full decoding onto the worker.
type workerPacket struct {
	data           []byte
	ts             time.Time
	link           layers.LinkType
	raw            bool
	key            flowKey
	src, dst       [16]byte
	sport, dport   uint16
	seq            uint32
	flags          uint8
	macSrc, macDst [6]byte
	ethernet       bool
}

type workerBatch struct {
	arena   []byte
	packets []workerPacket
	used    int
}

type tcpWorker struct {
	pool        *TrafficPool
	queue, free chan *workerBatch
	current     *workerBatch // dispatcher lock only
}

type tcpWorkers struct {
	mu              sync.Mutex // orders all submissions and queue closure
	closeOnce       sync.Once
	closed          bool
	root            *TrafficPool
	workers         []*tcpWorker
	budget          *reassemblyBudget
	stats           reassemblyCounters
	stop, timerDone chan struct{}
	wg              sync.WaitGroup
	errorMu         sync.Mutex
	err             error // first error, bounded even during a flood
	devices         []TCPDeviceCaptureStats
	lastSubmit      time.Time
}

func (p *TrafficPool) startWorkers() {
	if p.options.Workers <= 1 {
		return
	}
	d := &tcpWorkers{root: p, budget: &reassemblyBudget{options: p.options}, stop: make(chan struct{}), timerDone: make(chan struct{}), lastSubmit: time.Now()}
	opts := p.options
	opts.Workers = 1
	for i := 0; i < p.options.Workers; i++ {
		child := newTrafficPoolWithExpiry(context.WithoutCancel(p.ctx), opts, false)
		child.owner, child.parallel, child.counters = p, d, &reassemblyCounters{}
		child.flowCache.budget = d.budget
		child.onFlowCreated, child.onFlowClosed = p.onFlowCreated, p.onFlowClosed
		child.onFlowFrameDataFrameArrived = p.onFlowFrameDataFrameArrived
		child.onFlowFrameDataFrameReassembled = p.onFlowFrameDataFrameReassembled
		child._onHTTPFlow = p._onHTTPFlow
		if p.captureConf != nil {
			conf := *p.captureConf
			conf.trafficPool = child
			child.captureConf = &conf
		}
		w := &tcpWorker{pool: child, queue: make(chan *workerBatch, opts.WorkerQueueDepth), free: make(chan *workerBatch, opts.WorkerQueueDepth+2)}
		for j := 0; j < opts.WorkerQueueDepth+2; j++ {
			w.free <- &workerBatch{arena: make([]byte, opts.WorkerBatchBytes), packets: make([]workerPacket, 0, opts.WorkerBatchPackets)}
		}
		d.workers = append(d.workers, w)
	}
	p.parallel = d
	for _, w := range d.workers {
		d.wg.Add(1)
		go d.run(w)
	}
	go d.flushLoop()
}

func (d *tcpWorkers) fail(err error) {
	d.errorMu.Lock()
	if d.err == nil {
		d.err = err
	}
	d.errorMu.Unlock()
}

func (p *TrafficPool) Err() error {
	if p.parallel == nil {
		if e := p.firstError.Load(); e != nil {
			return e.err
		}
		return nil
	}
	p.parallel.errorMu.Lock()
	defer p.parallel.errorMu.Unlock()
	return p.parallel.err
}

func (p *TrafficPool) Stats() TCPReassemblyStats {
	d := p.parallel
	if d == nil {
		p.deviceStatsMu.Lock()
		devices := append([]TCPDeviceCaptureStats(nil), p.singleDevices...)
		p.deviceStatsMu.Unlock()
		return TCPReassemblyStats{Workers: 1, CaptureAccountingAvailable: p.captureAccountingAvailable, CapturedPackets: p.capturedPackets.Load(), CapturedBytes: p.capturedBytes.Load(), UnreassembledBytes: p.singleUnreassembledBytes.Load(), UnreassembledSegments: p.singleUnreassembledSegments.Load(), Devices: devices,
			InvalidSegments: p.singleDiagnostics.invalid.Load(), DecodeErrors: p.singleDiagnostics.decodeErrors.Load(), CallbackPanics: p.singleDiagnostics.panics.Load(), ResourceLimitEvents: p.singleDiagnostics.limits.Load(), TruncatedCaptures: p.singleDiagnostics.truncated.Load()}
	}
	s := TCPReassemblyStats{Workers: len(d.workers), AcceptedPackets: d.stats.accepted.Load(), ProcessedPackets: d.stats.processed.Load(), RejectedPackets: d.stats.rejected.Load(), BackpressureEvents: d.stats.blocked.Load(), BackpressureTime: time.Duration(d.stats.blockedNS.Load()), QueueBufferBytes: int64(len(d.workers) * (d.root.options.WorkerQueueDepth + 2) * d.root.options.WorkerBatchBytes)}
	s.AccountingAvailable = true
	s.CaptureAccountingAvailable, s.CapturedPackets, s.CapturedBytes = p.captureAccountingAvailable, p.capturedPackets.Load(), p.capturedBytes.Load()
	s.QueuePacketCapacity = len(d.workers) * (d.root.options.WorkerQueueDepth + 2) * d.root.options.WorkerBatchPackets
	s.TruncatedCaptures = d.stats.truncated.Load()
	s.DecodeErrors = d.stats.decodeErrors.Load()
	s.InvalidSegments = d.stats.invalid.Load()
	s.CallbackPanics = d.stats.panics.Load()
	for _, w := range d.workers {
		c := w.pool.counters
		s.DeliveredBytes += c.delivered.Load()
		s.CallbackPanics += c.panics.Load()
		s.DecodeErrors += c.decodeErrors.Load()
		s.InvalidSegments += c.invalid.Load()
		s.ResourceLimitEvents += c.limits.Load()
		s.UnreassembledBytes += c.unreassembledBytes.Load()
		s.UnreassembledSegments += c.unreassembledSegments.Load()
	}
	d.errorMu.Lock()
	s.Devices = append([]TCPDeviceCaptureStats(nil), d.devices...)
	d.errorMu.Unlock()
	return s
}

// WithTCPReassemblyStats reports the final, drained multi-worker counters.
func WithTCPReassemblyStats(h func(TCPReassemblyStats)) CaptureOption {
	return func(c *CaptureConfig) error { c.onReassemblyStats = h; return nil }
}

func workerHash(k flowKey) uint64 {
	a, b := k.src.Addr().As16(), k.dst.Addr().As16()
	x := binary.LittleEndian.Uint64(a[:8]) ^ binary.LittleEndian.Uint64(a[8:])
	x ^= binary.LittleEndian.Uint64(b[:8])*0x9e3779b97f4a7c15 ^ binary.LittleEndian.Uint64(b[8:])
	x ^= uint64(k.src.Port())<<32 | uint64(k.dst.Port())
	if k.ipv6 {
		x ^= 0x9e3779b97f4a7c15
	}
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ x>>31
}

func (d *tcpWorkers) submit(packet workerPacket) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		d.stats.rejected.Add(1)
		d.fail(fmt.Errorf("TCP packet rejected after cancellation or queue closure"))
		return
	}
	if len(packet.data) > d.root.options.WorkerBatchBytes {
		d.stats.rejected.Add(1)
		d.fail(fmt.Errorf("TCP packet size %d exceeds worker batch byte limit %d", len(packet.data), d.root.options.WorkerBatchBytes))
		return
	}
	w := d.workers[workerHash(packet.key)%uint64(len(d.workers))]
	if w.current != nil && (len(w.current.packets) == cap(w.current.packets) || len(packet.data) > len(w.current.arena)-w.current.used) {
		d.publish(w)
	}
	if w.current == nil {
		w.current = <-w.free
	}
	b := w.current
	n := copy(b.arena[b.used:], packet.data)
	packet.data = b.arena[b.used : b.used+n : b.used+n]
	b.used += n
	b.packets = append(b.packets, packet)
	d.stats.accepted.Add(1)
	if len(b.packets) == cap(b.packets) || b.used == len(b.arena) {
		d.publish(w)
	}
}

func (d *tcpWorkers) submitLayers(eth *layers.Ethernet, network gopacket.SerializableLayer, tcp *layers.TCP, tss []time.Time) {
	if tcp == nil {
		return
	}
	var srcIP, dstIP net.IP
	var ipv6 bool
	switch ip := network.(type) {
	case *layers.IPv4:
		srcIP, dstIP = ip.SrcIP, ip.DstIP
	case *layers.IPv6:
		srcIP, dstIP, ipv6 = ip.SrcIP, ip.DstIP, true
	default:
		return
	}
	key, ok := makeFlowKey(srcIP, dstIP, uint16(tcp.SrcPort), uint16(tcp.DstPort), ipv6)
	if !ok {
		return
	}
	var ts time.Time
	if len(tss) > 0 {
		ts = tss[0]
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	src, _ := netip.AddrFromSlice(srcIP)
	dst, _ := netip.AddrFromSlice(dstIP)
	r := workerPacket{data: tcp.Payload, ts: ts, key: key, src: src.As16(), dst: dst.As16(), sport: uint16(tcp.SrcPort), dport: uint16(tcp.DstPort), seq: tcp.Seq}
	if tcp.SYN {
		r.flags |= 1
	}
	if tcp.ACK {
		r.flags |= 2
	}
	if tcp.FIN {
		r.flags |= 4
	}
	if tcp.RST {
		r.flags |= 8
	}
	if eth != nil {
		r.ethernet = true
		copy(r.macSrc[:], eth.SrcMAC)
		copy(r.macDst[:], eth.DstMAC)
	}
	d.submit(r)
}

func (p *TrafficPool) deviceStats(h *PcapHandleWrapper) {
	s := TCPDeviceCaptureStats{Device: h.device}
	h.mutex.RLock()
	if h.isClose {
		s.Error = "handle closed before final statistics"
	} else {
		raw, err := h.handle.Stats()
		if err != nil {
			s.Error = err.Error()
		} else {
			s.Available = true
			s.Received = raw.PacketsReceived
			s.Dropped = raw.PacketsDropped
			s.InterfaceDropped = raw.PacketsIfDropped
		}
	}
	h.mutex.RUnlock()
	if p.parallel != nil {
		p.parallel.recordDeviceStats(s)
		return
	}
	p.deviceStatsMu.Lock()
	p.singleDevices = append(p.singleDevices, s)
	p.deviceStatsMu.Unlock()
	if !s.Available {
		p.reassemblyFailure("pcap final drop statistics unavailable: " + s.Error)
	} else if s.Dropped > 0 || s.InterfaceDropped > 0 {
		p.reassemblyFailure(fmt.Sprintf("pcap dropped packets: capture=%d interface=%d", s.Dropped, s.InterfaceDropped))
	}
}

func (d *tcpWorkers) recordDeviceStats(s TCPDeviceCaptureStats) {
	d.errorMu.Lock()
	d.devices = append(d.devices, s)
	d.errorMu.Unlock()
	if !s.Available {
		d.fail(fmt.Errorf("pcap final drop statistics unavailable: %s", s.Error))
	} else if s.Dropped > 0 || s.InterfaceDropped > 0 {
		d.fail(fmt.Errorf("pcap dropped packets: capture=%d interface=%d", s.Dropped, s.InterfaceDropped))
	}
}

func (d *tcpWorkers) publish(w *tcpWorker) {
	if w.current == nil || len(w.current.packets) == 0 {
		return
	}
	b := w.current
	w.current = nil
	d.lastSubmit = time.Now()
	select {
	case w.queue <- b:
	default:
		start := time.Now()
		d.stats.blocked.Add(1)
		w.queue <- b // backpressure: never drop or process out of order
		d.stats.blockedNS.Add(int64(time.Since(start)))
	}
}

func (d *tcpWorkers) flushLoop() {
	defer close(d.timerDone)
	t := time.NewTicker(d.root.options.WorkerFlushInterval)
	defer t.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-t.C:
		}
		d.mu.Lock()
		for _, w := range d.workers {
			d.publish(w)
		}
		// Expiry during a backlog would mistake scheduling delay for TCP idle
		// time. Only expire when all accepted packets have completed.
		if d.stats.accepted.Load() == d.stats.processed.Load() && time.Since(d.lastSubmit) >= d.root.options.IdleTimeout {
			for _, w := range d.workers {
				w.pool.mu.Lock()
				w.pool.flowCache.expire()
				w.pool.retireFlows()
				w.pool.mu.Unlock()
			}
		}
		d.mu.Unlock()
	}
}

func (d *tcpWorkers) run(w *tcpWorker) {
	defer d.wg.Done()
	conf := w.pool.captureConf
	if conf == nil {
		conf = NewDefaultConfig()
		conf.trafficPool = w.pool
	}
	decoder := offlineDecoder{conf: conf}
	for b := range w.queue {
		for i := range b.packets {
			func() {
				defer func() {
					if err := recover(); err != nil {
						w.pool.notePanic(err)
					}
				}()
				r := &b.packets[i]
				if r.raw {
					decoder.link = r.link
					decoder.feed(w.pool.ctx, r.data, gopacket.CaptureInfo{Timestamp: r.ts, CaptureLength: len(r.data), Length: len(r.data)})
				} else {
					tcp := layers.TCP{SrcPort: layers.TCPPort(r.sport), DstPort: layers.TCPPort(r.dport), Seq: r.seq, SYN: r.flags&1 != 0, ACK: r.flags&2 != 0, FIN: r.flags&4 != 0, RST: r.flags&8 != 0}
					tcp.Payload = r.data
					var network gopacket.SerializableLayer = &layers.IPv4{SrcIP: net.IP(r.src[:]), DstIP: net.IP(r.dst[:])}
					if r.key.ipv6 {
						network = &layers.IPv6{SrcIP: net.IP(r.src[:]), DstIP: net.IP(r.dst[:])}
					}
					var eth *layers.Ethernet
					if r.ethernet {
						eth = &layers.Ethernet{SrcMAC: net.HardwareAddr(r.macSrc[:]), DstMAC: net.HardwareAddr(r.macDst[:])}
					}
					w.pool.Feed(eth, network, &tcp, r.ts)
				}
			}()
		}
		d.stats.processed.Add(uint64(len(b.packets)))
		clear(b.packets)
		b.packets = b.packets[:0]
		b.used = 0
		w.free <- b
	}
	func() {
		defer func() {
			if err := recover(); err != nil {
				w.pool.notePanic(err)
			}
		}()
		w.pool.Close()
	}()
	// Caller-retained flows must not retain the capture's reusable arenas.
	for len(w.free) > 0 {
		b := <-w.free
		b.arena = nil
		b.packets = nil
	}
}

func (d *tcpWorkers) close() {
	d.closeOnce.Do(func() {
		close(d.stop)
		<-d.timerDone
		d.mu.Lock()
		d.closed = true
		for _, w := range d.workers {
			d.publish(w)
			close(w.queue)
		}
		d.mu.Unlock()
		d.wg.Wait()
	})
}

func (p *TrafficPool) notePanic(value any) {
	if p.parallel == nil {
		p.singleDiagnostics.panics.Add(1)
		p.reassemblyFailure(fmt.Sprintf("TCP callback panic: %v", value))
		return
	}
	if p.counters == nil {
		p.parallel.stats.panics.Add(1)
	} else {
		p.counters.panics.Add(1)
	}
	p.parallel.fail(fmt.Errorf("TCP worker callback panic: %v", value))
}

func (d *tcpWorkers) checkTruncation(ci gopacket.CaptureInfo) {
	if ci.CaptureLength < ci.Length {
		d.stats.truncated.Add(1)
		d.fail(fmt.Errorf("capture contains truncated packets (captured %d of %d bytes)", ci.CaptureLength, ci.Length))
	}
}

func (p *TrafficPool) reservePending(bytes, segments int) bool {
	if p.owner != nil {
		if !p.parallel.budget.reserve(bytes, segments) {
			return false
		}
	} else if bytes > p.options.MaxTotalPendingBytes-p.pendingBytes || segments > p.options.MaxTotalPendingSegments-p.pendingSegments {
		return false
	}
	p.pendingBytes += bytes
	p.pendingSegments += segments
	return true
}
func (p *TrafficPool) releasePending(bytes, segments int) {
	p.pendingBytes -= bytes
	p.pendingSegments -= segments
	if p.owner != nil {
		p.parallel.budget.release(bytes, segments)
	}
}

func rawFlowKey(raw []byte, link layers.LinkType) (flowKey, bool, error) {
	if link == layers.LinkTypeEthernet && len(raw) >= 14 {
		kind, offset := binary.BigEndian.Uint16(raw[12:14]), 14
		for (kind == 0x8100 || kind == 0x88a8) && len(raw) >= offset+4 {
			kind = binary.BigEndian.Uint16(raw[offset+2 : offset+4])
			offset += 4
		}
		ip := raw[offset:]
		if kind == 0x0800 && len(ip) >= 20 && ip[9] == 6 && binary.BigEndian.Uint16(ip[6:8])&0x3fff == 0 {
			h := int(ip[0]&15) * 4
			if h >= 20 && len(ip) >= h+20 {
				key, ok := makeFlowKey(ip[12:16], ip[16:20], binary.BigEndian.Uint16(ip[h:h+2]), binary.BigEndian.Uint16(ip[h+2:h+4]), false)
				return key, ok, nil
			}
		}
		if kind == 0x86dd && len(ip) >= 60 && ip[6] == 6 {
			key, ok := makeFlowKey(ip[8:24], ip[24:40], binary.BigEndian.Uint16(ip[40:42]), binary.BigEndian.Uint16(ip[42:44]), true)
			return key, ok, nil
		}
	}
	packet := gopacket.NewPacket(raw, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: true})
	tcp, ok := packet.TransportLayer().(*layers.TCP)
	if !ok && packet.TransportLayer() != nil {
		return flowKey{}, false, nil
	}
	if failure := packet.ErrorLayer(); failure != nil {
		return flowKey{}, false, failure.Error()
	}
	if !ok {
		return flowKey{}, false, nil
	}
	switch ip := packet.NetworkLayer().(type) {
	case *layers.IPv4:
		key, ok := makeFlowKey(ip.SrcIP, ip.DstIP, uint16(tcp.SrcPort), uint16(tcp.DstPort), false)
		return key, ok, nil
	case *layers.IPv6:
		key, ok := makeFlowKey(ip.SrcIP, ip.DstIP, uint16(tcp.SrcPort), uint16(tcp.DstPort), true)
		return key, ok, nil
	}
	return flowKey{}, false, nil
}
