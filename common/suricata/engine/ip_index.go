package engine

import (
	"encoding/binary"
	"net/netip"
	"sort"
	"strings"

	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/yaklang/common/suricata/rule"
)

type sweepSegment struct {
	start uint32
	end   uint32
	off   uint32
	count uint32
}

type sweepEvent struct {
	pos     uint64
	planIdx int32
	add     bool
}

// ipv4SweepIndex maps disjoint sorted IPv4 [start, end] intervals to the exact
// set of candidate plan indices covering that interval via one O(log M) binary
// search with zero heap allocations on lookup.
type ipv4SweepIndex struct {
	segments []sweepSegment
	plans    []int32
}

func buildIPv4SweepIndex(events []sweepEvent) *ipv4SweepIndex {
	if len(events) == 0 {
		return nil
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].pos != events[j].pos {
			return events[i].pos < events[j].pos
		}
		if events[i].add != events[j].add {
			// Process removals before additions at the exact same boundary.
			return !events[i].add
		}
		return events[i].planIdx < events[j].planIdx
	})

	idx := &ipv4SweepIndex{}
	active := make(map[int32]int)
	var scratch []int32

	i := 0
	for i < len(events) {
		curPos := events[i].pos
		for i < len(events) && events[i].pos == curPos {
			ev := events[i]
			if ev.add {
				active[ev.planIdx]++
			} else {
				if c := active[ev.planIdx] - 1; c <= 0 {
					delete(active, ev.planIdx)
				} else {
					active[ev.planIdx] = c
				}
			}
			i++
		}
		if len(active) == 0 || i >= len(events) {
			continue
		}
		nextPos := events[i].pos
		if nextPos <= curPos || curPos > 0xFFFFFFFF {
			continue
		}
		endPos := nextPos - 1
		if endPos > 0xFFFFFFFF {
			endPos = 0xFFFFFFFF
		}

		scratch = scratch[:0]
		for p := range active {
			scratch = append(scratch, p)
		}
		sort.Slice(scratch, func(a, b int) bool { return scratch[a] < scratch[b] })

		off := uint32(len(idx.plans))
		idx.plans = append(idx.plans, scratch...)
		idx.segments = append(idx.segments, sweepSegment{
			start: uint32(curPos),
			end:   uint32(endPos),
			off:   off,
			count: uint32(len(scratch)),
		})
	}
	if len(idx.segments) == 0 {
		return nil
	}
	return idx
}

