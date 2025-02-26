package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

const emptyStackHash = "__EmptyStack__"

type intraProcess struct {
	valueVisited  *omap.OrderedMap[int64, struct{}]
	objectVisited *omap.OrderedMap[string, struct{}]
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

// tryCrossProcess determines if a cross-process action is needed.
// Returns:
//   - shouldExit: true if a cross-process is required, false otherwise.
//   - recoverCrossProcess: A function to recover the cross-process state if needed.
func (c *processAnalysisManager) tryCrossProcess() (shouldExit bool, recoverCrossProcess func()) {
	current, last, err := c.getCurrentAndLastNode()
	if err != nil {
		return false, func() {}
	}
	if !c.needCrossProcess(last, current) {
		return false, func() {}
	}

	intra := newIntraProcess()
	hash := c.calcCrossProcessHash(last, current)
	if c.crossProcessMap.Have(hash) {
		return true, func() {}
	}
	c.crossProcessStack.Push(hash)
	c.crossProcessMap.Set(hash, intra)
	c.causeStack.Push(last)
	return false, func() {
		// Recover Cross Process
		c.causeStack.Pop()
		hash = c.crossProcessStack.Pop()
		c.crossProcessMap.Delete(hash)
	}
}

// needCrossProcess determines whether a cross-process transition is required by comparing
// the functions of two values (from and to). If either value or its associated node is nil,
// it returns false. Otherwise, it checks if the IDs of the functions associated with the
// two values are different, indicating a need for a cross-process transition.
func (c *processAnalysisManager) needCrossProcess(from *Value, to *Value) bool {
	if utils.IsNil(from) || utils.IsNil(from.node) || utils.IsNil(to) || utils.IsNil(to.node) {
		return false
	}
	return from.GetFunction().GetId() != to.GetFunction().GetId()
}

// haveCrossProcess determines if the next analysis of the given value will involve a cross-process analysis.
// This function is typically used in scenarios where it's necessary to know whether the subsequent analysis
// (e.g., top-def or bottom-use) for the current value will require a cross-process transition.
// Returns:
//   - bool: true if the next analysis is a cross-process analysis, false otherwise.
func (c *processAnalysisManager) haveCrossProcess(current *Value) bool {
	if c.nodeStack.Len() == 0 || utils.IsNil(current) || utils.IsNil(current.node) {
		return false
	}

	last := c.nodeStack.Peek()
	hash := c.calcCrossProcessHash(last, current)
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
		intra.valueVisited.Set(v.GetId(), struct{}{})
		return true, func() {
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

func (c *processAnalysisManager) pushNode(value *Value) {
	c.nodeStack.Push(value)
}

func (c *processAnalysisManager) popNode() *Value {
	return c.nodeStack.Pop()
}

func (c *processAnalysisManager) getCurrentAndLastNode() (current *Value, last *Value, err error) {
	if c == nil || c.nodeStack == nil {
		return nil, nil, utils.Errorf("BUG: nodeStack is nil")
	}
	if c.nodeStack.Len() < 2 {
		return nil, nil, utils.Errorf("BUG: nodeStack length is less than 2")
	}
	current = c.nodeStack.Peek()
	last = c.nodeStack.PeekN(1)
	if utils.IsNil(current) || utils.IsNil(current.node) || utils.IsNil(last) || utils.IsNil(last.node) {
		return nil, nil, utils.Errorf("BUG: current or last node is nil")
	}
	return current, last, nil
}

func (c *processAnalysisManager) getLastCauseCall(typ AnalysisType) *Value {
	value := c.causeStack.Pop()
	if value == nil {
		return nil
	}
	switch ret := value.node.(type) {
	case *ssa.Call:
		return value
	case *ssa.SideEffect:
		if typ == TopDefAnalysis {
			return value.NewTopDefValue(ret.CallSite)
		} else if typ == BottomUseAnalysis {
			return value.NewBottomUseValue(ret.CallSite)
		}
	}
	return nil
}

func (c *processAnalysisManager) calcCrossProcessHash(from *Value, to *Value) string {
	var (
		fromFuncId, toFuncId int64
		fromId, toId         int64
	)
	//getObjectHash := func(v *Value) string {
	//	if v == nil || v.node == nil || !v.IsCall() {
	//		return ""
	//	}
	//	obj, key, member := a.getCurrentObject()
	//	return utils.CalcSha1(obj.GetId(), key.GetId(), member.GetId())
	//}
	if from == nil || from.node == nil {
		fromFuncId = -1
		fromId = -1
	} else {
		fromId = from.GetId()
		fromFuncId = from.GetFunction().GetId()
		//objectHash = getObjectHash(from)
	}
	if to == nil || to.node == nil {
		toFuncId = -1
		toId = -1
	} else {
		toFuncId = to.GetFunction().GetId()
		toId = to.GetId()
	}
	// TODO感觉可以不用fromId和toId
	hash := utils.CalcSha1(fromFuncId, toFuncId, fromId, toId)
	return hash
}
