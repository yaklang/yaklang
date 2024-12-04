package rewriter

import (
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	utils2 "github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"strings"
)

type RewriteManager struct {
	currentNodeId    int
	RootNode         *core.Node
	PreNode          *core.Node
	CircleEntryPoint []*core.Node
	WhileNode        []*core.Node
	IfNodes          []*core.Node
	SwitchNode       []*core.Node
	TryNodes         []*core.Node
	DominatorMap     map[*core.Node][]*core.Node
	LabelId          int
	visitedNodeSet   *utils.Set[*core.Node]
}

func NewRootStatementManager(node *core.Node) *RewriteManager {
	if node == nil {
		return nil
	}
	manager := &RewriteManager{
		RootNode:       node,
		visitedNodeSet: utils.NewSet[*core.Node](),
	}
	return manager
}

func (s *RewriteManager) CheckVisitedNode(node *core.Node) error {
	if s.visitedNodeSet.Has(node) {
		return fmt.Errorf("current node %d has been visited", node.Id)
	}
	s.visitedNodeSet.Add(node)
	return nil
}
func (s *RewriteManager) NewLoopLabel() string {
	s.LabelId++
	return fmt.Sprintf("LOOP_%d", s.LabelId)
}

func (s *RewriteManager) MergeIf() {
	ifNodes := utils2.NodeFilter(WalkNodeToList(s.RootNode), func(node *core.Node) bool {
		_, ok := node.Statement.(*statements.ConditionStatement)
		return ok
	})
	ifNodes = NodeDeduplication(ifNodes)
	delNodesSet := utils.NewSet[*core.Node]()
	for _, node := range ifNodes {
		if delNodesSet.Has(node) {
			continue
		}
		for i, n := range node.Next {
			if slices.Contains(ifNodes, n) {
				parentNode := node
				childNode := n
				if parentNode.Id >= childNode.Id {
					continue
				}
				if !slices.Contains(childNode.Next, node.Next[1-i]) {
					continue
				}
				s.DominatorMap = GenerateDominatorTree(s.RootNode)
				CalcEnd(s.DominatorMap, parentNode)
				CalcEnd(s.DominatorMap, childNode)
				if parentNode.MergeNode != nil && parentNode.MergeNode == childNode.MergeNode {
					if parentNode.TrueNode() == parentNode.Next[1-i] {
						if parentNode.TrueNode() == childNode.TrueNode() {
							ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
							ifStat2 := childNode.Statement.(*statements.ConditionStatement)
							ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, ifStat2.Condition, "||", types.NewJavaPrimer(types.JavaBoolean))
							parentNode.Next[i] = childNode.FalseNode()
							parentNode.Next[i].RemoveSource(childNode)
							parentNode.Next[i].AddSource(parentNode)
							delNodesSet.Add(childNode)
						} else {
							ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
							ifStat2 := childNode.Statement.(*statements.ConditionStatement)
							ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
								return fmt.Sprintf("!%s", ifStat2.Condition.String(funcCtx))
							}, func() types.JavaType {
								return types.NewJavaPrimer(types.JavaBoolean)
							}), "&&", types.NewJavaPrimer(types.JavaBoolean))
							parentNode.Next[i] = childNode.TrueNode()
							parentNode.Next[i].RemoveSource(childNode)
							parentNode.Next[i].AddSource(parentNode)
							delNodesSet.Add(childNode)
						}
					} else {
						if parentNode.FalseNode() == childNode.FalseNode() {
							ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
							ifStat2 := childNode.Statement.(*statements.ConditionStatement)
							ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, ifStat2.Condition, "||", types.NewJavaPrimer(types.JavaBoolean))
							parentNode.Next[i] = childNode.TrueNode()
							parentNode.Next[i].RemoveSource(childNode)
							parentNode.Next[i].AddSource(parentNode)
							delNodesSet.Add(childNode)
						} else {
							ifStat1 := parentNode.Statement.(*statements.ConditionStatement)
							ifStat2 := childNode.Statement.(*statements.ConditionStatement)
							ifStat1.Condition = values.NewBinaryExpression(ifStat1.Condition, values.NewCustomValue(func(funcCtx *class_context.ClassContext) string {
								return fmt.Sprintf("!%s", ifStat2.Condition.String(funcCtx))
							}, func() types.JavaType {
								return types.NewJavaPrimer(types.JavaBoolean)
							}), "&&", types.NewJavaPrimer(types.JavaBoolean))
							parentNode.Next[i] = childNode.FalseNode()
							parentNode.Next[i].RemoveSource(childNode)
							parentNode.Next[i].AddSource(parentNode)
							delNodesSet.Add(childNode)
						}
					}
				}
			}
		}
	}
}
func (s *RewriteManager) SetId(id int) {
	s.currentNodeId = id
}
func (s *RewriteManager) NewNode(st statements.Statement) *core.Node {
	node := core.NewNode(st)
	node.Id = s.GetNewNodeId()
	return node
}
func (s *RewriteManager) GetNewNodeId() int {
	s.currentNodeId++
	return s.currentNodeId
}
func (s *RewriteManager) ScanStatementSimple(handle func(node *core.Node) error) error {
	return s.ScanStatement(func(node *core.Node) (error, bool) {
		err := handle(node)
		if err != nil {
			return err, false
		}
		return nil, true
	})
}
func (s *RewriteManager) ScanStatement(handle func(node *core.Node) (error, bool)) error {
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

func (s *RewriteManager) ToStatementsFromNode(node *core.Node, stopCheck func(node *core.Node) bool) ([]*core.Node, error) {
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
		err := s.CheckVisitedNode(current)
		if err != nil {
			return nil, err
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
		if len(current.Next) > 1 {
			return nil, ErrMultipleNext
		}
		current = current.Next[0]
	}
	return result, nil
}
func (s *RewriteManager) ToStatements(stopCheck func(node *core.Node) bool) ([]*core.Node, error) {
	return s.ToStatementsFromNode(s.RootNode, stopCheck)
}

type NodeExtInfo struct {
	PreNodeRoute    *NodeRoute
	AllPreNodeRoute []*NodeRoute
	AllCircleRoute  []*NodeRoute
	CircleRoute     *utils.Set[*core.Node]
}

func (s *RewriteManager) ScanCoreInfo() error {
	s.DominatorMap = GenerateDominatorTree(s.RootNode)
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
			} else if v, ok := current.Statement.(*statements.MiddleStatement); ok && v.Flag == statements.MiddleTryStart {
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
	//for _, node := range s.SwitchNode {
	//	caseItemMap := node.Statement.(*statements.MiddleStatement).Data.([]any)[0].(map[int]*core.Node)
	//	itemMap := map[*core.Node]struct{}{}
	//	for _, item := range caseItemMap {
	//		itemMap[item] = struct{}{}
	//	}
	//	for _, n := range s.DominatorMap[node] {
	//		if _, ok := itemMap[n]; !ok {
	//			node.SwitchMergeNode = n
	//			break
	//		}
	//	}
	//}
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
			if v, ok := node.Statement.(*statements.MiddleStatement); ok && v.Flag == statements.MiddleTryStart {
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
	for circleNodeEntry, outPointMap := range circleNodeEntryToOutPoint {
		var mergeNode *core.Node
		edgeSet := utils.NewSet[*core.Node]()
		values := maps.Values(outPointMap)
		if len(values) == 0 {
		} else if len(values) == 1 {
			mergeNode = values[0]
		} else {
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
	//for _, node := range circleNodes {
	//	for _, c := range s.DominatorMap[node] {
	//		if !node.CircleNodesSet.Has(c) {
	//			node.LoopEndNode = c
	//		}
	//	}
	//}
	return nil
}
func (s *RewriteManager) Rewrite() error {
	err := s.ScanCoreInfo()
	if err != nil {
		return err
	}
	err = RebuildLoopNode(s)
	if err != nil {
		return err
	}
	s.DominatorMap = GenerateDominatorTree(s.RootNode)
	nodeToRewriter := map[*core.Node]rewriterFunc{}
	keyNodes := []*core.Node{}
	for _, node := range s.WhileNode {
		nodeToRewriter[node] = LoopRewriter
		keyNodes = append(keyNodes, node)
	}

	for _, node := range s.SwitchNode {
		nodeToRewriter[node] = SwitchRewriter
		keyNodes = append(keyNodes, node)
	}
	for _, node := range s.IfNodes {
		nodeToRewriter[node] = IfRewriter
		keyNodes = append(keyNodes, node)
	}
	for _, node := range s.TryNodes {
		nodeToRewriter[node] = TryRewriter
		keyNodes = append(keyNodes, node)
	}
	for _, node := range s.TopologicalSortReverse(s.SwitchNode) {
		err := SwitchRewriter1(s, node)
		if err != nil {
			return err
		}
		s.DominatorMap = GenerateDominatorTree(s.RootNode)
	}
	order := s.TopologicalSortReverse(keyNodes)
	loopJmpRewriterRecoed := map[*core.Node]struct{}{}
	for i := 0; i < len(order); i++ {
		s.DominatorMap = GenerateDominatorTree(s.RootNode)
		node := order[i]
		if slices.Contains(s.IfNodes, node) {
			for j := i; j < len(order); j++ {
				n := order[j]
				if slices.Contains(s.WhileNode, n) && utils2.IsDominate(s.DominatorMap, n, node) {
					if _, ok := loopJmpRewriterRecoed[n]; ok {
						break
					}
					err := LoopJmpRewriter(s, n)
					if err != nil {
						return err
					}
					loopJmpRewriterRecoed[n] = struct{}{}
					s.DominatorMap = GenerateDominatorTree(s.RootNode)
					break
				}
			}
		}
		err := nodeToRewriter[node](s, node)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *RewriteManager) TopologicalSortReverse(nodes []*core.Node) []*core.Node {
	order := []*core.Node{}
	nodesMap := map[*core.Node]struct{}{}
	for _, node := range nodes {
		nodesMap[node] = struct{}{}
	}
	core.WalkGraph[*core.Node](s.RootNode, func(node *core.Node) ([]*core.Node, error) {
		if _, ok := nodesMap[node]; ok {
			order = append(order, node)
		}
		return s.DominatorMap[node], nil
	})
	slices.Reverse(order)
	return order
}
func (s *RewriteManager) DumpDominatorTree() {
	var sb strings.Builder
	sb.WriteString("digraph G {\n")
	toString := func(node *core.Node) string {
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
	// println(sb.String())
}
