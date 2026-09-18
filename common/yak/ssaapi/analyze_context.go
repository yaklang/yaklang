package ssaapi

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type objectItem struct {
	object       *Value
	key          *Value
	member       *Value
	recoverIntra func()
}

type AnalysisType string

const (
	TopDefAnalysis    AnalysisType = "top_def"
	BottomUseAnalysis AnalysisType = "bottom_use"
)

const (
	recursiveStackLimit = 5000
	dataflowValueLimit  = 100
)

var errRecursiveDepth = fmt.Errorf("recursive call is over 10000, stop it")

type AnalyzeContext struct {
	// Self
	Self       *Value
	direct     AnalysisType
	config     *OperationConfig
	untilMatch Values
	// recursive depth limited
	depth               int
	reachedDepthLimited bool
	// cross process manager
	*processAnalysisManager
	//object
	_objectStack *utils.Stack[*objectItem]

	callStack *utils.Stack[*ssa.Call]

	// Use for recursive depth limit
	recursiveCounter int64

	// savedPathEdges counts saveDataflowPath calls in this descent; SavePath
	// stops building the edge graph once it reaches maxSavedPathEdges. Bounds
	// the live DependOn/EffectOn graph for a long/heavy rule (path display is
	// best-effort; taint result unaffected).
	savedPathEdges int64

	// savedPath map[*Value]struct{}
	recursiveStatusIsLeaf *utils.Stack[node]

	// resolvedInstCache memoizes the resolved underlying instruction for each
	// (program, inst-id) touched during THIS descent (one GetTopDefs /
	// GetBottomUses AnalyzeContext lifetime). A2 in scan-perf-optimization-plan:
	// on large projects the ProgramCache instruction residency has a short TTL /
	// small cap, so the same inst-id can be evicted and re-resolved (re-running
	// AnyToBytes / rune-offset / NewLazyInstructionFromIrCode / range) many times
	// within one descent. Caching the resolved instruction here skips that repeat
	// work. The cache is scoped to the descent and dropped when the AnalyzeContext
	// is discarded, so it never leaks across rules / paths (design principle 1).
	// A descent is single-threaded, so no locking is needed.
	resolvedInstCache map[resolvedInstKey]ssa.Instruction

	// widen counts object/member widening done during THIS descent. It exists
	// only to explain a "too many values" limit hit and is nil unless the
	// opt-in tracer is enabled (YAK_SSA_DATAFLOW_TRACE), so the hot path pays
	// a single nil check in the normal case.
	widen *widenTrace
}

// widenTrace records object/member widening for one descent. A descent runs on
// one goroutine; the atomics only guard against an aborted descent leaving a
// half-updated trace behind.
type widenTrace struct {
	objectsExpanded     atomic.Int64
	membersEnumerated   atomic.Int64
	maxMembersPerObject atomic.Int64
	nodeVisits          atomic.Int64
	userFanout          atomic.Int64
	calledByFanout      atomic.Int64
}

// dataflowTraceEnabled turns on the widening tracer for limit-hit diagnostics.
// Off by default: the counters are useless in a healthy run and the extra
// bookkeeping is unnecessary on a hot path.
var dataflowTraceEnabled = envFlagEnabled("YAK_SSA_DATAFLOW_TRACE")

// setDataflowTraceEnabled forces the widening tracer on/off and reports the
// previous value. Tests use it to measure fan-out for a specific shape without
// depending on a process-level environment variable.
func setDataflowTraceEnabled(enabled bool) (prev bool) {
	prev = dataflowTraceEnabled
	dataflowTraceEnabled = enabled
	return prev
}

// lastWidenTrace holds the counters of the most recently finished descent.
// Only populated while the tracer is enabled; tests read it right after a
// GetTopDefs / GetBottomUses call to measure one descent's fan-out.
var lastWidenTrace atomic.Pointer[widenTrace]

// resolvedInstKey identifies a resolved instruction by program + inst-id. The
// program pointer is stable for the lifetime of a Program; inst-id is unique
// within a program.
type resolvedInstKey struct {
	prog   *ssa.Program
	instID int64
}

type node struct {
	leaf bool
	node *Value
}

