package core

import (
	"slices"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/utils"
)

type Node struct {
	Id                  int
	LoopBreak           bool
	Statement           statements.Statement
	Source              []*Node
	HideNext            *Node
	Next                []*Node
	IsJmp               bool
	IsDel               bool
	TrueNode, FalseNode func() *Node
	JmpNode             *Node
	MergeNode           *Node
	IsCircle            bool
	IsMerge             bool
	IsIf                bool
	// SwitchEmptyDefaultMerge marks a switch node whose `default` target was found to be the switch's
	// own post-switch merge point (an EMPTY `default: break;` reached by >=2 case bodies that break to
	// it). SwitchRewriter uses it to DROP the default case (equivalent to no default) and emit the
	// merge code after the switch, instead of absorbing it as the default body and dropping the case
	// `break`s (Bug K). It is NOT set for the ordinary fallback where the default genuinely owns the
	// body (e.g. fall-through-into-default), so that case is structured unchanged.
	SwitchEmptyDefaultMerge bool
	// SwitchEmptyCaseMerge marks a switch node whose post-switch merge point is the start node of an
	// EMPTY non-default case (`case K: break;` whose only body in bytecode is `goto merge`). After
	// goto-folding that case's start node IS the merge, which the dominator-based merge search excludes
	// (it is a case start) and the empty-default fallback misses (the default here genuinely throws).
	// Without detecting it, the merge/tail code was absorbed into that case and every case `break` was
	// dropped, so cases fell through into `default: throw` (commons-codec Base64/Base32 EOF switch:
	// `Impossible modulus N` for any non-block-aligned input). When set, SwitchRewriter renders the
	// matching case as an empty `case K: break;` and emits the merge code after the switch.
	SwitchEmptyCaseMerge bool
	// SwitchEmptyCaseMergeNode persists the empty-case merge node across the two SwitchRewriter1
	// invocations per switch (the prep loop, then the call inside SwitchRewriter). After the first run
	// inserts the case `break`s, the structure no longer re-derives this merge (unlike an empty default,
	// whose merge coincides with caseMap[-1] so the generic fallback restores it), so without saving it
	// the second run would corrupt MergeNode to the default/throw node. Reused on re-entry.
	SwitchEmptyCaseMergeNode *Node
	IsTryCatch               bool
	// IsCatchStart marks a node that is the entry of an exception handler (catch / finally-desugar)
	// block, set when the try node is built from the exception table. TryRewriter uses it to classify
	// a try node's successors structurally instead of inferring the handler from its body's first
	// statement: an empty catch whose unused exception is discarded with `pop` (the ECJ idiom) has no
	// leading exception-store assignment, so the body-content heuristic alone mis-classified it as the
	// try body and produced a malformed try with no catch handler.
	IsCatchStart        bool
	TryNodeId           int
	CatchNodeInfo       []*CatchNode
	IsDoWhile           bool
	BodyNodeStart       *Node
	GetLoopEndNode      func() *Node
	SetLoopEndNode      func(*Node, *Node)
	ConditionNode       []*Node
	SwitchMergeNode     *Node
	CircleNodesSet      *utils.Set[*Node]
	IsInCircle          bool
	OutNodeMap          map[*Node]*Node
	LoopEndNode         *Node
	UncertainBreakNodes map[*Node]*Node
	SourceConditionNode *Node
	//CircleRoute         *SubNodeMap
	//PreNodeRoute          *SubNodeMap
	//AllPreNodeRoute       []*SubNodeMap
}

