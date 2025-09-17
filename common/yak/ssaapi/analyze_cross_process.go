package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const emptyStackHash = "__EmptyStack__"

type intraProcess struct {
	valueVisited  *omap.OrderedMap[int64, struct{}]  // valueId->struct{}
	objectVisited *omap.OrderedMap[string, struct{}] // object hash -> struct{}
}

type processAnalysisManager struct {
	// Use For Cross Process
	// each process stack have an intra process
	crossProcessStack *utils.Stack[string]
	crossProcessMap   *omap.OrderedMap[string, *intraProcess]
	// Each time the value of top-def or bottom-use is called, it is a node.
	nodeStack *utils.Stack[*Value]
	// The value that leads to cross process occurrence
	causeStack *utils.Stack[*Value]
	// needRollBack is a flag that indicates whether the current analysis needs to be rolled back.
	needRollBack bool
}

func newAnalysisManager() *processAnalysisManager {
	c := &processAnalysisManager{
		crossProcessStack: utils.NewStack[string](),
		crossProcessMap:   omap.NewEmptyOrderedMap[string, *intraProcess](),
		nodeStack:         utils.NewStack[*Value](),
		causeStack:        utils.NewStack[*Value](),
	}
	// init cross process stack status
	c.crossProcessStack.Push(emptyStackHash)
	c.crossProcessMap.Set(emptyStackHash, newIntraProcess())
	return c
}

func newIntraProcess() *intraProcess {
	return &intraProcess{
		valueVisited:  omap.NewOrderedMap(map[int64]struct{}{}),
		objectVisited: omap.NewOrderedMap(map[string]struct{}{}),
	}
}

func (p *processAnalysisManager) setRollBack() {
	p.needRollBack = true
}

// tryCrossProcess determines if a cross-process action is needed.
// Returns:
//   - shouldExit: true if a cross-process is required, false otherwise.
//   - recoverCrossProcess: A function to recover the cross-process state if needed.
func (c *processAnalysisManager) tryCrossProcess(v *Value) (shouldExit bool, recoverCrossProcess func()) {
	if !c.needCrossProcess(v) {
		return false, func() {}
	}
	if c.existCrossProcess(v) {
		return true, func() {}
	}
	if c.needRollBack {
		return false, c.rollbackCrossProcess()
	}
	intra := newIntraProcess()
	hash := c.calcCrossProcessHash(v)
	c.crossProcessStack.Push(hash)
	c.crossProcessMap.Set(hash, intra)
	//log.Infof("----> cross process")
	c.pushCause(c.peekNode())
	return false, func() {
		// Recover Cross Process
		c.causeStack.Pop()
		hash = c.crossProcessStack.Pop()
		c.crossProcessMap.Delete(hash)
		//log.Infof("<----- recover cross process")
	}
}

func (c *processAnalysisManager) rollbackCrossProcess() func() {
	cause := c.popCause()
	node := c.popNode()
	hash := c.crossProcessStack.Pop()
	//log.Infof("====> rollback")
	var intra *intraProcess
	if hash != "" && hash != emptyStackHash {
		intra, _ = c.crossProcessMap.Get(hash)
		c.crossProcessMap.Delete(hash)
	}
	return func() {
		//log.Infof("<==== recover rollback")
		c.pushNode(node)
		c.pushCause(cause)
		if intra != nil {
			c.crossProcessStack.Push(hash)
			c.crossProcessMap.Set(hash, intra)
		}
	}
}

func (c *processAnalysisManager) needCrossProcess(v *Value) bool {
	last := c.nodeStack.Peek()
	if utils.IsNil(last) {
		return false
	}
	return needCrossProcess(last, v)
}

func (c *processAnalysisManager) existCrossProcess(current *Value) bool {
	if c.nodeStack.Len() == 0 || utils.IsNil(current) || utils.IsNil(current.innerValue) {
		return false
	}
	hash := c.calcCrossProcessHash(current)
	return c.crossProcessMap.Have(hash)
}

func (c *processAnalysisManager) getCurrentIntraProcess() (*intraProcess, bool) {
	if c.crossProcessStack.Len() == 0 {
		log.Errorf("BUG:The cross process table is empty")
		return nil, false
	}
	hash := c.crossProcessStack.Peek()
	return c.crossProcessMap.Get(hash)
}

