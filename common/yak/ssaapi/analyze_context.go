package ssaapi

import (
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	objectStack *utils.Stack[objectItem]
	// Use for recursive depth limit
	recursiveCounter int64
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		processAnalysisManager: newAnalysisManager(),
		objectStack:            utils.NewStack[objectItem](),
		config:                 NewOperations(opt...),
		depth:                  -1,
	}
	return actx
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
	needVisited, recoverIntraProcess := a.valueShould(v)
	recoverStack = func() {
		recoverCrossProcess()
		recoverIntraProcess()
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
	if !utils.InGithubActions() {
		if a.getRecursiveCounter() > 10000 {
			log.Warnf("recursive call is over 10000, stop it")
			return
		}
	}
	if a.depth > 0 && a.config.MaxDepth > 0 && a.depth > a.config.MaxDepth {
		a.reachedDepthLimited = true
		return
	}
	if a.depth < 0 && a.config.MinDepth < 0 && a.depth < a.config.MinDepth {
		a.reachedDepthLimited = true
		return
	}
	needExit = false
	return
}

// needCrossProcess If the SSA-ID of the function from-value and to-value is different,
// it is considered to cross the function boundary,
// which means it is trying to cross process.
func (a *AnalyzeContext) needCrossProcess(from *Value, to *Value) bool {
	if from == nil || from.node == nil || to == nil || to.node == nil {
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
		return utils.Errorf("BUG: (objectStack is not clean!) ObjectStack cannot recv %T", obj.node)
	}
	shouldVisited, recoverIntra := g.theObjectShouldBeVisited(obj, key, member)
	if !shouldVisited {
		return utils.Errorf("This make object(%d) key(%d) member(%d) valueVisited, skip", obj.GetId(), key.GetId(), member.GetId())
	}
	g.objectStack.Push(objectItem{
		object:       obj,
		key:          key,
		member:       member,
		recoverIntra: recoverIntra,
	})
	return nil
}

func (g *AnalyzeContext) popObject() (*Value, *Value, *Value) {
	if g.objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g.objectStack.Pop()
	item.recoverIntra()
	return item.object, item.key, item.member
}

func (g *AnalyzeContext) getCurrentObject() (*Value, *Value, *Value) {
	if g.objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g.objectStack.Peek()
	return item.object, item.key, item.member
}

func (a *AnalyzeContext) theObjectShouldBeVisited(object, key, member *Value) (bool, func()) {
	return a.objectShould(object, key, member)
}
