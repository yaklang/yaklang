package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
)

type EdgeType string

const (
	EdgeTypeDependOn    = "depend_on"
	EdgeTypeEffectOn    = "effect_on"
	EdgeTypeDataflow    = "dataflow"
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
			case Predecessors_BottomUseLabel, Predecessors_TopDefLabel:
				dfs(prev.Node, func(v *Value) (Values, error) {
					nexts := v.GetDataFlow()
					for _, next := range nexts {
						if err := g.CreateEdge(Edge{
							From: v,
							To:   next,
							Kind: EdgeTypeDataflow,
						}); err != nil {
							return nil, err
						}
					}
					return nexts, nil
				})
			default:
			}
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
