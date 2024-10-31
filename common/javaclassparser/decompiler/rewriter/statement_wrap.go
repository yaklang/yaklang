package rewriter

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"strings"
)

type StatementManager struct {
	currentNodeId       int
	RootNode            *core.Node
	PreNode             *core.Node
	idToNode            map[int]*core.Node
	CircleEntryPoint    []*core.Node
	WhileNode           []*core.Node
	UncertainBreakNodes [][2]*core.Node
	//MergePoint      []*core.Node
	IfNodes           []*core.Node
	RepeatNodeMap     map[*core.Node]*core.Node
	FinalActions      []func() error
	LoopOccupiedNodes *utils.Set[*core.Node]
	SwitchNode        []*core.Node
	TryNodes          []*core.Node
	nodeSet           *utils.Set[*core.Node]
	edgeSet           *utils.Set[[2]*core.Node]
	DominatorMap      map[*core.Node][]*core.Node
	LabelId           int
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
		RepeatNodeMap:     map[*core.Node]*core.Node{},
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
func (s *StatementManager) NewLoopLabel() string {
	s.LabelId++
	return fmt.Sprintf("LOOP_%d", s.LabelId)
}
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
	AllCircleRoute  []*NodeRoute
	CircleRoute     *utils.Set[*core.Node]
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
	s.DominatorMap = GenerateDominatorTree(s.RootNode)
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
	tryNodesSet := utils.NewSet[*core.Node]()

	var walkIfStatement func(node *core.Node, subNodeRoute *NodeRoute)
	walkIfStatement = func(node *core.Node, subNodeRoute *NodeRoute) {
		getNodeInfo(node).PreNodeRoute = subNodeRoute
		stack.Push(node)
		for stack.Len() > 0 {
			current := stack.Pop()
			getNodeInfo(current).AllPreNodeRoute = append(getNodeInfo(current).AllPreNodeRoute, subNodeRoute)
			if m, ok := getNodeInfo(node).PreNodeRoute.Has(current); ok {
				getNodeInfo(node).AllCircleRoute = append(getNodeInfo(node).AllCircleRoute, subNodeRoute)
				route, _ := getNodeInfo(node).PreNodeRoute.HasPre(current)
				if v := getNodeInfo(current).CircleRoute; v != nil {
					getNodeInfo(current).CircleRoute = v.Or(route)
				} else {
					getNodeInfo(current).CircleRoute = route
					core.WalkGraph[*core.Node](current, func(node *core.Node) ([]*core.Node, error) {
						route.Add(node)
						if len(node.Next) > 1 {
							return nil, nil
						}
						return node.Next, nil
					})
				}
				circleNodes = append(circleNodes, current)
				_ = m
				continue
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
			} else if v, ok := current.Statement.(*statements.MiddleStatement); ok && v.Flag == statements.MiddleSwitch {
				s.SwitchNode = append(s.SwitchNode, current)
				for _, n := range current.Next {
					newRoute := subNodeRoute.NewChild(current)
					newRoute.ConditionNode = nil
					newRoute.SwitchNode = current
					walkIfStatement(n, newRoute)
				}
				continue
			} else if v, ok := current.Statement.(*statements.MiddleStatement); ok && v.Flag == "tryStart" {
				tryNodesSet.Add(current)
				for _, n := range current.Next {
					newRoute := subNodeRoute.NewChild(current)
					newRoute.ConditionNode = nil
					newRoute.TryNode = current
					walkIfStatement(n, newRoute)
				}
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
	switchSet := utils.NewSet[*core.Node]()
	switchSet.AddList(s.SwitchNode)
	s.SwitchNode = switchSet.List()
	for _, node := range s.SwitchNode {
		caseItemMap := node.Statement.(*statements.MiddleStatement).Data.([]any)[0].(map[int]*core.Node)
		itemMap := map[*core.Node]struct{}{}
		for _, item := range caseItemMap {
			itemMap[item] = struct{}{}
		}
		for _, n := range s.DominatorMap[node] {
			if _, ok := itemMap[n]; !ok {
				node.SwitchMergeNode = n
				break
			}
		}
	}
	//for switchNode, nodes := range switchMergeNodeCandidates {
	//	caseMap := switchNode.Statement.(*statements.MiddleStatement).Data.([]any)[0].(map[int]*core.Node)
	//	allOk := false
	//	for _, node := range nodes {
	//		for _, route := range getNodeInfo(node).AllPreNodeRoute {
	//
	//		}
	//	}
	//}
	s.TryNodes = tryNodesSet.List()
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
	//		return CheckIsPreNode(getNodeInfo, node.LoopEndNode, n) && !CheckIsPreNode(getNodeInfo, node, n)
	//	}
	//}
	circleNodeEntryToOutPoint := map[*core.Node]map[*core.Node]*core.Node{}
	for _, circleNodeEntry := range circleNodes {
		outPointMap := map[*core.Node]*core.Node{}
		circleNodeEntry.CircleNodesSet = getNodeInfo(circleNodeEntry).CircleRoute
		core.WalkGraph(circleNodeEntry, func(node *core.Node) ([]*core.Node, error) {
			next := funk.Filter(node.Next, func(n *core.Node) bool {
				isInCircle := circleNodeEntry.CircleNodesSet.Has(n)
				if n == circleNodeEntry {
					isInCircle = true
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
		//if len(outPointMap) == 0 {
		//	return errors.New("invalid circle")
		//}
		tryNodes := funk.Filter(maps.Keys(outPointMap), func(node *core.Node) bool {
			if v, ok := node.Statement.(*statements.MiddleStatement); ok && v.Flag == "tryStart" {
				return true
			}
			return false
		}).([]*core.Node)
		for _, node := range tryNodes {
			delete(outPointMap, node)
		}
		circleNodeEntry.OutNodeMap = outPointMap
		circleNodeEntryToOutPoint[circleNodeEntry] = outPointMap
	}
	circleNodeEntryToOutPoint = nil
	for circleNodeEntry, outPointMap := range circleNodeEntryToOutPoint {
		var mergeNode *core.Node
		if len(outPointMap) == 0 {
		} else if len(outPointMap) == 1 {
			mergeNode = maps.Values(outPointMap)[0]
		} else {
			edgeSet := utils.NewSet[*core.Node]()
			values := maps.Values(outPointMap)
			values = funk.Filter(values, func(item *core.Node) bool {
				return item.Id > circleNodeEntry.Id
			}).([]*core.Node)
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
		for c, _ := range outPointMap {
			check := func(node *core.Node) bool {
				if _, ok := c.Statement.(*statements.ConditionStatement); ok {
					return true
				}
				return false
			}
			if check(c) {
				circleNodeEntry.ConditionNode = append(circleNodeEntry.ConditionNode, c)
			}
		}
		circleNodeEntry.GetLoopEndNode = func() *core.Node {
			return nil
		}
		circleNodeEntry.SetLoopEndNode = func(node *core.Node, node2 *core.Node) {

		}
		loopEndNodeMap := map[*core.Node]*core.Node{}
		if mergeNode != nil {
			loopEndNodeMap[mergeNode] = mergeNode
			circleNodeEntry.GetLoopEndNode = func() *core.Node {
				return loopEndNodeMap[mergeNode]
			}
			circleNodeEntry.SetLoopEndNode = func(node1, node2 *core.Node) {
				loopEndNodeMap[node1] = node2
			}
		}
	}
	//mergeNodeToCircleNode := map[*core.Node][]*core.Node{}
	//for circleNodeEntry, _ := range circleNodeEntryToOutPoint {
	//	if circleNodeEntry.MergeNode != nil {
	//		mergeNodeToCircleNode[circleNodeEntry.MergeNode] = append(mergeNodeToCircleNode[circleNodeEntry.MergeNode], circleNodeEntry)
	//	}
	//}
	//for mergeNode, circleNodes := range mergeNodeToCircleNode {
	//	if len(circleNodes) == 1 {
	//		continue
	//	}
	//	sort.Slice(circleNodes, func(i, j int) bool {
	//		return circleNodes[i].Id < circleNodes[j].Id
	//	})
	//	for _, node := range circleNodes {
	//
	//	}
	//}
	s.CircleEntryPoint = circleNodes
	s.IfNodes = ifNodes
	for _, node := range circleNodes {
		for _, c := range s.DominatorMap[node] {
			if !node.CircleNodesSet.Has(c) {
				node.LoopEndNode = c
			}
		}
	}
	return nil
}
func (s *StatementManager) Rewrite() error {
	err := s.ScanCoreInfo()
	if err != nil {
		return err
	}
	err = LoopRewriter1(s)
	if err != nil {
		return err
	}
	println(utils2.DumpNodesToDotExp(s.RootNode))
	s.DominatorMap = GenerateDominatorTree(s.RootNode)
	for _, ifNode := range s.IfNodes {
		ifNode.MergeNode = nil
		trueNode := ifNode.TrueNode()
		falseNode := ifNode.FalseNode()
		doms := s.DominatorMap[ifNode]
		switch len(doms) {
		case 1:
			ok1 := false
			err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == doms[0] {
					ok1 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
			ok2 := false
			err = core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
				if node == ifNode {
					return nil, nil
				}
				if node == doms[0] {
					ok2 = true
					return nil, nil
				}
				return node.Next, nil
			})
			if err != nil {
				return err
			}
			if ok1 && ok2 {
				ifNode.MergeNode = doms[0]
			}
		case 2:
			for _, dom := range doms {
				ok1 := false
				err := core.WalkGraph[*core.Node](trueNode, func(node *core.Node) ([]*core.Node, error) {
					if node == ifNode {
						return nil, nil
					}
					if node == dom {
						ok1 = true
						return nil, nil
					}
					return node.Next, nil
				})
				if err != nil {
					return err
				}
				ok2 := false
				err = core.WalkGraph[*core.Node](falseNode, func(node *core.Node) ([]*core.Node, error) {
					if node == ifNode {
						return nil, nil
					}
					if node == dom {
						ok2 = true
						return nil, nil
					}
					return node.Next, nil
				})
				if err != nil {
					return err
				}
				if ok1 && ok2 {
					ifNode.MergeNode = dom
					break
				}
			}
		case 3:
			ifNode.MergeNode = utils2.NodeFilter(doms, func(node *core.Node) bool {
				return node != trueNode && node != falseNode
			})[0]
		}
	}
	//s.DumpDominatorTree()
	err = SwitchRewriter(s)
	if err != nil {
		return err
	}
	whileNodes := []*core.Node{}
	core.WalkGraph[*core.Node](s.RootNode, func(node *core.Node) ([]*core.Node, error) {
		if slices.Contains(s.WhileNode,node){
			whileNodes = append(whileNodes,node)
		}
		return s.DominatorMap[node],nil
	})
	for i := len(whileNodes)-1; i >=0; i-- {
		err = LoopRewriter(s,whileNodes[i])
		if err != nil {
			return err
		}
	}

	println(utils2.DumpNodesToDotExp(s.RootNode))
	rewriters := []rewriterFunc{IfRewriter, TryRewriter, LabelRewriter}
	for _, rewriter := range rewriters {
		err := rewriter(s)
		if err != nil {
			return err
		}
	}
	//s.DominatorMap = GenerateDominatorTree(s.RootNode)
	for _, action := range s.FinalActions {
		err := action()
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *StatementManager) DumpDominatorTree() {
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	toString := func(node *core.Node) string {
		//return strconv.Quote(node.Statement.String(&ClassContext{}))
		s := strings.Replace(node.Statement.String(&class_context.ClassContext{}), "\"", "", -1)
		s = strings.Replace(s, "\n", " ", -1)
		return s
	}
	for node, dom := range s.DominatorMap {
		for _, n := range dom {
			sb.WriteString(fmt.Sprintf("\"%d%s\" -> \"%d%s\"\n", n.Id, toString(n), node.Id, toString(node)))
		}
	}
	sb.WriteString("}\n")
	println(sb.String())
}