func (a *AnalyzeContext) structAllowValue(v *Value) bool {
	if a == nil || a.config == nil || a.config.structBound == nil {
		return true
	}
	if v == nil {
		return true
	}
	inst := v.getValue()
	if inst == nil {
		return true
	}
	return a.config.structBound.Allow(inst)
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		processAnalysisManager: newAnalysisManager(),
		_objectStack:           utils.NewStack[*objectItem](),
		config:                 NewOperations(opt...),
		depth:                  -1,
		callStack:              utils.NewStack[*ssa.Call](),
		recursiveStatusIsLeaf:  utils.NewStack[node](),
	}
	if dataflowTraceEnabled {
		actx.widen = &widenTrace{}
	}
	return actx
}

// traceObjectExpansion records one object member enumeration for the opt-in
// limit-hit tracer. It is a no-op in a normal run.
func (a *AnalyzeContext) traceObjectExpansion(members int) {
	if a == nil || a.widen == nil {
		return
	}
	a.widen.objectsExpanded.Add(1)
	a.widen.membersEnumerated.Add(int64(members))
	for {
		max := a.widen.maxMembersPerObject.Load()
		if int64(members) <= max || a.widen.maxMembersPerObject.CompareAndSwap(max, int64(members)) {
			break
		}
	}
}

// traceNodeVisit counts one entry into the recursive descent.
func (a *AnalyzeContext) traceNodeVisit() {
	if a == nil || a.widen == nil {
		return
	}
	a.widen.nodeVisits.Add(1)
}

// traceUserFanout counts values reached by following users (bottom-use) and
// traceCalledByFanout counts values reached by following callers
// (top-def ignore-call-stack fallback).
func (a *AnalyzeContext) traceUserFanout(n int) {
	if a == nil || a.widen == nil || n <= 0 {
		return
	}
	a.widen.userFanout.Add(int64(n))
}

func (a *AnalyzeContext) traceCalledByFanout(n int) {
	if a == nil || a.widen == nil || n <= 0 {
		return
	}
	a.widen.calledByFanout.Add(int64(n))
}

// widenReport renders the widening counters for a limit-hit diagnostic. It
// returns "" when the tracer is off.
func (a *AnalyzeContext) widenReport() string {
	if a == nil || a.widen == nil {
		return ""
	}
	return fmt.Sprintf("objectsExpanded=%d membersEnumerated=%d maxMembersPerObject=%d "+
		"nodeVisits=%d userFanout=%d calledByFanout=%d",
		a.widen.objectsExpanded.Load(), a.widen.membersEnumerated.Load(),
		a.widen.maxMembersPerObject.Load(),
		a.widen.nodeVisits.Load(), a.widen.userFanout.Load(), a.widen.calledByFanout.Load())
}

// getResolvedValue returns the resolved ssa.Value for (inst, id), memoizing the
// fully-resolved value for the rest of this descent. It is the A2 dedup point:
// on large projects the ProgramCache instruction residency has a short TTL /
// small cap, so the same inst-id can be evicted and re-resolved (re-running
// AnyToBytes / rune-offset / NewLazyInstructionFromIrCode / range) many times
// within one descent. Caching the resolved value here skips that repeat work.
//
// The cache is scoped to the descent (AnalyzeContext lifetime) and dropped when
// the descent ends, so it never leaks across rules / paths (design principle 1).
// A descent is single-threaded, so no locking is needed.
func (a *AnalyzeContext) getResolvedValue(inst ssa.Instruction, id int64) (ssa.Value, bool) {
	if a == nil || inst == nil || id <= 0 {
		return nil, false
	}
	prog := inst.GetProgram()
	if prog == nil {
		return nil, false
	}
	if a.resolvedInstCache == nil {
		a.resolvedInstCache = make(map[resolvedInstKey]ssa.Instruction)
	}
	key := resolvedInstKey{prog: prog, instID: id}
	if cached, ok := a.resolvedInstCache[key]; ok {
		if v, ok := ssa.ToValue(cached); ok {
			return v, true
		}
		return nil, false
	}
	v, ok := inst.GetValueById(id)
	if !ok || v == nil {
		return nil, false
	}
	a.resolvedInstCache[key] = v
	return v, true
}

func (a *AnalyzeContext) pushCall(call *ssa.Call) {
	a.callStack.Push(call)
}
func (a *AnalyzeContext) popCall() *ssa.Call {
	return a.callStack.Pop()
}
func (a *AnalyzeContext) peekCall(index int) *ssa.Call {
	return a.callStack.PeekN(index)
}

func saveDataflowPath(direct AnalysisType, from, to *Value) {
	switch direct {
	case TopDefAnalysis:
		// from(user) -> to(def)
		from.AppendDependOn(to)
	case BottomUseAnalysis:
		// from(def) -> to(user)
		from.AppendEffectOn(to)
	}
}

