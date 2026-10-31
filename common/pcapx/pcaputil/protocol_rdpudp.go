package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/yaklang/yaklang/common/bin-parser/parser/stream_parser"
)

type rdpUDPStore struct {
	entries   map[binUDPKey]*rdpUDPConnection
	charge    int64
	untracked bool
}
type rdpUDPOffer struct {
	wire   *stream_parser.RDPUDPWire
	parent *ProtocolEvent
	digest [32]byte
	id     uint64
}
type rdpUDPConnection struct {
	client, server string
	last           time.Time
	offer, reply   *rdpUDPOffer
	active         *rdpUDPEpoch
	pending        []rdpUDPChunk // complete datagrams preceding the captured peer SYN
}
type rdpUDPEpoch struct {
	id          uint64
	version     uint16
	lossy       bool
	tls         *carrierTLS
	directions  [2]rdpUDPReceiver
	quarantined bool
}
type rdpUDPChunk struct {
	data   []byte
	parent *ProtocolEvent
}
type rdpUDPDelivered struct {
	digest [32]byte
	parent uint64
}
type rdpUDPReceiver struct {
	started         bool
	next, reference int64
	queue           map[int64]rdpUDPChunk
	history         map[int64]rdpUDPDelivered
	order           []int64
}

func carrierParent(e *ProtocolEvent) *ProtocolEvent {
	p := *e
	cloneEvidence(&p)
	p.Raw, p.Fields, p.Session, p.Structured, p.semanticFields, p.semanticSession = nil, nil, nil, nil, nil, nil
	p.semanticMetadata, p.plan, p.sessionError = nil, nil, nil
	return &p
}

func (s *rdpUDPStore) storage() int64 {
	n := int64(512 + len(s.entries)*2048)
	for _, c := range s.entries {
		for _, p := range c.pending {
			n += 512 + int64(cap(p.data)+len(p.parent.SourceBytes.PacketRefs)*64)
		}
		for _, o := range []*rdpUDPOffer{c.offer, c.reply} {
			if o != nil {
				n += 1024 + int64(3*len(o.wire.Normalized)+len(o.parent.SourceBytes.PacketRefs)*64)
			}
		}
		if c.active == nil {
			continue
		}
		n += 2048
		for _, d := range c.active.directions {
			n += int64(len(d.history)*96 + cap(d.order)*8)
			for _, v := range d.queue {
				n += 256 + int64(cap(v.data)+len(v.parent.SourceBytes.PacketRefs)*64)
			}
		}
	}
	return n
}
func (s *rdpUDPStore) reserve(a *binParser, extra int64) bool {
	target := s.storage() + extra
	if target > s.charge {
		if !a.reserveEvidence(target - s.charge) {
			return false
		}
	} else {
		a.buffered.Add(target - s.charge)
	}
	s.charge = target
	return true
}
func rdpUDPKey(e *ProtocolEvent) binUDPKey {
	x, y := e.Source, e.Destination
	if x > y {
		x, y = y, x
	}
	return binUDPKey{"rdpudp/" + x, y, e.Domain}
}
func rdpUDPSYN(w []byte) (*stream_parser.RDPUDPWire, error) {
	if len(w) < 16 || w[7]&1 == 0 {
		return nil, fmt.Errorf("rdpudp: no SYN evidence")
	}
	p, err := stream_parser.DecodeRDPUDPWire(w, 1)
	if err != nil || !p.SYN {
		return nil, fmt.Errorf("rdpudp: incomplete SYN profile")
	}
	return p, nil
}
func rdpUDPParent(a *binParser, base *ProtocolEvent, p *stream_parser.RDPUDPWire, err error) *ProtocolEvent {
	e := *base
	cloneEvidence(&e)
	e.ID = a.ids.Add(1)
	e.Protocol, e.Profile, e.Admission, e.Completeness = "rdpudp", "rdpudp-observed-carrier", "complete-selected-carrier-envelope", "headers"
	if p != nil {
		e.Session = cloneSession(p.Fields)
		e.semanticFields = cloneSession(p.Fields)
		e.Raw = bytes.Clone(p.Normalized)
		if p.Version == 0x101 {
			e.SourceBytes.Kind = "unwrapped"
		}
	} else {
		e.Raw = bytes.Clone(base.Raw)
	}
	e.nativeCarrierObservation = true
	if err != nil {
		e.Status, e.sessionError = classifySessionError(err)
		e.Error = err.Error()
	} else {
		e.Status = "decoded"
	}
	e.Summary = "RDPUDP carrier"
	return &e
}