func (n *Node) RemoveAllSource() {
	source := make([]*Node, len(n.Source))
	copy(source, n.Source)
	for _, node := range source {
		node.RemoveNext(n)
	}
}
func (n *Node) RemoveSource(node *Node) {
	node.RemoveNext(n)
}
func (n *Node) RemoveAllNext() {
	next := make([]*Node, len(n.Next))
	copy(next, n.Next)
	for _, node := range next {
		n.RemoveNext(node)
	}
}
func (n *Node) ReplaceNext(node1, node2 *Node) {
	for i, next := range n.Next {
		if next == node1 {
			n.Next[i] = node2
			break
		}
	}
}
func (n *Node) RemoveNext(node *Node) {
	for i, next := range n.Next {
		if next == node {
			n.Next = append(n.Next[:i], n.Next[i+1:]...)
			break
		}
	}
	for i, source := range node.Source {
		if source == n {
			node.Source = append(node.Source[:i], node.Source[i+1:]...)
			break
		}
	}
}
func (n *Node) AddSource(node *Node) {
	node.AddNext(n)
}
func (n *Node) Replace(node *Node) {
	next := slices.Clone(n.Next)
	for _, source := range n.Source {
		source.ReplaceNext(n, node)
		node.AddSource(source)
	}
	for _, n2 := range next {
		node.AddNext(n2)
	}
	n.RemoveAllSource()
}
func (n *Node) AddNext(node *Node) {
	var found bool
	for _, next := range n.Next {
		if next == node {
			found = true
			break
		}
	}
	if !found {
		n.Next = append(n.Next, node)
	}
	found = false
	for _, source := range node.Source {
		if source == n {
			found = true
			break
		}
	}
	if !found {
		node.Source = append(node.Source, n)
	}
}
// ReplaceNextSliceKeepOrder replaces oldNode in n.Next with the given successors, spliced in
// place at oldNode's original position, preserving successor ordering. This is required for
// ConditionStatement nodes: if-opcodes never populate JmpNode (opcode.Jmp stays 0), so the
// downstream TrueNode/FalseNode wiring depends purely on Next order ([falseBranch, trueBranch]).
// The previous remove-then-AddNext(append) rewiring pushed the rewired successor to the end of
// the predecessor's Next, silently swapping an if-node's two branches whenever a foldable node
// (e.g. an intervening local store) sat on the jump-target branch (Bug M). Source back-links are
// kept consistent and duplicates are removed.
func (n *Node) ReplaceNextSliceKeepOrder(oldNode *Node, news []*Node) {
	for i, s := range oldNode.Source {
		if s == n {
			oldNode.Source = append(oldNode.Source[:i], oldNode.Source[i+1:]...)
			break
		}
	}
	newNext := make([]*Node, 0, len(n.Next)+len(news))
	appendUnique := func(x *Node) {
		for _, e := range newNext {
			if e == x {
				return
			}
		}
		newNext = append(newNext, x)
	}
	for _, nx := range n.Next {
		if nx == oldNode {
			for _, repl := range news {
				appendUnique(repl)
			}
		} else {
			appendUnique(nx)
		}
	}
	n.Next = newNext
	for _, nn := range news {
		found := false
		for _, s := range nn.Source {
			if s == n {
				found = true
				break
			}
		}
		if !found {
			nn.Source = append(nn.Source, n)
		}
	}
}

// isEarlyReturnGuardFold reports whether beforeNode is an if-guard (a ConditionStatement with
// exactly two successors) whose successor other than assignNode is a bare `return` terminal, and
// whose fold target does NOT flow back to the guard. This is the early-return-guard shape behind
// Bug M, where the to-be-deleted store sits on the guard's jump-target branch and the fall-through
// branch returns. Position-preserving rewiring is only safe (against loop reconstruction) in this
// narrow shape: a loop header also has a return on its exit branch, but its body flows back to the
// header (a back-edge), and the do-while normalization relies on the historical append order, so
// such loop conditions are excluded by the reachability check.
func isEarlyReturnGuardFold(beforeNode, assignNode, node *Node) bool {
	if beforeNode == nil || node == nil {
		return false
	}
	if _, ok := beforeNode.Statement.(*statements.ConditionStatement); !ok {
		return false
	}
	if len(beforeNode.Next) != 2 {
		return false
	}
	var sibling *Node
	for _, nx := range beforeNode.Next {
		if nx != assignNode {
			sibling = nx
		}
	}
	if sibling == nil {
		return false
	}
	if _, isReturn := sibling.Statement.(*statements.ReturnStatement); !isReturn {
		return false
	}
	// Exclude loop headers: if the fold target can reach the guard again, this is a back-edge.
	return !canReachNode(node, beforeNode)
}

// canReachNode reports whether target is reachable from start by following Next edges. It is used
// to detect loop back-edges during folding; the per-method node graph is small so the bounded walk
// is cheap.
func canReachNode(start, target *Node) bool {
	if start == nil || target == nil {
		return false
	}
	visited := map[*Node]bool{}
	stack := []*Node{start}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == nil || visited[cur] {
			continue
		}
		visited[cur] = true
		for _, nx := range cur.Next {
			if nx == target {
				return true
			}
			if !visited[nx] {
				stack = append(stack, nx)
			}
		}
	}
	return false
}

func NewNode(statement statements.Statement) *Node {
	return &Node{Statement: statement}
}
