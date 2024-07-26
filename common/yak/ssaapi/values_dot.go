package ssaapi

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
)

type ValueGraph struct {
	*dot.Graph

	RootValue map[*Value]int // root value to graph node id
	NodeInfo  map[int]*Value // graph node id to value
}

func NewValueGraph(v ...*Value) *ValueGraph {
	graph := dot.New()
	graph.MakeDirected()
	graph.GraphAttribute("rankdir", "BT")
	vg := &ValueGraph{
		Graph:     graph,
		RootValue: make(map[*Value]int),
		NodeInfo:  make(map[int]*Value),
	}
	for _, value := range v {
		n := graph.AddNode(value.GetVerboseName())
		vg.RootValue[value] = n
		vg._marshal(n, value)
	}
	return vg
}

func (g *ValueGraph) _marshal(self int, t *Value) {
	g.NodeInfo[self] = t

	if len(t.DependOn) == 0 && len(t.EffectOn) == 0 && len(t.Predecessors) == 0 {
		return
	}

	createNode := func(node *Value) int {
		id := g.GetOrCreateNode(node.GetVerboseName())
		if _, ok := g.NodeInfo[id]; !ok {
			g._marshal(id, node)
		}
		return id
	}

	for _, node := range t.DependOn {
		id := createNode(node)
		g.AddEdge(self, id, "")
	}
	for _, node := range t.EffectOn {
		id := createNode(node)
		g.AddEdge(id, self, "")
	}

	for _, predecessor := range t.Predecessors {
		if predecessor.Node == nil {
			continue
		}

		predecessorNodeID := createNode(predecessor.Node)
		edges := g.GetEdges(predecessorNodeID, self)

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
			edgeId := g.AddEdge(predecessorNodeID, self, edgeLabel)
			g.EdgeAttribute(edgeId, "color", "red")
			g.EdgeAttribute(edgeId, "fontcolor", "red")
			g.EdgeAttribute(edgeId, "penwidth", "3.0")
		}
	}
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

func (v *Value) AnalyzeDepth() int {
	return v.GetDepth()
}

func CreateDotGraph(i ...sfvm.ValueOperator) (string, error) {
	om := make(map[int64]struct{})
	var vals Values
	_ = sfvm.MergeValues(i...).Recursive(func(operator sfvm.ValueOperator) error {
		if v, ok := operator.(*Value); ok {
			if _, existed := om[v.GetId()]; !existed {
				vals = append(vals, v)
				om[v.GetId()] = struct{}{}
			}
		}
		return nil
	})
	if len(vals) <= 0 {
		return "", utils.Error("no values found")
	}
	totalGraph := vals.DotGraph()
	return totalGraph, nil
}