// No port number establishes this identity. Complete SYN/SYN-ACK fields,
// acknowledgement of the captured random ISN, direction, and selected version
// establish a captured generation. This is observation, not cookie/peer auth.
func (a *binParser) decodeRDPUDPDatagram(base *ProtocolEvent, w []byte) (out []*ProtocolEvent, handled bool) {
	defer func() {
		for _, e := range out {
			if e.Protocol != "rdpudp" {
				continue
			}
			e.Length = len(e.Raw)
			a.messages.Add(1)
			a.messageBytes.Add(uint64(e.Length))
			switch e.Status {
			case "decoded":
				a.decoded.Add(1)
			case "context-required":
				a.contextRequired.Add(1)
			case "limited":
				a.limited.Add(uint64(e.Length))
			case "incomplete":
				a.incomplete.Add(1)
			default:
				a.malformed.Add(1)
			}
		}
	}()
	a.carrierMu.Lock()
	defer a.carrierMu.Unlock()
	key := rdpUDPKey(base)
	var c *rdpUDPConnection
	if a.rdpUDP != nil {
		c = a.rdpUDP.entries[key]
	}
	syn, synErr := rdpUDPSYN(w)
	if c == nil && synErr != nil {
		return nil, false
	}
	if a.rdpUDP == nil {
		a.rdpUDP = &rdpUDPStore{entries: make(map[binUDPKey]*rdpUDPConnection)}
	}
	s := a.rdpUDP
	if c == nil {
		if s.untracked || len(s.entries) >= min(256, a.budget.MaxCollectionElements) || !s.reserve(a, 8192) {
			s.untracked = true
			e := rdpUDPParent(a, base, syn, protocolError(ErrResourceExceeded, "RDPUDP conversation budget"))
			return []*ProtocolEvent{e}, true
		}
		c = &rdpUDPConnection{last: base.Timestamp}
		s.entries[key] = c
	}
	var events []*ProtocolEvent
	if base.Timestamp.After(c.last.Add(10*time.Minute)) && c.active != nil {
		events = append(events, c.active.pendingDiagnostics(a, "RDPUDP capture idle")...)
		events = append(events, c.active.tls.close("RDPUDP capture idle")...)
		c.active = nil
		c.offer = nil
		c.reply = nil
	}
	if base.Timestamp.After(c.last) {
		c.last = base.Timestamp
	}
	if synErr == nil {
		e := rdpUDPParent(a, base, syn, nil)
		e.Session["Connection State"] = "handshake-pending"
		digest := sha256.Sum256(w)
		if !syn.ACK {
			if c.offer != nil && c.offer.digest != digest && c.active == nil {
				e.Status = "context-required"
				e.Error = "RDPUDP competing SYNs before a unique SYN-ACK"
				return append(events, e), true
			}
			if c.offer == nil || c.offer.digest != digest {
				if !s.reserve(a, 8192) {
					e.Status = "limited"
					e.Error = "RDPUDP SYN retention budget"
					return append(events, e), true
				}
				c.offer = &rdpUDPOffer{wire: syn, parent: carrierParent(e), digest: digest, id: a.flows.Add(1)}
				c.client, c.server = base.Source, base.Destination
			} else {
				e.Session["Retransmission"] = true
			}
		} else {
			if !s.reserve(a, 8192) {
				e.Status = "limited"
				e.Error = "RDPUDP SYN-ACK retention budget"
				return append(events, e), true
			}
			c.reply = &rdpUDPOffer{wire: syn, parent: carrierParent(e), digest: digest}
		}
		if c.offer != nil {
			e.FlowID = c.offer.id
		}
		if c.offer != nil && c.reply != nil {
			client, server := c.offer, c.reply
			if server.parent.Source == c.server && server.parent.Destination == c.client && server.wire.SourceACK == client.wire.InitialSequence && server.wire.OfferedVersion <= client.wire.OfferedVersion && server.wire.Lossy == client.wire.Lossy {
				if c.active == nil || c.active.id != client.id {
					if c.active != nil {
						events = append(events, c.active.pendingDiagnostics(a, "RDPUDP observed new SYN generation")...)
						events = append(events, c.active.tls.close("RDPUDP observed new SYN generation")...)
					}
					v := server.wire.OfferedVersion
					ep := &rdpUDPEpoch{id: client.id, version: v, lossy: client.wire.Lossy}
					label := "RDPUDP"
					if v == 0x101 {
						label = "RDPUDP2"
					}
					ep.tls = newCarrierTLS(a, ep.id, [2]string{c.client, c.server}, base.Domain, label)
					for d := range ep.directions {
						r := &ep.directions[d]
						r.queue = make(map[int64]rdpUDPChunk)
						r.history = make(map[int64]rdpUDPDelivered)
						if v != 0x101 {
							initial := client.wire.InitialSequence
							if d == 1 {
								initial = server.wire.InitialSequence
							}
							r.next = int64(uint64(initial) + 1)
							r.reference = r.next
							r.started = true
						}
					}
					c.active = ep
				}
				e.FlowID = c.active.id
				e.Session["Connection State"] = "SYN-and-matching-SYN-ACK-observed"
				e.Session["Selected Version"] = c.active.version
				e.Session["Cookie Hash Verified"] = false
			} else {
				e.Status = "context-required"
				e.Error = "RDPUDP SYN-ACK does not match captured SYN/context"
			}
		}
		e.semanticFields = cloneSession(e.Session)
		events = append(events, e)
		if c.active != nil && c.offer != nil && c.active.id == c.offer.id && len(c.pending) > 0 {
			pending := c.pending
			for _, p := range pending {
				events = append(events, a.rdpUDPData(c, p.parent, p.data, s)...)
			}
			c.pending = nil
		}
		s.reserve(a, 0)
		return events, true
	}
	if c.active == nil {
		e := rdpUDPParent(a, base, nil, protocolError(ErrContextRequired, "RDPUDP data before complete observed SYN exchange"))
		e.Raw = bytes.Clone(w)
		if len(c.pending) < min(64, a.budget.MaxCollectionElements) && s.reserve(a, int64(len(w)+len(base.SourceBytes.PacketRefs)*64+1024)) {
			c.pending = append(c.pending, rdpUDPChunk{data: bytes.Clone(w), parent: carrierParent(e)})
		} else {
			e.Status, e.Error = "limited", "RDPUDP pre-handshake reorder budget"
		}
		return append(events, e), true
	}
	return append(events, a.rdpUDPData(c, base, w, s)...), true
}

