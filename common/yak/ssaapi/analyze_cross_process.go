package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

const emptyStackHash = "__EmptyStack__"

// processStackInfo 保存每次跨过程的详细信息
// 包括用于防止递归的valueVisited和objectVisited
// 以及导致此次跨过程的causeValue
type processStackInfo struct {
	valueVisited  map[int64]struct{}
	objectVisited map[int64]struct{}
	causeValue    *Value
}

// crossProcess 保存各个跨过程的信息
// 其中crossProcessStack保存跨过程唯一hash
// crossProcessMap保存hash到跨过程详细信息的映射
type crossProcess struct {
	crossProcessStack *utils.Stack[string]
	crossProcessMap   *omap.OrderedMap[string, *processStackInfo]
	_recursiveCounter int64
}

func newCrossProcessTable() *crossProcess {
	c := &crossProcess{
		crossProcessStack: utils.NewStack[string](),
		crossProcessMap:   omap.NewEmptyOrderedMap[string, *processStackInfo](),
	}
	// init cross process stack status
	c.crossProcessStack.Push(emptyStackHash)
	c.crossProcessMap.Set(emptyStackHash, newProcessInfo(nil))
	return c
}

func newProcessInfo(causeValue *Value) *processStackInfo {
	return &processStackInfo{
		valueVisited:  make(map[int64]struct{}),
		objectVisited: make(map[int64]struct{}),
		causeValue:    causeValue,
	}
}

func (c *crossProcess) pushCrossProcess(from *Value, to *Value, causeValue *Value) func() {
	hash := calcCrossProcessHash(from, to)
	info := newProcessInfo(causeValue)
	return c.pushCrossProcessWithInfo(hash, info)
}

func (c *crossProcess) pushCrossProcessWithInfo(hash string, info *processStackInfo) func() {
	if hash == "" {
		return func() {}
	}
	//the cross process is already exist,
	if c.crossProcessMap.Have(hash) {
		c.crossProcessStack.Push(hash)
		return func() {
			hash = c.crossProcessStack.Pop()
		}
	}
	//the cross process is not exist
	c.crossProcessStack.Push(hash)
	c.crossProcessMap.Set(hash, info)
	return func() {
		hash = c.crossProcessStack.Pop()
		c.crossProcessMap.Delete(hash)
	}
}

func (c *crossProcess) getValueVisitedTable() *omap.OrderedMap[string, *processStackInfo] {
	return c.crossProcessMap
}

func (c *crossProcess) deleteCurrentCauseValue(causeValue *Value) {
	hash := c.crossProcessStack.Peek()
	info, ok := c.crossProcessMap.Get(hash)
	if ok {
		delete(info.valueVisited, causeValue.GetId())
	}
}

func (c *crossProcess) getCurrentProcessInfo() (*processStackInfo, bool) {
	if c.crossProcessStack.Len() == 0 {
		log.Errorf("BUG:The cross process table is empty")
		return nil, false
	}
	hash := c.crossProcessStack.Peek()
	return c.crossProcessMap.Get(hash)
}

func (c *crossProcess) valueShould(v *Value) bool {
	info, ok := c.getCurrentProcessInfo()
	if !ok {
		return false
	}
	if _, ok := info.valueVisited[v.GetId()]; !ok {
		info.valueVisited[v.GetId()] = struct{}{}
		return true
	}
	return false
}

func (c *crossProcess) memberShould(v *Value) bool {
	info, ok := c.getCurrentProcessInfo()
	if !ok {
		return false
	}
	if _, ok := info.objectVisited[v.GetId()]; !ok {
		info.objectVisited[v.GetId()] = struct{}{}
		return true
	}
	return false
}

// calcCrossProcessHash Calculate cross process hash using the ssa-id of functions from-value and to-value.
// If from-value and to-value do not have a function, then use -1 for calculation
func calcCrossProcessHash(from *Value, to *Value) string {
	var fromId, toId int64
	if from == nil {
		fromId = -1
	} else {
		fromId = from.GetFunction().GetId()
	}
	if to == nil {
		toId = -1
	} else {
		toId = to.GetFunction().GetId()
	}
	hash := utils.CalcSha1(fromId, toId)
	return hash
}
