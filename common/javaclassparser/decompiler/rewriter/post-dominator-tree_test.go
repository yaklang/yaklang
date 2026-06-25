package rewriter

import (
	"math/rand"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

// referencePostDominatorMap is an independent, map/set-based ground truth for the
// bitset GeneratePostDominatorMap. It uses the same backward dataflow with a single
// virtual sink but a deliberately different (naive) representation, so a bug in the
// bitset bookkeeping cannot hide behind a shared implementation.
func referencePostDominatorMap(rootNode *core.Node) map[*core.Node]*core.Node {
	nodes := []*core.Node{}
	_ = core.WalkGraph[*core.Node](rootNode, func(node *core.Node) ([]*core.Node, error) {
		nodes = append(nodes, node)
		return node.Next, nil
	})
	n := len(nodes)
	if n == 0 {
		return map[*core.Node]*core.Node{}
	}
	id := map[*core.Node]int{}
	for i, nd := range nodes {
		id[nd] = i
	}
	sink := n
	succ := make([][]int, n)
	isExit := make([]bool, n)
	for i, nd := range nodes {
		for _, s := range nd.Next {
			if sid, ok := id[s]; ok {
				succ[i] = append(succ[i], sid)
			}
		}
		if len(succ[i]) == 0 {
			isExit[i] = true
		}
	}
	// reverse reachability from exits
	pred := make([][]int, n)
	for i := 0; i < n; i++ {
		for _, s := range succ[i] {
			pred[s] = append(pred[s], i)
		}
	}
	canReach := make([]bool, n)
	stack := []int{}
	for i := 0; i < n; i++ {
		if isExit[i] {
			canReach[i] = true
			stack = append(stack, i)
		}
	}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, p := range pred[c] {
			if !canReach[p] {
				canReach[p] = true
				stack = append(stack, p)
			}
		}
	}

	full := func() map[int]bool {
		m := map[int]bool{sink: true}
		for i := 0; i < n; i++ {
			if canReach[i] {
				m[i] = true
			}
		}
		return m
	}
	pdom := make([]map[int]bool, n+1)
	pdom[sink] = map[int]bool{sink: true}
	for i := 0; i < n; i++ {
		if canReach[i] {
			pdom[i] = full()
		}
	}
	changed := true
	for changed {
		changed = false
		for i := 0; i < n; i++ {
			if !canReach[i] {
				continue
			}
			var cur map[int]bool
			first := true
			meet := func(set map[int]bool) {
				if first {
					cur = map[int]bool{}
					for k := range set {
						cur[k] = true
					}
					first = false
					return
				}
				for k := range cur {
					if !set[k] {
						delete(cur, k)
					}
				}
			}
			for _, s := range succ[i] {
				if pdom[s] == nil {
					continue
				}
				meet(pdom[s])
			}
			if isExit[i] {
				meet(pdom[sink])
			}
			if first {
				cur = map[int]bool{sink: true}
			}
			cur[i] = true
			if !sameSet(cur, pdom[i]) {
				pdom[i] = cur
				changed = true
			}
		}
	}

	res := map[*core.Node]*core.Node{}
	for i := 0; i < n; i++ {
		if !canReach[i] {
			continue
		}
		best := -1
		bestCard := -1
		for k := range pdom[i] {
			if k == i || k == sink {
				continue
			}
			card := len(pdom[k])
			if card > bestCard {
				bestCard = card
				best = k
			}
		}
		if best == -1 {
			continue
		}
		res[nodes[i]] = nodes[best]
	}
	return res
}

func sameSet(a, b map[int]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// TestGeneratePostDominatorEquivalence fuzzes random CFGs and asserts the bitset
// post-dominator map matches the independent reference on every input.
func TestGeneratePostDominatorEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(0xBADF00D))
	const iterations = 4000
	for it := 0; it < iterations; it++ {
		n := 1 + rng.Intn(40)
		maxOut := 1 + rng.Intn(4)
		root := buildRandomCFG(rng, n, maxOut)

		got := GeneratePostDominatorMap(root)
		want := referencePostDominatorMap(root)
		if !ipdomMapsEqual(got, want) {
			t.Fatalf("iteration %d (n=%d maxOut=%d): post-dominator maps differ\nbitset=%v\nref=%v",
				it, n, maxOut, summarizeIpdom(got), summarizeIpdom(want))
		}
	}
}

func ipdomMapsEqual(a, b map[*core.Node]*core.Node) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || av != bv {
			return false
		}
	}
	return true
}

func summarizeIpdom(m map[*core.Node]*core.Node) map[int]int {
	out := map[int]int{}
	for k, v := range m {
		out[k.Id] = v.Id
	}
	return out
}
