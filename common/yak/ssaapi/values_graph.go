package ssaapi

import (
	"context"

	"github.com/yaklang/yaklang/common/utils"
)

type EdgeType string

const (
	EdgeTypeDependOn    = "depend_on"
	EdgeTypeEffectOn    = "effect_on"
	EdgeTypePredecessor = "predecessor"
)

type GraphKind string

const (
	GraphKindShow = "Show" // show graph, show from(prev) -> to(v) , not show duplicate edge
	GraphKindDump = "Dump" // dump and save graph, save from(v) -> to(prev) raw point. and save all edge
)

type Graph interface {
	CreateEdge(Edge) error
	GetGraphKind() GraphKind
}

type Edge struct {
	From, To *Value
	Kind     EdgeType
	Msg      map[string]any
	// Label and Step carry the predecessor metadata without allocating a map
	// for every edge on the dataflow hot path. Msg remains for callers that
	// still use the generic form.
	Label string
	Step  int64
}

func (v *Value) GenerateGraph(g Graph, ctxs ...context.Context) error {
	ctx := context.Background()
	if len(ctxs) > 0 && !utils.IsNil(ctxs[0]) {
		ctx = ctxs[0]
	}

	sendRawEdge := func(from, to *Value, kind EdgeType, label string, step int64) error {
		switch g.GetGraphKind() {
		case GraphKindShow:
			from, to = to, from // show graph, show from(prev) -> to(v)
			// this two kind is reverse
			if kind == EdgeTypeEffectOn {
				kind = EdgeTypeDependOn // effect on is depend on
			} else if kind == EdgeTypeDependOn {
				kind = EdgeTypeEffectOn // depend on is effect on
			}
		case GraphKindDump: // dump raw graph, save from(v) -> to(prev) prev point
		default:
			return utils.Errorf("unknown graph kind: %v", g.GetGraphKind())
		}
		err := g.CreateEdge(Edge{
			From:  from,
			To:    to,
			Kind:  kind,
			Label: label,
			Step:  step,
		})
		if err != nil {
			return err
		}
		return nil
	}
	show := false
	valueDFS(v, func(v *Value) (Values, error) {
		prevs := v.GetPredecessors()
		next := make([]*Value, 0, len(prevs))
		for _, prev := range prevs {
			show = true
			// log.Errorf("%v prev: %v", v, prev.Node)
			switch prev.Info.Label {
			case Predecessors_BottomUseLabel:
				valueDFS(v, func(v *Value) (Values, error) {
					depende := v.GetDependOn()
					// log.Errorf("%v prev: %v", v, prev)
					for _, d := range depende {
						if err := sendRawEdge(v, d, EdgeTypeDependOn, "", 0); err != nil {
							return nil, err
						}
					}
					return depende, nil
				}, ctx)
			case Predecessors_TopDefLabel:
				valueDFS(v, func(v *Value) (Values, error) {
					effect := v.GetEffectOn()
					// log.Errorf("%v prev: %v", v, prev)
					for _, e := range effect {
						if err := sendRawEdge(v, e, EdgeTypeEffectOn, "", 0); err != nil {
							return nil, err
						}
					}
					return effect, nil
				}, ctx)
			default:
			}
			next = append(next, prev.Node)
			// add predecessor edge
			if err := sendRawEdge(v, prev.Node, EdgeTypePredecessor, prev.Info.Label, int64(prev.Info.Step)); err != nil {
				return nil, err
			}
		}
		return next, nil
	}, ctx)
	if !show {
		if v.GetDependOn().Len() > 0 {
			valueDFS(v, func(v *Value) (Values, error) {
				dependon := v.GetDependOn()
				// log.Errorf("%v prev: %v", v, prev)
				for _, depend := range dependon {
					if err := sendRawEdge(v, depend, EdgeTypeDependOn, "", 0); err != nil {
						return nil, err
					}
				}
				return dependon, nil
			}, ctx)
		}
		if v.GetEffectOn().Len() > 0 {
			valueDFS(v, func(v *Value) (Values, error) {
				effecton := v.GetEffectOn()
				// log.Errorf("%v prev: %v", v, prev)
				for _, effect := range effecton {
					if err := sendRawEdge(v, effect, EdgeTypeEffectOn, "", 0); err != nil {
						return nil, err
					}
				}
				return effecton, nil
			}, ctx)
		}
	}
	return nil
}

const MAXLevel = 1000

func valueDFS(node *Value, handler func(*Value) (Values, error), ctx context.Context) error {
	// Only membership is used here; the ordered map's key slice was pure
	// allocation. Traversal order is defined by handler's returned Values, not
	// by the visited set.
	visited := make(map[int64]struct{}, 64)
	level := 0
	var dfs func(v *Value) error
	dfs = func(v *Value) error {
		if utils.IsNil(v) {
			return utils.Error("nil value")
		}
		if level++; level > MAXLevel {
			return utils.Errorf("max level reached")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		uid := v.GetUID()
		if _, ok := visited[uid]; ok {
			return nil
		}
		visited[uid] = struct{}{}
		defer delete(visited, uid)

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
