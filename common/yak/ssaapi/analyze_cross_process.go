package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

const emptyStackHash = "__EmptyStack__"

// processStackInfo 保存跨过程中调用栈信息
// 包括用于防止递归的valueVisited和objectVisited，以及实现跨过程的调用call
type processStackInfo struct {
	valueVisited  map[int64]struct{}
	objectVisited map[int64]struct{}
	call          *Value
}

// crossProcessVisitedTable 保存跨过程中信息与调用栈
// 其中crossProcessStack保存跨过程一个值到另外一个值的哈希值
// crossProcessMap保存哈希值到调用栈信息的映射
type crossProcessVisitedTable struct {
	crossProcessStack *utils.Stack[string]
	crossProcessMap   *omap.OrderedMap[string, *processStackInfo]
	_recursiveCounter int64
}

func newCrossProcessTable() *crossProcessVisitedTable {
	c := &crossProcessVisitedTable{
		crossProcessStack: utils.NewStack[string](),
		crossProcessMap:   omap.NewEmptyOrderedMap[string, *processStackInfo](),
	}
	// init cross process stack status
	c.crossProcessStack.Push(emptyStackHash)
	c.crossProcessMap.Set(emptyStackHash, newProcessInfo(nil))
	return c
}

func newProcessInfo(call *Value) *processStackInfo {
	return &processStackInfo{
		valueVisited:  make(map[int64]struct{}),
		objectVisited: make(map[int64]struct{}),
		call:          call,
	}
}

// crossProcess用于记录跨过程行为
// 使用跨过程前和跨过程后的节点做哈希作为唯一一次跨过程行为
// 如果跨过程行为是由call发起的，则是正向跨过程；如果call为nil，则为反向跨过程。
func (c *crossProcessVisitedTable) pushCrossProcess(from *Value, to *Value, call *Value) bool {
	if from == nil || to == nil {
		return false
	}
	hash := calcCrossProcessHash(from, to)
	//log.Infof("cross process from:%s(id:%d)to:%s(id:%d)  call:%s", from.String(), from.GetId(), to.String(), to.GetId(), call.String())
	info := newProcessInfo(call)
	if call != nil && !call.IsCall() {
		log.Errorf("BUG: Cross process behavior is not initiated by a call,but by:%s", call.String())
		return false
	}
	return c.pushCrossProcessWithInfo(hash, info)
}

func (c *crossProcessVisitedTable) pushCrossProcessWithInfo(hash string, info *processStackInfo) bool {
	if hash == "" {
		return false
	}
	if !c.crossProcessMap.Have(hash) {
		c.crossProcessStack.Push(hash)
		c.crossProcessMap.Set(hash, info)
		return true
	}
	return false
}

func (c *crossProcessVisitedTable) popCrossProcess() (string, *processStackInfo) {
	if c.crossProcessStack.Len() == 1 {
		log.Errorf("BUG:Pop CrossProcess fail.The cross process table is empty")
	}
	hash := c.crossProcessStack.Pop()
	info, ok := c.crossProcessMap.Get(hash)
	if ok {
		c.crossProcessMap.Delete(hash)
		return hash, info
	}
	return "", nil
}

func (c *crossProcessVisitedTable) getValueVisitedTable() *omap.OrderedMap[string, *processStackInfo] {
	return c.crossProcessMap
}

func (c *crossProcessVisitedTable) getCurrentProcessInfo() (*processStackInfo, bool) {
	if c.crossProcessStack.Len() == 0 {
		log.Errorf("BUG:The cross process table is empty")
		return nil, false
	}
	hash := c.crossProcessStack.Peek()
	return c.crossProcessMap.Get(hash)
}

func (c *crossProcessVisitedTable) valueShould(v *Value) bool {
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

func (c *crossProcessVisitedTable) memberShould(v *Value) bool {
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

func calcCrossProcessHash(from *Value, to *Value) string {
	fromId := from.GetId()
	//toId := to.GetFunction().GetId()
	toId := to.GetId()
	hash := utils.CalcSha1(fromId, toId)
	return hash
}
