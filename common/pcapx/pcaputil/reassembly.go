package pcaputil

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/lowtun/netstack/gvisor/pkg/tcpip/seqnum"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var connectionPool = &sync.Pool{ // TrafficConnection
	New: func() any {
		return &TrafficConnection{}
	},
}

type futureFrame struct {
	Payload   []byte
	Seq       uint32
	FIN       bool
	Timestamp time.Time
}

// TrafficConnection is a tcp connection
type TrafficConnection struct {
	hash                 string
	pendingBytes         int
	pendingBySeq         map[uint32]*futureFrame
	localAddr            net.Addr
	remoteAddr           net.Addr
	closed               atomic.Bool
	writer               *tcpStreamBuffer
	Flow                 *TrafficFlow
	timestamps           streamTimestamps
	reader               *tcpStreamBuffer
	remoteIP             net.IP
	localIP              net.IP
	waitGroup            []*futureFrame
	remotePort           int
	localPort            int
	isn                  uint32
	nextSeq              uint32
	finSeq               uint32
	finSeen              bool
	resetPending         bool
	resetSeq             uint32
	currentSeq           uint32
	waitACK              bool
	initialed            bool
	initHttpPacketDirect bool
	isHttpRequestConn    bool
}

func (t *TrafficConnection) MarkAsHttpRequestConn(b bool) {
	if t.initHttpPacketDirect {
		return
	}
	t.initHttpPacketDirect = true
	t.isHttpRequestConn = b
}

func (t *TrafficConnection) IsMarkedAsHttpPacket() bool {
	return t.initHttpPacketDirect
}

func (t *TrafficConnection) IsHttpRequestConn() bool {
	return t.isHttpRequestConn
}

func (t *TrafficConnection) IsHttpResponseConn() bool {
	return !t.isHttpRequestConn
}

func (t *TrafficConnection) Read(buf []byte) (int, error) {
	if t.reader == nil {
		return 0, io.EOF
	}
	return t.reader.Read(buf)
}

func (t *TrafficConnection) String() string {
	return fmt.Sprintf("%v -> %v", t.localAddr, t.remoteAddr)
}

func (t *TrafficConnection) LocalAddr() net.Addr {
	return t.localAddr
}

func (t *TrafficConnection) LocalIP() net.IP {
	return t.localIP
}

func (t *TrafficConnection) LocalPort() int {
	return t.localPort
}

func (t *TrafficConnection) RemoteAddr() net.Addr {
	return t.remoteAddr
}

func (t *TrafficConnection) RemoteIP() net.IP {
	return t.remoteIP
}

func (t *TrafficConnection) RemotePort() int {
	return t.remotePort
}

func (t *TrafficConnection) Hash() string {
	return t.hash
}

func (t *TrafficConnection) IsClosed() bool {
	return t.closed.Load() || t.Flow == nil || t.Flow.stopped()
}

func (t *TrafficConnection) Close() bool {
	t.closed.Store(true)
	if t.reader != nil {
		t.reader.Close()
	}
	return t.IsClosed()
}

func (t *TrafficConnection) CloseFlow() bool {
	t.closed.Store(true)
	if t.Flow != nil {
		t.Flow.Close()
	}
	return t.IsClosed()
}

func (t *TrafficConnection) Release() {
	t.localAddr = nil
	t.remoteAddr = nil
	t.closed.Store(false)
	t.reader, t.writer = nil, nil
	t.Flow = nil
	t.timestamps.clear()
	t.localIP, t.remoteIP = nil, nil
	t.waitGroup = nil
	t.pendingBySeq = nil
	t.pendingBytes = 0
	t.hash = ""
	t.localPort, t.remotePort = 0, 0
	t.isn, t.nextSeq, t.currentSeq = 0, 0, 0
	t.finSeq, t.finSeen = 0, false
	t.resetPending, t.resetSeq = false, 0
	t.waitACK, t.initialed, t.initHttpPacketDirect, t.isHttpRequestConn = false, false, false, false

	connectionPool.Put(t)
}

