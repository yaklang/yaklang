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

func (a *AnalyzeContext) GetRecursiveCounter() int64 {
	return atomic.LoadInt64(&a.crossProcessVisitedTable._recursiveCounter)
}

func (a *AnalyzeContext) EnterRecursive() {
	atomic.AddInt64(&a.crossProcessVisitedTable._recursiveCounter, 1)
}

func NewAnalyzeContext(opt ...OperationOption) *AnalyzeContext {
	actx := &AnalyzeContext{
		crossProcessVisitedTable: newCrossProcessTable(),
		_objectStack:             utils.NewStack[objectItem](),
		config:                   NewOperations(opt...),
		depth:                    -1,
	}
	return actx
}

func (a *AnalyzeContext) PushCrossProcess(from *Value, to *Value, call *Value) {
	a.crossProcessVisitedTable.pushCrossProcess(from, to, call)
}

func (a *AnalyzeContext) PushCrossProcessWithHash(hash string, c *CallStackInfo) {
	a.crossProcessVisitedTable.pushCrossProcessWithHash(hash, c)
}

func (a *AnalyzeContext) PopCrossProcess() {
	a.crossProcessVisitedTable.popCrossProcess()
}

func (a *AnalyzeContext) PopCrossProcessStackInfo() (string, *CallStackInfo) {
	return a.crossProcessVisitedTable.popCrossProcess()
}

// GetLastCallStackCall get the last call stack's call
func (a *AnalyzeContext) GetLastCallStackCall() *Value {
	table := a.crossProcessVisitedTable
	if table == nil {
		return nil
	}
	if table.crossProcessStack.Len() <= 0 {
		return nil
	}
	hash := table.crossProcessStack.Peek()
	if hash == emptyStackHash {
		return nil
	}
	visited, ok := table.crossProcessMap.Get(hash)
	if !ok {
		return nil
	}
	return visited.call
}

func (a *AnalyzeContext) TheValueShouldBeVisited(i *Value) bool {
	if i.IsFunction() {
		return true
	}
	valueVisited, ok := a.crossProcessVisitedTable.getCurrentVisited()
	if !ok {
		return false
	}
	if _, ok := valueVisited.valueVisited[i.GetId()]; !ok {
		valueVisited.valueVisited[i.GetId()] = struct{}{}
		return true
	}
	//log.Infof("value(%s) has been visited", i.String())
	return false
}

func (a *AnalyzeContext) TheMemberShouldBeVisited(i *Value) bool {
	valueVisited, ok := a.crossProcessVisitedTable.getCurrentVisited()
	if !ok {
		return false
	}
	if _, ok := valueVisited.objectVisited[i.GetId()]; !ok {
		valueVisited.objectVisited[i.GetId()] = struct{}{}
		return true
	}
	return false
}

func (a *AnalyzeContext) HaveTheCrossProcess(from *Value, to *Value) bool {
	visited := a.crossProcessVisitedTable.crossProcessMap
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
		return utils.Errorf("This member(%d) valueVisited, skip", member.GetId())
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
