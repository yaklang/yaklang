package sharkcli

import (
	"container/list"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/reassembly"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const (
	defaultStreamBytes = 512 << 10
	streamMemoryLimit  = 32 << 20
	streamPrefixLimit  = 16 << 10
)

type streamKey struct {
	network, transport gopacket.Flow
	domain             pcaputil.CaptureDomain
}

func connectionKey(network, transport gopacket.Flow) streamKey {
	if network.Src().LessThan(network.Dst()) || network.Src() == network.Dst() && transport.Src().LessThan(transport.Dst()) {
		return streamKey{network: network, transport: transport}
	}
	return streamKey{network: network.Reverse(), transport: transport.Reverse()}
}

type streamChunk struct {
	pdu       uint64
	direction int
	offset    uint64
	gap       int // -1: capture starts midstream; >0: missing sequence bytes
	data      []byte
}
type streamSide struct {
	prefixTime       time.Time
	startKnown       bool
	endpoint         string
	port             uint16
	bytes            uint64
	prefix           []byte
	broken, syn, fin bool
	synSeq           uint32
}
type tcpStream struct {
	nativeFlowID                                    uint64
	protocolStack                                   []string
	nativeIdentity                                  bool
	firstReference, lastReference                   uint64
	acceptedPacketNumber                            uint64
	lastTLSRecordID                                 uint64
	tlsRecordCount                                  uint8
	sshEncrypted                                    [2]bool
	first                                           time.Time
	store                                           *streamStore
	id                                              uint64
	key                                             streamKey
	lru                                             *list.Element
	sides                                           [2]streamSide
	chunks                                          []streamChunk
	protocol, evidence, closed                      string
	decodeDiagnostic                                string
	packets, retransmits, outOfOrder, gaps, missing uint64
	retained, omitted                               int
	version                                         uint64
	last                                            time.Time
	evicted                                         bool
}

// All assembly happens before the lossy display queue. The UI only reads copied
// snapshots, so display skips and a pinned packet list cannot lose stream bytes.
type streamStore struct {
	captureReplay                   bool
	protocolConversations           map[protocolConversationKey]*list.Element
	protocolConversationLRU         list.List
	history                         *pcaputil.ProtocolInspector
	mu                              sync.RWMutex
	assembler                       *reassembly.Assembler
	assemblers                      map[pcaputil.CaptureDomain]*reassembly.Assembler
	currentDomain                   pcaputil.CaptureDomain
	active                          map[streamKey]*tcpStream
	records                         map[uint64]*tcpStream
	lru                             list.List
	next                            uint64
	retained, maxStreams, perStream int
	clock, lastFlush                time.Time
	flushReason                     string
}

func newStreamStore(maxStreams, perStream int) *streamStore {
	if maxStreams == 0 {
		maxStreams = 256
	}
	if perStream == 0 {
		perStream = defaultStreamBytes
	}
	s := &streamStore{active: map[streamKey]*tcpStream{}, records: map[uint64]*tcpStream{}, maxStreams: maxStreams, perStream: perStream}
	s.assembler = reassembly.NewAssembler(reassembly.NewStreamPool(s))
	s.assemblers = map[pcaputil.CaptureDomain]*reassembly.Assembler{{}: s.assembler}
	s.assembler.MaxBufferedPagesTotal = 128
	s.assembler.MaxBufferedPagesPerConnection = 128
	return s
}

type streamContext struct {
	ci     gopacket.CaptureInfo
	number uint64
}

func (c streamContext) GetCaptureInfo() gopacket.CaptureInfo { return c.ci }

