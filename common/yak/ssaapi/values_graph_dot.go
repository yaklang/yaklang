package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/utils/graph"
)

type ValueGraph struct {
	*dot.Graph

	Value2Node     map[*Value]int   // ssaapi.Value -> node-id
	marshaledValue map[int]struct{} // node-id ->  ssaapi.value
	Node2Value     map[int]*Value
}

func NewValueGraph(v ...*Value) *ValueGraph {
	graphGraph := dot.New()
	graphGraph.MakeDirected()
	graphGraph.GraphAttribute("rankdir", "BT")
	g := &ValueGraph{
		Graph:          graphGraph,
		Value2Node:     make(map[*Value]int),
		marshaledValue: make(map[int]struct{}),
		Node2Value:     make(map[int]*Value),
	}

	builder := graph.NewGraphBuilder[int, *Value](
		g.getNodeIdByValue,
		g.getNeighbors,
		g.handleEdge,
	)
	for _, value := range v {
		builder.BuildGraph(value)
	}
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

func (g *ValueGraph) getNodeIdByValue(value *Value) int {
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

func (g *ValueGraph) getValueByNodeId(node int) *Value {
	return g.Node2Value[node]
}

func (g *ValueGraph) getNeighbors(node int) []graph.NeighborWithEdgeType[*Value] {
	value := g.getValueByNodeId(node)
	if value == nil {
		return nil
	}

	var res []graph.NeighborWithEdgeType[*Value]
	for _, v := range value.GetDependOn() {
		res = append(res, graph.NewNeighbor(v, "depend_on"))
	}
	for _, v := range value.GetEffectOn() {
		res = append(res, graph.NewNeighbor(v, "effect_on"))
	}
	for _, predecessor := range value.GetPredecessors() {
		if predecessor.Node == nil {
			continue
		}
		neighbor := graph.NewNeighbor(predecessor.Node, "predecessor")
		neighbor.AddExtraMsg("label", predecessor.Info.Label)
		neighbor.AddExtraMsg("step", predecessor.Info.Step)
	}
	return res
}

func (g *ValueGraph) handleEdge(fromNode int, toNode int, edgeType string, extraMsg map[string]any) {
	switch edgeType {
	case "depend_on":
		g.AddEdge(fromNode, toNode, "")
	case "effect_on":
		g.AddEdge(toNode, fromNode, "")
	case "predecessor":
		edges := g.GetEdges(fromNode, toNode)
		var (
			label, edgeLabel string
			step             int64
		)
		if extraMsg != nil {
			label = extraMsg["label"].(string)
			step = extraMsg["step"].(int64)
		}
		if step > 0 {
			edgeLabel = fmt.Sprintf(`step[%v]: %v`, step, label)
		}
		if len(edges) > 0 {
			for _, edge := range edges {
				g.EdgeAttribute(edge, "color", "red")
				g.EdgeAttribute(edge, "fontcolor", "red")
				g.EdgeAttribute(edge, "penwidth", "3.0")
				g.EdgeAttribute(edge, "label", edgeLabel)
			}
		} else {
			edgeId := g.AddEdge(toNode, fromNode, edgeLabel)
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