// valueShould determines if a value should be processed.
// It checks the current intra-process state to see if the value has already been visited.
// Returns:
//   - bool: true if the value should be processed (not visited before), false otherwise.
//   - func(): A cleanup function to mark the value as unvisited if it was processed.
func (c *processAnalysisManager) valueShould(v *Value) (bool, func()) {
	intra, ok := c.getCurrentIntraProcess()
	if !ok {
		return false, func() {}
	}
	if _, ok = intra.valueVisited.Get(v.GetId()); !ok {
		c.pushNode(v)
		intra.valueVisited.Set(v.GetId(), struct{}{})
		return true, func() {
			c.popNode()
			intra.valueVisited.Delete(v.GetId())
		}
	}
	return false, func() {}
}

func (c *processAnalysisManager) objectShould(object, key, member *Value) (bool, func()) {
	intra, ok := c.getCurrentIntraProcess()
	if !ok {
		return false, func() {}
	}
	if utils.IsNil(object) || utils.IsNil(member) || utils.IsNil(key) {
		return false, func() {}
	}
	hash := utils.CalcSha1(object.GetId(), member.GetId(), key.GetId())
	if _, ok = intra.objectVisited.Get(hash); !ok {
		intra.objectVisited.Set(hash, struct{}{})
		return true, func() {
			intra.objectVisited.Delete(hash)
		}
	}
	return false, func() {}
}

func (c *processAnalysisManager) getLastCauseCall(typ AnalysisType) (result *Value) {
	value := c.peekCause()
	if value == nil {
		return nil
	}
	switch ret := value.innerValue.(type) {
	case *ssa.Call:
		result = value
	case *ssa.SideEffect:
		callSide, ok := ret.GetValueById(ret.CallSite)
		if !ok {
			return nil
		}
		result = value.NewValue(callSide)
	}
	return result
}

func (c *processAnalysisManager) getLastRecursiveNode() *Value {
	return c.peekNNode(1)
}

func (c *processAnalysisManager) calcCrossProcessHash(v *Value) string {
	if c == nil || c.nodeStack == nil {
		return ""
	}
	if c.nodeStack.Len() < 1 {
		return ""
	}
	last := c.nodeStack.Peek()
	return calcCrossProcessHash(last, v)
}

// needCrossProcess determines whether a cross-process transition is required by comparing
// the functions of two values (from and to). If either value or its associated node is nil,
// it returns false. Otherwise, it checks if the IDs of the functions associated with the
// two values are different, indicating a need for a cross-process transition.
func needCrossProcess(from *Value, to *Value) bool {
	if utils.IsNil(from) || utils.IsNil(from.innerValue) || utils.IsNil(to) || utils.IsNil(to.innerValue) {
		return false
	}
	return from.GetFunction().GetId() != to.GetFunction().GetId()
}

func (c *processAnalysisManager) Path() []*Value {
	return c.nodeStack.Values()
}

func (c *processAnalysisManager) pushNode(value *Value) {
	c.nodeStack.Push(value)
}

func (c *processAnalysisManager) popNode() *Value {
	return c.nodeStack.Pop()
}

func (c *processAnalysisManager) popCause() *Value {
	return c.causeStack.Pop()
}

func (c *processAnalysisManager) peekNode() *Value {
	return c.nodeStack.Peek()
}

func (c *processAnalysisManager) peekNNode(n int) *Value {
	return c.nodeStack.PeekN(n)
}

func (c *processAnalysisManager) peekCause() *Value {
	return c.causeStack.Peek()
}

func (c *processAnalysisManager) pushCause(v *Value) {
	c.causeStack.Push(v)
}

func calcCrossProcessHash(from *Value, to *Value) string {
	var (
		fromFuncId, toFuncId int64
		fromId, toId         int64
	)
	if from == nil || from.innerValue == nil {
		fromFuncId = -1
		fromId = -1
	} else {
		fromId = from.GetId()
		fromFuncId = from.GetFunction().GetId()
	}
	if to == nil || to.innerValue == nil {
		toFuncId = -1
		toId = -1
	} else {
		toFuncId = to.GetFunction().GetId()
		toId = to.GetId()
	}
	hash := utils.CalcSha1(fromFuncId, toFuncId, fromId, toId)
	return hash
}
