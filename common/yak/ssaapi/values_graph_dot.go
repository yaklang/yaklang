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
	dot        string
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

func (g *DotGraph) GetGraphKind() GraphKind {
	return GraphKindShow
}

func (g *DotGraph) createNode(value *Value, isEntry ...bool) int {
	if node, ok := g.value2Node[value]; ok {
		return node
	}
	entry := false
	if len(isEntry) > 0 {
		entry = isEntry[0]
	}
	// nodeId := 0
	code := ""
	if r := value.GetRange(); r != nil {
		code = r.GetText()
		if len(code) > 100 {
			code = code[:100] + "..."
		}
		code = removeEscapes(code)
	} else {
		code = value.GetVerboseName()
	}
	if entry {
		code = fmt.Sprintf("<entry>%s</entry>", code)
	}
	nodeId := g.AddNode(code)
	g.value2Node[value] = nodeId
	return nodeId
}

func (g *DotGraph) CreateEdge(edge Edge) error {
	fromNode := g.createNode(edge.From)
	toNode := g.createNode(edge.To)

	var (
		edgeLabel string
		step      int64
	)

	edgeLabel = string(edge.Kind)
	if edge.Msg != nil {
		if label, ok := edge.Msg["label"].(string); ok {
			edgeLabel = label
		}
		if s, ok := edge.Msg["step"].(int); ok {
			step = int64(s)
		}
	}
	if step > 0 {
		edgeLabel = fmt.Sprintf(`step[%v]: %v`, step, edgeLabel)
	}

	// if IsDataFlowLabel(edgeLabel) {
	// 	// skip top-def bottom-use  edge label
	// 	// have dataflow edge already
	// 	return nil
	// }

	dotEdge := g.AddEdge(fromNode, toNode, edgeLabel)
	if edge.Kind == EdgeTypePredecessor {
		g.EdgeAttribute(dotEdge, "color", "red")
		g.EdgeAttribute(dotEdge, "fontcolor", "red")
		g.EdgeAttribute(dotEdge, "penwidth", "3.0")
	}
	return nil
}

func (g *DotGraph) String() string {
	if g.dot != "" {
		return g.dot
	}
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	g.dot = buf.String()
	return g.dot
}

func (g *DotGraph) NodeName(v *Value) string {
	id, ok := g.value2Node[v]
	if !ok {
		return ""
	}
	return dot.NodeName(id)
}

func (g *DotGraph) NodeCount() int {
	return len(g.value2Node)
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

func (g *DotGraph) DeepFirstGraphNext(value *Value) [][]string {
	nodeID, ok := g.value2Node[value]
	if !ok {
		return nil
	}
	return dot.GraphPathNext(g.Graph, nodeID)
}

func (g *DotGraph) Show() {
	// dot.ShowDotGraphToAsciiArt(g.String())
	log.Infof(g.String())
}
