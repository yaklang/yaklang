package rewriter

import (
	"math/bits"
	"sort"

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

	changedFlag := true
	for changedFlag {
		changedFlag = false
		for i := 0; i < n; i++ {
			preds := predIds[i]
			var netSet []uint64
			if len(preds) == 0 {
				// mirror the original aliasing: with no predecessors the working set is
				// dom[i] itself, so adding self mutates it in place and never reports a change.
				netSet = dom[i]
			} else {
				netSet = make([]uint64, words)
				copy(netSet, dom[i])
				for _, pid := range preds {
					dp := dom[pid]
					for w := 0; w < words; w++ {
						netSet[w] &= dp[w]
					}
				}
			}
			setBit(netSet, i)
			// utils.Set.Diff is a *symmetric* difference, so the original
			// `netSet.Diff(dom[i]).Len() != 0` is true whenever the two sets differ in
			// either direction. The bitset equivalent is a per-word XOR being non-zero.
			// (Dominator sets only shrink, so in practice this is dom[i] losing bits, but
			// XOR matches the original semantics exactly regardless of direction.)
			changed := false
			cur := dom[i]
			for w := 0; w < words; w++ {
				if netSet[w]^cur[w] != 0 {
					changed = true
					break
				}
			}
			if changed {
				dom[i] = netSet
				changedFlag = true
			}
		}
	}

	dominatorMap := map[*core.Node][]*core.Node{}
	for i := 0; i < n; i++ {
		node := nodes[i]
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
		if idomId == -1 {
			continue
		}
		idom := nodes[idomId]
		dominatorMap[idom] = append(dominatorMap[idom], node)
	}
	for _, nodeList := range dominatorMap {
		sort.Slice(nodeList, func(i, j int) bool {
			return nodeToId[nodeList[i]] < nodeToId[nodeList[j]]
		})
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
