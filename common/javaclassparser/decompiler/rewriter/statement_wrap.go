package rewriter

import (
	"errors"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
)

type GuessIsBreak func(getPoint func() int, resolve func(), reject func())
type RewriterContext struct {
	labelId       int
	currentNodeId int
	checkPoint    map[int][]int
	ifChildSet    map[int]*utils.Set[*core.Node]
	BlockStack    *utils.Stack[any]
}

func NewRewriterContext() *RewriterContext {
	return &RewriterContext{
		checkPoint: map[int][]int{},
		ifChildSet: map[int]*utils.Set[*core.Node]{},
		BlockStack: utils.NewStack[any](),
	}
}

type StatementManager struct {
	RewriterContext *RewriterContext
	RootNode        *core.Node
	PreNode         *core.Node
	idToNode        map[int]*core.Node
}

func NewStatementManager(node *core.Node, parent *StatementManager) *StatementManager {
	manager := NewRootStatementManager(node)
	manager.RewriterContext = parent.RewriterContext
	return manager
}

func NewRootStatementManager(node *core.Node) *StatementManager {
	if node == nil {
		return nil
	}
	manager := &StatementManager{
		RewriterContext: NewRewriterContext(),
		RootNode:        node,
		idToNode:        map[int]*core.Node{},
	}
	manager.generateIdToNodeMap()
	manager.ScanStatementSimple(func(node *core.Node) error {
		for k, rewriter := range rewriters {
			if rewriter.checkStartNode(node, manager) {
				manager.RewriterContext.checkPoint[node.Id] = append(manager.RewriterContext.checkPoint[node.Id], k)
			}
		}
		return nil
	})
	//manager.GenerateIfChildSet()
	return manager
}

//	func (s *StatementManager) GenerateIfChildSet() {
//		stack := utils.NewStack[*core.Node]()
//		visited := utils.NewSet[any]()
//		stack.Push(s.RootNode)
//		for stack.Len() > 0 {
//			current := stack.Pop()
//			if visited.Has(current) {
//				continue
//			}
//			visited.Add(current)
//			if _, ok := current.Statement.(*core.ConditionStatement); ok {
//				s.RewriterContext.ifChildSet[current.Id] = utils.NewSet[*core.Node]()
//				stack.Push(current.Next[1])
//				stack.Push(current.Next[0])
//			} else {
//				for _, n := range current.Next {
//					stack.Push(n)
//				}
//			}
//		}
//		return
//	}
func (s *StatementManager) SetId(id int) {
	s.RewriterContext.currentNodeId = id
}
func (s *StatementManager) GetNewNodeId() int {
	s.RewriterContext.currentNodeId++
	return s.RewriterContext.currentNodeId
}
func (s *StatementManager) SetRootNode(node *core.Node) {
	s.RootNode = node
	s.generateIdToNodeMap()
}
func (s *StatementManager) GetNodeById(id int) *core.Node {
	return s.idToNode[id]
}

var n = 0

func (s *StatementManager) ScanStatementSimple(handle func(node *core.Node) error) error {
	return s.ScanStatement(func(node *core.Node) (error, bool) {
		n++
		if n > 10000 {
			print()
		}
		err := handle(node)
		if err != nil {
			return err, false
		}
		return nil, true
	})
}
func (s *StatementManager) ScanStatement(handle func(node *core.Node) (error, bool)) error {
	err := core.WalkGraph[*core.Node](s.RootNode, func(node *core.Node) ([]*core.Node, error) {
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

//func (s *StatementManager) AppendStatement(statement Statement) *core.Node {
//	defer s.generateIdToNodeMap()
//	node := NewNode(statement)
//	s.Nodes = append(s.Nodes, node)
//	s.idToNodeIndex[node.Id] = len(s.Nodes) - 1
//	return node
//}

func (s *StatementManager) ToStatements(stopCheck func(node *core.Node) bool) ([]*core.Node, error) {
	var ErrMultipleNext = errors.New("multiple next")
	var ErrHasCircle = errors.New("has circle")
	result := []*core.Node{}
	current := s.RootNode
	visited := utils.NewSet[*core.Node]()
	for {
		if current == nil {
			break
		}
		if visited.Has(current) {
			return nil, ErrHasCircle
		}
		if !stopCheck(current) {
			break
		}
		visited.Add(current)
		if _, ok := current.Statement.(*core.MiddleStatement); !ok {
			result = append(result, current)
		}
		if len(current.Next) == 0 {
			break
		}
		if current == s.PreNode {
			break
		}
		if len(current.Next) > 1 {
			return nil, ErrMultipleNext
		}
		current = current.Next[0]
	}
	return result, nil
}
func (s *StatementManager) InsertStatementAfterId(id int, statement core.Statement) {
	defer s.generateIdToNodeMap()
	preNode := s.GetNodeById(id)
	node := core.NewNode(statement)
	node.Source = append(node.Source, preNode)
	node.Next = preNode.Next
	preNode.Next = []*core.Node{node}
}
func (s *StatementManager) DeleteStatementById(id int) {
	defer s.generateIdToNodeMap()
	deletedNode := s.GetNodeById(id)
	for _, node := range deletedNode.Source {
		node.Next = funk.Filter(node.Next, func(item *core.Node) bool {
			return item != deletedNode
		}).([]*core.Node)
		node.Next = append(node.Next, deletedNode.Next...)
	}
	for _, node := range deletedNode.Next {
		node.Source = funk.Filter(node.Source, func(item *core.Node) bool {
			return item != deletedNode
		}).([]*core.Node)
		node.Source = append(node.Source, deletedNode.Source...)
	}
}

func (s *StatementManager) generateIdToNodeMap() {
	s.ScanStatementSimple(func(node *core.Node) error {
		s.idToNode[node.Id] = node
		return nil
	})
}
func (s *StatementManager) Rewrite() error {
	rewritersOrder := []int{LoopRewriterFlag, DoWhileReWriterFlag, IfRewriterFlag, BreakRewriterFlag}
	//visited := utils.NewSet[any]()
	for _, flag := range rewritersOrder {
		rewriter, ok := rewriters[flag]
		if !ok {
			continue
		}
		keys := maps.Keys(s.RewriterContext.checkPoint)
		keys = funk.Filter(keys, func(item int) bool {
			return item >= s.RootNode.Id
		}).([]int)
		for _, key := range keys {
			flags := s.RewriterContext.checkPoint[key]
			if utils.IntArrayContains(flags, flag) {
				node := s.GetNodeById(key)
				if node == nil {
					continue
				}
				err := rewriter.rewriterFunc(s, node)
				if err != nil {
					return err
				}
				s.RewriterContext.checkPoint[key] = funk.Filter(flags, func(item int) bool {
					return item != flag
				}).([]int)
			}
		}
	}
	//err := s.ScanStatement(func(node *core.Node) (error, bool) {
	//	if visited.Has(node) {
	//		return nil, false
	//	}
	//	visited.Add(node)
	//	s.PreNode = node
	//	for _, rewriter := range rewriters {
	//		err := rewriter(s, node)
	//		if err != nil {
	//			return err, true
	//		}
	//	}
	//	if !stopCheck(node) {
	//		return nil, false
	//	}
	//	return nil, true
	//})
	//if err != nil {
	//	return err
	//}
	return nil
}