func (t *TrafficConnection) Write(b []byte, seq int64, ts time.Time) (int, error) {
	if ts.IsZero() {
		ts = time.Now()
	}
	frame := &TrafficFrame{ConnHash: t.hash, Seq: uint32(seq), Payload: b, Timestamp: ts, Connection: t}
	if !t.Flow.pool.options.Stream {
		// Timestamp bookkeeping only: retaining payload here kept every packet alive
		// even after its bytes had already been copied to the stream and frame.
		t.timestamps.add(len(b), ts)
		if _, err := t.writer.Write(b); err != nil {
			return 0, err
		}
	}
	t.Flow.onFrame(frame)
	if t.Flow.pool.counters != nil {
		t.Flow.pool.counters.delivered.Add(uint64(len(b)))
	}
	return len(b), nil
}

func seqBefore(a, b uint32) bool { return seqnum.Value(a).LessThan(seqnum.Value(b)) }

// queueFuture uses a typed min-heap: O(log n) insertion/removal without reflection
// or sorting the entire backlog on every packet. TCP comparisons wrap at 2^32.
func (t *TrafficConnection) queueFuture(seq uint32, payload []byte, fin bool, ts time.Time) {
	if old := t.pendingBySeq[seq]; old != nil {
		if !bytes.Equal(old.Payload[:min(len(old.Payload), len(payload))], payload[:min(len(old.Payload), len(payload))]) {
			t.Flow.pool.invalidSegment("conflicting TCP retransmission at the same sequence")
		}
		// First-seen bytes win; a longer retransmission may supply a missing suffix.
		if len(payload) <= len(old.Payload) {
			if fin && len(payload) == len(old.Payload) {
				old.FIN = true
			}
			return
		}
		extra := len(payload) - len(old.Payload)
		if t.pendingBytes+extra > t.Flow.pool.options.MaxPendingBytes || !t.Flow.pool.reservePending(extra, 0) {
			t.Flow.closeWithReason(TrafficFlowCloseReason_RESOURCE_LIMIT)
			return
		}
		old.Payload = append(old.Payload, payload[len(old.Payload):]...)
		old.FIN = fin
		t.pendingBytes += extra
		return
	}
	opts := &t.Flow.pool.options
	if len(t.waitGroup) >= opts.MaxPendingSegments || len(payload) > opts.MaxPendingBytes-t.pendingBytes || !t.Flow.pool.reservePending(len(payload), 1) {
		t.Flow.closeWithReason(TrafficFlowCloseReason_RESOURCE_LIMIT)
		return
	}
	f := &futureFrame{Seq: seq, Payload: bytes.Clone(payload), FIN: fin, Timestamp: ts}
	if t.pendingBySeq == nil {
		t.pendingBySeq = make(map[uint32]*futureFrame)
	}
	t.pendingBySeq[seq] = f
	t.pendingBytes += len(payload)
	t.waitGroup = append(t.waitGroup, f)
	i := len(t.waitGroup) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !seqBefore(f.Seq, t.waitGroup[parent].Seq) {
			break
		}
		t.waitGroup[i] = t.waitGroup[parent]
		i = parent
	}
	t.waitGroup[i] = f
}

func (t *TrafficConnection) popFuture() *futureFrame {
	f := t.waitGroup[0]
	n := len(t.waitGroup) - 1
	last := t.waitGroup[n]
	t.waitGroup[n] = nil
	t.waitGroup = t.waitGroup[:n]
	if n > 0 {
		i := 0
		for {
			child := i*2 + 1
			if child >= n {
				break
			}
			if child+1 < n && seqBefore(t.waitGroup[child+1].Seq, t.waitGroup[child].Seq) {
				child++
			}
			if !seqBefore(t.waitGroup[child].Seq, last.Seq) {
				break
			}
			t.waitGroup[i] = t.waitGroup[child]
			i = child
		}
		t.waitGroup[i] = last
	}
	delete(t.pendingBySeq, f.Seq)
	t.pendingBytes -= len(f.Payload)
	t.Flow.pool.releasePending(len(f.Payload), 1)
	return f
}

