package sharkcli

import (
	"time"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

type nativeStreamKey struct {
	domain pcaputil.CaptureDomain
	flow   uint64
}

func nativeKey(flow *pcaputil.TrafficFlow) nativeStreamKey {
	return nativeStreamKey{domain: flow.CaptureDomain(), flow: flow.Index}
}

// Native callbacks copy bounded observations only. No pooled flow/connection,
// borrowed packet, or independent sequence queue is retained by the page.
func (s *streamStore) nativeCreated(flow *pcaputil.TrafficFlow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nativeRecords == nil {
		s.nativeRecords = make(map[nativeStreamKey]*tcpStream)
	}
	key := nativeKey(flow)
	if s.nativeRecords[key] != nil {
		return
	}
	s.next++
	r := &tcpStream{store: s, id: s.next, nativeFlowID: flow.Index}
	r.key.domain = key.domain
	r.sides[0] = streamSide{endpoint: flow.ClientConn.LocalAddr().String(), port: uint16(flow.ClientConn.LocalPort())}
	r.sides[1] = streamSide{endpoint: flow.ServerConn.LocalAddr().String(), port: uint16(flow.ServerConn.LocalPort())}
	r.protocol = portProtocol("tcp", r.sides[0].port, r.sides[1].port)
	if r.protocol != "" {
		r.evidence = "port hint"
	}
	r.lru = s.lru.PushBack(r)
	s.records[r.id], s.nativeRecords[key] = r, r
	s.trim()
}

func (s *streamStore) nativePacket(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, refs []pcaputil.PacketReference, ts time.Time, current *capturedPacket) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.nativeRecords[nativeKey(flow)]
	if r == nil {
		return // An evicted, closed record must not be recreated by a late ACK.
	}
	r.packets++
	r.version++
	if r.packets == 1 {
		r.first, r.last = ts, ts
	} else {
		if ts.Before(r.first) {
			r.first = ts
		}
		if ts.After(r.last) {
			r.last = ts
		}
	}
	for _, ref := range refs {
		if ref.Number != 0 && (r.firstReference == 0 || ref.Number < r.firstReference) {
			r.firstReference = ref.Number
		}
		if ref.Number > r.lastReference {
			r.lastReference = ref.Number
		}
	}
	// These are observed overlap/reorder counts against the native cursor,
	// separate from whether the packet is admitted. Held SYN copies have no
	// payload and need no retained packet headers to count them exactly once.
	if current != nil && current.tcpFlowID == flow.Index && current.tcpFlowDomain == r.key.domain {
		state := conn.SequenceObservation()
		if v := current.transportView; v != nil && v.Protocol == "tcp" && v.PayloadLength > 0 && state.Initialized {
			seq := v.Sequence
			if v.TCPFlags&2 != 0 {
				seq++
			}
			if diff := int32(seq - state.NextSequence); diff < 0 {
				r.retransmits++
			} else if diff > 0 {
				r.outOfOrder++
			}
		}
	}
}

func (s *streamStore) nativeData(flow *pcaputil.TrafficFlow, conn *pcaputil.TrafficConnection, frame *pcaputil.TrafficFrame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.nativeRecords[nativeKey(flow)]
	if r == nil || r.evicted {
		return
	}
	d := 0
	if conn != flow.ClientConn {
		d = 1
	}
	r.sides[d].syn = conn.SequenceObservation().SYNObserved
	skip := 0
	if r.sides[d].bytes == 0 && !r.sides[d].syn {
		skip = -1 // Captured prefix only; no unknown sequence bytes invented.
	}
	r.appendData(d, frame.Payload, skip, frame.Timestamp)
	s.trim()
}

func (s *streamStore) nativeClosed(reason pcaputil.TrafficFlowCloseReason, flow *pcaputil.TrafficFlow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := nativeKey(flow)
	r := s.nativeRecords[key]
	if r == nil {
		return
	}
	r.nativeClosed = true
	r.closed = string(reason)
	switch reason {
	case pcaputil.TrafficFlowCloseReason_FIN:
		r.closed = "FIN"
	case pcaputil.TrafficFlowCloseReason_RST:
		r.closed = "RST"
	case pcaputil.TrafficFlowCloseReason_CTX_CANCEL:
		r.closed = "capture ended"
	case pcaputil.TrafficFlowCloseReason_INACTIVE:
		r.closed = "idle timeout"
	case pcaputil.TrafficFlowCloseReason_RESOURCE_LIMIT:
		r.closed = "reassembly resource limit"
	}
	if flow.HasPendingGaps() {
		r.gaps++
		r.retentionNotice = "Native TCP reassembly ended incomplete; unassembled or rejected bytes are unavailable in this preview."
		r.sides[0].broken = flow.ClientConn.SequenceObservation().Pending
		r.sides[1].broken = flow.ServerConn.SequenceObservation().Pending
	}
	r.version++
	if r.evicted {
		delete(s.nativeRecords, key)
	}
}

func (s *streamStore) attachNative(raw *capturedPacket) {
	if raw.tcpFlowID == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r := s.nativeRecords[nativeStreamKey{domain: raw.tcpFlowDomain, flow: raw.tcpFlowID}]; r != nil {
		s.attachPacket(raw, r)
	}
}
