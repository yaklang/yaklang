package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
)

type EdgeType string

const (
	EdgeTypeDependOn    = "depend_on"
	EdgeTypeEffectOn    = "effect_on"
	EdgeTypePredecessor = "predecessor"
)

type Graph interface {
	CreateEdge(Edge) error
}

type Edge struct {
	From, To *Value
	Kind     EdgeType
	Msg      map[string]any
}

func (v *Value) GenerateGraph(g Graph) error {
	dfs(v, func(v *Value) (Values, error) {
		prevs := v.GetPredecessors()
		next := make([]*Value, 0, len(prevs))
		for _, prev := range prevs {
			switch prev.Info.Label {
			case Predecessors_BottomUseLabel:
				// value(prev) --effectOn--> user(v)
				dfs(v, func(v *Value) (Values, error) {
					dependOn := v.GetDependOn()
					for _, d := range dependOn {
						if err := g.CreateEdge(Edge{
							From: v,
							To:   d,
							Kind: EdgeTypeDependOn,
						}); err != nil {
							return nil, err
						}
					}
					return dependOn, nil
				})
			case Predecessors_TopDefLabel:
				// value(prev) --dependOn--> def (v)
				// def(v)      --effectOn--> value(prev)
				dfs(v, func(v *Value) (Values, error) {
					effectOn := v.GetEffectOn()
					for _, e := range effectOn {
						if err := g.CreateEdge(Edge{
							From: e,
							To:   v,
							Kind: EdgeTypeDependOn,
						}); err != nil {
							return nil, err
						}
					}
					return effectOn, nil
				})
			default:
				// add predecessor edge
				if err := g.CreateEdge(Edge{
					From: prev.Node,
					To:   v,
					Kind: EdgeTypePredecessor,
					Msg: map[string]any{
						"label": prev.Info.Label,
						"step":  prev.Info.Step,
					},
				}); err != nil {
					return nil, err
				}
				next = append(next, prev.Node)
			}
		}
		return next, nil
	})
	return nil
}

func dfs(node *Value, handler func(*Value) (Values, error)) error {
	// Perform DFS traversal
	stack := utils.NewStack[*Value]()
	stack.Push(node)
	for !stack.IsEmpty() {
		curr := stack.Pop()
		vs, err := handler(curr)
		if err != nil {
			return err
		}
		for _, v := range vs {
			// Push the next node onto the stack
			stack.Push(v)
		}
	}
	return nil
}
