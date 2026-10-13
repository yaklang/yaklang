package pcaputil

import (
	"container/heap"
	"sort"
)

// WithDNP3TypedStaticReads selects TCP static G1V0/1/2, G10V0/2 and G30V0..6
// all/range READ observations plus G1V1 packed responses. Ordered selector
// coverage and the selected G1V1 quality promotion follow the pinned reference
// database model. Default variations, device completeness and authentication
// are not inferred from an observed request or a partial response.
func WithDNP3TypedStaticReads(enabled bool) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.binParserConfig == nil {
			c.binParserConfig = &BinParserConfig{}
		}
		c.binParserConfig.DecodeDNP3TypedStaticReads = enabled
		return nil
	}
}

// WithSessionDNP3TypedStaticReads selects the same bounded observation on TCP.
func WithSessionDNP3TypedStaticReads(enabled bool) ProtocolSessionOption {
	return func(s *captureSession) error { s.f.a.config.DecodeDNP3TypedStaticReads = enabled; return nil }
}

type dnp3SelectorBoundary struct {
	at, id int
	start  bool
}
type dnp3SelectorPriority []int

func (a dnp3SelectorPriority) Len() int           { return len(a) }
func (a dnp3SelectorPriority) Less(i, j int) bool { return a[i] > a[j] }
func (a dnp3SelectorPriority) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a *dnp3SelectorPriority) Push(v any)        { *a = append(*a, v.(int)) }
func (a *dnp3SelectorPriority) Pop() any {
	old := *a
	v := old[len(old)-1]
	*a = old[:len(old)-1]
	return v
}

// A later header replaces the selected variation at overlapping indices. Sweep
// bounded interval endpoints rather than expanding all 65536 possible indices
// or scanning every selector per response point. Retained capacity is charged
// by memoryTarget before any pending request is admitted.
func normalizeDNP3TypedSelectors(a []dnp3StaticSelector) []dnp3StaticSelector {
	out := []dnp3StaticSelector{}
	groups := [256]bool{}
	for _, x := range a {
		groups[x.group] = true
	}
	for group := range groups {
		if !groups[group] {
			continue
		}
		g := byte(group)
		if g == 22 || g == 23 {
			// Event headers select successive unselected records; finite quotas
			// remain ordered until an unbounded header ends selection of this
			// type. Class/V0 defaults remain unreported.
			for _, x := range a {
				if x.group == g {
					x.first, x.last, x.all = 0, 65535, false
					out = append(out, x)
					if x.count == 0 {
						break
					}
				}
			}
			continue
		}
		events := []dnp3SelectorBoundary{}
		for id, x := range a {
			if x.group != g {
				continue
			}
			first, last := int(x.first), int(x.last)
			if x.all {
				first, last = 0, 65535
			}
			events = append(events, dnp3SelectorBoundary{first, id, true}, dnp3SelectorBoundary{last + 1, id, false})
		}
		sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })
		active := make([]bool, len(a))
		priority := &dnp3SelectorPriority{}
		for i := 0; i < len(events); {
			at := events[i].at
			for i < len(events) && events[i].at == at {
				e := events[i]
				active[e.id] = e.start
				if e.start {
					heap.Push(priority, e.id)
				}
				i++
			}
			for priority.Len() > 0 && !active[(*priority)[0]] {
				heap.Pop(priority)
			}
			if i == len(events) || priority.Len() == 0 {
				continue
			}
			next := events[i].at
			chosen := a[(*priority)[0]]
			if len(out) > 0 {
				old := &out[len(out)-1]
				if old.group == g && old.variation == chosen.variation && int(old.last)+1 == at {
					old.last = uint16(next - 1)
					continue
				}
			}
			out = append(out, dnp3StaticSelector{group: g, variation: chosen.variation, first: uint16(at), last: uint16(next - 1)})
		}
	}
	return out
}

func dnp3TypedSelectorsCover(selectors []dnp3StaticSelector, objects []any) bool {
	// Count selectors select successive event records, including repeated
	// point indices. Maintain ephemeral per-type ordinal cursors so a rejected
	// response cannot mutate retained request coverage or consume its quota.
	type cursor struct {
		at, used    int
		initialized bool
	}
	var eventCursor [2]cursor
	for _, o := range objects {
		obj := o.(map[string]any)
		group := byte(obj["Group"].(int))
		variation := byte(obj["Variation"].(int))
		if group == 22 || group == 23 {
			c := &eventCursor[group-22]
			if !c.initialized {
				c.at = sort.Search(len(selectors), func(i int) bool { return selectors[i].group >= group })
				c.initialized = true
			}
			for range obj["Points"].([]any) {
				for c.at < len(selectors) && selectors[c.at].group == group && selectors[c.at].count != 0 && c.used == int(selectors[c.at].count) {
					c.at++
					c.used = 0
				}
				if c.at == len(selectors) || selectors[c.at].group != group {
					return false
				}
				x := selectors[c.at]
				if x.variation != 0 && x.variation != variation {
					return false
				}
				if x.count != 0 {
					c.used++
				}
			}
			continue
		}
		for _, p := range obj["Points"].([]any) {
			point := p.(map[string]any)
			index := point["Index"].(int)
			at := sort.Search(len(selectors), func(i int) bool {
				return selectors[i].group > group || (selectors[i].group == group && int(selectors[i].last) >= index)
			})
			if at == len(selectors) || selectors[at].group != group || int(selectors[at].first) > index {
				return false
			}
			requested := selectors[at].variation
			if requested == 0 || requested == variation {
				continue
			}
			// The reference BinarySpec promotes packed G1V1 when quality is not
			// online-only. A flagged online-only reply is not evidence of promotion.
			if group == 1 && requested == 1 && variation == 2 && point["Flags"].(int)&127 != 1 {
				continue
			}
			return false
		}
	}
	return true
}
