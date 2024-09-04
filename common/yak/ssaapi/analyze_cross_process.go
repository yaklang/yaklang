package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type valueVisited struct {
	from             *Value
	to               *Value
	visitedPhi       map[int64]struct{}
	visitedObject    map[int64]struct{}
	visitedDefault   map[int64]struct{}
	visitedCall      map[int64]struct{}
	visitedParameter map[int64]struct{}
}

type crossProcessVisitedTable struct {
	positiveHashStack    *utils.Stack[string]
	nonPositiveHashStack *utils.Stack[string]
	valueVisitedTable    *omap.OrderedMap[string, *valueVisited]
	_recursiveCounter    int64
}

func newCrossProcessVisitedTable() *crossProcessVisitedTable {
	c := &crossProcessVisitedTable{
		positiveHashStack:    utils.NewStack[string](),
		nonPositiveHashStack: utils.NewStack[string](),
		valueVisitedTable:    omap.NewEmptyOrderedMap[string, *valueVisited](),
	}
	// init empty stack
	hash := codec.Sha1("empty")
	c.nonPositiveHashStack.Push(hash)
	visited := newDefaultValueVisited()
	c.valueVisitedTable.Set(hash, visited)
	return c
}

func newValueVisited(from *Value, to *Value) *valueVisited {
	return &valueVisited{
		from:             from,
		to:               to,
		visitedPhi:       make(map[int64]struct{}),
		visitedObject:    make(map[int64]struct{}),
		visitedDefault:   make(map[int64]struct{}),
		visitedCall:      make(map[int64]struct{}),
		visitedParameter: make(map[int64]struct{}),
	}
}

func newDefaultValueVisited() *valueVisited {
	return &valueVisited{
		from:             nil,
		to:               nil,
		visitedPhi:       make(map[int64]struct{}),
		visitedObject:    make(map[int64]struct{}),
		visitedDefault:   make(map[int64]struct{}),
		visitedCall:      make(map[int64]struct{}),
		visitedParameter: make(map[int64]struct{}),
	}
}

func (c *crossProcessVisitedTable) crossProcess(from *Value, to *Value) (crossSuccess bool) {
	if from == nil || to == nil {
		return false
	}
	hash := calcCrossProcessHash(from, to)
	//log.Infof("cross process from:%s to:%s hash:%s", from.String(), to.String(), hash)
	if c.valueVisitedTable.Have(hash) {
		return false
	}
	visited := newValueVisited(from, to)
	c.valueVisitedTable.Set(hash, visited)
	c.positiveHashStack.Push(hash)
	return true
}

func (c *crossProcessVisitedTable) recoverCrossProcess() {
	if c.positiveHashStack.Len() == 0 {
		log.Warnf("Pop CrossProcess fail.The cross process table is empty")
	}
	c.positiveHashStack.Pop()
}

func (c *crossProcessVisitedTable) isInPositiveStack() bool {
	return c.positiveHashStack.Len() > 0
}

func (c *crossProcessVisitedTable) isInNonNegativeStack() bool {
	return c.positiveHashStack.Len() > 0 || c.nonPositiveHashStack.Len() == 1
}

func (c *crossProcessVisitedTable) reverseProcess(from *Value, to *Value) (hash string, reverseSuccess bool) {
	if !c.isInPositiveStack() {
		if from == nil || to == nil {
			return "", false
		}
		hash := calcCrossProcessHash(from, to)
		//log.Infof("reverse process from:%s to:%s", from.String(), to.String())
		if c.valueVisitedTable.Have(hash) {
			return hash, false
		}
		c.nonPositiveHashStack.Push(hash)
		visited := newValueVisited(from, to)
		c.valueVisitedTable.Set(hash, visited)
		return hash, true
	}
	return c.positiveHashStack.Pop(), true
}

func (c *crossProcessVisitedTable) recoverReverseProcess(hash string) {
	if !c.isInNonNegativeStack() {
		c.nonPositiveHashStack.Pop()
	} else {
		c.positiveHashStack.Push(hash)
	}
}

func (c *crossProcessVisitedTable) getValueVisitedTable() *omap.OrderedMap[string, *valueVisited] {
	return c.valueVisitedTable
}

func (c *crossProcessVisitedTable) getCurrentVisited() (*valueVisited, bool) {
	if c.isInPositiveStack() {
		hash := c.positiveHashStack.Peek()
		return c.valueVisitedTable.Get(hash)
	} else {
		if c.nonPositiveHashStack.Len() == 0 {
			return nil, false
		}
		hash := c.nonPositiveHashStack.Peek()
		return c.valueVisitedTable.Get(hash)
	}
}

func calcCrossProcessHash(from *Value, to *Value) string {
	fromId := from.GetId()
	//toId := to.GetFunction().GetId()
	toId := to.GetId()
	hash := utils.CalcSha1(fromId,toId)
	return hash
}
