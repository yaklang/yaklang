package reactloops

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// ForkJob describes one programmatic sub-agent run (InitTask / orchestrator driven).
type ForkJob struct {
	Order          int
	Identifier     string
	Goal           string
	TaskName       string
	UserInput      string
	ResultContract string
}

// ForkLoopFactory builds the ReAct loop executed inside a forked sub-agent.
type ForkLoopFactory func(childInvoker aicommon.AIInvokeRuntime, job ForkJob) (*ReActLoop, error)

// ForkResult is returned after a forked sub-loop run completes.
type ForkResult struct {
	Order      int
	Identifier string
	SubTaskID  string
	SubTask    aicommon.AIStatefulTask
	SubLoop    *ReActLoop
	Fork       *aicommon.TimelineFork
	ExecErr    error
	DurationMs int64
}

// ForkInvokerCallback runs arbitrary logic on a forked child invoker without starting a ReAct loop.
// Timeline noise stays on the branch; the parent timeline is not truncated afterward.
type ForkInvokerCallback func(childInvoker aicommon.AIInvokeRuntime, childTask aicommon.AIStatefulTask) error
