package reactloops

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// swapEmitterForRun forwards the parent task's emitter to the parent config for
// the duration of an in-place nested run, returning a restore function (or nil).
func swapEmitterForRun(parentInvoker aicommon.AIInvokeRuntime, parentTask aicommon.AIStatefulTask) func() {
	if parentTask == nil {
		return nil
	}
	if cfg, ok := parentInvoker.GetConfig().(*aicommon.Config); ok && cfg != nil {
		if parentTask.GetEmitter() != nil {
			return cfg.SwapEmitter(parentTask.GetEmitter())
		}
	}
	return nil
}

// runWithCurrentTask swaps the invoker's (and parent loop's) current task to the
// sub-task for the duration of run, restoring both afterwards. Returns run's
// result. Used by in-place nested runs that share the parent timeline.
func runWithCurrentTask[T any](
	parentInvoker aicommon.AIInvokeRuntime,
	subTask aicommon.AIStatefulTask,
	run func() T,
) T {
	prevInvokerTask := parentInvoker.GetCurrentTask()
	var prevParentLoop *ReActLoop
	if prevInvokerTask != nil {
		if parent, ok := prevInvokerTask.GetReActLoop().(*ReActLoop); ok {
			prevParentLoop = parent
		}
	}
	parentInvoker.SetCurrentTask(subTask)
	defer parentInvoker.SetCurrentTask(prevInvokerTask)
	defer func() {
		if prevParentLoop != nil {
			prevParentLoop.SetCurrentTask(prevInvokerTask)
		}
	}()
	return run()
}

// timelineRollbackCheckpoint captures the parent timeline's max id and returns a
// restore function that truncates any entries added after the checkpoint. If the
// parent invoker has no usable timeline, the returned restore is a no-op.
func timelineRollbackCheckpoint(parentInvoker aicommon.AIInvokeRuntime) func() {
	if cfg := parentInvoker.GetConfig(); cfg != nil {
		if c, ok := cfg.(*aicommon.Config); ok && c.Timeline != nil {
			timeline := c.Timeline
			checkpoint := timeline.GetMaxID()
			return func() {
				removed := countTimelineIDsAfter(timeline, checkpoint)
				timeline.TruncateAfter(checkpoint)
				if removed > 0 {
					log.Infof("[SubAgent] timeline rollback: removed %d entries", removed)
				}
			}
		}
	}
	return func() {}
}

// runSubLoopWithHandle builds (via CreateLoopByName) and executes a sub-loop,
// registering a progress handle so the parent's stall heartbeat / verification
// watchdog can observe the sub-agent. The handle is unregistered on completion
// or error. opts are the loop options; configure optionally configures the loop
// before execution. Returns the executed sub-loop and its execution error. When
// loop creation fails, createErr != nil and subLoop is nil.
func runSubLoopWithHandle(
	invoker aicommon.AIInvokeRuntime,
	loopName string,
	subTask aicommon.AIStatefulTask,
	identifier string,
	registry *ProgressRegistry,
	startedAt time.Time,
	opts []ReActLoopOption,
	configure func(subLoop *ReActLoop),
) (subLoop *ReActLoop, execErr error) {
	handle := registerHandle(registry, subTask.GetId(), identifier, subTask, startedAt)

	loop, createErr := CreateLoopByName(loopName, invoker, opts...)
	if createErr != nil {
		unregisterHandle(handle, registry, subTask.GetId(), createErr)
		return nil, createErr
	}
	if handle != nil {
		handle.SubLoop = loop
	}
	if configure != nil {
		configure(loop)
	}

	subTask.SetStatus(aicommon.AITaskState_Processing)
	execErr = loop.ExecuteWithExistedTask(subTask)
	unregisterHandle(handle, registry, subTask.GetId(), execErr)
	return loop, execErr
}

// forkJobFromNested converts a SubAgentJob to the SubAgentJob used by the fork path.
func forkJobFromNested(job SubAgentJob) SubAgentJob {
	return SubAgentJob{
		Order:      job.Order,
		Identifier: job.Identifier,
		Goal:       job.Goal,
		TaskName:   job.TaskName,
		UserInput:  job.UserInput,
	}
}

// nestedScopeName derives the nested sub-task scope name from a SubAgentJob.
func nestedScopeName(job SubAgentJob) string {
	return deriveScopeName(job.LoopName, job.TaskName, job.Identifier)
}

// validateNestedJob checks the invariants required before a nested run. It
// trims job.LoopName in place and verifies the loop factory exists.
func validateNestedJob(job *SubAgentJob) error {
	if job == nil {
		return utils.Error("nested job is nil")
	}
	job.LoopName = strings.TrimSpace(job.LoopName)
	if job.LoopName == "" {
		return utils.Error("nested job loop_name is required")
	}
	if _, ok := GetLoopFactory(job.LoopName); !ok {
		return utils.Errorf("nested job loop_name %q is not registered", job.LoopName)
	}
	return nil
}

// runNestedForked runs one SubAgentJob in a forked timeline branch and registers
// its progress into registry. Shared by RunNestedJobWithProgress (ForkTimeline).
func runNestedForked(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	registry *ProgressRegistry,
	configure func(subLoop *ReActLoop),
	opts []ReActLoopOption,
	startedAt time.Time,
) (*SubAgentResult, error) {
	childInvoker, subTask, fork, jobCancel, err := PrepareForkedSubAgent(parentInvoker, parentTask, forkJobFromNested(job))
	if jobCancel != nil {
		defer jobCancel()
	}
	if err != nil {
		return &SubAgentResult{SubAgentJob: job, ExecErr: err, Duration: time.Since(startedAt)}, nil
	}
	subLoop, execErr := runSubLoopWithHandle(
		childInvoker, job.LoopName, subTask, job.Identifier, registry, startedAt,
		append(opts, DefaultForkOptions()...), configure,
	)
	_ = fork
	return &SubAgentResult{
		SubAgentJob: job, SubLoop: subLoop, SubTask: subTask, ExecErr: execErr, Duration: time.Since(startedAt),
	}, nil
}

// runNestedInPlace runs one SubAgentJob on the parent timeline (no fork), rolling
// back any timeline entries created during the run, and registers its progress
// into registry. Shared by RunNestedJobWithProgress (!ForkTimeline).
func runNestedInPlace(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	registry *ProgressRegistry,
	configure func(subLoop *ReActLoop),
	opts []ReActLoopOption,
	startedAt time.Time,
) (*SubAgentResult, error) {
	defer timelineRollbackCheckpoint(parentInvoker)()

	nestedTask := newNestedSubTask(parentTask, nestedScopeName(job))
	userInput := strings.TrimSpace(job.UserInput)
	if userInput == "" {
		userInput = strings.TrimSpace(job.Goal)
	}
	if userInput != "" {
		nestedTask.SetUserInput(userInput)
	}

	restoreEmitter := swapEmitterForRun(parentInvoker, parentTask)
	if restoreEmitter != nil {
		defer restoreEmitter()
	}

	type out struct{ loop *ReActLoop; execErr error }
	o := runWithCurrentTask(parentInvoker, nestedTask, func() out {
		loop, execErr := runSubLoopWithHandle(
			parentInvoker, job.LoopName, nestedTask, job.Identifier, registry, startedAt,
			append(opts, DefaultForkOptions()...), configure,
		)
		return out{loop, execErr}
	})
	return &SubAgentResult{
		SubAgentJob: job, SubLoop: o.loop, SubTask: nestedTask, ExecErr: o.execErr, Duration: time.Since(startedAt),
	}, nil
}
