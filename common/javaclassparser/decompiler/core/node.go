package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/utils"
)

type Node struct {
	Id                  int
	Statement           statements.Statement
	Source              []*Node
	Next                []*Node
	TrueNode, FalseNode func() *Node
	JmpNode             *Node
	MergeNode           *Node
	IsCircle            bool
	IsMerge             bool
	IsIf                bool
	GetLoopEndNode      func() *Node
	SetLoopEndNode      func(*Node, *Node)
	ConditionNode       []*Node
	BreakNode           []*Node
	SwitchMergeNode     *Node
	CircleNodesSet      *utils.Set[*Node]
	IsInCircle          bool
	//CircleRoute         *SubNodeMap
	//PreNodeRoute          *SubNodeMap
	//AllPreNodeRoute       []*SubNodeMap
}

func (n *Node) RemoveAllSource() {
	for _, node := range n.Source {
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
func NewNode(statement statements.Statement) *Node {
	return &Node{Statement: statement}
}
