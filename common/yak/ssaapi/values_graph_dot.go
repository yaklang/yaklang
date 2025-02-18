package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/dot"
)

type ValueGraph struct {
	*dot.Graph

	Value2Node     map[*Value]int   // ssaapi.Value -> node-id
	marshaledValue map[int]struct{} // node-id ->  ssaapi.value
	Node2Value     map[int]*Value
}

func NewValueGraph(v ...*Value) *ValueGraph {
	graph := dot.New()
	graph.MakeDirected()
	graph.GraphAttribute("rankdir", "BT")
	g := &ValueGraph{
		Graph:          graph,
		Value2Node:     make(map[*Value]int),
		marshaledValue: make(map[int]struct{}),
		Node2Value:     make(map[int]*Value),
	}
	for _, value := range v {
		g.CreateNode(value)
	}
	g.marshaledValue = nil
	return g
}

func (g *ValueGraph) Dot() string {
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	return buf.String()
}

func (g *ValueGraph) ShowDot() {
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	fmt.Println(buf.String())
}

func (g *ValueGraph) GetNodeIdByValue(value *Value) int {
	log.Infof("create node %d: %v, %p", value.GetId(), value.GetVerboseName(), value)
	// get node id, if existed, no need to create

	nodeId, ok := g.Value2Node[value]
	if !ok {
		// value.getVerboseName can be same in some different value,
		// so if value not exist, just create, don't use `GetOrCreateNode`
		nodeId = g.AddNode(value.GetVerboseName())
		g.Value2Node[value] = nodeId
	}
	// marshal
	// add node2Value, just use bare ssa.value
	if _, ok := g.Node2Value[nodeId]; !ok {
		g.Node2Value[nodeId] = value
	}
	return nodeId
}

func (g *ValueGraph) CreateNode(value *Value) int {
	nodeId := g.GetNodeIdByValue(value)
	g._marshal(nodeId, value)
	return nodeId
}

func (g *ValueGraph) _marshal(selfID int, value *Value) {
	if _, ok := g.marshaledValue[selfID]; ok {
		return
	}
	g.marshaledValue[selfID] = struct{}{}

	for _, node := range value.GetDependOn() {
		id := g.CreateNode(node)
		g.AddEdge(selfID, id, "")
	}
	for _, node := range value.GetEffectOn() {
		id := g.CreateNode(node)
		g.AddEdge(id, selfID, "")
	}

	for _, predecessor := range value.GetPredecessors() {
		if predecessor.Node == nil {
			continue
		}
		predecessorNodeID := g.CreateNode(predecessor.Node)
		edges := g.GetEdges(predecessorNodeID, selfID)

		edgeLabel := predecessor.Info.Label
		if predecessor.Info.Step > 0 {
			edgeLabel = fmt.Sprintf(`step[%v]: %v`, predecessor.Info.Step, edgeLabel)
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

func (g *ValueGraph) DeepFirstGraphPrev(value *Value) [][]string {
	nodeID, ok := g.Value2Node[value]
	if !ok {
		return nil
	}
	return dot.GraphPathPrev(g.Graph, nodeID)
}

func (g *ValueGraph) DeepFirstGraphNext(value *Value) [][]string {
	nodeID, ok := g.Value2Node[value]
	if !ok {
		return nil
	}
	return dot.GraphPathNext(g.Graph, nodeID)
}

func (V Values) ShowDot() Values {
	for _, v := range V {
		v.ShowDot()
	}
	return V
}

func (v Values) DotGraphs() []string {
	var ret []string
	for _, val := range v {
		ret = append(ret, val.DotGraph())
	}
	return ret
}

func (v *Value) DotGraph() string {
	vg := NewValueGraph(v)
	var buf bytes.Buffer
	vg.GenerateDOT(&buf)
	return buf.String()
}

func (v *Value) ShowDot() *Value {
	dotGraph := v.DotGraph()
	fmt.Println(dotGraph)
	return v
}