func (t *TrafficConnection) consume(seq uint32, payload []byte, fin bool, ts time.Time) {
	end := seq + uint32(len(payload))
	if seqBefore(seq, t.nextSeq) {
		trim := uint32(t.nextSeq - seq)
		if trim > uint32(len(payload)) {
			return
		}
		payload = payload[trim:]
		seq = t.nextSeq
	}
	// A FIN fixes the end of this direction, including when it was captured
	// before a missing prefix or before an overlapping retransmission.
	if t.finSeen && !seqBefore(end, t.finSeq) {
		if seqBefore(t.finSeq, seq) {
			return
		}
		payload = payload[:uint32(t.finSeq-seq)]
		end, fin = t.finSeq, true
	}
	if len(payload) > 0 {
		if len(t.waitGroup) > 0 && t.pendingConflict(0, seq, payload) {
			t.Flow.pool.invalidSegment("conflicting TCP overlap with buffered data")
		}
		t.currentSeq = seq
		t.nextSeq = end
		t.Write(payload, int64(seq), ts)
	}
	if fin && end == t.nextSeq {
		t.nextSeq++
		if t.resolvePendingReset() {
			return
		}
		t.Close()
		t.Flow.IsClosed()
	} else {
		t.resolvePendingReset()
	}
}

func (t *TrafficConnection) resolvePendingReset() bool {
	if !t.resetPending || seqBefore(t.nextSeq, t.resetSeq) {
		return false
	}
	t.resetPending = false
	if t.nextSeq == t.resetSeq {
		t.Flow.Close()
		return true
	}
	t.Flow.pool.invalidSegment("TCP RST sequence falls inside subsequently captured data")
	return false
}

// Heap ordering lets us prune complete subtrees starting beyond this segment.
// Stack depth is logarithmic in the bounded number of pending segments; a
// reverse-order, nonoverlapping stream only inspects the root per consume.
func (t *TrafficConnection) pendingConflict(i int, seq uint32, payload []byte) bool {
	if i >= len(t.waitGroup) {
		return false
	}
	f := t.waitGroup[i]
	end := seq + uint32(len(payload))
	if !seqBefore(f.Seq, end) {
		return false
	}
	start := f.Seq
	if seqBefore(start, seq) {
		start = seq
	}
	stop := f.Seq + uint32(len(f.Payload))
	if seqBefore(end, stop) {
		stop = end
	}
	if seqBefore(start, stop) && !bytes.Equal(payload[uint32(start-seq):uint32(stop-seq)], f.Payload[uint32(start-f.Seq):uint32(stop-f.Seq)]) {
		return true
	}
	return t.pendingConflict(2*i+1, seq, payload) || t.pendingConflict(2*i+2, seq, payload)
}

