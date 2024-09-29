package core

type Node struct {
	Id        int
	Statement Statement
	Source    []*Node
	Next      []*Node
}

func (n *Node) AddSource(node *Node) {
	n.Source = append(n.Source, node)
}
func (n *Node) AddNext(node *Node) {
	n.Next = append(n.Next, node)
}
func NewNode(statement Statement) *Node {
	return &Node{Statement: statement}
}
