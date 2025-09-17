package ssaapi

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
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

func (v *Value) GenerateGraph(g Graph, ctxs ...context.Context) error {
	ctx := context.Background()
	if len(ctxs) > 0 && !utils.IsNil(ctxs[0]) {
		ctx = ctxs[0]
	}
	valueDFS(v, func(v *Value) (Values, error) {
		prevs := v.GetPredecessors()
		next := make([]*Value, 0, len(prevs))
		for _, prev := range prevs {
			// log.Errorf("%v prev: %v", v, prev.Node)
			switch prev.Info.Label {
			case Predecessors_BottomUseLabel:
				valueDFS(v, func(v *Value) (Values, error) {
					prev := v.GetEffectOn()
					// log.Errorf("%v prev: %v", v, prev)
					for _, prev := range prev {
						if err := g.CreateEdge(Edge{
							From: prev,
							To:   v,
							Kind: EdgeTypeEffectOn,
						}); err != nil {
							return nil, err
						}
					}
					return prev, nil
				}, ctx)
			case Predecessors_TopDefLabel:
				valueDFS(v, func(v *Value) (Values, error) {
					// from(user) -effect-> to(def) (v)
					// (v) -depend-> (prev)
					prev := v.GetDependOn()
					// log.Errorf("%v prev: %v", v, prev)
					for _, prev := range prev {
						if err := g.CreateEdge(Edge{
							From: prev,
							To:   v,
							Kind: EdgeTypeDependOn,
						}); err != nil {
							return nil, err
						}
					}
					return prev, nil
				}, ctx)
				valueDFS(v, func(v *Value) (Values, error) {
					prev := v.GetEffectOn()
					// log.Errorf("%v prev: %v", v, prev)
					for _, prev := range prev {
						if err := g.CreateEdge(Edge{
							From: prev,
							To:   v,
							Kind: EdgeTypeEffectOn,
						}); err != nil {
							return nil, err
						}
					}
					return prev, nil
				}, ctx)
			default:
			}
			next = append(next, prev.Node)
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
	}, ctx)
	return nil
}

const MAXLevel = 1000

func valueDFS(node *Value, handler func(*Value) (Values, error), ctx context.Context) error {
	// Perform DFS traversal
	stack := omap.NewEmptyOrderedMap[string, *Value]()
	level := 0
	var dfs func(v *Value) error
	dfs = func(v *Value) error {
		if level++; level > MAXLevel {
			return utils.Errorf("max level reached")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if stack.Have(v.GetUUID()) {
			return nil
		}
		stack.PushKey(v.GetUUID(), v)
		defer stack.Pop()

		vs, err := handler(v)
		if err != nil {
			return err
		}
		for _, v := range vs {
			if err := dfs(v); err != nil {
				return err
			}
		}
		return nil
	}

	return dfs(node)
}
