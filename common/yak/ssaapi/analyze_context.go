package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync/atomic"
)

type objectItem struct {
	object *Value
	key    *Value
	member *Value
}

type AnalyzeContext struct {
	// Self
	Self                        *Value
	crossProcessVisitedTable    *crossProcessVisitedTable
	_objectStack                *utils.Stack[objectItem]
	config                      *OperationConfig
	depth                       int
	haveBeenReachedDepthLimited bool
	_recursiveCounter           int64
}

func (a *AnalyzeContext) ReachDepthLimited() {
	a.haveBeenReachedDepthLimited = true
}

func (a *AnalyzeContext) IsReachedDepthLimited() bool {
	return a.haveBeenReachedDepthLimited
}

func (a *AnalyzeContext) GetRecursiveCounter() int64 {
	return atomic.LoadInt64(&a._recursiveCounter)
}

func (a *AnalyzeContext) EnterRecursive() {
	atomic.AddInt64(&a._recursiveCounter, 1)
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		crossProcessVisitedTable: newCrossProcessVisitedTable(),
		//_callStack:   utils.NewStack[*Value](),
		//_negativeCallStack: utils.NewStack[*Value](),
		_objectStack: utils.NewStack[objectItem](),
		//_callTable:   omap.NewEmptyOrderedMap[int64, *CallVisited](),
		config: NewOperations(opt...),
		depth:  -1,
	}
	//actx._callTable.Set(-1, NewCallVisited())
	return actx
}

func (a *AnalyzeContext) CrossProcess(from *Value, to *Value) bool {
	return a.crossProcessVisitedTable.crossProcess(from, to)
}

func (a *AnalyzeContext) RecoverCrossProcess() {
	a.crossProcessVisitedTable.recoverCrossProcess()
}

// ReverseProcess 逆向跨过程，如果正栈为空，将停止逆向过程
func (a *AnalyzeContext) ReverseProcess() (string, bool) {
	return a.crossProcessVisitedTable.reverseProcess(nil, nil)
}

// ReverseProcessWithDirection 逆向跨过程，如果正栈为空，继续往非正栈中继续逆向过程
func (a *AnalyzeContext) ReverseProcessWithDirection(from *Value, to *Value) (string, bool) {
	return a.crossProcessVisitedTable.reverseProcess(from, to)
}

func (a *AnalyzeContext) RecoverReverseProcess(hash string) {
	a.crossProcessVisitedTable.recoverReverseProcess(hash)
}

func (a *AnalyzeContext) GetCallFromLastCrossProcess() *Value {
	table := a.crossProcessVisitedTable.getValueVisitedTable()
	if table.Len() <= 0 {
		return nil
	}
	if a.crossProcessVisitedTable.positiveHashStack.Len() == 0 {
		return nil
	}
	hash := a.crossProcessVisitedTable.positiveHashStack.Peek()
	visited, ok := table.Get(hash)
	if ok {
		if visited.to.IsCall() {
			return visited.to
		} else if visited.from.IsCall() {
			return visited.from
		}
	}
	return nil
}

func (a *AnalyzeContext) TheDefaultShouldBeVisited(i *Value) bool {
	log.Infof("mark visited %s ", i.String())
	valueVisited, ok := a.crossProcessVisitedTable.getCurrentVisited()
	if !ok {
		return false
	}
	if _, ok := valueVisited.visitedDefault[i.GetId()]; !ok {
		valueVisited.visitedDefault[i.GetId()] = struct{}{}
		return true
	}
	return false
}

func (a *AnalyzeContext) ThePhiShouldBeVisited(i *Value) bool {
	valueVisited, ok := a.crossProcessVisitedTable.getCurrentVisited()
	if !ok {
		return false
	}
	if _, ok := valueVisited.visitedPhi[i.GetId()]; !ok {
		valueVisited.visitedPhi[i.GetId()] = struct{}{}
		return true
	}
	return false
}

func (a *AnalyzeContext) TheMemberShouldBeVisited(i *Value) bool {
	valueVisited, ok := a.crossProcessVisitedTable.getCurrentVisited()
	if !ok {
		return false
	}
	if _, ok := valueVisited.visitedObject[i.GetId()]; !ok {
		valueVisited.visitedObject[i.GetId()] = struct{}{}
		return true
	}
	return false
}

func (a *AnalyzeContext) TheCallShouldBeVisited(i *Value) bool {
	valueVisited, ok := a.crossProcessVisitedTable.getCurrentVisited()
	if !ok {
		return false
	}
	if _, ok := valueVisited.visitedCall[i.GetId()]; !ok {
		valueVisited.visitedCall[i.GetId()] = struct{}{}
		return true
	}
	return false
}

// ========================================== OBJECT STACK ==========================================

func (g *AnalyzeContext) PushObject(obj, key, member *Value) error {
	if !g.TheMemberShouldBeVisited(member) {
		return utils.Errorf("This member(%d) visited, skip", member.GetId())
	}
	if !obj.IsObject() {
		return utils.Errorf("BUG: (objectStack is not clean!) ObjectStack cannot recv %T", obj.node)
	}
	if g._objectStack.HaveLastStackValue() {
		last := g._objectStack.LastStackValue()
		if ValueCompare(last.object, obj) &&
			ValueCompare(last.key, key) &&
			ValueCompare(last.member, member) {
			return utils.Errorf("BUG: This object-key recursive.")
		}
	}
	g._objectStack.Push(objectItem{
		object: obj,
		key:    key,
		member: member,
	})
	return nil
}

func (g *AnalyzeContext) PopObject() (*Value, *Value, *Value) {
	if g._objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g._objectStack.Pop()
	return item.object, item.key, item.member
}

func (g *AnalyzeContext) GetCurrentObject() (*Value, *Value, *Value) {
	if g._objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g._objectStack.Peek()
	return item.object, item.key, item.member
}