// FeedClient and FeedServer share the passive receive algorithm. Each direction
// can start midstream; SYN retransmits never reset an established cursor.
func (t *TrafficConnection) FeedClient(tcp *layers.TCP, ts time.Time) {
	if t.IsClosed() {
		// FIN closes one direction. A later in-sequence RST still closes the
		// whole flow, allowing a subsequent SYN to reuse this four-tuple.
		if t.Flow != nil && !t.Flow.stopped() && tcp.RST && !tcp.SYN && !tcp.FIN && t.initialed && tcp.Seq == t.nextSeq {
			t.Flow.Close()
		}
		return
	}
	if uint64(len(tcp.Payload)) > uint64(t.Flow.pool.options.MaxSequenceGap) {
		t.Flow.pool.invalidSegment("TCP payload exceeds the sequence window")
		return
	}
	if tcp.SYN && (tcp.FIN || tcp.RST) {
		t.Flow.pool.invalidSegment("TCP SYN combined with FIN or RST")
		return
	}
	if tcp.RST {
		if t.initialed && tcp.Seq != t.nextSeq {
			// A passive capture can expose RST before preceding data/FIN.
			// Retain at most one sequence (no payload allocation). Never advance
			// the cursor or accept the reset until observed contiguous bytes
			// reach it exactly. Missing gaps remain errors on flow disposal.
			distance := uint32(tcp.Seq - t.nextSeq)
			if distance < 1<<31 && uint64(distance) <= uint64(t.Flow.pool.options.MaxSequenceGap) && len(tcp.Payload) == 0 && !tcp.FIN && (!t.resetPending || t.resetSeq == tcp.Seq) {
				t.resetPending, t.resetSeq = true, tcp.Seq
				return
			}
			t.Flow.pool.invalidSegment(fmt.Sprintf("TCP RST does not match the receive cursor: %s seq=%d expected=%d ack=%d pending=%d", t, tcp.Seq, t.nextSeq, tcp.Ack, len(t.waitGroup)))
			return
		}
		t.Flow.Close()
		return
	}
	seq := tcp.Seq
	if tcp.SYN {
		if t.initialed && tcp.Seq != t.isn {
			t.Flow.pool.invalidSegment("TCP SYN changes an established initial sequence number")
			return
		}
		seq++
		if !t.initialed {
			t.initialed, t.isn, t.nextSeq = true, tcp.Seq, seq
		}
	}
	if !t.initialed {
		if len(tcp.Payload) == 0 && !tcp.FIN {
			return
		}
		t.initialed, t.isn, t.nextSeq = true, seq, seq
	}
	if len(tcp.Payload) == 0 && !tcp.FIN {
		return
	}
	distance := uint32(seq - t.nextSeq)
	if distance == 1<<31 ||
		(distance < 1<<31 && uint64(distance)+uint64(len(tcp.Payload)) > uint64(t.Flow.pool.options.MaxSequenceGap)) {
		t.Flow.pool.invalidSegment("TCP segment exceeds the unambiguous sequence window")
		return
	}
	end := seq + uint32(len(tcp.Payload))
	if tcp.FIN && !seqBefore(end, t.nextSeq) {
		if t.finSeen && end != t.finSeq {
			t.Flow.pool.invalidSegment("TCP FIN changes an established stream end")
			return
		}
		t.finSeen, t.finSeq = true, end
	}
	if t.finSeen && seqBefore(t.finSeq, seq) {
		return
	}
	if seqBefore(t.nextSeq, seq) {
		t.queueFuture(seq, tcp.Payload, tcp.FIN, ts)
		return
	}
	t.consume(seq, tcp.Payload, tcp.FIN, ts)
	for !t.IsClosed() && len(t.waitGroup) > 0 && !seqBefore(t.nextSeq, t.waitGroup[0].Seq) {
		f := t.popFuture()
		t.consume(f.Seq, f.Payload, f.FIN, f.Timestamp)
	}
	if len(t.waitGroup) == 0 {
		t.waitGroup = nil
		t.pendingBySeq = nil
	}
}

func (t *TrafficConnection) FeedServer(tcp *layers.TCP, ts time.Time) { t.FeedClient(tcp, ts) }

func (p *TrafficPool) NewFlow(netType string, srcAddr, dstAddr string) (*TrafficFlow, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	defer p.retireFlows()
	if p.closed {
		return nil, utils.Error("TCP traffic pool is closed")
	}
	if p.parallel != nil && p.owner == nil {
		return nil, utils.Error("NewFlow is unavailable on a multi-worker coordinator; submit packets with Feed")
	}
	return p.newFlow(netType, srcAddr, dstAddr)
}

func (p *TrafficPool) newFlow(netType string, srcAddr, dstAddr string) (*TrafficFlow, error) {
	dst, err := net.ResolveTCPAddr(netType, dstAddr)
	if err != nil {
		return nil, utils.Errorf("parse [%v] to addr failed: %s", dstAddr, err)
	}
	src, err := net.ResolveTCPAddr(netType, srcAddr)
	if err != nil {
		return nil, utils.Errorf("parse [%v] to addr failed: %s", srcAddr, err)
	}

	flow := p.newFlowWithAddrs(netType, src, dst)
	if flow == nil {
		return nil, utils.Error("TCP flow capacity exhausted")
	}
	// Public NewFlow preserves identifiers derived from its original strings,
	// including hostnames and noncanonical spellings.
	flow.Hash = p.flowhash(netType, srcAddr, dstAddr)
	return flow, nil
}

