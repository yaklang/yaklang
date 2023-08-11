package yakvm

import "sync/atomic"

type Breakpoint struct {
	ID                   int
	On                   bool
	CodeIndex, LineIndex int
	ConditionCode        string
	State                string
}

func (g *Debugger) NewBreakPoint(codeIndex, lineIndex int, conditionCode, state string) *Breakpoint {
	atomic.AddInt32(&g.breakPointCount, 1)
	return &Breakpoint{
		ID:            int(g.breakPointCount),
		On:            true,
		CodeIndex:     codeIndex,
		LineIndex:     lineIndex,
		ConditionCode: conditionCode,
		State:         state,
	}
}

func (bp *Breakpoint) Enable() {
	bp.On = true
}

func (bp *Breakpoint) Disable() {
	bp.On = false
}
