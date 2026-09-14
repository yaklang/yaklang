package pcaputil

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type flowKey struct {
	src, dst netip.AddrPort
	ipv6     bool
}

func makeFlowKey(srcIP, dstIP net.IP, srcPort, dstPort uint16, ipv6 bool) (flowKey, bool) {
	src, ok := netip.AddrFromSlice(srcIP)
	if !ok {
		return flowKey{}, false
	}
	dst, ok := netip.AddrFromSlice(dstIP)
	if !ok {
		return flowKey{}, false
	}
	a, b := netip.AddrPortFrom(src.Unmap(), srcPort), netip.AddrPortFrom(dst.Unmap(), dstPort)
	if a.Compare(b) > 0 {
		a, b = b, a
	}
	return flowKey{src: a, dst: b, ipv6: ipv6}, true
}

type TrafficPool struct {
	// Default Feed is serialized. Optional workers preserve each flow's packet
	// order and invoke callbacks concurrently across flows. Callbacks must not
	// recursively call Feed, NewFlow or Close on this pool.
	mu              sync.Mutex
	closeOnce       sync.Once
	options         TCPReassemblyOptions
	pendingBytes    int
	pendingSegments int
	closed          bool
	ctx             context.Context
	done            <-chan struct{}
	captureConf     *CaptureConfig
	flowCache       *trafficFlowCache
	onFlowCreated   func(flow *TrafficFlow)
	onFlowClosed    func(reason TrafficFlowCloseReason, flow *TrafficFlow)
	// internal field, not for user
	_onHTTPFlow                     func(flow *TrafficFlow, r *http.Request, response *http.Response)
	onFlowFrameDataFrameArrived     []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)
	onFlowFrameDataFrameReassembled []func(flow *TrafficFlow, conn *TrafficConnection, frame *TrafficFrame)
	currentStreamIndex              uint64
	parallel                        *tcpWorkers
	owner                           *TrafficPool
	counters                        *reassemblyCounters
	firstError                      atomic.Pointer[reassemblyError]
}

func NewTrafficPool(ctx context.Context) *TrafficPool {
	return newTrafficPool(ctx, TCPReassemblyOptions{})
}

func newTrafficPool(ctx context.Context, options TCPReassemblyOptions) *TrafficPool {
	return newTrafficPoolWithExpiry(ctx, options, true)
}

func newTrafficPoolWithExpiry(ctx context.Context, options TCPReassemblyOptions, expiry bool) *TrafficPool {
	if ctx == nil {
		ctx = context.Background()
	}
	options, _ = options.normalized()
	pool := &TrafficPool{ctx: ctx, done: ctx.Done(), options: options}
	pool.flowCache = newTrafficFlowCache(options)
	pool.flowCache.checkExpiry = expiry
	if !expiry {
		close(pool.flowCache.done)
		return pool
	}
	go func() {
		defer close(pool.flowCache.done)
		interval := options.IdleTimeout / 2
		if interval > time.Second {
			interval = time.Second
		}
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-pool.flowCache.stop:
				return
			case <-ticker.C:
				pool.mu.Lock()
				if !pool.closed {
					pool.flowCache.expire()
					pool.retireFlows()
				}
				pool.mu.Unlock()
			}
		}
	}()
	return pool
}

// Close flushes trailing frames, stops readers and expiry workers, and waits
// for HTTP parsing. Call it after feeding a standalone TrafficPool.
func (p *TrafficPool) Close() {
	p.closeOnce.Do(func() {
		if p.parallel != nil && p.owner == nil {
			p.parallel.close()
		}
		p.mu.Lock()
		defer func() { p.mu.Unlock(); p.flowCache.Close() }()
		p.closed = true
		p.retireFlows()
		p.flowCache.ForEach(func(_ string, f *TrafficFlow) {
			p.finishFlow(f, TrafficFlowCloseReason_CTX_CANCEL)
		})
	})
}

func (p *TrafficPool) drainHTTP(f *TrafficFlow) {
	f.ForceShutdownConnection()
	f.ClientConn.discardPending()
	f.ServerConn.discardPending()
	if p._onHTTPFlow != nil {
		for f.CanShiftHTTPFlow() {
			req, rsp := f.ShiftFlow()
			p._onHTTPFlow(f, req, rsp)
		}
	}
}

