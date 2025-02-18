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
	valueVisited  *omap.OrderedMap[int64, struct{}]
	objectVisited *omap.OrderedMap[string, struct{}]
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
		valueVisited:  omap.NewOrderedMap(map[int64]struct{}{}),
		objectVisited: omap.NewOrderedMap(map[string]struct{}{}),
		causeValue:    causeValue,
	}
}

func (c *crossProcess) Cross(hash string, causeValue *Value) func() {
	if hash == "" {
		return func() {}
	}
	info := newProcessInfo(causeValue)
	c.crossProcessStack.Push(hash)
	if !c.crossProcessMap.Have(hash) {
		c.crossProcessMap.Set(hash, info)
	}
	return func() {
		hash = c.crossProcessStack.Pop()
	}
}

func (c *crossProcess) getValueVisitedTable() *omap.OrderedMap[string, *processStackInfo] {
	return c.crossProcessMap
}

func (c *crossProcess) deleteCurrentCauseValue(causeValue *Value) {
	hash := c.crossProcessStack.Peek()
	info, ok := c.crossProcessMap.Get(hash)
	if ok {
		info.valueVisited.Delete(causeValue.GetId())
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
	if _, ok = info.valueVisited.Get(v.GetId()); !ok {
		info.valueVisited.Set(v.GetId(), struct{}{})
		return true
	}
	return false
}
func (c *crossProcess) objectShould(object, key, member *Value) bool {
	info, ok := c.getCurrentProcessInfo()
	if !ok {
		return false
	}
	if utils.IsNil(object) || utils.IsNil(member) || utils.IsNil(key) {
		return false
	}
	hash := utils.CalcSha1(object.GetId(), member.GetId(), key.GetId())
	if _, ok = info.objectVisited.Get(hash); !ok {
		info.objectVisited.Set(hash, struct{}{})
		return true
	}
	return false
}

func (c *crossProcess) nextNode() func() {
	info, ok := c.getCurrentProcessInfo()
	if !ok {
		return func() {}
	}
	valueBackUp := info.valueVisited
	objectBackUp := info.objectVisited
	return func() {
		info.valueVisited = valueBackUp.Copy()
		info.objectVisited = objectBackUp.Copy()
	}
}
