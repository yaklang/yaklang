package node

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
)

type tickerFunc struct {
	Name            string
	IntervalSeconds int
	F               func()

	first         bool
	firstExecuted *utils.AtomicBool
	currentMod    int
}

func (n *NodeBase) RegisterTickerFunc(name string, intervalSec int, first bool, f func()) error {
	if _, ok := n.tickerFuncs.Load(name); ok {
		return errors.Errorf("register %v failed: %v", name, "existed ticker function")
	}

	n.tickerFuncs.Store(name, &tickerFunc{
		Name:            name,
		IntervalSeconds: intervalSec,
		first:           first,
		firstExecuted:   utils.NewBool(false),
		F:               f,
	})
	return nil
}

func (n *NodeBase) WalkTickerFunc(cb func(name string, f *tickerFunc)) {
	n.tickerFuncs.Range(func(key, value interface{}) bool {
		r := value.(*tickerFunc)
		cb(r.Name, r)
		return true
	})
}

func (n *NodeBase) UnregisterTickerFunc(name string) {
	n.tickerFuncs.Delete(name)
}