func (s *streamStore) New(network, transport gopacket.Flow, tcp *layers.TCP, ac reassembly.AssemblerContext) reassembly.Stream {
	s.next++
	r := &tcpStream{store: s, id: s.next, key: connectionKey(network, transport)}
	r.key.domain = s.currentDomain
	r.sides[0] = streamSide{endpoint: net.JoinHostPort(network.Src().String(), transport.Src().String()), port: uint16(tcp.SrcPort)}
	r.sides[1] = streamSide{endpoint: net.JoinHostPort(network.Dst().String(), transport.Dst().String()), port: uint16(tcp.DstPort)}
	r.protocol = portProtocol("tcp", uint16(tcp.SrcPort), uint16(tcp.DstPort))
	if r.protocol != "" {
		r.evidence = "port hint"
	}
	r.lru = s.lru.PushBack(r)
	s.active[r.key], s.records[r.id] = r, r
	return r
}

func directionIndex(d reassembly.TCPFlowDirection) int {
	if d == reassembly.TCPDirServerToClient {
		return 1
	}
	return 0
}
func (r *tcpStream) Accept(tcp *layers.TCP, ci gopacket.CaptureInfo, dir reassembly.TCPFlowDirection, next reassembly.Sequence, start *bool, ac reassembly.AssemblerContext) bool {
	if c, ok := ac.(streamContext); ok {
		r.acceptedPacketNumber = c.number
	}
	d := directionIndex(dir)
	if tcp.SYN {
		r.sides[d].syn, r.sides[d].synSeq = true, tcp.Seq
	}
	if tcp.FIN {
		r.sides[d].fin = true
	}
	if tcp.RST {
		r.closed = "RST"
	}
	r.packets++
	r.version++
	if r.first.IsZero() {
		r.first = ci.Timestamp
	}
	if ci.Timestamp.Before(r.first) {
		r.first = ci.Timestamp
	}
	if ci.Timestamp.After(r.last) {
		r.last = ci.Timestamp
	}
	if next >= 0 && len(tcp.Payload) > 0 {
		diff := int32(tcp.Seq - uint32(next))
		if diff < 0 {
			r.retransmits++
		} else if diff > 0 {
			r.outOfOrder++
		}
	}
	// Without SYN, let the reorder queue collect earlier segments until its
	// timeout/EOF; forcing the first observed segment would discard late prefixes.
	return true
}

func (r *tcpStream) ReassembledSG(sg reassembly.ScatterGather, ac reassembly.AssemblerContext) {
	if r.evicted {
		return
	}
	dir, _, _, skip := sg.Info()
	d := directionIndex(dir)
	n, _ := sg.Lengths()
	if n == 0 && skip == 0 {
		return
	}
	side := &r.sides[d]
	if skip != 0 {
		r.gaps++
		if skip > 0 {
			r.missing += uint64(skip)
			side.bytes += uint64(skip)
			side.broken = true
		}
	}
	var data []byte
	if n > 0 {
		data = sg.Fetch(n)
	}
	if !side.broken && len(side.prefix) < streamPrefixLimit {
		if len(side.prefix) == 0 && n > 0 {
			side.prefixTime = sg.CaptureInfo(0).Timestamp
			side.startKnown = side.syn && skip == 0 && side.bytes == 0
		}
		side.prefix = append(side.prefix, data[:min(n, streamPrefixLimit-len(side.prefix))]...)
		if protocol := sniffStreamPayload(side.prefix, side.port, r.sides[1-d].port); protocol != "" && !r.nativeIdentity && r.store.history == nil {
			if r.protocol != protocol || r.evidence != "bin-parser + verified prefix" {
				r.protocol, r.evidence = protocol, "verified prefix"
			}
		}
	}
	offset := side.bytes
	side.bytes += uint64(n)
	// Keep the recent stream window; offsets and an explicit omission count
	// distinguish a bounded preview from a complete conversation.
	if n > r.store.perStream {
		r.omitted += n - r.store.perStream
		offset += uint64(n - r.store.perStream)
		data = data[n-r.store.perStream:]
	}
	for len(r.chunks) > 0 && (r.retained+len(data) > r.store.perStream || len(r.chunks) >= 4096) {
		old := len(r.chunks[0].data)
		r.retained -= old
		r.store.retained -= old
		r.omitted += old
		r.chunks[0] = streamChunk{}
		r.chunks = r.chunks[1:]
	}
	if len(r.chunks) > 0 {
		last := &r.chunks[len(r.chunks)-1]
		if skip == 0 && last.direction == d && len(last.data)+len(data) <= 16<<10 {
			last.data = append(last.data, data...)
		} else {
			r.chunks = append(r.chunks, streamChunk{direction: d, offset: offset, gap: skip, data: append([]byte(nil), data...)})
		}
	} else {
		r.chunks = append(r.chunks, streamChunk{direction: d, offset: offset, gap: skip, data: append([]byte(nil), data...)})
	}
	r.retained += len(data)
	r.store.retained += len(data)
	r.version++
}

