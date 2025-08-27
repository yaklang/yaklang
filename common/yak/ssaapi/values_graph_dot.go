package ssaapi

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/dot"
)

type DotGraph struct {
	*dot.Graph
	value2Node map[*Value]int // ssaapi.Value -> node-id
}

func NewDotGraph() *DotGraph {
	graphGraph := dot.New()
	graphGraph.MakeDirected()
	graphGraph.GraphAttribute("rankdir", "BT")
	return &DotGraph{
		Graph:      graphGraph,
		value2Node: make(map[*Value]int),
	}
}

var _ Graph = (*DotGraph)(nil)

func removeEscapes(s string) string {
	s = strings.ReplaceAll(s, "\t", "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func (g *DotGraph) createNode(value *Value) int {
	if node, ok := g.value2Node[value]; ok {
		return node
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

	g.value2Node[value] = nodeId
	return nodeId
}

func (g *DotGraph) CreateEdge(edge Edge) error {
	fromNode := g.createNode(edge.From)
	toNode := g.createNode(edge.To)
	dotEdge := g.AddEdge(fromNode, toNode, string(edge.Kind))

	var (
		edgeLabel string
		step      int64
	)
	if edge.Msg != nil {
		edgeLabel = edge.Msg["label"].(string)
		step = int64(edge.Msg["step"].(int))
	}
	if step > 0 {
		edgeLabel = fmt.Sprintf(`step[%v]: %v`, step, edgeLabel)
	}
	switch edge.Kind {
	case EdgeTypePredecessor:
		g.EdgeAttribute(dotEdge, "color", "red")
		g.EdgeAttribute(dotEdge, "fontcolor", "red")
	case EdgeTypeDependOn:
		edgeLabel = "dependOn"
	case EdgeTypeEffectOn:
		edgeLabel = "effectOn"
	}
	g.EdgeAttribute(dotEdge, "penwidth", "3.0")
	g.EdgeAttribute(dotEdge, "label", edgeLabel)
	return nil
}

func (g *DotGraph) String() string {
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	return buf.String()
}

func (g *DotGraph) NodeName(v *Value) string {
	id, ok := g.value2Node[v]
	if !ok {
		return ""
	}
	return dot.NodeName(id)
}

func (g *DotGraph) ForEach(f func(string, *Value)) {
	for value, id := range g.value2Node {
		idStr := dot.NodeName(id)
		f(idStr, value)
	}
}
func (g *DotGraph) DeepFirstGraphPrev(value *Value) [][]string {
	nodeID, ok := g.value2Node[value]
	if !ok {
		return nil
	}
	return dot.GraphPathPrevWithFilter(g.Graph, nodeID, func(edge *dot.Edge) bool {
		return true
	})
}
