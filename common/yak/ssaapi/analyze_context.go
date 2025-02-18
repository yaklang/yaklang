package ssaapi

import (
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type objectItem struct {
	object *Value
	key    *Value
	member *Value
}

type AnalyzeContext struct {
	// Self
	Self                        *Value
	config                      *OperationConfig
	depth                       int
	haveBeenReachedDepthLimited bool
	// cross process
	crossProcess *crossProcess
	_valueStack  utils.Stack[*Value]
	_causeValue  *Value
	//object
	_objectStack *utils.Stack[objectItem]
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		crossProcess: newCrossProcessTable(),
		_objectStack: utils.NewStack[objectItem](),
		config:       NewOperations(opt...),
		depth:        -1,
	}
	return actx
}

func (a *AnalyzeContext) check(v *Value) (needExit bool, recoverStack func()) {
	// cross process
	a._valueStack.Push(v)
	recoverCrossProcess := a.tryCrossProcess()
	recoverStack = func() {
		recoverCrossProcess()
		a._valueStack.Pop()
	}
	// depth limited check
	needExit = true
	if a.haveBeenReachedDepthLimited {
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
		a.haveBeenReachedDepthLimited = true
		return
	}
	if a.depth < 0 && a.config.MinDepth < 0 && a.depth < a.config.MinDepth {
		a.haveBeenReachedDepthLimited = true
		return
	}
	needExit = false
	return
}

func (a *AnalyzeContext) tryCrossProcess() func() {
	defer func() {
		a._causeValue = nil
	}()

	var recoverCrossProcess func()
	if a._valueStack.Len() > 1 {
		currentValue := a._valueStack.Peek()
		lastValue := a._valueStack.PeekN(1)
		if a.needCrossProcess(lastValue, currentValue) {
			// When the cross process is about to take place, it is necessary to
			// remove the causeValue in current process stack info from the visited table.
			// Because causeValue is the information of the next layer's process stack info.
			if a._causeValue != nil {
				a.crossProcess.deleteCurrentCauseValue(a._causeValue)
			}
			hash := a.calcCrossProcessHash(lastValue, currentValue)
			recoverCrossProcess = a.crossProcess.Cross(hash, a._causeValue)
		}
	}

	recoverWithinProcess := a.crossProcess.nextNode()
	return func() {
		// recover cross process
		if recoverCrossProcess != nil {
			recoverCrossProcess()
		}
		// recover within process
		if recoverWithinProcess != nil {
			recoverWithinProcess()
		}
	}
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

func (a *AnalyzeContext) setCauseValue(v *Value) {
	a._causeValue = v
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

func (a *AnalyzeContext) getRecursiveCounter() int64 {
	return atomic.LoadInt64(&a.crossProcess._recursiveCounter)
}

func (a *AnalyzeContext) enterRecursive() {
	atomic.AddInt64(&a.crossProcess._recursiveCounter, 1)
}

func (a *AnalyzeContext) haveTheCrossProcess(next *Value) bool {
	if a._valueStack.Len() == 0 {
		return false
	}
	lastValue := a._valueStack.Peek()
	hash := a.calcCrossProcessHash(lastValue, next)
	return a.crossProcess.crossProcessMap.Have(hash)
}

func (a *AnalyzeContext) getLastCauseValue() *Value {
	cp := a.crossProcess
	if cp.crossProcessStack.Len() <= 0 {
		return nil
	}
	info, ok := cp.getCurrentProcessInfo()
	if !ok {
		return nil
	}
	return info.causeValue
}

func (a *AnalyzeContext) theValueShouldBeVisited(i *Value) bool {
	return a.crossProcess.valueShould(i)
}

func (a *AnalyzeContext) theObjectShouldBeVisited(object, key, member *Value) bool {
	return a.crossProcess.objectShould(object, key, member)
}

func (a *AnalyzeContext) calcCrossProcessHash(from *Value, to *Value) string {
	var (
		fromFuncId, toFuncId int64
		fromId, toId         int64
		objectHash           string
	)

	getObjectHash := func(v *Value) string {
		if v == nil || v.node == nil || !v.IsCall() {
			return ""
		}
		obj, key, member := a.getCurrentObject()
		return utils.CalcSha1(obj.GetId(), key.GetId(), member.GetId())
	}

	if from == nil || from.node == nil {
		fromFuncId = -1
		fromId = -1
	} else {
		fromId = from.GetId()
		fromFuncId = from.GetFunction().GetId()
		objectHash = getObjectHash(from)
	}

	if to == nil || to.node == nil {
		toFuncId = -1
		toId = -1
	} else {
		toFuncId = to.GetFunction().GetId()
		toFuncId = to.GetId()
	}
	hash := utils.CalcSha1(fromFuncId, toFuncId, fromId, toId, objectHash)
	return hash
}

// ========================================== OBJECT STACK ==========================================

func (g *AnalyzeContext) pushObject(obj, key, member *Value) error {
	if !obj.IsObject() {
		return utils.Errorf("BUG: (objectStack is not clean!) ObjectStack cannot recv %T", obj.node)
	}
	if !g.theObjectShouldBeVisited(obj, key, member) {
		return utils.Errorf("This make object(%d) key(%d) member(%d) valueVisited, skip", obj.GetId(), key.GetId(), member.GetId())
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

func (g *AnalyzeContext) popObject() (*Value, *Value, *Value) {
	if g._objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g._objectStack.Pop()
	return item.object, item.key, item.member
}

func (g *AnalyzeContext) getCurrentObject() (*Value, *Value, *Value) {
	if g._objectStack.Len() <= 0 {
		return nil, nil, nil
	}
	item := g._objectStack.Peek()
	return item.object, item.key, item.member
}