func (r *tcpStream) ReassemblyComplete(reassembly.AssemblerContext) bool {
	if r.closed == "" {
		r.closed = r.store.flushReason
		if r.closed == "" {
			r.closed = "FIN"
		}
	}
	delete(r.store.active, r.key)
	r.version++
	return true
}

func (s *streamStore) add(raw *capturedPacket) {
	p := gopacket.NewPacket(raw.data, pcaputil.CaptureLinkDecoder(raw.link), gopacket.DecodeOptions{Lazy: true, NoCopy: true, DecodeStreamsAsDatagrams: false})
	s.addDecoded(raw, p, pcaputil.PacketDomain(raw.data, raw.ci, raw.link))
}

func (s *streamStore) addDecoded(raw *capturedPacket, p gopacket.Packet, domain pcaputil.CaptureDomain) {
	// Capture-backed previews require an actual core flow association. A
	// malformed or orphan segment must not allocate an independent app stream.
	if raw.history != nil && raw.tcpFlowID == 0 {
		return
	}
	network, selectedTransport := pcaputil.CaptureTransport(p)
	tcp, ok := selectedTransport.(*layers.TCP)
	if network == nil || !ok {
		return
	}
	// gopacket can publish a partially initialized TCP layer before reporting
	// a short header. Such endpoints cannot enter the assembler (their port
	// formatters require two captured bytes).
	if p.ErrorLayer() != nil || len(tcp.LayerContents()) < 20 || tcp.DataOffset < 5 {
		return
	}
	key := connectionKey(network.NetworkFlow(), tcp.TransportFlow())
	key.domain = domain
	s.mu.Lock()
	defer s.mu.Unlock()
	if raw.tcpFlowID != 0 && !tcp.SYN && len(tcp.Payload) == 0 && s.active[key] == nil {
		for e := s.lru.Back(); e != nil; e = e.Prev() {
			r := e.Value.(*tcpStream)
			if r.nativeFlowID == raw.tcpFlowID && r.key == key && r.closed != "" && !r.evicted {
				r.packets++
				r.version++
				s.attachPacket(raw, r)
				return
			}
		}
	}
	if r := s.active[key]; r != nil && r.closed == "RST" && !(tcp.SYN && !tcp.ACK) {
		// Without a new SYN, late data and a new generation are ambiguous.
		// Keep these packet rows unassociated instead of inheriting the old
		// TLS/application identity or joining their bytes to a reset stream.
		return
	}
	s.currentDomain = key.domain
	assembler := s.assemblers[key.domain]
	if assembler == nil {
		if len(s.assemblers) >= 32 {
			s.flushReason = "capture domain budget eviction"
			for k, a := range s.assemblers {
				a.FlushAll()
				delete(s.assemblers, k)
			}
			s.flushReason = ""
		}
		assembler = reassembly.NewAssembler(reassembly.NewStreamPool(s))
		assembler.MaxBufferedPagesTotal = 128
		assembler.MaxBufferedPagesPerConnection = 128
		s.assemblers[key.domain] = assembler
	}
	// The assembler owns connection lifetimes. Bound active state independently
	// of the history, and distinguish a reused tuple's new SYN/ISN.
	reused := false
	if r := s.active[key]; r != nil && tcp.SYN && !tcp.ACK {
		sender := net.JoinHostPort(network.NetworkFlow().Src().String(), fmt.Sprint(uint16(tcp.SrcPort)))
		for _, side := range r.sides {
			if side.endpoint == sender && (side.syn && side.synSeq != tcp.Seq || !side.syn && r.nativeIdentity) {
				if raw.tcpFlowID != 0 && r.nativeFlowID == raw.tcpFlowID {
					// The core rejected a conflicting SYN in this generation.
					// Do not reset the preview or its unrelated connections.
					s.attachPacket(raw, r)
					return
				}
				reused = true
			}
		}
	}
	if reused || s.active[key] == nil && len(s.active) >= s.maxStreams {
		s.flushReason = "assembly window reset"
		for _, a := range s.assemblers {
			a.FlushAll()
		}
		s.flushReason = ""
	}
	assembler.AssembleWithContext(network.NetworkFlow(), tcp, streamContext{ci: raw.ci, number: raw.number})
	// A FIN/RST may close the stream during this call, so use the latest record
	// for this key if it is no longer in the active map.
	r := s.active[key]
	if r == nil {
		for e := s.lru.Back(); e != nil; e = e.Prev() {
			if candidate := e.Value.(*tcpStream); candidate.key == key && candidate.acceptedPacketNumber == raw.number {
				r = candidate
				break
			}
		}
	}
	if r != nil {
		r.nativeFlowID = raw.tcpFlowID
		s.attachPacket(raw, r)
	}
	s.flushLocked(raw.ci.Timestamp)
	s.trim()
}