// maxSavedPathEdges caps the TOTAL dataflow-path edges one AnalyzeContext
// (one getTopDefs/getBottomUses descent) will record via saveDataflowPath. The
// per-Value cap (maxDataflowEdgesPerValue) bounds a single hub; this caps the
// whole descent so a rule that explores millions of nodes can't accumulate an
// unbounded edge graph (the #1 live allocator on javacms-core, retained for
// the whole rule). Path display is best-effort; the taint result is unaffected.
const maxSavedPathEdges = 200000

func (a *AnalyzeContext) SavePath(result Values) {
	if a.recursiveStatusIsLeaf.Len() > 1000 {
		return
	}
	// Total-edge cap: stop building the path edge graph once the descent has
	// recorded maxSavedPathEdges. Path display truncates; the rule's match/alert
	// result (computed from opcode value-sets, not the edge graph) is unaffected.
	if a.savedPathEdges >= maxSavedPathEdges {
		return
	}
	shouldSave := func() bool {
		return a.recursiveStatusIsLeaf.Peek().leaf
	}
	for _, ret := range result {
		if shouldSave() {
			// if len(ret.PrevDataflowPath) == 0 {
			// log.Error("========================")
			{
				// log.Errorf("Ret [%v] StackValue: %v", ret, a.recursiveStatusIsLeaf.Values())
				size := a.recursiveStatusIsLeaf.Len()            // [current, ..... , origin]
				current := a.recursiveStatusIsLeaf.PeekN(0).node // current
				if !ValueCompare(current, ret) {
					return
				}
				for i := 0; i < size; i++ {
					prev := a.recursiveStatusIsLeaf.PeekN(i).node //
					// log.Errorf("Value[%v] prev [%v]", current, prev)
					saveDataflowPath(a.direct, prev, current)
					a.savedPathEdges++
					current = prev
				}
			}
			// log.Error("========================")

			// log.Errorf("node: %v", node)
			// cause
			// cause := actx.causeStack.Values()
			// _ = cause
			// log.Errorf("cause: %v", cause)

			// call stack
			// callStack := actx.callStack.Values()
			// _ = callStack
			// log.Errorf("call stack : %v", callStack)

			// ret.PrevDataflowPath = append(ret.PrevDataflowPath, node...)
			// ret.SetDataflowPath = true
		}
	}

}

// check determines whether to switch the analysis stack based on cross-process and intra-process analysis.
// It ensures that the SSA API analysis maintains correct paths and avoids excessive recursion.
// Returns:
//   - needExit: A boolean indicating whether the analysis should exit early.
//   - recoverStack: A function to restore the state of the analysis stack if needed.
func (a *AnalyzeContext) check(v *Value) (needExit bool, recoverStack func()) {
	defer func() {
		a.needRollBack = false
	}()
	// 跨过程分析
	exit, recoverCrossProcess := a.tryCrossProcess(v)
	if exit {
		return true, recoverCrossProcess
	}
	// 过程内分析
	prev := a.recursiveStatusIsLeaf.Pop()
	prev.leaf = false
	a.recursiveStatusIsLeaf.Push(prev)          // prev status is false, because it have next recursive
	a.recursiveStatusIsLeaf.Push(node{true, v}) // current status is true

	needVisited, recoverIntraProcess := a.valueShould(v)
	recoverStack = func() {
		recoverCrossProcess()
		recoverIntraProcess()
		a.recursiveStatusIsLeaf.Pop()
	}
	if !needVisited {
		return true, recoverStack
	}

	needExit = true
	// depth limited check
	if a.reachedDepthLimited {
		// log.Warnf("reached depth limit,stop it")
		return
	}
	a.enterRecursive()
	// Per-rule total-work budget: bounds cumulative node visits across ALL
	// sources in one dataflow analysis (shared via OperationConfig.workBudget,
	// threaded from sfvm.Config by DataFlowWithSFConfig). Unlike
	// recursiveCounter (per-source, checked next), this is the cross-source
	// bound that prevents N-sources × 5000 node visits on heavy rules. When
	// exceeded it also cancels the rule ctx (budget.cancel) so execRule / other
	// native loops bail via their existing ctx.Done() checks.
	if a.config.workBudget != nil && a.config.workBudget.EnterWork() {
		a.reachedDepthLimited = true
		return
	}
	// 1w recursive call check
	// if !utils.InGithubActions() {
	if a.IsRecursiveLimit() {
		log.Warnf("recursive visit limit reached (%d), stop descent", recursiveStackLimit)
		a.reachedDepthLimited = true
		return
	}
	// }
	if a.depth > 0 && a.config.MaxDepth > 0 && a.depth > a.config.MaxDepth {
		log.Warnf("reached depth limit,stop it")
		a.reachedDepthLimited = true
		return
	}
	if a.depth < 0 && a.config.MinDepth < 0 && a.depth < a.config.MinDepth {
		log.Warnf("reached depth limit,stop it")
		a.reachedDepthLimited = true
		return
	}

	ctx := a.getContext()
	select {
	case <-ctx.Done():
		log.Warnf("context is done, stop it")
		return true, recoverStack
	default:
	}

	needExit = false
	return
}

