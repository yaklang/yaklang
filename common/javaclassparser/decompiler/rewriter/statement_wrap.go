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
}

func NewRewriterContext() *RewriterContext {
	return &RewriterContext{
		checkPoint: map[int][]int{},
		ifChildSet: map[int]*utils.Set[*core.Node]{},
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
	core.WalkGraph[*core.Node](manager.RootNode, func(node *core.Node) ([]*core.Node, error) {
		for _, set := range manager.RewriterContext.ifChildSet {
			set.Add(node)
		}
		if _, ok := node.Statement.(*core.ConditionStatement); ok {
			if manager.RewriterContext.ifChildSet[node.Id] == nil {
				manager.RewriterContext.ifChildSet[node.Id] = utils.NewSet[*core.Node]()
				manager.RewriterContext.ifChildSet[node.Id].Add(node)
				return node.Next[1:], nil
			}
			manager.RewriterContext.ifChildSet[node.Id].Add(node)
		}
		return node.Next, nil
	})
	return manager
}

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
		result = append(result, current)
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
	rewritersOrder := []int{IfWriter}
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
				s.RewriterContext.checkPoint[key] = funk.Filter(flags, func(item int) bool {
					return item != flag
				}).([]int)
				err := rewriter.rewriterFunc(s, s.GetNodeById(key))
				if err != nil {
					return err
				}
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