func (s *streamStore) attachPacket(raw *capturedPacket, r *tcpStream) {
	ref := raw.reference.Number
	if ref == 0 {
		ref = raw.number
	}
	if r.firstReference == 0 {
		r.firstReference = ref
	}
	r.lastReference = ref
	raw.streamID = r.id
	raw.application, raw.evidence = r.protocol, r.evidence
	if r.lru != nil {
		s.lru.MoveToBack(r.lru)
	}
}

func (s *streamStore) trim() {
	for len(s.records) > s.maxStreams || s.retained > streamMemoryLimit {
		e := s.lru.Front()
		if e == nil {
			break
		}
		r := e.Value.(*tcpStream)
		// Active streams keep sequence state only until their normal close; an
		// evicted conversation is explicitly unavailable, never silently spliced.
		s.retained -= r.retained
		r.omitted += r.retained
		r.retained = 0
		r.chunks = nil
		r.evicted = true
		for d := range r.sides {
			r.sides[d].prefix = nil
		}
		delete(s.records, r.id)
		s.lru.Remove(e)
		r.lru = nil
	}
}

func (s *streamStore) flushLocked(now time.Time) {
	if s.captureReplay {
		// A file's unrelated interfaces can have different capture epochs. Do
		// not turn that clock jump or a processing pause into preview gaps or
		// a peer close. EOF, actual controls and the existing page/stream/byte
		// budgets bound replay; native protocol messages remain authoritative.
		return
	}
	if now.After(s.clock) {
		s.clock = now
	}
	if s.clock.Sub(s.lastFlush) < time.Second {
		return
	}
	s.lastFlush = s.clock
	s.flushReason = "idle timeout"
	for _, a := range s.assemblers {
		a.FlushWithOptions(reassembly.FlushOptions{T: s.clock.Add(-2 * time.Second), TC: s.clock.Add(-2 * time.Minute)})
	}
	s.flushReason = ""
}
func (s *streamStore) tick(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked(now)
	s.trim()
}
func (s *streamStore) finish() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushReason = "capture ended"
	for _, a := range s.assemblers {
		a.FlushAll()
	}
	s.flushReason = ""
	s.trim()
}
func (s *streamStore) label(id uint64) (string, string) {
	if s == nil || id == 0 {
		return "", ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r := s.records[id]; r != nil {
		return r.protocol, r.evidence
	}
	return "", ""
}

type streamSnapshot struct {
	associationProofs                               []*pcaputil.ProtocolEvent
	associationUnavailable                          []uint64
	associationLimited                              bool
	retentionNotice                                 string
	transport                                       string
	protocolStack                                   []string
	domain                                          pcaputil.CaptureDomain
	first, last                                     time.Time
	events                                          []*pcaputil.ProtocolEvent
	startKnown                                      [2]bool
	prefixGap                                       [2]bool
	prefixTime                                      [2]time.Time
	decodeVersion                                   uint64
	id, version                                     uint64
	endpoints                                       [2]string
	ports                                           [2]uint16
	prefix                                          [2][]byte
	chunks                                          []streamChunk
	protocol, evidence, closed                      string
	packets, retransmits, outOfOrder, gaps, missing uint64
	bytes                                           [2]uint64
	omitted                                         int
}

func (s *streamStore) snapshot(id uint64) *streamSnapshot {
	if s == nil || id == 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := s.records[id]
	if r == nil {
		return nil
	}
	v := &streamSnapshot{first: r.first, last: r.last, id: r.id, version: r.version, protocol: r.protocol, evidence: r.evidence, closed: r.closed, packets: r.packets, retransmits: r.retransmits, outOfOrder: r.outOfOrder, gaps: r.gaps, missing: r.missing, omitted: r.omitted}
	v.protocolStack = append([]string(nil), r.protocolStack...)
	for d, side := range r.sides {
		v.startKnown[d], v.prefixGap[d], v.prefixTime[d] = side.startKnown, side.broken, side.prefixTime
		v.endpoints[d] = side.endpoint
		v.ports[d] = side.port
		v.bytes[d] = side.bytes
		v.prefix[d] = append([]byte(nil), side.prefix...)
	}
	v.domain = r.key.domain
	v.events = retainedStreamEvents(s.history, v)
	v.decodeVersion = r.version
	for _, chunk := range r.chunks {
		c := chunk
		c.data = append([]byte(nil), c.data...)
		v.chunks = append(v.chunks, c)
	}
	return v
}
func (v *streamSnapshot) description() string {
	if v.transport == "network" {
		return fmt.Sprintf("Association #%d · IP › %s · retained messages", v.id & ^(uint64(1)<<63), strings.Join(v.protocolStack, " › "))
	}
	if v.transport == "udp" {
		return fmt.Sprintf("Conversation #%d · UDP › %s · retained datagrams", v.id & ^(uint64(1)<<63), strings.Join(v.protocolStack, " › "))
	}
	if len(v.protocolStack) > 0 {
		return fmt.Sprintf("Stream #%d · TCP › %s · observed connection", v.id, strings.Join(v.protocolStack, " › "))
	}
	if p := presentEvents(v.events); p.protocol != "" {
		return fmt.Sprintf("Stream #%d · TCP › %s · parsed messages", v.id, strings.Join(p.stack, " › "))
	}
	protocol := v.protocol
	if protocol == "" {
		protocol = "TCP / unidentified application"
	}
	if v.evidence == "port hint" {
		return fmt.Sprintf("Stream #%d · TCP · unverified port hint: %s", v.id, protocol)
	}
	return fmt.Sprintf("Stream #%d · prefix protocol: %s · %s", v.id, protocol, v.evidence)
}

func (v *streamSnapshot) messageBoundaries() bool {
	return v.transport == "udp" || v.transport == "network"
}

func (s *streamStore) version(id uint64) uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if r := s.records[id]; r != nil {
		return r.version
	}
	return 0
}

func (s *streamStore) identify(id uint64, protocol string) bool {
	if s == nil || protocol == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.records[id]; r != nil {
		if r.nativeIdentity {
			return false
		}
		verified := false
		for d, side := range r.sides {
			if sniffStreamPayload(side.prefix, side.port, r.sides[1-d].port) == protocol {
				verified = true
			}
		}
		// A worker result is not proof of application identity. Recheck the
		// retained bytes; especially never promote permissive HTTP/YAML parses.
		if !verified {
			return false
		}
		r.protocol, r.evidence = protocol, "bin-parser + verified prefix"
		r.version++
		return true
	}
	return false
}