func (a *AnalyzeContext) getContext() context.Context {
	if a.config != nil && a.config.ctx != nil {
		return a.config.ctx
	}
	return context.Background()
}

// needCrossProcess If the SSA-ID of the function from-value ·and to-value is different,
// it is considered to cross the function boundary,
// which means it is trying to cross process.
func (a *AnalyzeContext) needCrossProcess(from *Value, to *Value) bool {
	if from == nil || from.getValue() == nil || to == nil || to.getValue() == nil {
		return false
	}
	return from.GetFunction().GetId() != to.GetFunction().GetId()
}

func (a *AnalyzeContext) hook(i *Value) error {
	if i.IsLazy() {
		return nil
	}
	if len(a.config.HookEveryNode) > 0 {
		for _, hook := range a.config.HookEveryNode {
			if err := hook(i); err != nil {
				if err.Error() != "abort" {
					log.Errorf("hook-every-node error: %v", err)
				}
				return err
			}
		}
	}
	return nil
}

func (a *AnalyzeContext) isUntilNode(v *Value) bool {
	if a.config.UntilNode != nil {
		if a.config.UntilNode(v) {
			a.untilMatch = append(a.untilMatch, v)
			return true
		} else {
			return false
		}
	}
	return false
}

func (a *AnalyzeContext) HasUntilNode() bool {
	return a.config.UntilNode != nil
}

// ========================================== Recursive Depth Limit ==========================================

func (a *AnalyzeContext) IsRecursiveLimit() bool {
	return atomic.LoadInt64(&a.recursiveCounter) >= recursiveStackLimit
}

func (a *AnalyzeContext) enterRecursive() {
	atomic.AddInt64(&a.recursiveCounter, 1)

}

// ========================================== OBJECT STACK ==========================================

func (g *AnalyzeContext) pushObject(obj, key, member *Value) error {
	if utils.IsNil(obj) || utils.IsNil(key) || utils.IsNil(member) {
		return utils.Errorf("objectStack cannot push nil value")
	}
	if !obj.IsObject() {
		return utils.Errorf("BUG: (objectStack is not clean!) ObjectStack cannot recv")
	}
	shouldVisited, recoverIntra := g.theObjectShouldBeVisited(obj, key, member)
	if !shouldVisited {
		return utils.Errorf("This make object(%d) key(%d) member(%d) valueVisited, skip", obj.GetId(), key.GetId(), member.GetId())
	}
	g._objectStack.Push(&objectItem{
		object:       obj,
		key:          key,
		member:       member,
		recoverIntra: recoverIntra,
	})
	return nil
}

func (g *AnalyzeContext) popObject() (*Value, *Value, *Value) {
	if g._objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g._objectStack.Pop()
	item.recoverIntra()
	return item.object, item.key, item.member
}

func (g *AnalyzeContext) getCurrentObject() (*Value, *Value, *Value) {
	if g._objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g._objectStack.Peek()
	return item.object, item.key, item.member
}
func (g *AnalyzeContext) foreachObjectStack(f func(*Value, *Value, *Value) bool) {
	for i := 0; i < g._objectStack.Len(); i++ {
		item := g._objectStack.PeekN(i)
		if !f(item.object, item.key, item.member) {
			return
		}
	}
}
func (g *AnalyzeContext) CurrentObjectStack() *objectItem {
	return g._objectStack.Peek()
}

func (a *AnalyzeContext) theObjectShouldBeVisited(object, key, member *Value) (bool, func()) {
	return a.objectShould(object, key, member)
}

func (g *AnalyzeContext) AllowIgnoreCallStack() bool {
	if g == nil || g.config == nil {
		return false
	}
	return g.config.AllowIgnoreCallStack
}
