package rewriter

import (
	"math/bits"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

// GeneratePostDominatorMap computes the immediate post-dominator of every node in
// the CFG rooted at rootNode and returns a node -> immediate-post-dominator map.
//
// Post-dominators are the dual of dominators: node p post-dominates node n iff
// every path from n to a method exit (a node with no successor, i.e. return/throw)
// passes through p. The immediate post-dominator ipdom(n) is the closest strict
// post-dominator of n; it is the natural reconvergence point ("merge node") of a
// branch headed at n and gives a sound single-exit boundary for if/region
// structuring even when the head is itself a control-flow join.
//
// The backward dataflow uses a single virtual sink that all real exit nodes flow
// into, so multi-exit methods (several return/throw sites) still yield a
// well-defined post-dominator tree:
//
//	pdom(sink)      = {sink}
//	pdom(n != sink) = {n} ∪ ⋂_{s ∈ succ(n)} pdom(s)
//
// succ(n) here is the ORIGINAL graph successor set; exit nodes additionally have
// the virtual sink as a successor. Only nodes that can actually reach an exit are
// processed; a node trapped in an infinite loop with no exit has no post-dominator
// and is simply absent from the result. Implementation mirrors the allocation-free
// bitset style of GenerateDominatorTree. Verified against a brute-force reference by
// TestGeneratePostDominatorEquivalence.
func GeneratePostDominatorMap(rootNode *core.Node) map[*core.Node]*core.Node {
	nodes := []*core.Node{}
	succMap := make(map[*core.Node][]*core.Node)
	err := core.WalkGraph[*core.Node](rootNode, func(node *core.Node) ([]*core.Node, error) {
		nodes = append(nodes, node)
		return node.Next, nil
	})
	if err != nil {
		return nil
	}
	n := len(nodes)
	if n == 0 {
		return map[*core.Node]*core.Node{}
	}

	nodeToId := make(map[*core.Node]int, n)
	for i, nd := range nodes {
		nodeToId[nd] = i
	}
	// successor ids per node, restricted to nodes reachable from root.
	succIds := make([][]int, n)
	for i, nd := range nodes {
		for _, s := range nd.Next {
			if sid, ok := nodeToId[s]; ok {
				succIds[i] = append(succIds[i], sid)
			}
		}
		succMap[nd] = nd.Next
	}

	// Virtual sink gets id n. Exit nodes (no reachable successor) flow into it.
	sink := n
	total := n + 1
	words := (total + 63) >> 6
	isExit := make([]bool, n)
	for i := 0; i < n; i++ {
		if len(succIds[i]) == 0 {
			isExit[i] = true
		}
	}

	// Only process nodes that can reach the sink (reach some exit). Nodes trapped in
	// an exit-less loop never converge in the backward dataflow and have no
	// post-dominator; we exclude them up front via reverse reachability from exits.
	canReachSink := make([]bool, n)
	// predecessor ids (reverse edges) for the reverse BFS.
	predIds := make([][]int, n)
	for i := 0; i < n; i++ {
		for _, sid := range succIds[i] {
			predIds[sid] = append(predIds[sid], i)
		}
	}
	queue := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if isExit[i] {
			canReachSink[i] = true
			queue = append(queue, i)
		}
	}
	for len(queue) > 0 {
		cur := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, p := range predIds[cur] {
			if !canReachSink[p] {
				canReachSink[p] = true
				queue = append(queue, p)
			}
		}
	}

	// pdom[i] is the post-dominator bitset of nodes[i] (sink is index n).
	// sink starts as {sink}; every processed node starts as the full set.
	pdom := make([][]uint64, total)
	pdom[sink] = make([]uint64, words)
	setBit(pdom[sink], sink)
	for i := 0; i < n; i++ {
		if !canReachSink[i] {
			continue
		}
		pdom[i] = newFullBitset(total, words)
	}

	netSet := make([]uint64, words)
	changedFlag := true
	for changedFlag {
		changedFlag = false
		for i := 0; i < n; i++ {
			if !canReachSink[i] {
				continue
			}
			cur := pdom[i]
			// meet over successors' pdom sets; exit nodes meet with {sink}.
			first := true
			meet := func(set []uint64) {
				if first {
					copy(netSet, set)
					first = false
					return
				}
				for w := 0; w < words; w++ {
					netSet[w] &= set[w]
				}
			}
			for _, sid := range succIds[i] {
				if pdom[sid] == nil {
					// successor cannot reach sink; it contributes nothing usable.
					continue
				}
				meet(pdom[sid])
			}
			if isExit[i] {
				meet(pdom[sink])
			}
			if first {
				// no usable successor (all successors are exit-less). Treat as exit
				// into the sink so the node still gets a post-dominator.
				copy(netSet, pdom[sink])
			}
			setBit(netSet, i)
			changed := false
			for w := 0; w < words; w++ {
				if netSet[w]^cur[w] != 0 {
					changed = true
					break
				}
			}
			if changed {
				copy(cur, netSet)
				changedFlag = true
			}
		}
	}

	// immediate post-dominator = strict post-dominator with the largest pdom set
	// (it is post-dominated by every other strict post-dominator, so its own set is
	// the most inclusive). The sink as ipdom means "exits the method" -> nil.
	result := make(map[*core.Node]*core.Node, n)
	for i := 0; i < n; i++ {
		if !canReachSink[i] {
			continue
		}
		b := pdom[i]
		bestId := -1
		bestCard := -1
		for w := 0; w < words; w++ {
			bitsLeft := b[w]
			for bitsLeft != 0 {
				tz := bits.TrailingZeros64(bitsLeft)
				bitsLeft &= bitsLeft - 1
				pos := w<<6 + tz
				if pos == i || pos >= total {
					continue
				}
				card := bitsetCard(pdom[pos], words)
				if card > bestCard {
					bestCard = card
					bestId = pos
				}
			}
		}
		if bestId == -1 || bestId == sink {
			continue
		}
		result[nodes[i]] = nodes[bestId]
	}
	return result
}

// RegionExit returns the immediate post-dominator of head within the CFG rooted at
// rootNode, i.e. the reconvergence point of a branch headed at head. It returns nil
// when head exits the method on every path (no real post-dominator). This is the
// sound single-exit boundary used to bound an if/region body during structuring.
func RegionExit(rootNode, head *core.Node) *core.Node {
	if rootNode == nil || head == nil {
		return nil
	}
	m := GeneratePostDominatorMap(rootNode)
	return m[head]
}

func bitsetCard(b []uint64, words int) int {
	if b == nil {
		return 0
	}
	c := 0
	for w := 0; w < words; w++ {
		c += bits.OnesCount64(b[w])
	}
	return c
}
