package sfvm

import "sync"

// DataflowFanoutCollector is the descent-side observation surface the dataflow
// engine (ssaapi.AnalyzeContext) uses to feed one descent's expanded nodes
// back to a dataflowFanoutEliminator. ssaapi implements it per descent; the
// sfvm side owns and consumes it. The methods take the ssa instruction id —
// the eliminator never inspects values, so no sfvm<->ssaapi import cycle is
// needed.
type DataflowFanoutCollector interface {
	// MarkVisited is called for each node whose subtree the descent fully
	// expanded (the node passed AnalyzeContext.check: not a cycle cut, not a
	// limit exit, and no object-expansion context active at the visit).
	MarkVisited(id int64)
	// OnCycleCut is called when the descent revisits a node already on its
	// current path. A segment computed under such a cut is NOT an independent
	// topdef of the cut node, so the session must not publish.
	OnCycleCut()
	// Publish is called exactly once when a descent finishes complete and
	// untruncated; it promotes the session's marks into the eliminator's
	// eliminated set. A truncated descent must never call Publish.
	Publish()
}

// dataflowFanoutEliminator skips redundant per-source descents in one
// topdef/bottomuse opcode fanout based on the coverage law: when the descent
// from input A completes and, on its way, fully expands another input A1,
// then independent topdef(A1) ⊆ topdef(A) (equality for passthrough nodes),
// so A1's own descent would only re-emit values A already emitted and can be
// skipped. On a large project one opcode was fed 52,842 inputs (after dedup)
// whose upstream closures overlap heavily; the union closure is bounded by
// what a handful of descents visit, while N independent descents burn N×5000
// node visits.
//
// Safety boundaries:
//   - anchor scopes: elimination is disabled while a filter scope is active,
//     because each source's results carry its own anchor bits and skipping a
//     source would silently drop its filter attribution.
//   - until/hook/depth configs: `#{until:...}`, `#{hook:...}` and depth
//     windows make a node's topdef depend on its distance from the source, so
//     the coverage law no longer holds; the opcode wiring disables the
//     eliminator for those SFIs.
//   - object expansion: nodes visited while an object-expansion context is
//     active have entry-context-dependent segment results and are never
//     marked (the descent-side hook enforces this).
//   - truncation: only descents that return complete (no recursive-counter /
//     depth / ctx abort and under dataflowValueLimit) publish their marks.
//
// Concurrency: the >inlinePipelineThreshold fanout runs its per-source
// closures on a 10-worker pipe (utils/pipeline default concurrency), so
// sessions run concurrently on separate goroutines. All shared state is
// guarded by mu; a descentSession itself is single-descent and unshared.
type dataflowFanoutEliminator struct {
	mu         sync.Mutex
	pending    map[int64]dedupKey // input instruction-id -> input identity
	eliminated map[dedupKey]struct{}
	skipped    int // sources skipped by Eliminated; read via Skipped() for diagnostics
}

// NewDataflowEliminator indexes the fanout input for elimination. The input
// MUST already be exact-deduped (dedupDataflowFanoutInput): the identity key
// is the same rule MergeValues uses, and a duplicate entry would let one
// elimination silently cover a second occurrence that carried its own anchor
// provenance.
func NewDataflowEliminator(input Values) *dataflowFanoutEliminator {
	pending := make(map[int64]dedupKey, input.Count())
	eliminated := make(map[dedupKey]struct{}, input.Count())
	_ = input.Recursive(func(vo ValueOperator) error {
		key, ok := valueDedupKey(vo)
		if !ok {
			return nil
		}
		if id, ok := fetchId(vo); ok {
			pending[id] = key
		}
		// No stable instruction id (e.g. pattern hits): descent marks are keyed
		// by node id, so such an input can never be matched as covered. It is
		// not indexed at all — indexing it in `eliminated` would skip its
		// descent without any descent having covered it.
		return nil
	})
	return &dataflowFanoutEliminator{
		pending:    pending,
		eliminated: eliminated,
	}
}

// Eliminated reports whether vo's descent is already covered by a previously
// completed descent (vo is an input that a finished descent fully expanded).
// The source closure then returns (nil, nil) — safe on both pipeline paths.
func (e *dataflowFanoutEliminator) Eliminated(vo ValueOperator) bool {
	if e == nil {
		return false
	}
	key, ok := valueDedupKey(vo)
	if !ok {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, hit := e.eliminated[key]; !hit {
		return false
	}
	e.skipped++
	return true
}

// Skipped reports how many fanout sources this eliminator has skipped so far.
// Diagnostic surface for tests and progress reporting.
func (e *dataflowFanoutEliminator) Skipped() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.skipped
}

// StartDescent opens the per-source descent session that records which inputs
// the descent fully expanded. The session must be closed by Publish (promote
// marks) or abandoned (publish nothing); the descent funnels decide based on
// whether the descent completed untruncated.
func (e *dataflowFanoutEliminator) StartDescent() DataflowFanoutCollector {
	return &descentSession{eliminator: e}
}

// FinishDescent promotes the session's marks into the eliminated set. An
// input still pending gains coverage; anything else in the marks is ignored
// (already covered, or eliminated while this descent ran — its coverage
// predates this descent either way).
func (e *dataflowFanoutEliminator) FinishDescent(s *descentSession) {
	if e == nil || s == nil || len(s.marks) == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range s.marks {
		if key, ok := e.pending[id]; ok {
			e.eliminated[key] = struct{}{}
			delete(e.pending, id)
		}
	}
}

// descentSession accumulates one descent's coverage marks. It is created by
// StartDescent, driven by the ssaapi AnalyzeContext hooks through the
// DataflowFanoutCollector interface, and closed by Publish.
type descentSession struct {
	eliminator *dataflowFanoutEliminator
	marks      map[int64]struct{}
	aborted    bool
}

func (s *descentSession) MarkVisited(id int64) {
	if s == nil || s.aborted || id <= 0 {
		return
	}
	if s.marks == nil {
		s.marks = make(map[int64]struct{})
	}
	s.marks[id] = struct{}{}
}

func (s *descentSession) OnCycleCut() {
	if s == nil {
		return
	}
	// One cut anywhere invalidates every mark: a later segment through the
	// cut point returned a "path node as result" that an independent descent
	// would have expanded further. Conservative by design.
	s.aborted = true
	s.marks = nil
}

func (s *descentSession) Publish() {
	s.eliminator.FinishDescent(s)
}
