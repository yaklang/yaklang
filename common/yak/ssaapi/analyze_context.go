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

	// savedPath map[*Value]struct{}
	recursiveStatusIsLeaf *utils.Stack[node]
}

type node struct {
	leaf bool
	node *Value
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
	return actx
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

func (a *AnalyzeContext) SavePath(result Values) {
	if a.recursiveStatusIsLeaf.Len() > 1000 {
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
	// 1w recursive call check
	// if !utils.InGithubActions() {
	if a.IsRecursiveLimit() {
		log.Warnf("recursive call is over 10000, stop it")
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
	if from == nil || from.innerValue == nil || to == nil || to.innerValue == nil {
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