// Feed already has decoded endpoints. Resolve names only for public NewFlow.
func (p *TrafficPool) newFlowWithAddrs(netType string, src, dst *net.TCPAddr) *TrafficFlow {
	srcAddr, dstAddr := src.String(), dst.String()
	var clientReader, serverReader *tcpStreamBuffer
	var clientWriter, serverWriter *tcpStreamBuffer
	if !p.options.Stream {
		clientReader = newTCPStreamBuffer()
		clientWriter = clientReader
		serverReader = newTCPStreamBuffer()
		serverWriter = serverReader
	}

	c2sConn := connectionPool.Get().(*TrafficConnection)
	{
		c2sConn.reader = clientReader
		c2sConn.writer = clientWriter
		c2sConn.localAddr = src
		c2sConn.remoteAddr = dst
		c2sConn.localIP, c2sConn.remoteIP = src.IP, dst.IP
		c2sConn.localPort, c2sConn.remotePort = src.Port, dst.Port
		c2sConn.hash = codec.Sha256(srcAddr + " -> " + dstAddr)
	}

	s2cConn := connectionPool.Get().(*TrafficConnection)
	{
		s2cConn.reader = serverReader
		s2cConn.writer = serverWriter
		s2cConn.localAddr = dst
		s2cConn.remoteAddr = src
		s2cConn.localIP, s2cConn.remoteIP = dst.IP, src.IP
		s2cConn.localPort, s2cConn.remotePort = dst.Port, src.Port
		s2cConn.hash = codec.Sha256(dstAddr + " -> " + srcAddr)

	}

	// bind flow
	flow := flowPool.Get().(*TrafficFlow)
	{
		flow.ClientConn = c2sConn
		flow.ServerConn = s2cConn
		flow.Index = p.nextStream()
		flow.pool = p
		flow.Hash = p.flowhash(netType, srcAddr, dstAddr)
	}
	c2sConn.Flow = flow
	s2cConn.Flow = flow
	flow.key, _ = makeFlowKey(src.IP, dst.IP, uint16(src.Port), uint16(dst.Port), netType == "tcp6")
	if !p.flowCache.Set(flow.key, flow) {
		return nil
	}
	return flow
}

func (c *TrafficConnection) GetBuffer() io.Reader {
	if c.reader == nil {
		return bytes.NewReader(nil)
	}
	return c.reader
}

// Called by the pool while packet feeding is stopped/serialized.
func (t *TrafficConnection) discardPending() {
	p := t.Flow.pool
	if t.resetPending {
		p.invalidSegment(fmt.Sprintf("TCP RST does not match the receive cursor: %s seq=%d; preceding data/FIN was not captured", t, t.resetSeq))
		t.resetPending = false
	}
	if len(t.waitGroup) > 0 && !(t.finSeen && t.nextSeq == t.finSeq+1) {
		if p.counters != nil {
			p.counters.unreassembledBytes.Add(uint64(t.pendingBytes))
			p.counters.unreassembledSegments.Add(uint64(len(t.waitGroup)))
		} else {
			p.singleUnreassembledBytes.Add(uint64(t.pendingBytes))
			p.singleUnreassembledSegments.Add(uint64(len(t.waitGroup)))
		}
		reason := fmt.Sprintf("TCP stream closed with an unfilled sequence gap (%d buffered bytes)", t.pendingBytes)
		p.reassemblyFailure(reason)
		if p.captureConf != nil && p.captureConf.binParser != nil {
			if t.Flow.binState == nil {
				t.Flow.binState = p.captureConf.binParser.newFlow(t.Flow)
			}
			f := t.Flow.binState
			dir := 0
			if t != t.Flow.ClientConn {
				dir = 1
			}
			e := f.event(dir, nil, "incomplete", reason)
			e.Timestamp = t.waitGroup[0].Timestamp
			e.Length = t.pendingBytes
			f.a.incomplete.Add(1)
			f.a.emit(e)
		}
	}
	p.releasePending(t.pendingBytes, len(t.waitGroup))
	t.pendingBytes = 0
	t.waitGroup = nil
	t.pendingBySeq = nil
}
