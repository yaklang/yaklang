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

	// one ssa.value can be create many ssaapi.Value,
	// so we use SSA-ID(int64) to graph node-id
	Value2Node map[int64]int

	// many ssaapi.value can contain different context, even this is same ssa.value
	// use this map contain node-id to marshaled ssaapi.value
	// in same node-id, that mean this ssaapi.value is same ssa.value
	// ! this field just use in graph build
	marshaledValue map[int]map[*Value]struct{} // node-id ->  ssaapi.value

	// graph node id to value, this value just use bare ssa.value
	Node2Value map[int]*Value
}

func NewValueGraph(v ...*Value) *ValueGraph {
	graph := dot.New()
	graph.MakeDirected()
	graph.GraphAttribute("rankdir", "BT")
	g := &ValueGraph{
		Graph:          graph,
		Value2Node:     make(map[int64]int),
		marshaledValue: make(map[int]map[*Value]struct{}),
		Node2Value:     make(map[int]*Value),
	}
	for _, value := range v {
		// log.Infof("start graph %v", value.GetVerboseName())
		g.CreateNode(value)
	}
	g.marshaledValue = nil
	return g
}

func (g *ValueGraph) CreateNode(value *Value) int {
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
		g.Node2Value[id] = value.NewValue(value.node)
	}

	if marshaledValue, ok := g.marshaledValue[id]; ok {
		// if this node-id not contain this ssaapi.value, marshal
		if _, ok := marshaledValue[value]; !ok {
			g._marshal(id, value)
		}
	} else {
		// if this node-id not exist, make and marshal
		g.marshaledValue[id] = make(map[*Value]struct{})
		g._marshal(id, value)
	}
	return id
}

func (g *ValueGraph) _marshal(selfID int, value *Value) {
	g.marshaledValue[selfID][value] = struct{}{}

	if len(value.DependOn) == 0 && len(value.EffectOn) == 0 && len(value.Predecessors) == 0 {
		return
	}

	for _, node := range value.DependOn {
		id := g.CreateNode(node)
		g.AddEdge(selfID, id, "")
	}
	for _, node := range value.EffectOn {
		id := g.CreateNode(node)
		g.AddEdge(id, selfID, "")
	}

	for _, predecessor := range value.Predecessors {
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
