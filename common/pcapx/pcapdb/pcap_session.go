package pcapdb

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"net"
	"sort"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

const streamChunkBytes = 64 << 10
const maxActiveSessions = 4096

// Only live TCP flows and a bounded UDP LRU stay in memory. Closed summaries,
// associations and chunks are flushed in the same transaction as the manifest.
type sessionState struct {
	session PCAPSession
	streams [2]PCAPStream
	last    time.Time
}
type packetSession struct{ session, stream uint }
type udpSession struct {
	key   string
	state *sessionState
}
type sessionIndexer struct {
	meta         *PCAPFileDBMetadata
	tcp          map[*pcaputil.TrafficFlow]*sessionState
	udp          map[string]*list.Element
	lru          *list.List
	dirty        map[uint]*sessionState
	packets      []PCAPSessionPacket
	packetLookup map[uint64]packetSession
	chunks       []PCAPStreamChunk
	chunkPackets []PCAPStreamChunkPacket
	hasher       hash.Hash
	bytes        int
}

func newSessionIndexer(meta *PCAPFileDBMetadata) *sessionIndexer {
	return &sessionIndexer{meta: meta, hasher: sha256.New(), tcp: make(map[*pcaputil.TrafficFlow]*sessionState), udp: make(map[string]*list.Element), lru: list.New(), dirty: make(map[uint]*sessionState), packetLookup: make(map[uint64]packetSession)}
}

