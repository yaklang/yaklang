package yakvm

import (
	"strconv"
	"sync/atomic"
)

type Breakpoint struct {
	ID                      int
	On                      bool
	CodeIndex, LineIndex    int
	Condition, HitCondition string
	State                   string

	HitCount int // 命中次数
}

func (g *Debugger) NewBreakPoint(codeIndex, lineIndex int, condition, hitCondition, state string) *Breakpoint {
	hitCount := 0
	if hitCondition != "" {
		hitCount, _ = strconv.Atoi(hitCondition)
		// 如果存在hitCount,设置hitCondition为空
		if hitCount > 0 {
			hitCondition = ""
		}
	}

	atomic.AddInt32(&g.breakPointCount, 1)
	return &Breakpoint{
		ID:           int(g.breakPointCount),
		On:           true,
		CodeIndex:    codeIndex,
		LineIndex:    lineIndex,
		Condition:    condition,
		HitCondition: hitCondition,
		State:        state,
		HitCount:     hitCount,
	}
}

func (bp *Breakpoint) Enable() {
	bp.On = true
}

func (bp *Breakpoint) Disable() {
	bp.On = false
}
