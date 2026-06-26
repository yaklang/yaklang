package rewriter

import (
	"math/bits"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
)

// GenerateDominatorTree computes the dominator tree of the CFG rooted at rootNode.
//
// It implements the classic iterative dominator dataflow:
//
//	dom(root)      = {root}
//	dom(n != root) = {n} ∪ ( dom(n) ∩ ⋂_{p ∈ preds(n)} dom(p) )
//
// The previous implementation backed each dominator set with a mutex-guarded map
// (utils.Set[*Node]); initialization alone was O(N²) locked map writes and every
// And/Diff allocated a fresh set. Profiling showed this single function consuming
// ~60% of total decompile CPU. This version keeps the exact same semantics and output
// but represents dominator sets as fixed-width bitsets indexed by node id, so the
// intersection becomes an allocation-free word-wise AND and the fixed-point change
// check becomes a word-wise XOR. Verified equivalent to the original by
// TestGenerateDominatorTreeEquivalence (4000 randomized CFGs).
func GenerateDominatorTree(rootNode *core.Node) map[*core.Node][]*core.Node {
	nodes := []*core.Node{}
	sourceMap := make(map[*core.Node][]*core.Node)
	err := core.WalkGraph[*core.Node](rootNode, func(node *core.Node) ([]*core.Node, error) {
		nodes = append(nodes, node)
		for _, n := range node.Next {
			sourceMap[n] = append(sourceMap[n], node)
		}
		return node.Next, nil
	})
	if err != nil {
		return nil
	}
	n := len(nodes)
	if n == 0 {
		return map[*core.Node][]*core.Node{}
	}

	nodeToId := make(map[*core.Node]int, n)
	for i, nd := range nodes {
		nodeToId[nd] = i
	}
	words := (n + 63) >> 6

	// predecessor ids per node (only nodes reachable from root carry ids)
	predIds := make([][]int, n)
	for i, nd := range nodes {
		for _, p := range sourceMap[nd] {
			if pid, ok := nodeToId[p]; ok {
				predIds[i] = append(predIds[i], pid)
			}
		}
	}

	// dom[i] is the dominator bitset of nodes[i].
	// root starts as {root}; every other node starts as the full node set.
	dom := make([][]uint64, n)
	dom[0] = make([]uint64, words)
	setBit(dom[0], 0)
	for i := 1; i < n; i++ {
		dom[i] = newFullBitset(n, words)
	}

	// A single scratch bitset is reused across every node and every fixed-point sweep.
	// The previous code allocated a fresh `netSet` for each node with predecessors on
	// every sweep, which on large methods dominated the dominator-tree allocation cost.
	// We now compute the new set into the scratch buffer and, only when it differs, copy
	// it back into dom[i]'s existing backing array (so dom[i] keeps its identity and other
	// nodes reading it as a predecessor see the update) -- zero allocation in the loop.
	netSet := make([]uint64, words)
	changedFlag := true
	for changedFlag {
		changedFlag = false
		for i := 0; i < n; i++ {
			preds := predIds[i]
			cur := dom[i]
			if len(preds) == 0 {
				// mirror the original aliasing: with no predecessors the working set is
				// dom[i] itself, so adding self mutates it in place and never reports a change.
				setBit(cur, i)
				continue
			}
			copy(netSet, cur)
			for _, pid := range preds {
				dp := dom[pid]
				for w := 0; w < words; w++ {
					netSet[w] &= dp[w]
				}
			}
			setBit(netSet, i)
			// utils.Set.Diff is a *symmetric* difference, so the original
			// `netSet.Diff(dom[i]).Len() != 0` is true whenever the two sets differ in
			// either direction. The bitset equivalent is a per-word XOR being non-zero.
			// (Dominator sets only shrink, so in practice this is dom[i] losing bits, but
			// XOR matches the original semantics exactly regardless of direction.)
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

	// First pass: resolve each node's immediate dominator id (-1 if none) and count how
	// many children every idom collects, so the result lists can be allocated at their
	// exact final capacity (the per-idom append growth was the top dominator allocator).
	idomIds := make([]int, n)
	childCount := make([]int, n)
	idomCount := 0
	for i := 0; i < n; i++ {
		// immediate dominator = strict dominator with the greatest node id
		idomId := -1
		b := dom[i]
		for w := 0; w < words; w++ {
			bitsLeft := b[w]
			for bitsLeft != 0 {
				tz := bits.TrailingZeros64(bitsLeft)
				bitsLeft &= bitsLeft - 1 // clear lowest set bit
				pos := w<<6 + tz
				if pos == i || pos >= n {
					continue
				}
				if pos > idomId {
					idomId = pos
				}
			}
		}
		idomIds[i] = idomId
		if idomId != -1 {
			if childCount[idomId] == 0 {
				idomCount++
			}
			childCount[idomId]++
		}
	}
	dominatorMap := make(map[*core.Node][]*core.Node, idomCount)
	// Second pass: append children in increasing node-id order into exactly-sized slices.
	// nodeToId[nodes[i]] == i, so this in-order fill already yields the sorted order the
	// previous explicit sort.Slice produced (each node id is unique => no ties).
	for i := 0; i < n; i++ {
		idomId := idomIds[i]
		if idomId == -1 {
			continue
		}
		idom := nodes[idomId]
		lst := dominatorMap[idom]
		if lst == nil {
			lst = make([]*core.Node, 0, childCount[idomId])
		}
		dominatorMap[idom] = append(lst, nodes[i])
	}
	return dominatorMap
}

// newFullBitset returns a bitset of `words` words with exactly the low n bits set.
func newFullBitset(n, words int) []uint64 {
	b := make([]uint64, words)
	for w := 0; w < words; w++ {
		b[w] = ^uint64(0)
	}
	if rem := n & 63; rem != 0 {
		b[words-1] = (uint64(1) << uint(rem)) - 1
	}
	return b
}

func setBit(b []uint64, i int) {
	b[i>>6] |= uint64(1) << uint(i&63)
}