func (s *sessionIndexer) newSession(kind string, domain pcaputil.CaptureDomain, srcIP, dstIP string, srcPort, dstPort int, midstream bool) *sessionState {
	s.meta.SessionCount++
	id := uint(s.meta.SessionCount)
	state := &sessionState{session: PCAPSession{Model: gorm.Model{ID: id, CreatedAt: time.Now().UTC()}, Transport: kind, Section: int64(domain.Section), Interface: domain.Interface, Encapsulation: domain.Encapsulation, SourceIP: srcIP, DestinationIP: dstIP, SourcePort: srcPort, DestinationPort: dstPort, Midstream: midstream, CloseReason: "open"}}
	for direction := 0; direction < 2; direction++ {
		s.meta.StreamCount++
		streamKind := kind
		if kind == "udp" {
			streamKind = "datagram"
		}
		state.streams[direction] = PCAPStream{Model: gorm.Model{ID: uint(s.meta.StreamCount), CreatedAt: time.Now().UTC()}, SessionID: id, Direction: direction, Kind: streamKind}
	}
	s.dirty[id] = state
	return state
}
func (s *sessionIndexer) tcpPacket(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, ts time.Time) error {
	state := s.tcp[flow]
	if state == nil {
		state = s.newSession("tcp", flow.CaptureDomain(), flow.ClientConn.LocalIP().String(), flow.ClientConn.RemoteIP().String(), flow.ClientConn.LocalPort(), flow.ClientConn.RemotePort(), flow.IsHalfOpen)
		s.tcp[flow] = state
	}
	direction := 0
	if conn != flow.ClientConn {
		direction = 1
	}
	return s.packet(state, direction, refs, ts)
}
func (s *sessionIndexer) packet(state *sessionState, direction int, refs []pcaputil.PacketReference, ts time.Time) error {
	stamp := timestampNano(ts)
	if state.session.FirstTimestampNS == nil || stamp != nil && *stamp < *state.session.FirstTimestampNS {
		state.session.FirstTimestampNS = stamp
	}
	if state.session.LastTimestampNS == nil || stamp != nil && *stamp > *state.session.LastTimestampNS {
		state.session.LastTimestampNS = stamp
	}
	state.last = ts
	for _, ref := range refs {
		if ref.Number == 0 || ref.Number > uint64(s.meta.PacketCount) {
			return fmt.Errorf("pcapdb: session refers to unknown packet %d", ref.Number)
		}
		if _, exists := s.packetLookup[ref.Number]; exists {
			continue
		}
		pair := packetSession{state.session.ID, state.streams[direction].ID}
		s.packets = append(s.packets, PCAPSessionPacket{SessionID: pair.session, StreamID: pair.stream, PacketID: uint(ref.Number)})
		s.packetLookup[ref.Number] = pair
		state.session.PacketCount++
	}
	s.dirty[state.session.ID] = state
	return nil
}
func (s *sessionIndexer) tcpData(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) error {
	state := s.tcp[flow]
	if state == nil {
		return fmt.Errorf("pcapdb: reassembled data has no session")
	}
	direction := 0
	if conn != flow.ClientConn {
		direction = 1
	}
	return s.data(state, direction, frame.Payload, frame.Seq, frame.Timestamp, frame.PacketReferences())
}
func (s *sessionIndexer) data(state *sessionState, direction int, data []byte, sequence uint32, ts time.Time, refs []pcaputil.PacketReference) error {
	stream := &state.streams[direction]
	for len(data) > 0 {
		n := min(len(data), streamChunkBytes)
		s.meta.StreamChunkCount++
		id := uint(s.meta.StreamChunkCount)
		s.chunks = append(s.chunks, PCAPStreamChunk{Model: gorm.Model{ID: id}, StreamID: stream.ID, ByteOffset: stream.ByteCount, Length: n, Sequence: sequence, TimestampNS: timestampNano(ts), ReferencesComplete: len(refs) > 0, Data: append([]byte{}, data[:n]...)})
		seen := make(map[uint64]bool, len(refs))
		for _, ref := range refs {
			if ref.Number == 0 || ref.Number > uint64(s.meta.PacketCount) {
				return fmt.Errorf("pcapdb: stream refers to unknown packet %d", ref.Number)
			}
			if !seen[ref.Number] {
				s.chunkPackets = append(s.chunkPackets, PCAPStreamChunkPacket{ChunkID: id, PacketID: uint(ref.Number)})
				seen[ref.Number] = true
			}
		}
		s.hasher.Write(data[:n])
		stream.ByteCount += int64(n)
		stream.ChunkCount++
		state.session.ByteCount += int64(n)
		s.meta.StreamDataSize += int64(n)
		s.bytes += n
		data = data[n:]
		sequence += uint32(n)
	}
	s.dirty[state.session.ID] = state
	return nil
}
func (s *sessionIndexer) closeTCP(reason pcaputil.TrafficFlowCloseReason, flow *pcaputil.TrafficFlow) {
	state := s.tcp[flow]
	if state == nil {
		return
	}
	state.session.HasGaps = flow.HasPendingGaps()
	state.session.CloseReason = string(reason)
	if reason == pcaputil.TrafficFlowCloseReason_CTX_CANCEL {
		state.session.CloseReason = "capture-end"
	}
	state.session.Complete = reason == pcaputil.TrafficFlowCloseReason_FIN && !state.session.Midstream && !state.session.HasGaps
	s.dirty[state.session.ID] = state
	delete(s.tcp, flow)
}
func (s *sessionIndexer) datagram(packet gopacket.Packet) error {
	// Use innermost decoded network/transport endpoints, as reassembly does.
	var srcIP, dstIP string
	var udp *layers.UDP
	for _, layer := range packet.Layers() {
		if network, ok := layer.(gopacket.NetworkLayer); ok {
			srcIP, dstIP = network.NetworkFlow().Src().String(), network.NetworkFlow().Dst().String()
		}
		switch transport := layer.(type) {
		case *layers.UDP:
			udp = transport
		case *layers.TCP:
			udp = nil
		}
	}
	if udp == nil || srcIP == "" {
		return nil
	}
	refs := pcaputil.CapturePacketReferences(packet)
	if len(refs) == 0 {
		return nil
	}
	ref := refs[0]
	a := net.JoinHostPort(srcIP, fmt.Sprint(udp.SrcPort))
	b := net.JoinHostPort(dstIP, fmt.Sprint(udp.DstPort))
	if a > b {
		a, b = b, a
	}
	key := fmt.Sprintf("%d/%d/%s/%s/%s", ref.Domain.Section, ref.Domain.Interface, ref.Domain.Encapsulation, a, b)
	ts := packet.Metadata().Timestamp
	element := s.udp[key]
	if element != nil && ts.Sub(element.Value.(*udpSession).state.last) > 30*time.Second {
		s.closeUDP(element, "idle")
		element = nil
	}
	if element == nil {
		if s.lru.Len() >= maxActiveSessions {
			s.closeUDP(s.lru.Back(), "resource-limit")
		}
		state := s.newSession("udp", ref.Domain, srcIP, dstIP, int(udp.SrcPort), int(udp.DstPort), false)
		element = s.lru.PushFront(&udpSession{key: key, state: state})
		s.udp[key] = element
	} else {
		s.lru.MoveToFront(element)
	}
	state := element.Value.(*udpSession).state
	if packet.Metadata().Truncated || int(udp.Length) > len(udp.Payload)+8 {
		state.session.HasGaps = true
	}
	direction := 0
	if state.session.SourceIP != srcIP || state.session.SourcePort != int(udp.SrcPort) {
		direction = 1
	}
	if err := s.packet(state, direction, refs, ts); err != nil {
		return err
	}
	return s.data(state, direction, udp.Payload, 0, ts, refs)
}
func (s *sessionIndexer) closeUDP(element *list.Element, reason string) {
	entry := element.Value.(*udpSession)
	entry.state.session.CloseReason = reason
	entry.state.session.Complete = !entry.state.session.HasGaps // completeness is per datagram, not a TCP handshake
	s.dirty[entry.state.session.ID] = entry.state
	delete(s.udp, entry.key)
	s.lru.Remove(element)
}
func (s *sessionIndexer) digest() string { return hex.EncodeToString(s.hasher.Sum(nil)) }
func (s *sessionIndexer) finish() {
	for s.lru.Len() > 0 {
		s.closeUDP(s.lru.Back(), "capture-end")
	}
}
func (s *sessionIndexer) pending() int { return len(s.packets) + len(s.chunks) + len(s.dirty) }
func (s *sessionIndexer) flush(ctx context.Context, db *gorm.DB) error {
	// Sort IDs so insertion/update order is deterministic and page friendly.
	ids := make([]uint, 0, len(s.dirty))
	for id := range s.dirty {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	summaries := make([]PCAPSession, 0, len(ids))
	streams := make([]PCAPStream, 0, len(ids)*2)
	for _, id := range ids {
		state := s.dirty[id]
		summaries = append(summaries, state.session)
		streams = append(streams, state.streams[:]...)
	}
	if err := indexWithContext(ctx, db).OnConflictDoUpdate("id", "updated_at", "first_timestamp_ns", "last_timestamp_ns", "packet_count", "byte_count", "has_gaps", "complete", "close_reason").CreateInBatches(&summaries).Error; err != nil {
		return err
	}
	if err := indexWithContext(ctx, db).OnConflictDoUpdate("id", "updated_at", "byte_count", "chunk_count").CreateInBatches(&streams).Error; err != nil {
		return err
	}
	for _, rows := range []interface{}{&s.packets, &s.chunks, &s.chunkPackets} {
		if err := db.CreateInBatches(rows).Error; err != nil {
			return err
		}
	}
	return ctx.Err()
}
func (s *sessionIndexer) clearBatch() {
	clear(s.dirty)
	clear(s.packetLookup)
	clear(s.packets)
	s.packets = s.packets[:0]
	clear(s.chunks)
	s.chunks = s.chunks[:0]
	clear(s.chunkPackets)
	s.chunkPackets = s.chunkPackets[:0]
	s.bytes = 0
}
func (s *sessionIndexer) association(ctx context.Context, db *gorm.DB, event *pcaputil.ProtocolEvent) (uint, uint, error) {
	for _, ref := range event.SourceBytes.PacketRefs {
		pair, exists := s.packetLookup[ref.Number]
		if !exists {
			var record PCAPSessionPacket
			err := indexWithContext(ctx, db).Where("packet_id = ?", ref.Number).Order("id").First(&record).Error
			if gorm.IsRecordNotFoundError(err) {
				continue
			}
			if err != nil {
				return 0, 0, err
			}
			pair = packetSession{record.SessionID, record.StreamID}
		}
		return pair.session, pair.stream, nil
	}
	return 0, 0, nil
}

func validateStreamContent(ctx context.Context, db *gorm.DB) error {
	scoped := indexWithContext(ctx, db)
	var bad int64
	if err := scoped.Model(&PCAPStream{}).Where("byte_count < 0 OR chunk_count < 0 OR direction NOT IN (0,1) OR byte_count != (SELECT COALESCE(sum(length),0) FROM stream_chunks WHERE stream_id=streams.id AND deleted_at IS NULL) OR chunk_count != (SELECT count(*) FROM stream_chunks WHERE stream_id=streams.id AND deleted_at IS NULL)").Count(&bad).Error; err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("pcapdb: stream summaries disagree with chunks")
	}
	// Neighbor lookup uses the unique stream/offset index; no quadratic scan.
	if err := scoped.Model(&PCAPStreamChunk{}).Where("byte_offset != 0 AND byte_offset != COALESCE((SELECT previous.byte_offset+previous.length FROM stream_chunks AS previous WHERE previous.stream_id=stream_chunks.stream_id AND previous.byte_offset < stream_chunks.byte_offset AND previous.deleted_at IS NULL ORDER BY previous.byte_offset DESC LIMIT 1),-1)").Count(&bad).Error; err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("pcapdb: stream chunk offsets are discontinuous")
	}
	if err := scoped.Model(&PCAPSessionPacket{}).Joins("JOIN streams AS st ON st.id=session_packets.stream_id").Where("st.session_id != session_packets.session_id").Count(&bad).Error; err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("pcapdb: stream/session packet associations disagree")
	}
	if err := scoped.Model(&PCAPSession{}).Where("packet_count != (SELECT count(*) FROM session_packets WHERE session_id=sessions.id AND deleted_at IS NULL) OR byte_count != (SELECT COALESCE(sum(byte_count),0) FROM streams WHERE session_id=sessions.id AND deleted_at IS NULL) OR 2 != (SELECT count(*) FROM streams WHERE session_id=sessions.id AND deleted_at IS NULL)").Count(&bad).Error; err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("pcapdb: session summaries disagree with associated packets/streams")
	}
	if err := scoped.Model(&PCAPProtocolMessage{}).Joins("LEFT JOIN streams AS st ON st.id=protocol_messages.stream_id").Where("protocol_messages.stream_id IS NOT NULL AND (protocol_messages.session_id IS NULL OR st.session_id != protocol_messages.session_id)").Count(&bad).Error; err != nil {
		return err
	}
	if bad != 0 {
		return fmt.Errorf("pcapdb: protocol session/stream associations disagree")
	}
	return nil
}