func (idx *ipv4SweepIndex) lookup(ip uint32, emit func(planIdx int)) {
	if idx == nil || len(idx.segments) == 0 {
		return
	}
	lo, hi := 0, len(idx.segments)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if idx.segments[mid].end < ip {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(idx.segments) && idx.segments[lo].start <= ip && ip <= idx.segments[lo].end {
		seg := idx.segments[lo]
		end := seg.off + seg.count
		for i := seg.off; i < end; i++ {
			emit(int(idx.plans[i]))
		}
	}
}

type tcpFlagsFastMask struct {
	valid      bool
	mode       byte
	wanted     uint16
	ignoreMask uint16
}

func compileTCPFlagsFastMask(raw string) tcpFlagsFastMask {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tcpFlagsFastMask{}
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 2 {
		return tcpFlagsFastMask{}
	}
	flags := strings.ToUpper(strings.TrimSpace(parts[0]))
	var mode byte
	if len(flags) > 0 {
		switch flags[0] {
		case '!', '*', '+':
			mode, flags = flags[0], flags[1:]
		}
	}
	ignored := ""
	if len(parts) > 1 {
		ignored = strings.ToUpper(strings.TrimSpace(parts[1]))
	}
	wanted, ok := parseFastTCPFlagBits(flags)
	if !ok {
		return tcpFlagsFastMask{}
	}
	ignoreMask, ok := parseFastTCPFlagBits(ignored)
	if !ok {
		return tcpFlagsFastMask{}
	}
	return tcpFlagsFastMask{
		valid:      true,
		mode:       mode,
		wanted:     wanted &^ ignoreMask,
		ignoreMask: ignoreMask,
	}
}

func parseFastTCPFlagBits(raw string) (uint16, bool) {
	var mask uint16
	for _, flag := range raw {
		switch flag {
		case 'F':
			mask |= 1 << 0
		case 'S':
			mask |= 1 << 1
		case 'R':
			mask |= 1 << 2
		case 'P':
			mask |= 1 << 3
		case 'A':
			mask |= 1 << 4
		case 'U':
			mask |= 1 << 5
		case 'E':
			mask |= 1 << 6
		case 'C':
			mask |= 1 << 7
		case '0':
		case 'N', '1':
			mask |= 1 << 8
		case '2':
			mask |= 1 << 9
		default:
			return 0, false
		}
	}
	return mask, true
}

func tcpLayerFlagMask(tcp *layers.TCP) uint16 {
	if tcp == nil {
		return 0
	}
	var mask uint16
	if tcp.FIN {
		mask |= 1 << 0
	}
	if tcp.SYN {
		mask |= 1 << 1
	}
	if tcp.RST {
		mask |= 1 << 2
	}
	if tcp.PSH {
		mask |= 1 << 3
	}
	if tcp.ACK {
		mask |= 1 << 4
	}
	if tcp.URG {
		mask |= 1 << 5
	}
	if tcp.ECE {
		mask |= 1 << 6
	}
	if tcp.CWR {
		mask |= 1 << 7
	}
	if tcp.NS {
		mask |= 1 << 8
	}
	return mask
}

func (m tcpFlagsFastMask) matches(current uint16) bool {
	if !m.valid {
		return true
	}
	masked := current &^ m.ignoreMask
	switch m.mode {
	case '!':
		return masked&m.wanted == 0
	case '*':
		return masked&m.wanted != 0
	case '+':
		return masked&m.wanted == m.wanted
	default:
		return masked == m.wanted
	}
}

type fallbackRuleEntry struct {
	planIdx  int
	tcpFlags tcpFlagsFastMask
}

// ipFilteredSet accelerates alwaysEval rule sets (such as hundreds of IP
// blocklist / reputation / TCP-flag rules) using global IPv4 sweep-line
// indexes for concrete CIDR rules plus TCP flag bitmasks on fallback rules.
type ipFilteredSet struct {
	srcIndex *ipv4SweepIndex
	dstIndex *ipv4SweepIndex
	fallback []fallbackRuleEntry
}

func buildIPFilteredSet(plans []*CompiledRulePlan, indices []int, checkTCPFlags bool) ipFilteredSet {
	if len(indices) == 0 {
		return ipFilteredSet{}
	}
	var srcEvents, dstEvents []sweepEvent
	var fallback []fallbackRuleEntry

	const maxSingleRangeSpan = uint64(1 << 24) // avoid exploding sweep segments on huge /8 ranges

	rangesIndexable := func(ranges [][2]uint32) bool {
		if len(ranges) == 0 {
			return false
		}
		for _, r := range ranges {
			if uint64(r[1])-uint64(r[0]) >= maxSingleRangeSpan {
				return false
			}
		}
		return true
	}

	addEvents := func(dst *[]sweepEvent, ranges [][2]uint32, planIdx int) {
		p := int32(planIdx)
		for _, r := range ranges {
			*dst = append(*dst,
				sweepEvent{pos: uint64(r[0]), planIdx: p, add: true},
				sweepEvent{pos: uint64(r[1]) + 1, planIdx: p, add: false},
			)
		}
	}

	for _, idx := range indices {
		var r *rule.Rule
		if idx >= 0 && idx < len(plans) && plans[idx] != nil {
			r = plans[idx].Rule
		}
		if r == nil {
			continue
		}
		srcRanges, srcOK := r.SourceAddress.IPv4SupersetRanges()
		dstRanges, dstOK := r.DestinationAddress.IPv4SupersetRanges()
		srcOK = srcOK && rangesIndexable(srcRanges)
		dstOK = dstOK && rangesIndexable(dstRanges)

		if r.Direction == "<>" {
			if srcOK {
				addEvents(&srcEvents, srcRanges, idx)
				addEvents(&dstEvents, srcRanges, idx)
				continue
			}
			if dstOK {
				addEvents(&srcEvents, dstRanges, idx)
				addEvents(&dstEvents, dstRanges, idx)
				continue
			}
		} else {
			if srcOK {
				addEvents(&srcEvents, srcRanges, idx)
				continue
			}
			if dstOK {
				addEvents(&dstEvents, dstRanges, idx)
				continue
			}
		}

		entry := fallbackRuleEntry{planIdx: idx}
		if checkTCPFlags && r.ContentRuleConfig != nil && r.ContentRuleConfig.TcpConfig != nil && r.ContentRuleConfig.TcpConfig.Flags != "" {
			entry.tcpFlags = compileTCPFlagsFastMask(r.ContentRuleConfig.TcpConfig.Flags)
		}
		fallback = append(fallback, entry)
	}

	return ipFilteredSet{
		srcIndex: buildIPv4SweepIndex(srcEvents),
		dstIndex: buildIPv4SweepIndex(dstEvents),
		fallback: fallback,
	}
}

func ipv4ToUint32(addr netip.Addr) (uint32, bool) {
	if !addr.IsValid() {
		return 0, false
	}
	addr = addr.Unmap()
	if !addr.Is4() {
		return 0, false
	}
	b := addr.As4()
	return binary.BigEndian.Uint32(b[:]), true
}

func (s *ipFilteredSet) collect(
	plans []*CompiledRulePlan,
	collector *candidateCollector,
	meta packetHeaderMeta,
	hasTCPFlags bool,
	tcpFlags uint16,
) {
	if !meta.hasAddr {
		for _, entry := range s.fallback {
			if hasTCPFlags && !entry.tcpFlags.matches(tcpFlags) {
				continue
			}
			collector.add(entry.planIdx)
		}
		// Non-IP packets cannot match IPv4-indexed rules; fail-safe if needed
		// is already handled when pk == nil.
		return
	}
	if s.srcIndex != nil {
		if srcV4, ok := ipv4ToUint32(meta.srcAddr); ok {
			s.srcIndex.lookup(srcV4, func(planIdx int) {
				if matchRuleHeaderFast(plans[planIdx].Rule, meta) {
					collector.add(planIdx)
				}
			})
		}
	}
	if s.dstIndex != nil {
		if dstV4, ok := ipv4ToUint32(meta.dstAddr); ok {
			s.dstIndex.lookup(dstV4, func(planIdx int) {
				if matchRuleHeaderFast(plans[planIdx].Rule, meta) {
					collector.add(planIdx)
				}
			})
		}
	}
	for _, entry := range s.fallback {
		if hasTCPFlags && !entry.tcpFlags.matches(tcpFlags) {
			continue
		}
		if matchRuleHeaderFast(plans[entry.planIdx].Rule, meta) {
			collector.add(entry.planIdx)
		}
	}
}
