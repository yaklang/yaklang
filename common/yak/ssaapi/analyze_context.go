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
	_callTable *omap.OrderedMap[int64, *CallVisited]

	// object visit stack
	_objectStack *utils.Stack[objectItem]

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

// CallVisited is used to record the visited phi\object\default,
// and only used in the single call
type CallVisited struct {
	_visitedPhi     map[int64]struct{}
	_visitedObject  map[int64]struct{}
	_visitedDefault map[int64]struct{}
}

func NewCallVisited() *CallVisited {
	ret := &CallVisited{
		_visitedPhi:     make(map[int64]struct{}),
		_visitedObject:  make(map[int64]struct{}),
		_visitedDefault: make(map[int64]struct{}),
	}
	return ret
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		_callStack:   utils.NewStack[*Value](),
		_objectStack: utils.NewStack[objectItem](),
		_callTable:   omap.NewEmptyOrderedMap[int64, *CallVisited](),
		config:       NewOperations(opt...),
		depth:        -1,
	}
	actx._callTable.Set(-1, NewCallVisited())
	return actx
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
	a._callTable.Set(i.GetId(), NewCallVisited())
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

func (a *AnalyzeContext) getVisit() *CallVisited {
	_, callvisited, ok := a._callTable.Last()
	if !ok {
		log.Warnf("peeked call[%v] not bind visited map", a._callStack.Peek().GetId())
		return nil
	}
	return callvisited
}

// ========================================== PHI STACK ==========================================
// ThePhiShouldBeVisited is used to check whether the phi should be visited
func (a *AnalyzeContext) ThePhiShouldBeVisited(i *Value) bool {
	callVisited := a.getVisit()
	if callVisited == nil {
		return false
	}
	if _, ok := callVisited._visitedPhi[i.GetId()]; !ok {
		callVisited._visitedPhi[i.GetId()] = struct{}{}
		return true
	}
	return false
}

// ========================================== DEFAULT STACK ==========================================

func (a *AnalyzeContext) TheDefaultShouldBeVisited(i *Value) bool {
	callVisited := a.getVisit()
	if callVisited == nil {
		return false
	}
	if _, ok := callVisited._visitedDefault[i.GetId()]; !ok {
		callVisited._visitedDefault[i.GetId()] = struct{}{}
		return true
	}
	return false
}

func (a *AnalyzeContext) TheMemberShouldBeVisited(i *Value) bool {
	callVisited := a.getVisit()
	if callVisited == nil {
		return false
	}
	if _, ok := callVisited._visitedObject[i.GetId()]; !ok {
		callVisited._visitedObject[i.GetId()] = struct{}{}
		return true
	}
	return false
}
