package engine

import "strings"

// rawByteAC is a compact, immutable, zero-allocation-on-scan Aho-Corasick
// automaton operating directly over raw bytes (0x00..0xFF). It complements
// minirehs for binary-only Suricata content signatures (containing NUL or
// high bytes >= 0x80) that cannot be expressed in UTF-8 rune-level RE2.
type rawByteAC struct {
	nodes []rawACNode
	// exactMatch maps output pattern ID -> plan indices.
	outputs [][]int
}

type rawACNode struct {
	// Dense 256-entry transition table for fast O(1) byte step; built only
	// when a lane actually has binary-only rules (typically 50–300 patterns,
	// ~150–400 states => ~150–400 KB RAM).
	next [256]int32
	out  []int32
}

type rawByteACBuilder struct {
	patterns []rawACPattern
	dedup    map[string]int
	outputs  [][]int
}

type rawACPattern struct {
	data     []byte
	caseless bool
}

func newRawByteACBuilder() *rawByteACBuilder {
	return &rawByteACBuilder{
		dedup: make(map[string]int),
	}
}

func (b *rawByteACBuilder) add(lit []byte, caseless bool, planIdx int) {
	if len(lit) < 2 {
		return
	}
	norm := make([]byte, len(lit))
	if caseless {
		for i, ch := range lit {
			if ch >= 'A' && ch <= 'Z' {
				norm[i] = ch + ('a' - 'A')
			} else {
				norm[i] = ch
			}
		}
	} else {
		copy(norm, lit)
	}
	var keyBuilder strings.Builder
	keyBuilder.Grow(len(norm) + 1)
	if caseless {
		keyBuilder.WriteByte('i')
	} else {
		keyBuilder.WriteByte('s')
	}
	keyBuilder.Write(norm)
	key := keyBuilder.String()

	if id, exists := b.dedup[key]; exists {
		plans := b.outputs[id]
		if len(plans) == 0 || plans[len(plans)-1] != planIdx {
			b.outputs[id] = append(plans, planIdx)
		}
		return
	}
	id := len(b.patterns)
	b.dedup[key] = id
	b.patterns = append(b.patterns, rawACPattern{data: norm, caseless: caseless})
	b.outputs = append(b.outputs, []int{planIdx})
}

func (b *rawByteACBuilder) build() *rawByteAC {
	if b == nil || len(b.patterns) == 0 {
		return nil
	}
	// Separate exact vs caseless? Even simpler: fold ASCII uppercase -> lowercase
	// in the trie AND scan with a single branchless ASCII-lower table! Because
	// ASCII-lowercasing is a sound superset for exact-case binary patterns
	// (every exact match still matches after ASCII lowercasing, and the full
	// matcher verifies exact case!), ONE single automaton handles both exact
	// and caseless binary literals with zero false negatives and half the RAM!
	nodes := make([]rawACNode, 1, len(b.patterns)*4)
	for i := range nodes[0].next {
		nodes[0].next[i] = -1
	}

	for patID, p := range b.patterns {
		state := int32(0)
		for _, ch := range p.data {
			if ch >= 'A' && ch <= 'Z' {
				ch += 'a' - 'A'
			}
			nxt := nodes[state].next[ch]
			if nxt < 0 {
				nxt = int32(len(nodes))
				var n rawACNode
				for i := range n.next {
					n.next[i] = -1
				}
				nodes = append(nodes, n)
				nodes[state].next[ch] = nxt
			}
			state = nxt
		}
		nodes[state].out = append(nodes[state].out, int32(patID))
	}

	// Build Aho-Corasick failure links and complete DFA transitions via BFS.
	fail := make([]int32, len(nodes))
	queue := make([]int32, 0, len(nodes))
	for c := 0; c < 256; c++ {
		if nxt := nodes[0].next[c]; nxt >= 0 {
			fail[nxt] = 0
			queue = append(queue, nxt)
		} else {
			nodes[0].next[c] = 0
		}
	}
	for head := 0; head < len(queue); head++ {
		u := queue[head]
		f := fail[u]
		if len(nodes[f].out) > 0 {
			nodes[u].out = append(nodes[u].out, nodes[f].out...)
		}
		for c := 0; c < 256; c++ {
			v := nodes[u].next[c]
			if v >= 0 {
				fail[v] = nodes[f].next[c]
				queue = append(queue, v)
			} else {
				nodes[u].next[c] = nodes[f].next[c]
			}
		}
	}

	return &rawByteAC{
		nodes:   nodes,
		outputs: b.outputs,
	}
}

func (ac *rawByteAC) scan(data []byte, collector *candidateCollector) {
	if ac == nil || len(data) == 0 || len(ac.nodes) == 0 {
		return
	}
	state := int32(0)
	nodes := ac.nodes
	outputs := ac.outputs
	for _, ch := range data {
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		state = nodes[state].next[ch]
		if outs := nodes[state].out; len(outs) > 0 {
			for _, patID := range outs {
				for _, planIdx := range outputs[patID] {
					collector.add(planIdx)
				}
			}
		}
	}
}
