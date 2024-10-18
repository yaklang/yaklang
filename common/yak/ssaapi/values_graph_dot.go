package ssaapi

import (
	"fmt"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/utils/dot"
)

type SSAValueMappingGraph struct {
	*dot.Graph

	// one ssa.value can be create many ssadb.GraphValue,
	// so we use SSA-ID(int64) to graph node-id
	Value2Node map[int64]int

	// many ssadb.GraphValue can contain different context, even this is same ssa.value
	// use this map contain node-id to marshaled ssadb.GraphValue
	// in same node-id, that mean this ssadb.GraphValue is same ssa.value
	// ! this field just use in graph build
	marshaledValue map[int]map[ssadb.GraphValue]struct{} // node-id ->  ssadb.GraphValue

	// graph node id to value, this value just use bare ssadb.GraphValue
	Node2Value map[int]ssadb.GraphValue
}

func CreateDotGraph(graphValues ...ssadb.GraphValue) *SSAValueMappingGraph {
	graph := dot.New()
	graph.MakeDirected()
	graph.GraphAttribute("rankdir", "BT")
	g := &SSAValueMappingGraph{
		Graph:          graph,
		Value2Node:     make(map[int64]int),
		marshaledValue: make(map[int]map[ssadb.GraphValue]struct{}),
		Node2Value:     make(map[int]ssadb.GraphValue),
	}
	for _, node := range graphValues {
		// log.Infof("start graph %v", value.GetVerboseName())
		g.CreateNode(node)
	}
	g.marshaledValue = nil
	return g
}

func (g *SSAValueMappingGraph) CreateNode(value ssadb.GraphValue) int {
	// get node id, if existed, no need to create
	id, ok := g.Value2Node[value.GetId()]
	if !ok {
		// value.getVerboseName can be same in some different value,
		// so if value not exist, just create, don't use `GetOrCreateNode`
		id = g.AddNode(value.GetVerboseName())
		g.Value2Node[value.GetId()] = id
	}

	// marshal
	// add node2Value, just use bare ssa.value
	if _, ok := g.Node2Value[id]; !ok {
		g.Node2Value[id] = value
	}

	if marshaledValue, ok := g.marshaledValue[id]; ok {
		// if this node-id not contain this ssadb.GraphValue, marshal
		if _, ok := marshaledValue[value]; !ok {
			g._marshal(id, value)
		}
	} else {
		// if this node-id not exist, make and marshal
		g.marshaledValue[id] = make(map[ssadb.GraphValue]struct{})
		g._marshal(id, value)
	}
	return id
}

func (g *SSAValueMappingGraph) _marshal(selfID int, value ssadb.GraphValue) {
	g.marshaledValue[selfID][value] = struct{}{}

	if len(value.GetDependOnGraphValues()) == 0 && len(value.GetEffectOnGraphValues()) == 0 && len(value.GetGraphPredecessors()) == 0 {
		return
	}

	for _, node := range value.GetDependOnGraphValues() {
		id := g.CreateNode(node)
		g.AddEdge(selfID, id, "dependOn")
	}
	for _, node := range value.GetEffectOnGraphValues() {
		id := g.CreateNode(node)
		g.AddEdge(id, selfID, "effectOn")
	}

	for _, predecessor := range value.GetGraphPredecessors() {
		if predecessor.GraphValue == nil {
			continue
		}

		predecessorNodeID := g.CreateNode(predecessor.GraphValue)
		edges := g.GetEdges(predecessorNodeID, selfID)

		edgeLabel := predecessor.Label
		if predecessor.Step > 0 {
			edgeLabel = fmt.Sprintf(`step[%v]: %v`, predecessor.Step, edgeLabel)
		}

		if len(edges) > 0 {
			for _, edge := range edges {
				g.EdgeAttribute(edge, "color", "red")
				g.EdgeAttribute(edge, "fontcolor", "red")
				g.EdgeAttribute(edge, "penwidth", "3.0")
				g.EdgeAttribute(edge, "label", edgeLabel)
			}
		} else {
			edgeId := g.AddEdge(predecessorNodeID, selfID, edgeLabel)
			g.EdgeAttribute(edgeId, "color", "red")
			g.EdgeAttribute(edgeId, "fontcolor", "red")
			g.EdgeAttribute(edgeId, "penwidth", "3.0")
		}
	}
}

func (v *Value) DotGraph() string {
	sg := NewSFGraph(v)
	return sg.DotGraph()
}

func (v *Value) ShowDot() *Value {
	dotGraph := v.DotGraph()
	fmt.Println(dotGraph)
	return v
}

func (v *Value) AnalyzeDepth() int {
	return v.GetDepth()
}
