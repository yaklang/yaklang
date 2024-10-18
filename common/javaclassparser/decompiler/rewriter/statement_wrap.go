package rewriter

import (
	"errors"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
)

type StatementManager struct {
	currentNodeId int
	RootNode      *core.Node
	PreNode       *core.Node
	idToNode      map[int]*core.Node
	CirclePoint   []*core.Node
	//MergePoint      []*core.Node
	IfNodes           []*core.Node
	FinalActions      []func() error
	LoopOccupiedNodes *utils.Set[*core.Node]

	nodeSet *utils.Set[*core.Node]
	edgeSet *utils.Set[[2]*core.Node]
}

func NewStatementManager(node *core.Node, parent *StatementManager) *StatementManager {
	manager := NewRootStatementManager(node)
	return manager
}

func NewRootStatementManager(node *core.Node) *StatementManager {
	if node == nil {
		return nil
	}
	manager := &StatementManager{
		RootNode:          node,
		idToNode:          map[int]*core.Node{},
		LoopOccupiedNodes: utils.NewSet[*core.Node](),
	}
	manager.generateIdToNodeMap()
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
	s.currentNodeId = id
}
func (s *StatementManager) NewNode(st statements.Statement) *core.Node {
	node := core.NewNode(st)
	node.Id = s.GetNewNodeId()
	return node
}
func (s *StatementManager) GetNewNodeId() int {
	s.currentNodeId++
	return s.currentNodeId
}
func (s *StatementManager) SetRootNode(node *core.Node) {
	s.RootNode = node
	s.generateIdToNodeMap()
}
func (s *StatementManager) AddFinalAction(f func() error) {
	s.FinalActions = append(s.FinalActions, f)
}
func (s *StatementManager) GetNodeById(id int) *core.Node {
	return s.idToNode[id]
}
func (s *StatementManager) ScanStatementSimple(handle func(node *core.Node) error) error {
	return s.ScanStatement(func(node *core.Node) (error, bool) {
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

func (s *StatementManager) ToStatementsFromNode(node *core.Node, stopCheck func(node *core.Node) bool) ([]*core.Node, error) {
	var ErrMultipleNext = errors.New("multiple next")
	var ErrHasCircle = errors.New("has circle")
	result := []*core.Node{}
	current := node
	visited := utils.NewSet[*core.Node]()
	for {
		if current == nil {
			break
		}
		if visited.Has(current) {
			return nil, ErrHasCircle
		}
		if stopCheck != nil && !stopCheck(current) {
			break
		}
		visited.Add(current)
		if _, ok := current.Statement.(*statements.MiddleStatement); !ok {
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
func (s *StatementManager) ToStatements(stopCheck func(node *core.Node) bool) ([]*core.Node, error) {
	return s.ToStatementsFromNode(s.RootNode, stopCheck)
}
func (s *StatementManager) InsertStatementAfterId(id int, statement statements.Statement) {
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

type NodeExtInfo struct {
	PreNodeRoute    *NodeRoute
	AllPreNodeRoute []*NodeRoute
}
type Block struct {
	StartNode *core.Node
	EndNode   *core.Node
	Source    []*Block
}

func NewBlock(startNode *core.Node) *Block {
	return &Block{
		StartNode: startNode,
	}
}

//	func (s *StatementManager) scanIf() error {
//		edgeSet := utils.NewSet[[2]*core.Node]()
//		stack := utils.NewStack[[2]*core.Node]()
//		stack.Push([2]*core.Node{nil, s.RootNode})
//		for {
//			if stack.Len() == 0 {
//				break
//			}
//			edge := stack.Pop()
//			if edgeSet.Has(edge) {
//				continue
//			}
//			edgeSet.Add(edge)
//			_, to := edge[0], edge[1]
//			for _, n := range to.Next {
//				stack.Push([2]*core.Node{to, n})
//			}
//		}
//		return nil
//	}
func (s *StatementManager) ScanCoreInfo() error {
	//err := s.scanIf()
	//if err != nil {
	//	return err
	//}
	nodeExtInfo := map[*core.Node]*NodeExtInfo{}
	getNodeInfo := func(node *core.Node) *NodeExtInfo {
		if info, ok := nodeExtInfo[node]; ok {
			return info
		}
		info := &NodeExtInfo{}
		nodeExtInfo[node] = info
		return info
	}
	stack := utils.NewStack[*core.Node]()
	//visited := NewRootNodeRoute()
	circleNodes := []*core.Node{}
	ifNodes := []*core.Node{}
	mergeNodesSet := utils.NewSet[*core.Node]()

	var walkIfStatement func(node *core.Node, subNodeRoute *NodeRoute)
	walkIfStatement = func(node *core.Node, subNodeRoute *NodeRoute) {
		getNodeInfo(node).PreNodeRoute = subNodeRoute
		stack.Push(node)
		for stack.Len() > 0 {
			current := stack.Pop()
			getNodeInfo(current).AllPreNodeRoute = append(getNodeInfo(current).AllPreNodeRoute, subNodeRoute)
			if m, ok := getNodeInfo(node).PreNodeRoute.Has(current); ok {
				current.IsCircle = true
				circleNodes = append(circleNodes, current)
				_ = m
				//continue
			}
			skip := len(getNodeInfo(current).AllPreNodeRoute) > 1
			getNodeInfo(node).PreNodeRoute.Add(current)
			if skip {
				mergeNodesSet.Add(current)
				current.IsMerge = true
				continue
			}

			if _, ok := current.Statement.(*statements.ConditionStatement); ok {
				current.IsIf = true
				ifNodes = append(ifNodes, current)
				for _, n := range current.Next {
					walkIfStatement(n, subNodeRoute.NewChild(current))
				}
				continue
			} else {
				for _, n := range current.Next {
					stack.Push(n)
				}
			}
		}
	}
	subNodeRoute := NewRootNodeRoute()
	walkIfStatement(s.RootNode, subNodeRoute)
	circleNodes = utils.NewSet[*core.Node](circleNodes).List()
	//for _, node := range circleNodes {
	//	//mergeNode := funk.Filter(node.Next, func(item *core.Node) bool {
	//	//	return !node.CircleNodesSet.Has(item)
	//	//}).([]*core.Node)
	//	node.MergeNode = node.FalseNode()
	//}

	for _, current := range mergeNodesSet.List() {
		for _, nodeMap := range getNodeInfo(current).AllPreNodeRoute {
			if nodeMap.ConditionNode == nil {
				continue
			}
			if nodeMap.ConditionNode.MergeNode != nil {
				continue
			}
			checkNode := []*core.Node{nodeMap.ConditionNode.TrueNode(), nodeMap.ConditionNode.FalseNode()}
			isPreNode := true
			for _, node := range checkNode {
				isPreNode = CheckIsPreNode(getNodeInfo, current, node) && isPreNode
			}
			if isPreNode {
				nodeMap.ConditionNode.MergeNode = current
			}
		}
	}
	//for _, node := range circleNodes {
	//	node := node
	//	node.InCircle = func(n *core.Node) bool {
	//		return CheckIsPreNode(getNodeInfo, node.OutPointMergeNode, n) && !CheckIsPreNode(getNodeInfo, node, n)
	//	}
	//}
	for _, circleNodeEntry := range circleNodes {
		outPointMap := map[*core.Node]*core.Node{}
		circleNodeEntry.CircleNodesSet = utils.NewSet[*core.Node]()
		inCircleSource := []*core.Node{}
		for _, node := range circleNodeEntry.Source {
			for _, n := range circleNodeEntry.Next {
				if CheckIsPreNode(getNodeInfo, node, n) {
					inCircleSource = append(inCircleSource, node)
					break
				}
			}
		}

		core.WalkGraph(circleNodeEntry, func(node *core.Node) ([]*core.Node, error) {
			next := funk.Filter(node.Next, func(n *core.Node) bool {
				var isInCircle bool
				for _, node := range inCircleSource {
					for _, route := range getNodeInfo(node).AllPreNodeRoute {
						if _, ok := route.Has(n); ok {
							isInCircle = true
						}
					}
					if isInCircle {
						break
					}
				}
				if !isInCircle {
					outPointMap[node] = n
					//outPoint = append(outPoint, n)
				}
				return isInCircle
			}).([]*core.Node)
			circleNodeEntry.CircleNodesSet.Add(node)
			return next, nil
		})
		if len(outPointMap) == 0 {
			return errors.New("invalid circle")
		}
		//mergeNode := outPointMap[circleNodeEntry]
		var mergeNode *core.Node
		if len(outPointMap) == 1 {
			mergeNode = maps.Values(outPointMap)[0]
		} else {
			edgeSet := utils.NewSet[*core.Node]()
			values := maps.Values(outPointMap)
			core.WalkGraph[*core.Node](values[0], func(node *core.Node) ([]*core.Node, error) {
				edgeSet.Add(node)
				return node.Next, nil
			})
			core.WalkGraph[*core.Node](values[1], func(node *core.Node) ([]*core.Node, error) {
				if edgeSet.Has(node) {
					mergeNode = node
					return nil, nil
				}
				return node.Next, nil
			})
		}

		//for _, node1 := range outPointMap {
		//	ok := true
		//	for c, node2 := range outPointMap {
		//		if node1 == node2 {
		//			continue
		//		}
		//		if !CheckIsPreNode(getNodeInfo, node1, c) {
		//			ok = false
		//			break
		//		}
		//	}
		//	if ok {
		//		mergeNode = node1
		//	}
		//}
		if mergeNode == nil {
			return errors.New("invalid circle")
		}
		for c, _ := range outPointMap {
			circleNodeEntry.ConditionNode = append(circleNodeEntry.ConditionNode, c)
		}
		//var outPointMergeNode *core.Node
		//for conditionNode, node := range outPointMap {
		//	if outPointMergeNode != nil {
		//		if outPointMergeNode != node {
		//			return errors.New("invalid break")
		//		}
		//	} else {
		//		outPointMergeNode = node
		//	}
		//	circleNodeEntry.ConditionNode = append(circleNodeEntry.ConditionNode, conditionNode)
		//}
		//if outPointMergeNode == nil {
		//	return utils.Errorf("found circle break node from code graph failed, circle start node id: %d", circleNodeEntry.Id)
		//}
		circleNodeEntry.OutPointMergeNode = mergeNode
	}
	s.CirclePoint = circleNodes
	s.IfNodes = funk.Filter(ifNodes, func(item *core.Node) bool {
		return item.IsIf && item.IsCircle == false
	}).([]*core.Node)
	//for _, node := range s.IfNodes {
	//	if node.MergeNode == nil {
	//		return utils.Errorf("if node merge node is nil, node id: %d", node.Id)
	//	}
	//}
	return nil
}
func (s *StatementManager) Rewrite() error {
	err := s.ScanCoreInfo()
	if err != nil {
		return err
	}
	rewriters := []rewriterFunc{LoopRewriter, IfRewriter}
	for _, rewriter := range rewriters {
		rewriter(s)
	}
	for _, action := range s.FinalActions {
		err := action()
		if err != nil {
			return err
		}
	}
	return nil
}
