package ssaapi

import (
	"context"
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

type AnalyzeContext struct {
	// Self
	Self   *Value
	config *OperationConfig
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
	recursiveStatusIsLeaf *utils.Stack[bool]
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		processAnalysisManager: newAnalysisManager(),
		_objectStack:           utils.NewStack[*objectItem](),
		config:                 NewOperations(opt...),
		depth:                  -1,
		callStack:              utils.NewStack[*ssa.Call](),
		recursiveStatusIsLeaf:  utils.NewStack[bool](),
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

func (a *AnalyzeContext) SavePath(result Values) {

	shouldSave := func() bool {
		return a.recursiveStatusIsLeaf.Peek()
	}
	for _, ret := range result {
		if shouldSave() {
			// if len(ret.PrevDataflowPath) == 0 {
			// log.Error("========================")
			{
				// log.Errorf("Ret [%v] StackValue: %v", ret, a.nodeStack.Values())
				size := a.nodeStack.Len()       // [current, ..... , origin]
				current := a.nodeStack.PeekN(0) // current
				if !ValueCompare(current, ret) {
					return
				}
				for i := 0; i < size; i++ {
					prev := a.nodeStack.PeekN(i) //
					// log.Errorf("Value[%v] effect-on [%v]", current, next)
					// log.Errorf("%v(%v) prev %v(%v)", current, current.GetUUID(), prev, prev.GetUUID())
					current.AppendDataFlow(prev)
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
	a.recursiveStatusIsLeaf.Pop()
	a.recursiveStatusIsLeaf.Push(false) // prev status is false, because it have next recursive
	a.recursiveStatusIsLeaf.Push(true)  // current status is true

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
		log.Warnf("reached depth limit,stop it")
		return
	}
	a.enterRecursive()
	// 1w recursive call check
	// if !utils.InGithubActions() {
	if a.getRecursiveCounter() > 10000 {
		log.Warnf("recursive call is over 10000, stop it")
		return
	}
	// }
	if a.depth > 0 && a.config.MaxDepth > 0 && a.depth > a.config.MaxDepth {
		a.reachedDepthLimited = true
		return
	}
	if a.depth < 0 && a.config.MinDepth < 0 && a.depth < a.config.MinDepth {
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

// ========================================== Recursive Depth Limit ==========================================

func (a *AnalyzeContext) getRecursiveCounter() int64 {
	return atomic.LoadInt64(&a.recursiveCounter)
}

func (a *AnalyzeContext) enterRecursive() {
	atomic.AddInt64(&a.recursiveCounter, 1)

}

// ========================================== OBJECT STACK ==========================================

func (g *AnalyzeContext) pushObject(obj, key, member *Value) error {
	if !obj.IsObject() {
		return utils.Errorf("BUG: (objectStack is not clean!) ObjectStack cannot recv %T", obj.innerValue)
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
