package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
	"sync"
)

type AnalyzeContext struct {
	Visited   *sync.Map
	CallStack *utils.Stack[*Value]
}

func NewAnalyzeContext() *AnalyzeContext {
	return &AnalyzeContext{
		Visited:   new(sync.Map),
		CallStack: utils.NewStack[*Value](),
	}
}
