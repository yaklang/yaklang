package alogrithm

import (
	"fmt"
	"testing"
)

type anNode struct {
	next []Node
	prev []Node
	data any
}

func (an *anNode) String() string {
	return fmt.Sprintf("anNode{%v}", an.data)
}
func NewAnNode(v any) *anNode {
	return &anNode{
		next: nil,
		prev: nil,
		data: v,
	}
}

func (a *anNode) Next() []Node {
	return a.next
}

func (a *anNode) Prev() []Node {
	return a.prev
}

func (a *anNode) Handler(*Node) any {
	return a.data
}

var _ Node = (*anNode)(nil)

// 给两个节点添加有向边
func AddEdge(a *anNode, b *anNode) {
	a.next = append(a.next, b)
	b.prev = append(b.prev, a)
}

func TestTarjan(t *testing.T) {
	rootNode := NewAnNode(1)
	seNode := NewAnNode(2)
	thrNode := NewAnNode(3)
	AddEdge(rootNode, seNode)
	AddEdge(seNode, rootNode)
	AddEdge(seNode, thrNode)

	result := Run(rootNode)
	// count := 1
	// for _, v := range result {
	// 	fmt.Println("scc_cnt: ", count)
	// 	for k, _ := range v.nodes {
	// 		fmt.Println("node: ", k)
	// 	}
	// 	count += 1
	// }

	// 图关系 scc1 3 scc2 1.2
	// root->sec
	// sec->root
	// sec->third

	var r SccResult = result

    scc1 := r.GetScc(thrNode)
	if (scc1.InNodes(rootNode) || scc1.InNodes(seNode)) {
		t.Error("scc count err")
	}

	if !scc1.InNodes(thrNode) {
		t.Error("scc count err")
	}

	scc2 := r.GetScc(rootNode)
    if !(scc2.InNodes(rootNode) && scc2.InNodes(seNode)) {
		t.Error("scc count err")
	}



	

}
