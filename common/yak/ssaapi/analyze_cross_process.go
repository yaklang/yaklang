package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

const emptyStackHash = "__EmptyStack__"

type intraProcess struct {
	valueVisited  *omap.OrderedMap[int64, struct{}]
	objectVisited *omap.OrderedMap[string, struct{}]
	// causeValue is the value which causes the analysis to
	// shift from previous intra process to current intra process.
	causeValue *Value
}

type processAnalysisManager struct {
	// Use For Cross Process
	crossProcessStack *utils.Stack[string]
	crossProcessMap   *omap.OrderedMap[string, *intraProcess]
	_recursiveCounter int64
}

func newAnalysisManager() *processAnalysisManager {
	c := &processAnalysisManager{
		crossProcessStack: utils.NewStack[string](),
		crossProcessMap:   omap.NewEmptyOrderedMap[string, *intraProcess](),
	}
	// init cross process stack status
	c.crossProcessStack.Push(emptyStackHash)
	c.crossProcessMap.Set(emptyStackHash, newIntraProcess(nil))
	return c
}

func newIntraProcess(causeValue *Value) *intraProcess {
	return &intraProcess{
		valueVisited:  omap.NewOrderedMap(map[int64]struct{}{}),
		objectVisited: omap.NewOrderedMap(map[string]struct{}{}),
		causeValue:    causeValue,
	}
}

func (c *processAnalysisManager) CrossProcess(hash string, causeValue *Value) func() {
	if hash == "" {
		return func() {}
	}
	intra := newIntraProcess(causeValue)
	c.crossProcessStack.Push(hash)
	if !c.crossProcessMap.Have(hash) {
		c.crossProcessMap.Set(hash, intra)
	}
	return func() {
		hash = c.crossProcessStack.Pop()
	}
}

func (c *processAnalysisManager) getCrossProcessMap() *omap.OrderedMap[string, *intraProcess] {
	return c.crossProcessMap
}

func (c *processAnalysisManager) deleteCurrentCauseValue(causeValue *Value) {
	hash := c.crossProcessStack.Peek()
	intra, ok := c.crossProcessMap.Get(hash)
	if ok {
		intra.valueVisited.Delete(causeValue.GetId())
	}
}

func (c *processAnalysisManager) getCurrentIntraProcess() (*intraProcess, bool) {
	if c.crossProcessStack.Len() == 0 {
		log.Errorf("BUG:The cross process table is empty")
		return nil, false
	}
	hash := c.crossProcessStack.Peek()
	return c.crossProcessMap.Get(hash)
}

func (c *processAnalysisManager) valueShould(v *Value) (bool, func()) {
	intra, ok := c.getCurrentIntraProcess()
	if !ok {
		return false, func() {}
	}
	if _, ok = intra.valueVisited.Get(v.GetId()); !ok {
		intra.valueVisited.Set(v.GetId(), struct{}{})
		return true, func() {
			log.Infof("recover intra process value: %v", v.String())
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

func (i *intraProcess) push() {

}