func (a *binParser) rdpUDPData(c *rdpUDPConnection, base *ProtocolEvent, w []byte, s *rdpUDPStore) (events []*ProtocolEvent) {
	ep := c.active
	p, err := stream_parser.DecodeRDPUDPWire(w, ep.version)
	if err != nil {
		e := rdpUDPParent(a, base, nil, protocolError(ErrMalformedMessage, "%s", err))
		e.Raw = bytes.Clone(w)
		e.FlowID = ep.id
		return append(events, e)
	}
	e := rdpUDPParent(a, base, p, nil)
	e.FlowID = ep.id
	e.Session["Connection State"] = "observed-generation"
	dir := 0
	if base.Source != c.client {
		dir = 1
	}
	e.Direction = dir
	if p.Data && !p.Dummy && !p.FEC && len(p.Payload) > 0 {
		if ep.quarantined {
			e.Status, e.Error = "context-required", "RDPUDP generation quarantined after conflicting or limited receiver state"
		} else if ep.lossy {
			e.Session["Inner Protocol"] = "DTLS"
			e.Session["Application Decoding"] = "unselected lossy-mode profile"
		} else if c.offer != nil && c.offer.id != ep.id {
			e.Status = "context-required"
			e.Error = "RDPUDP new handshake pending; data generation ambiguous"
		} else {
			chunks, err := ep.accept(dir, p, e, a, s)
			if err != nil {
				e.Status, e.sessionError = classifySessionError(err)
				e.Error = err.Error()
				ep.quarantined = true
				events = append(events, ep.tls.close("RDPUDP receiver refused conflicting/limited sequence")...)
			}
			for _, chunk := range chunks {
				events = append(events, ep.tls.feed(dir, chunk.data, chunk.parent)...)
			}
			// Server bytes can precede the captured ClientHello; release them only
			// after the client boundary has supplied real TLS handshake context.
			if ep.version == 0x101 && ep.tls.f.tls != nil && ep.tls.f.tls.client >= 0 {
				other := 1 - dir
				for _, chunk := range ep.bootstrapAndDrain(other, a) {
					events = append(events, ep.tls.feed(other, chunk.data, chunk.parent)...)
				}
			}
		}
	}
	if ep.tls.f.tls != nil && !ep.tls.broken {
		t := ep.tls.f.tls
		e.Session["TLS Context Observed"] = len(t.random) == 32
		e.Session["TLS Negotiated Version"], e.Session["TLS Cipher Suite"] = t.version, t.suite
	}
	if ep.version != 0x101 && p.Flags&2 != 0 {
		events = append(events, ep.pendingDiagnostics(a, "RDPUDP FIN observed")...)
		events = append(events, ep.tls.close("RDPUDP FIN observed")...)
		ep.quarantined = true
		e.Session["Connection State"] = "FIN-observed; TLS receiver closed"
	}
	e.semanticFields = cloneSession(e.Session)
	// Parent first, then the PDUs it completed; callbacks run after unlocking.
	events = append([]*ProtocolEvent{e}, events...)
	if !s.reserve(a, 0) {
		events = append(events, ep.tls.close("RDPUDP carrier retention budget")...)
		ep.directions = [2]rdpUDPReceiver{}
		ep.quarantined = true
		s.untracked = true
	}
	return events
}

