package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type AnalyzeContext struct {
	_callStack  *utils.Stack[*Value]
	_callTable  *omap.OrderedMap[int, *omap.OrderedMap[int, *Value]]
	_visitedPhi *omap.OrderedMap[int, *Value]

	config *OperationConfig

	depth int
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	return &AnalyzeContext{
		_callStack:  utils.NewStack[*Value](),
		_callTable:  omap.NewOrderedMap[int, *omap.OrderedMap[int, *Value]](map[int]*omap.OrderedMap[int, *Value]{}),
		_visitedPhi: omap.NewOrderedMap[int, *Value](map[int]*Value{}),
		config:      NewOperations(opt...),
		depth:       -1,
	}
}

func (a *AnalyzeContext) PushCall(i *Value) error {
	_, ok := i.node.(*ssa.Call)
	if !ok {
		return utils.Errorf("BUG: (callStack is not clean!) CallStack cannot recv %T", i.node)
	}
	if a._callTable.Have(i.GetId()) {
		return utils.Errorf("call[%v] is existed on s-runtime call stack %v", i.GetId(), i.String())
	}
	a._callStack.Push(i)
	a._callTable.Set(i.GetId(), omap.NewOrderedMap[int, *Value](map[int]*Value{}))
	return nil
}

func (a *AnalyzeContext) IsExistedInCallStack(i *Value) bool {
	return a._callTable.Have(i.GetId())
}

func (a *AnalyzeContext) TheCallShouldBeVisited(i *Value) bool {
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

func (g *AnalyzeContext) GetCurrentCall() *Value {
	if g._callStack.Len() <= 0 {
		return nil
	}
	return g._callStack.Peek()
}
