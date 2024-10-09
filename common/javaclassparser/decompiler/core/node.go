package core

type Node struct {
	Id                  int
	Statement           Statement
	Source              []*Node
	Next                []*Node
	TrueNode, FalseNode *Node
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
	for _, node := range n.Next {
		n.RemoveNext(node)
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
	if n.Id == 68 {
		print()
	}
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
func NewNode(statement Statement) *Node {
	return &Node{Statement: statement}
}
