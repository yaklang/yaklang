package ssaapi

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/log"

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

func (g *ValueGraph) Dot() string {
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	return buf.String()
}

func (g *ValueGraph) CreateNode(value *Value) int {
	log.Infof("create node %d: %v, %p", value.GetId(), value.GetVerboseName(), value)
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
	if g.theValueShouldMarshal(value, id) {
		g._marshal(id, value)
	}
	return id
}

func (g *ValueGraph) _marshal(selfID int, value *Value) {
	g.marshaledValue[selfID][value] = struct{}{}
	if len(value.GetDependOn()) == 0 && len(value.GetEffectOn()) == 0 && len(value.GetPredecessors()) == 0 {
		return
	}

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

func (g *ValueGraph) theValueShouldMarshal(value *Value, id int) bool {
	if marshaledValue, ok := g.marshaledValue[id]; ok {
		// if this node-id not contain this ssaapi.value, marshal
		if _, ok := marshaledValue[value]; !ok {
			return true
		}
		return false
	} else {
		// if this node-id not exist, make and marshal
		g.marshaledValue[id] = make(map[*Value]struct{})
		return true
	}
}

func (g *ValueGraph) DeepFirstGraph(valueID int64) [][]string {
	nodeID, ok := g.Value2Node[valueID]
	if !ok {
		return nil
	}
	return dot.GraphPathPrev(g.Graph, nodeID)
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
