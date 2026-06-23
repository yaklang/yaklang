package rewriter

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/utils"
)

// referenceDominatorTree is a verbatim copy of the previous map/utils.Set based
// implementation of GenerateDominatorTree. It serves as the ground truth the new
// bitset implementation must match exactly on every input.
func referenceDominatorTree(rootNode *core.Node) map[*core.Node][]*core.Node {
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
	nodeToId := map[*core.Node]int{}
	for i, n := range nodes {
		nodeToId[n] = i
	}
	dMap := map[*core.Node]*utils.Set[*core.Node]{}
	dMap[rootNode] = utils.NewSet[*core.Node]()
	dMap[rootNode].Add(rootNode)
	for i := 1; i < len(nodes); i++ {
		dMap[nodes[i]] = utils.NewSet[*core.Node]()
		dMap[nodes[i]].AddList(nodes)
	}
	flag := true
	for flag {
		flag = false
		for i := 0; i < len(nodes); i++ {
			netSet := dMap[nodes[i]]
			for _, p := range sourceMap[nodes[i]] {
				netSet = netSet.And(dMap[p])
			}
			netSet.Add(nodes[i])
			if netSet.Diff(dMap[nodes[i]]).Len() != 0 {
				dMap[nodes[i]] = netSet
				flag = true
			}
		}
	}

	dominatorMap := map[*core.Node][]*core.Node{}
	for node, dom := range dMap {
		var idom *core.Node
		for _, n := range dom.List() {
			if n == node {
				continue
			}
			if idom == nil {
				idom = n
			} else {
				if nodeToId[n] > nodeToId[idom] {
					idom = n
				}
			}
		}
		if idom == nil {
			continue
		}
		dominatorMap[idom] = append(dominatorMap[idom], node)
	}
	for _, nodeList := range dominatorMap {
		sort.Slice(nodeList, func(i, j int) bool {
			return nodeToId[nodeList[i]] < nodeToId[nodeList[j]]
		})
	}
	return dominatorMap
}

// buildRandomCFG creates a rooted graph of *core.Node with up to maxOut successors
// per node. Edges may point forward or backward (creating cycles) and may target
// any node, mirroring the irreducible/looping shapes real bytecode produces.
func buildRandomCFG(rng *rand.Rand, n, maxOut int) *core.Node {
	nodes := make([]*core.Node, n)
	for i := 0; i < n; i++ {
		nodes[i] = &core.Node{Id: i}
	}
	for i := 0; i < n; i++ {
		out := rng.Intn(maxOut + 1)
		for k := 0; k < out; k++ {
			tgt := rng.Intn(n)
			if tgt == i {
				continue
			}
			nodes[i].Next = append(nodes[i].Next, nodes[tgt])
		}
	}
	return nodes[0]
}

// domMapsEqual reports whether two dominator maps are identical: same idom keys
// and, per key, the same ordered list of dominated nodes.
func domMapsEqual(a, b map[*core.Node][]*core.Node) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		bv, ok := b[k]
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
	}
	return true
}

// TestGenerateDominatorTreeEquivalence fuzzes thousands of random CFGs of varying
// size/shape and asserts the bitset implementation produces a dominator map
// identical to the original map-based reference on every single one.
func TestGenerateDominatorTreeEquivalence(t *testing.T) {
	rng := rand.New(rand.NewSource(0xC0FFEE))
	const iterations = 4000
	for it := 0; it < iterations; it++ {
		n := 1 + rng.Intn(40)      // 1..40 nodes
		maxOut := 1 + rng.Intn(4)  // up to 1..4 successors
		root := buildRandomCFG(rng, n, maxOut)

		got := GenerateDominatorTree(root)
		want := referenceDominatorTree(root)
		if !domMapsEqual(got, want) {
			t.Fatalf("iteration %d (n=%d maxOut=%d): dominator maps differ\nbitset=%v\nref=%v",
				it, n, maxOut, summarizeDomMap(got), summarizeDomMap(want))
		}
	}
}

// summarizeDomMap renders a dominator map as idomId -> [childIds] for diagnostics.
func summarizeDomMap(m map[*core.Node][]*core.Node) map[int][]int {
	out := map[int][]int{}
	for k, v := range m {
		ids := make([]int, len(v))
		for i, nd := range v {
			ids[i] = nd.Id
		}
		out[k.Id] = ids
	}
	return out
}
