package rewriter

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core"
	"github.com/yaklang/yaklang/common/utils"
)

type SubNodeMap struct {
	ConditionNode *core.Node
	Parent        []*SubNodeMap
	NodeMap       *utils.Set[*core.Node]
}

func (s *SubNodeMap) AddParent(nodeMap *SubNodeMap) {
	s.Parent = append(s.Parent, nodeMap)
}
func (s *SubNodeMap) Add(node *core.Node) {
	s.NodeMap.Add(node)
}
func (s *SubNodeMap) getMapList() []*SubNodeMap {
	list := []*SubNodeMap{}
	stack := utils.NewStack[*SubNodeMap]()
	stack.Push(s)
	for stack.Len() > 0 {
		current := stack.Pop()
		list = append(list, current)
		for _, n := range current.Parent {
			stack.Push(n)
		}
	}
	return list
}
func (s *SubNodeMap) GetFirstSameParentCondition(m *SubNodeMap) *core.Node {
	list := s.getMapList()
	parentMap := map[*core.Node]bool{}
	for _, n := range m.getMapList() {
		parentMap[n.ConditionNode] = true
	}
	for i := 0; i < len(list); i++ {
		if _, ok := parentMap[list[i].ConditionNode]; ok {
			if list[i].ConditionNode == nil {
				continue
			}
			return list[i].ConditionNode
		}
	}
	return nil
}
func (s *SubNodeMap) Has(node *core.Node) (*SubNodeMap, bool) {
	stack := utils.NewStack[*SubNodeMap]()
	stack.Push(s)
	for stack.Len() > 0 {
		current := stack.Pop()
		if current.NodeMap.Has(node) {
			return current, true
		}
		for _, n := range current.Parent {
			stack.Push(n)
		}
	}
	return nil, false
}
func NewRootSubNodeMap() *SubNodeMap {
	return &SubNodeMap{
		NodeMap: utils.NewSet[*core.Node](),
	}
}
func (s *SubNodeMap) NewChild(conditionNode *core.Node) *SubNodeMap {
	m := NewRootSubNodeMap()
	m.Parent = []*SubNodeMap{s}
	m.ConditionNode = conditionNode
	return m
}
func CheckIsPreNode(infoGetter func(node *core.Node) *NodeExtInfo, node, pre *core.Node) bool {
	return _CheckIsPreNode(utils.NewSet[*core.Node](), infoGetter, node, pre)
}
func _CheckIsPreNode(checked *utils.Set[*core.Node], infoGetter func(node *core.Node) *NodeExtInfo, node, pre *core.Node) bool {
	if checked.Has(node) {
		return false
	}
	checked.Add(node)
	if pre == node {
		return true
	}
	for _, nodeMap := range infoGetter(node).AllPreNodeMaps {
		if _, ok := nodeMap.Has(pre); ok {
			return true
		}
	}
	for _, nodeMap := range infoGetter(node).AllPreNodeMaps {
		if _CheckIsPreNode(checked, infoGetter, nodeMap.ConditionNode, pre) {
			return true
		}
	}
	return false
}
