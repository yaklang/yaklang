package yakvm

type Breakpoint struct {
	On                   bool
	CodeIndex, LineIndex int
	ConditionCode        string
	State                string
}

func NewBreakPoint(codeIndex, lineIndex int, conditionCode, state string) *Breakpoint {
	return &Breakpoint{
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
