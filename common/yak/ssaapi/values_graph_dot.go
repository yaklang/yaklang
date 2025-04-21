package ssaapi

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/utils/graph"
)

type EdgeType string

const (
	EdgeTypeDependOn    = "depend_on"
	EdgeTypeEffectOn    = "effect_on"
	EdgeTypePredecessor = "predecessor"
)

func ValidEdgeType(edge string) EdgeType {
	switch edge {
	case "depend_on":
		return EdgeTypeDependOn
	case "effect_on":
		return EdgeTypeEffectOn
	case "predecessor":
		return EdgeTypePredecessor
	}
	return ""
}

type ValueGraph struct {
	*dot.Graph

	Value2Node     map[*Value]int   // ssaapi.Value -> node-id
	marshaledValue map[int]struct{} // node-id ->  ssaapi.value
	Node2Value     map[int]*Value
	dtype          Dtype
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
	for _, value := range v {
		graph.BuildGraphWithDFS[int, *Value](
			value,
			g.createNode,
			g.getNeighbors,
			g.handleEdge,
		)
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

func removeEscapes(s string) string {
	s = strings.ReplaceAll(s, "\t", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func (g *ValueGraph) createNode(value *Value) (int, error) {
	// nodeId := g.AddNode(value.GetVerboseName())
	// s := fmt.Sprintf("%s_%d_%d", value.GetVerboseName(), value.GetId(), nodeId)
	// g.SetNode(nodeId, s)

	if g.dtype == DT_None {
		if value.GetDependOn() != nil {
			g.dtype = DT_DependOn
		} else if value.GetEffectOn() != nil {
			g.dtype = DT_EffectOn
		}
	}

	nodeId := 0
	if r := value.GetRange(); r != nil {
		code := r.GetText()
		if len(code) > 100 {
			code = code[:100] + "..."
		}
		code = removeEscapes(code)
		nodeId = g.AddNode(code)
	} else {
		nodeId = g.AddNode(value.GetVerboseName())
	}

	g.Node2Value[nodeId] = value
	g.Value2Node[value] = nodeId
	return nodeId, nil
}

func (g *ValueGraph) getNeighbors(value *Value) []*graph.Neighbor[*Value] {
	if value == nil {
		return nil
	}

	var res []*graph.Neighbor[*Value]
	appendFunc := func() {}

	if g.dtype == DT_DependOn {
		appendFunc = func() {
			for _, v := range value.GetDependOn() {
				res = append(res, graph.NewNeighbor(v, EdgeTypeDependOn))
			}
		}
	} else if g.dtype == DT_EffectOn {
		appendFunc = func() {
			for _, v := range value.GetEffectOn() {
				res = append(res, graph.NewNeighbor(v, EdgeTypeEffectOn))
			}
		}
	}

	appendFunc()

	for _, predecessor := range value.GetPredecessors() {
		if predecessor.Node == nil {
			continue
		}
		if IsDataFlowLabel(predecessor.Info.Label) {
			continue
		}
		neighbor := graph.NewNeighbor(predecessor.Node, EdgeTypePredecessor)
		neighbor.AddExtraMsg("label", predecessor.Info.Label)
		neighbor.AddExtraMsg("step", predecessor.Info.Step)
		res = append(res, neighbor)
	}
	return res
}

func (g *ValueGraph) handleEdge(fromNode int, toNode int, edgeType string, extraMsg map[string]any) {
	switch ValidEdgeType(edgeType) {
	case EdgeTypeDependOn:
		g.AddEdge(toNode, fromNode, edgeType)
	case EdgeTypeEffectOn:
		g.AddEdge(toNode, fromNode, edgeType)
	case EdgeTypePredecessor:
		edges := g.GetEdges(toNode, fromNode)
		var (
			edgeLabel string
			step      int64
		)
		if extraMsg != nil {
			edgeLabel = extraMsg["label"].(string)
			step = int64(extraMsg["step"].(int))
		}
		if step > 0 {
			edgeLabel = fmt.Sprintf(`step[%v]: %v`, step, edgeLabel)
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
	return dot.GraphPathPrevWithFilter(g.Graph, nodeID, func(edge *dot.Edge) bool {
		// only use predecessor edge draw path
		return edge.Attribute("color") == "red"
	})
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
