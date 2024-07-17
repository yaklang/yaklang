package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"sync/atomic"
)

type objectItem struct {
	object *Value
	key    *Value
	member *Value
}

type AnalyzeContext struct {
	// Self
	Self *Value
	// function call stack
	_callStack *utils.Stack[*Value]
	// object visit stack
	_objectStack *utils.Stack[objectItem]

	// for PHI, create  visitedPhi map for  echo call stack
	_callTable *omap.OrderedMap[int64, *omap.OrderedMap[int64, *Value]]
	// in main function, no call stack, we need a global visitedPhi map
	_visitedPhi *omap.OrderedMap[int64, *Value]

	_visitedDefault map[int64]struct{}

	config *OperationConfig

	depth int

	_recursiveCounter int64
}

func (a *AnalyzeContext) GetRecursiveCounter() int64 {
	return atomic.LoadInt64(&a._recursiveCounter)
}

func (a *AnalyzeContext) EnterRecursive() {
	atomic.AddInt64(&a._recursiveCounter, 1)
}

func (a *AnalyzeContext) ExitRecursive() {
	atomic.AddInt64(&a._recursiveCounter, -1)
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	return &AnalyzeContext{
		_callStack:      utils.NewStack[*Value](),
		_objectStack:    utils.NewStack[objectItem](),
		_callTable:      omap.NewOrderedMap[int64, *omap.OrderedMap[int64, *Value]](map[int64]*omap.OrderedMap[int64, *Value]{}),
		_visitedPhi:     omap.NewOrderedMap[int64, *Value](map[int64]*Value{}),
		_visitedDefault: make(map[int64]struct{}),
		config:          NewOperations(opt...),
		depth:           -1,
	}
}

// ========================================== CALL STACK ==========================================

func (a *AnalyzeContext) PushCall(i *Value) error {
	if !i.IsCall() {
		return utils.Errorf("BUG: (callStack is not clean!) CallStack cannot recv %T", i.node)
	}
	if a._callTable.Have(i.GetId()) {
		return utils.Errorf("call[%v] is existed on s-runtime call stack %v", i.GetId(), i.String())
	}
	a._callStack.Push(i)
	a._callTable.Set(i.GetId(), omap.NewOrderedMap[int64, *Value](map[int64]*Value{}))
	return nil
}

func (a *AnalyzeContext) IsExistedInCallStack(i *Value) bool {
	return a._callTable.Have(i.GetId())
}

func (a *AnalyzeContext) TheCallShouldBeVisited(i *ssa.Call) bool {
	// return !a._callTable.Have(i.GetId()) && i.Method.GetId() != a.Self.GetId()
	return !a._callTable.Have(i.GetId())
}

func (a *AnalyzeContext) PopCall() *Value {
	if a._callStack.Len() <= 0 {
		return nil
	}
	val := a._callStack.Pop()
	a._callTable.Delete(val.GetId())
	return val
}

func (g *AnalyzeContext) GetCurrentCall() *Value {
	if g._callStack.Len() <= 0 {
		return nil
	}
	return g._callStack.Peek()
}

// ========================================== OBJECT STACK ==========================================

func (g *AnalyzeContext) PushObject(obj, key, member *Value) error {
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

func (g *AnalyzeContext) TheMemberShouldBeVisited(member *Value) bool {
	for i := 0; i < g._objectStack.Len(); i++ {
		item := g._objectStack.PeekN(i)
		if ValueCompare(item.member, member) {
			return false
		}
	}
	// should visited
	return true
}

// ========================================== PHI STACK ==========================================
// ThePhiShouldBeVisited is used to check whether the phi should be visited
func (a *AnalyzeContext) ThePhiShouldBeVisited(i *Value) bool {
	if a._callStack.Len() <= 0 {
		if a._visitedPhi.Have(i.GetId()) {
			return false
		}
		return true
	}

	visited, ok := a._callTable.Get(a._callStack.Peek().GetId())
	if !ok {
		log.Warnf("peeked call[%v] not bind visited map", a._callStack.Peek().GetId())
		return true
	}
	if !visited.Have(i.GetId()) {
		return true
	}
	return false
}

func (a *AnalyzeContext) VisitPhi(i *Value) {
	if a._callStack.Len() <= 0 {
		a._visitedPhi.Set(i.GetId(), i)
		return
	}
	visited, ok := a._callTable.Get(a._callStack.Peek().GetId())
	if !ok {
		log.Warnf("peeked call[%v] not bind visited map", a._callStack.Peek().GetId())
		return
	}
	visited.Set(i.GetId(), i)
}

// ========================================== DEFAULT STACK ==========================================

func (a *AnalyzeContext) TheDefaultShouldBeVisited(i *Value) bool {
	if _, ok := a._visitedDefault[i.GetId()]; ok {
		return false
	}
	a._visitedDefault[i.GetId()] = struct{}{}
	return true
}
