package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"sync"
)

func _marshal(m *sync.Map, g *dot.Graph, self int, t *Value) {
	_, ok := m.Load(t.GetId())
	if ok {
		return
	}
	m.Store(t.GetId(), t)

	if len(t.DependOn) == 0 && len(t.EffectOn) == 0 && len(t.Predecessors) == 0 {
		return
	}

	var deps []int
	var depsMap = make(map[int]*Value)
	for _, node := range t.DependOn {
		id := g.GetOrCreateNode(node.GetVerboseName())
		deps = append(deps)
		depsMap[id] = node
	}

	var effects []int
	var effectsMap = make(map[int]*Value)
	for _, node := range t.EffectOn {
		id := g.GetOrCreateNode(node.GetVerboseName())
		effects = append(effects, id)
		effectsMap[id] = node
	}
	for _, dep := range deps {
		direct := fmt.Sprintf(`%v->%v`, self, dep)
		_ = direct
		g.AddEdge(self, dep, "")
	}
	for _, effect := range effects {
		direct := fmt.Sprintf(`%v->%v`, effect, self)
		_ = direct
		// log.Infof("found edge: %v", direct)
		g.AddEdge(effect, self, "")
	}
	for id, node := range depsMap {
		_marshal(m, g, id, node)
	}
	for id, node := range effectsMap {
		_marshal(m, g, id, node)
	}

	for _, predecessor := range t.Predecessors {
		if predecessor.Node == nil {
			continue
		}

		name := predecessor.Node.GetVerboseName()
		predecessorNodeId := g.GetOrCreateNode(name)
		log.Infof("add predecessor nodeId(%v): %v", predecessorNodeId, name)
		edges := g.GetEdges(predecessorNodeId, self)

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
			edgeId := g.AddEdge(predecessorNodeId, self, edgeLabel)
			g.EdgeAttribute(edgeId, "color", "red")
			g.EdgeAttribute(edgeId, "fontcolor", "red")
			g.EdgeAttribute(edgeId, "penwidth", "3.0")
		}
		_marshal(m, g, predecessorNodeId, predecessor.Node)
	}
}

func (v *Value) DotGraph() string {
	g := dot.New()
	g.MakeDirected()
	g.GraphAttribute("rankdir", "BT")

	visisted := new(sync.Map)
	n := g.AddNode(v.GetVerboseName())
	_marshal(visisted, g, n, v)
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
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