func (p *TrafficPool) AddWaitGroupDelta(delta int) {
	p.captureConf.wg.Add(delta)
}

func (p *TrafficPool) Done() {
	p.captureConf.wg.Done()
}

func (p *TrafficPool) nextStream() uint64 {
	if p.owner != nil {
		return p.owner.nextStream()
	}
	return atomic.AddUint64(&p.currentStreamIndex, 1)
}

func (p *TrafficPool) canceled() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *TrafficPool) Feed(ethernetLayer *layers.Ethernet, networkLayer gopacket.SerializableLayer, tcp *layers.TCP, tss ...time.Time) {
	if p.parallel != nil && p.owner == nil {
		p.parallel.submitLayers(ethernetLayer, networkLayer, tcp, tss)
		return
	}
	if tcp == nil {
		return
	}
	var srcIP, dstIP net.IP
	var ipv6 bool
	network := "tcp4"
	switch ip := networkLayer.(type) {
	case *layers.IPv4:
		srcIP, dstIP = ip.SrcIP, ip.DstIP
	case *layers.IPv6:
		srcIP, dstIP, ipv6, network = ip.SrcIP, ip.DstIP, true, "tcp6"
	default:
		return
	}
	key, valid := makeFlowKey(srcIP, dstIP, uint16(tcp.SrcPort), uint16(tcp.DstPort), ipv6)
	if !valid {
		return
	}
	var ts time.Time
	if len(tss) > 0 {
		ts = tss[0]
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.canceled() {
		return
	}
	defer p.retireFlows()
	flow, ok := p.flowCache.Get(key)
	if ok && flow.IsClosed() {
		p.drainHTTP(flow)
		// A new SYN may immediately reuse a completed 4-tuple. Ignore late data.
		if !tcp.SYN || tcp.ACK {
			return
		}
		p.flowCache.Remove(key)
		ok = false
	}
	if !ok {
		if tcp.RST || (!tcp.SYN && len(tcp.Payload) == 0) {
			return
		}
		// Packet-layer address slices may be overwritten by the next read.
		src := &net.TCPAddr{IP: append(net.IP(nil), srcIP...), Port: int(tcp.SrcPort)}
		dst := &net.TCPAddr{IP: append(net.IP(nil), dstIP...), Port: int(tcp.DstPort)}
		flow = p.newFlowWithAddrs(network, src, dst)
		if flow == nil {
			return
		}
		flow.IsIpv4, flow.IsIpv6 = !ipv6, ipv6
		flow.IsHalfOpen = !tcp.SYN || tcp.ACK
		if ethernetLayer != nil {
			flow.IsEthernetLinkLayer = true
			flow.HardwareSrcMac, flow.HardwareDstMac = ethernetLayer.SrcMAC.String(), ethernetLayer.DstMAC.String()
		}
		flow.init(p.onFlowCreated, p.onFlowFrameDataFrameReassembled, p.onFlowFrameDataFrameArrived, p.onFlowClosed)
	}
	// Port alone cannot identify direction when endpoints use the same port.
	conn := flow.ServerConn
	if flow.ClientConn.localPort == int(tcp.SrcPort) && flow.ClientConn.localIP.Equal(srcIP) {
		conn = flow.ClientConn
	}
	conn.FeedClient(tcp, ts)
	if flow.ClientConn.IsClosed() {
		flow.ClientConn.discardPending()
	}
	if flow.ServerConn.IsClosed() {
		flow.ServerConn.discardPending()
	}
}

func (p *TrafficPool) flowhash(netType, srcAddr, dstAddr string) string {
	hashMaterial := []string{netType, srcAddr, dstAddr}
	sort.Strings(hashMaterial)
	return codec.Sha256(strings.Join(hashMaterial, "-"))
}

func (p *TrafficPool) retireFlows() {
	for _, retired := range p.flowCache.takeRetired() {
		p.finishFlow(retired.flow, retired.reason)
	}
}

func (p *TrafficPool) finishFlow(f *TrafficFlow, reason TrafficFlowCloseReason) {
	if p.owner != nil {
		defer func() {
			if err := recover(); err != nil {
				p.notePanic(err)
			}
		}()
		defer p.drainHTTP(f)
		f.closeWithReason(reason)
		return
	}
	f.closeWithReason(reason)
	p.drainHTTP(f)
}
