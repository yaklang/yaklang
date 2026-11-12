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
	// Label and Step carry the predecessor metadata without allocating a map
	// for every edge on the dataflow hot path.
	Label string
	Step  int64
}

type predecessorIdentity struct {
	node  int64
	label string
	step  int
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
	// A value can be reached through many paths. valueDFS intentionally keeps
	// per-path semantics for callers that need them, but graph generation only
	// needs every node expanded once: the graph deduplicates repeated edges.
	// Without these markers a diamond-shaped dataflow graph revisits every
	// shared node once per path, which is exponential on real audit graphs.
	mainVisited := make(map[int64]struct{}, 256)
	dependVisited := make(map[int64]struct{}, 256)
	effectVisited := make(map[int64]struct{}, 256)
	valueDFS(v, func(v *Value) (Values, error) {
		uid := v.GetUID()
		if _, ok := mainVisited[uid]; ok {
			return nil, nil
		}
		mainVisited[uid] = struct{}{}

		prevs := v.GetPredecessors()
		next := make([]*Value, 0, len(prevs))
		// A predecessor slice can contain the same edge twice when more than
		// one producer appended it. Emit it once here, so DBGraph does not
		// need a graph-wide string-keyed dedupe map.
		var seenPredecessors map[predecessorIdentity]struct{}
		if len(prevs) > 1 {
			seenPredecessors = make(map[predecessorIdentity]struct{}, len(prevs))
		}
		for _, prev := range prevs {
			if seenPredecessors != nil {
				key := predecessorIdentity{node: prev.Node.GetUID(), label: prev.Info.Label, step: prev.Info.Step}
				if _, ok := seenPredecessors[key]; ok {
					continue
				}
				seenPredecessors[key] = struct{}{}
			}
			show = true
			// log.Errorf("%v prev: %v", v, prev)
			switch prev.Info.Label {
			case Predecessors_BottomUseLabel:
				valueDFS(v, func(v *Value) (Values, error) {
					uid := v.GetUID()
					if _, ok := dependVisited[uid]; ok {
						return nil, nil
					}
					dependVisited[uid] = struct{}{}
					depende := v.GetDependOn()
					// log.Errorf("%v prev: %v", v, prev)
					var seenDepend map[int64]struct{}
					if len(depende) > 1 {
						seenDepend = make(map[int64]struct{}, len(depende))
					}
					for _, d := range depende {
						if seenDepend != nil {
							uid := d.GetUID()
							if _, ok := seenDepend[uid]; ok {
								continue
							}
							seenDepend[uid] = struct{}{}
						}
						if err := sendRawEdge(v, d, EdgeTypeDependOn, "", 0); err != nil {
							return nil, err
						}
					}
					return depende, nil
				}, ctx)
			case Predecessors_TopDefLabel:
				valueDFS(v, func(v *Value) (Values, error) {
					uid := v.GetUID()
					if _, ok := effectVisited[uid]; ok {
						return nil, nil
					}
					effectVisited[uid] = struct{}{}
					effect := v.GetEffectOn()
					// log.Errorf("%v prev: %v", v, prev)
					var seenEffect map[int64]struct{}
					if len(effect) > 1 {
						seenEffect = make(map[int64]struct{}, len(effect))
					}
					for _, e := range effect {
						if seenEffect != nil {
							uid := e.GetUID()
							if _, ok := seenEffect[uid]; ok {
								continue
							}
							seenEffect[uid] = struct{}{}
						}
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
				uid := v.GetUID()
				if _, ok := dependVisited[uid]; ok {
					return nil, nil
				}
				dependVisited[uid] = struct{}{}
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
				uid := v.GetUID()
				if _, ok := effectVisited[uid]; ok {
					return nil, nil
				}
				effectVisited[uid] = struct{}{}
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