func (ep *rdpUDPEpoch) accept(dir int, p *stream_parser.RDPUDPWire, parent *ProtocolEvent, a *binParser, s *rdpUDPStore) ([]rdpUDPChunk, error) {
	r := &ep.directions[dir]
	var seq int64
	if ep.version == 0x101 {
		ref := r.reference
		if r.started {
			ref = r.next
		}
		if len(r.queue) == 0 && !r.started {
			ref = int64(p.ChannelSequence)
			r.reference = ref
		}
		diff := uint16(p.ChannelSequence - uint16(ref))
		if diff == 0x8000 {
			return nil, protocolError(ErrContextRequired, "RDPUDP2 ambiguous channel sequence window")
		}
		seq = ref + int64(int16(diff))
	} else {
		seq = r.next + int64(int32(p.SourceSequence-uint32(r.next)))
	}
	digest := sha256.Sum256(p.Payload)
	if r.started && seq < r.next {
		parent.Session["Retransmission"] = true
		if old, ok := r.history[seq]; ok {
			parent.Session["Original Carrier PDU"] = old.parent
			if old.digest != digest {
				return nil, protocolError(ErrMalformedMessage, "RDPUDP conflicting delivered retransmission")
			}
		} else {
			parent.Session["Retransmission Bytes Verified"] = false
			parent.Session["Reassembly"] = "old source sequence outside retained duplicate history"
		}
		return nil, nil
	}
	if old, ok := r.queue[seq]; ok {
		parent.Session["Retransmission"] = true
		parent.Session["Original Carrier PDU"] = old.parent.ID
		if !bytes.Equal(old.data, p.Payload) {
			return nil, protocolError(ErrMalformedMessage, "RDPUDP conflicting queued retransmission")
		}
		return nil, nil
	}
	if len(r.queue) >= min(1024, a.budget.MaxCollectionElements) || r.started && seq-r.next >= int64(min(32767, a.budget.MaxCollectionElements)) {
		return nil, protocolError(ErrResourceExceeded, "RDPUDP receive reorder window")
	}
	if !s.reserve(a, int64(len(p.Payload)+len(parent.SourceBytes.PacketRefs)*64+512)) {
		return nil, protocolError(ErrResourceExceeded, "RDPUDP queued source budget")
	}
	r.queue[seq] = rdpUDPChunk{data: bytes.Clone(p.Payload), parent: carrierParent(parent)}
	if !r.started {
		parent.Session["Reassembly"] = "pending-first-observed-TLS-handshake"
	} else if seq != r.next {
		parent.Session["Reassembly"] = "pending-source-sequence-gap"
	}
	return ep.bootstrapAndDrain(dir, a), nil
}

