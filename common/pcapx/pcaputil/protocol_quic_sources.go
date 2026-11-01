package pcaputil

import "sort"

// Retained sufficient byte witnesses, not every retransmission observation.
// Offsets are logical STREAM offsets; packet references never invent encrypted
// physical-frame offsets. This state is private to connection/stream/direction.
type quicSourceRange struct {
	lo, hi, pdu  uint64
	refs         []PacketReference
	refsComplete bool
}
type quicStreamSources struct{ ranges []quicSourceRange }

func (s *quicStreamSources) memory() int64 {
	n := int64(cap(s.ranges)) * 64
	for _, r := range s.ranges {
		n += int64(cap(r.refs)) * 64
		for _, ref := range r.refs {
			n += int64(len(ref.Domain.Encapsulation))
		}
	}
	return n
}
func (s *quicStreamSources) remember(pdu, lo, hi uint64, refs []PacketReference, max int) {
	if pdu == 0 || lo >= hi {
		return
	}
	for _, r := range s.ranges {
		if r.lo <= lo && r.hi >= hi && r.refsComplete && len(r.refs) > 0 {
			return
		}
	}
	if len(s.ranges) >= min(max, 1024) {
		return
	} // missing coverage is reported by describe
	n := min(len(refs), 64)
	s.ranges = append(s.ranges, quicSourceRange{lo: lo, hi: hi, pdu: pdu, refs: append([]PacketReference(nil), refs[:n]...), refsComplete: n == len(refs)})
}
func (s *quicStreamSources) prune(before uint64) {
	kept := s.ranges[:0]
	for _, r := range s.ranges {
		if r.hi > before {
			r.lo = max64(r.lo, before)
			kept = append(kept, r)
		}
	}
	clear(s.ranges[len(kept):])
	s.ranges = kept
	if len(s.ranges) == 0 {
		s.ranges = nil
	}
}
func quicSourceCoverage(spans []quicRange, lo, hi uint64) bool {
	if lo == hi {
		return true
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].off < spans[j].off })
	at := lo
	for _, r := range spans {
		if r.off > at {
			return false
		}
		at = max64(at, r.end)
		if at >= hi {
			return true
		}
	}
	return false
}
func quicSourceReferences(refs []PacketReference) []map[string]any {
	out := make([]map[string]any, 0, len(refs))
	for _, r := range refs {
		out = append(out, map[string]any{"Packet": r.Number, "Section": r.Domain.Section, "Interface": r.Domain.Interface, "Encapsulation": r.Domain.Encapsulation})
	}
	return out
}
func (s *quicStreamSources) describe(sid uint64, dir int, lo, hi uint64) map[string]any {
	ids := map[uint64]bool{}
	packets := map[PacketReference]bool{}
	var spans, physical []quicRange
	var ranges []map[string]any
	for _, r := range s.ranges {
		a, b := max64(lo, r.lo), min64(hi, r.hi)
		if a >= b {
			continue
		}
		ids[r.pdu] = true
		spans = append(spans, quicRange{a, b})
		if r.refsComplete && len(r.refs) > 0 {
			physical = append(physical, quicRange{a, b})
		}
		for _, ref := range r.refs {
			packets[ref] = true
		}
		ranges = append(ranges, map[string]any{"Offset": a, "Length": b - a, "Parent PDU": r.pdu, "Packet References": quicSourceReferences(r.refs), "Packet References Complete": r.refsComplete && len(r.refs) > 0})
	}
	parents := make([]uint64, 0, len(ids))
	for id := range ids {
		parents = append(parents, id)
	}
	sort.Slice(ranges, func(i, j int) bool {
		a, b := ranges[i], ranges[j]
		if a["Offset"].(uint64) != b["Offset"].(uint64) {
			return a["Offset"].(uint64) < b["Offset"].(uint64)
		}
		return a["Parent PDU"].(uint64) < b["Parent PDU"].(uint64)
	})
	sort.Slice(parents, func(i, j int) bool { return parents[i] < parents[j] })
	refs := make([]PacketReference, 0, len(packets))
	for ref := range packets {
		refs = append(refs, ref)
	}
	sort.Slice(refs, func(i, j int) bool {
		a, b := refs[i], refs[j]
		if a.Number != b.Number {
			return a.Number < b.Number
		}
		if a.Domain.Section != b.Domain.Section {
			return a.Domain.Section < b.Domain.Section
		}
		if a.Domain.Interface != b.Domain.Interface {
			return a.Domain.Interface < b.Domain.Interface
		}
		return a.Domain.Encapsulation < b.Domain.Encapsulation
	})
	complete, packetComplete := quicSourceCoverage(spans, lo, hi), quicSourceCoverage(physical, lo, hi)
	coverage := "limited"
	if complete && packetComplete {
		coverage = "complete"
	}
	return map[string]any{"Kind": "quic-authenticated-stream-reassembly", "Stream ID": sid, "Direction": uint64(dir), "Offset": lo, "Length": hi - lo, "Parent PDUs": parents, "Packet References": quicSourceReferences(refs), "Ranges": ranges, "Byte Coverage Complete": complete, "Packet Coverage Complete": packetComplete, "Coverage": coverage, "Scope": "retained byte witnesses; not all retransmission copies, peer delivery or physical encrypted-byte offsets"}
}
