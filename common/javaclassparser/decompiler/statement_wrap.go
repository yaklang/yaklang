package decompiler

import (
	"errors"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/utils"
)

type Node struct {
	Statement Statement
	Id        int
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

type GuessIsBreak func(getPoint func() int, resolve func(), reject func())
type RewriterContext struct {
	Stack     *utils.Stack[any]
	LoopStack *utils.Stack[any]
}

func NewRewriterContext() *RewriterContext {
	return &RewriterContext{
		Stack:     utils.NewStack[any](),
		LoopStack: utils.NewStack[any](),
	}
}

type StatementManager struct {
	RewriterContext *RewriterContext
	RootNode        *Node
	PreNode         *Node
	idToNode        map[int]*Node
}

func NewStatementManager(node *Node) *StatementManager {
	if node == nil {
		return nil
	}
	manager := &StatementManager{
		RewriterContext: NewRewriterContext(),
		RootNode:        node,
		idToNode:        map[int]*Node{},
	}
	manager.generateIdToNodeMap()
	return manager
}

func (s *StatementManager) SetRootNode(node *Node) {
	s.RootNode = node
	s.generateIdToNodeMap()
}
func (s *StatementManager) GetNodeById(id int) *Node {
	return s.idToNode[id]
}

func (s *StatementManager) ScanStatementSimple(handle func(node *Node) error) error {
	return s.ScanStatement(func(node *Node) (error, bool) {
		err := handle(node)
		if err != nil {
			return err, false
		}
		return nil, true
	})
}
func (s *StatementManager) ScanStatement(handle func(node *Node) (error, bool)) error {
	err := WalkGraph[*Node](s.RootNode, func(node *Node) ([]*Node, error) {
		err, ok := handle(node)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		return node.Next, nil
	})
	return err
}

//func (s *StatementManager) AppendStatement(statement Statement) *Node {
//	defer s.generateIdToNodeMap()
//	node := NewNode(statement)
//	s.Nodes = append(s.Nodes, node)
//	s.idToNodeIndex[node.Id] = len(s.Nodes) - 1
//	return node
//}

func (s *StatementManager) ToStatements() ([]Statement, error) {
	var ErrMultipleNext = errors.New("multiple next")
	_ = ErrMultipleNext
	statements := []Statement{}
	current := s.RootNode
	for {
		if current == nil {
			break
		}
		statements = append(statements, current.Statement)
		if len(current.Next) == 0 {
			break
		}
		if current == s.PreNode {
			break
		}
		//if len(current.Next) > 1 {
		//	return nil, ErrMultipleNext
		//}
		current = current.Next[0]
	}
	return statements, nil
}
func (s *StatementManager) InsertStatementAfterId(id int, statement Statement) {
	defer s.generateIdToNodeMap()
	preNode := s.GetNodeById(id)
	node := NewNode(statement)
	node.Source = append(node.Source, preNode)
	node.Next = preNode.Next
	preNode.Next = []*Node{node}
}
func (s *StatementManager) DeleteStatementById(id int) {
	defer s.generateIdToNodeMap()
	deletedNode := s.GetNodeById(id)
	for _, node := range deletedNode.Source {
		node.Next = funk.Filter(node.Next, func(item *Node) bool {
			return item != deletedNode
		}).([]*Node)
		node.Next = append(node.Next, deletedNode.Next...)
	}
	for _, node := range deletedNode.Next {
		node.Source = funk.Filter(node.Source, func(item *Node) bool {
			return item != deletedNode
		}).([]*Node)
		node.Source = append(node.Source, deletedNode.Source...)
	}
}
func (s *StatementManager) generateIdToNodeMap() {
	s.ScanStatementSimple(func(node *Node) error {
		s.idToNode[node.Id] = node
		return nil
	})
}
func (s *StatementManager) Rewrite(stopCheck func(node *Node) bool) error {
	rewriters := []Rewriter{
		RewriteIf,
		SwitchRewriter,
		SynchronizedRewriter,
	}
	err := s.ScanStatement(func(node *Node) (error, bool) {
		s.PreNode = node
		for _, rewriter := range rewriters {
			err := rewriter(s, node)
			if err != nil {
				return err, true
			}
		}
		if !stopCheck(node) {
			return nil, false
		}
		return nil, true
	})
	if err != nil {
		return err
	}
	return nil
}
