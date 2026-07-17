package reactloops

import (
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// SubAgentJob is the single, unified descriptor for a sub-agent run. It covers
// every field that the former DispatchJob / ForkJob / NestedJob types carried;
// the distinguishing behavior (fork vs. in-place, AI-elaborated goal vs. raw
// goal, loop-factory vs. loop-name) is selected by the caller / runner, not by
// the type.
type SubAgentJob struct {
	// Order is the 1-based sequence number (used for result sorting / display).
	Order int `json:"order"`
	// Identifier is a stable label for the sub-agent (e.g. "scan_host_a").
	Identifier string `json:"identifier"`
	// Goal is the brief intent; used as userInput when UserInput is empty, or
	// passed to the goal-elaboration step in the dispatch path.
	Goal string `json:"goal"`
	// TaskName is the human-readable task name shown in the timeline / UI.
	TaskName string `json:"task_name"`
	// UserInput overrides Goal as the sub-task's user input if non-empty.
	UserInput string `json:"user_input,omitempty"`
	// ResultContract is an optional acceptance-criteria / output-format hint
	// appended to the user input for fork / dispatch runs.
	ResultContract string `json:"result_contract,omitempty"`
	// LoopName is the registered ReAct loop factory name to run. Defaults to
	// schema.AI_REACT_LOOP_NAME_DEFAULT when empty (dispatch / nested paths).
	LoopName string `json:"loop_name"`
	// ForkTimeline controls timeline isolation for the nested path:
	//   true  → fork the parent timeline (branch diff available, like RunForkedJob)
	//   false → run in-place on the parent timeline (entries rolled back after run,
	//           like RunNestedLoop)
	// The dispatch path always forks and ignores this flag.
	ForkTimeline bool `json:"fork_timeline,omitempty"`
}

// SubAgentResult is the single, unified outcome of a sub-agent run. It embeds
// the originating SubAgentJob so identity (Order / Identifier / ...) is promoted
// and the non-generic runJobsConcurrently can treat one slice as both the job
// carrier and the result. The union of outcome fields (SubLoop / SubTask /
// Record / Feedback / ...) covers all former result types; paths that don't use
// a field simply leave it zero.
type SubAgentResult struct {
	SubAgentJob

	// SubTaskID is the sub-agent task id (set on the fork path).
	SubTaskID string
	// SubTask is the sub-agent stateful task (fork / nested-in-place paths).
	SubTask aicommon.AIStatefulTask
	// SubLoop is the executed ReActLoop (always set on success; may be nil on
	// creation failure).
	SubLoop *ReActLoop
	// Fork is the timeline fork handle (fork path only).
	Fork *aicommon.TimelineFork
	// ExecErr is the execution error (nil on success).
	ExecErr error
	// DurationMs is the run duration in milliseconds (fork path).
	DurationMs int64
	// Duration is the run duration (nested path).
	Duration time.Duration
	// Record is the structured timeline record (dispatch path).
	Record TimelineRecord
	// Feedback is the short human-readable summary (dispatch path).
	Feedback string
}

// ForkLoopFactory builds the ReAct loop executed inside a forked sub-agent.
type ForkLoopFactory func(childInvoker aicommon.AIInvokeRuntime, job SubAgentJob) (*ReActLoop, error)

// ForkInvokerCallback runs arbitrary logic on a forked child invoker without
// starting a ReAct loop. Timeline noise stays on the branch; the parent
// timeline is not truncated afterward.
type ForkInvokerCallback func(childInvoker aicommon.AIInvokeRuntime, childTask aicommon.AIStatefulTask) error
