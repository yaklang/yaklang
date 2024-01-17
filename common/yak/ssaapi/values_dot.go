package ssaapi

import (
	"bytes"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/dot"
	"sync"
)

func _marshal(m *sync.Map, g *dot.Graph, self int, t *Value) {
	_, ok := m.Load(t.GetId())
	if ok {
		return
	}
	m.Store(t.GetId(), t)

	if len(t.DependOn) == 0 && len(t.EffectOn) == 0 {
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
}

func (v *Value) Dot() string {
	g := dot.New()
	g.DrawMultipleEdges()
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
	dotGraph := v.Dot()
	fmt.Println(dotGraph)
	return v
}
