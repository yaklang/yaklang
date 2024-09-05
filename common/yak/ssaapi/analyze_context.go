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
	crossProcessVisitedTable    *crossProcessVisitedTable
	_objectStack                *utils.Stack[objectItem]
	config                      *OperationConfig
	depth                       int
	haveBeenReachedDepthLimited bool
}

func (a *AnalyzeContext) check(opt ...OperationOption) bool {
	if a.haveBeenReachedDepthLimited {
		log.Warnf("reached depth limit,stop it")
		return true
	}

	a.EnterRecursive()
	// 1w recursive call check
	if !utils.InGithubActions() {
		if a.GetRecursiveCounter() > 10000 {
			log.Warnf("recursive call is over 10000, stop it")
			return true
		}
	}

	if a.depth > 0 && a.config.MaxDepth > 0 && a.depth > a.config.MaxDepth {
		a.haveBeenReachedDepthLimited = true
		return true
	}
	if a.depth < 0 && a.config.MinDepth < 0 && a.depth < a.config.MinDepth {
		a.haveBeenReachedDepthLimited = true
		return true
	}
	return false
}

func (a *AnalyzeContext) hook(i *Value) error {
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

func (a *AnalyzeContext) GetRecursiveCounter() int64 {
	return atomic.LoadInt64(&a.crossProcessVisitedTable._recursiveCounter)
}

func (a *AnalyzeContext) EnterRecursive() {
	atomic.AddInt64(&a.crossProcessVisitedTable._recursiveCounter, 1)
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		crossProcessVisitedTable: newCrossProcessVisitedTable(),
		_objectStack:             utils.NewStack[objectItem](),
		config:                   NewOperations(opt...),
		depth:                    -1,
	}
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

func (a *AnalyzeContext) TheValueShouldBeVisited(i *Value) bool {
	valueVisited, ok := a.crossProcessVisitedTable.getCurrentVisited()
	if !ok {
		return false
	}
	if _, ok := valueVisited.visited[i.GetId()]; !ok {
		valueVisited.visited[i.GetId()] = struct{}{}
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

func (a *AnalyzeContext) TheCrossProcessVisited(from *Value, to *Value) bool {
	visited := a.crossProcessVisitedTable.valueVisitedTable
	if visited == nil {
		return false
	}
	hash := calcCrossProcessHash(from, to)
	_, ok := visited.Get(hash)
	if ok {
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