func (ep *rdpUDPEpoch) pendingDiagnostics(a *binParser, reason string) (events []*ProtocolEvent) {
	for dir, r := range ep.directions {
		if len(r.queue) == 0 {
			continue
		}
		e := &ProtocolEvent{FlowID: ep.id, Transport: "udp", Direction: dir, Source: ep.tls.f.endpoints[dir], Destination: ep.tls.f.endpoints[1-dir], Domain: ep.tls.f.domain, Protocol: "rdpudp", Profile: "rdpudp-observed-carrier", Status: "incomplete", Completeness: "incomplete", Error: reason + ": source sequence gap or first TLS handshake boundary unavailable", Session: map[string]any{"Queued Source Chunks": len(r.queue), "Next Source Sequence": r.next, "Authentication Verified": false}, nativeCarrierObservation: true}
		e.SourceBytes.Kind = "reassembled"
		e.sessionError = &ProtocolError{Kind: ErrNeedMore, Message: e.Error}
		e.semanticFields = cloneSession(e.Session)
		keys := make([]int64, 0, len(r.queue))
		for k := range r.queue {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, k := range keys {
			p := r.queue[k].parent
			e.Timestamp = p.Timestamp
			e.SourceBytes.PacketRefs = append(e.SourceBytes.PacketRefs, p.SourceBytes.PacketRefs...)
			e.SourceBytes.ParentPDUs = append(e.SourceBytes.ParentPDUs, p.ID)
		}
		if len(e.SourceBytes.ParentPDUs) > 0 {
			e.SourceBytes.ParentPDU = e.SourceBytes.ParentPDUs[0]
		}
		events = append(events, e)
	}
	return events
}

func (ep *rdpUDPEpoch) bootstrapAndDrain(dir int, a *binParser) (chunks []rdpUDPChunk) {
	r := &ep.directions[dir]
	if !r.started {
		if dir == 1 && (ep.tls.f.tls == nil || ep.tls.f.tls.client < 0) {
			return nil
		}
		keys := make([]int64, 0, len(r.queue))
		for k := range r.queue {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, seq := range keys {
			b := r.queue[seq].data
			if len(b) == 0 || b[0] != 22 {
				continue
			}
			// A valid first hello can itself be split across carrier chunks.
			// Gather only adjacent observed source sequences, under a fixed TLS
			// record bound; a gap never becomes a guessed record boundary.
			// Size scratch storage from actually retained adjacent bytes, never
			// a declared handshake length or the maximum configured budget.
			limit := min(1<<20, a.budget.MaxMessageBytes)
			capacity := 0
			for next := seq; capacity < limit; next++ {
				part, ok := r.queue[next]
				if !ok || len(part.data) > limit-capacity {
					break
				}
				capacity += len(part.data)
			}
			if capacity < 5 {
				continue
			}
			if !a.reserveEvidence(int64(2 * capacity)) {
				continue
			}
			joined := make([]byte, 0, capacity)
			workspace := make([]byte, capacity)
			complete := false
			for next := seq; len(joined) < capacity; next++ {
				part, ok := r.queue[next]
				if !ok {
					break
				}
				if len(part.data) > capacity-len(joined) {
					break
				}
				joined = append(joined, part.data...)
				var invalid bool
				complete, invalid = ep.helloBoundary(joined, workspace, dir, a.budget.MaxCollectionElements)
				if complete || invalid {
					break
				}
			}
			a.buffered.Add(-int64(2 * capacity))
			if !complete {
				continue
			}
			r.next, r.started = seq, true
			break
		}
	}
	if !r.started {
		return nil
	}
	for {
		v, ok := r.queue[r.next]
		if !ok {
			break
		}
		delete(r.queue, r.next)
		r.history[r.next] = rdpUDPDelivered{sha256.Sum256(v.data), v.parent.ID}
		r.order = append(r.order, r.next)
		if len(r.order) > min(256, a.budget.MaxCollectionElements) {
			delete(r.history, r.order[0])
			r.order = r.order[1:]
		}
		r.next++
		chunks = append(chunks, v)
	}
	return chunks
}

// Handshake messages may span both carrier chunks and TLS records. Validate
// the complete first hello without admitting a record-prefix match as identity.
// workspace is precharged by the caller and never sized from a declared length.
func (ep *rdpUDPEpoch) helloBoundary(w, workspace []byte, dir, max int) (bool, bool) {
	used := 0
	for at := 0; at < len(w); {
		if len(w)-at < 5 {
			return false, false
		}
		n, _, err := ep.tls.f.frame(w[at:])
		if err != nil || w[at] != 22 {
			return false, true
		}
		if n > len(w)-at {
			return false, false
		}
		payload := w[at+5 : at+n]
		if len(payload) > len(workspace)-used {
			return false, true
		}
		copy(workspace[used:], payload)
		used += len(payload)
		at += n
		if used < 4 {
			continue
		}
		if workspace[0] != byte(1+dir) {
			return false, true
		}
		h := 4 + tlsU24(workspace[1:4])
		if h > len(workspace) {
			return false, true
		}
		if used < h {
			continue
		}
		_, err = radiusTLSHelloFields(workspace[4:h], dir == 0, max)
		return err == nil, err != nil
	}
	return false, false
}
