package reactloops

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// swapEmitterForRun 在 in-place nested 运行期间将父任务的 emitter 转发到父
// config，返回恢复函数（无需恢复时返回 nil）。
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

// runWithCurrentTask 在 run 执行期间将 invoker（及父 loop）的 current task 切换
// 为子任务，执行结束后恢复两者。返回 run 的结果。被共享父 timeline 的 in-place
// nested 运行使用。
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

// timelineRollbackCheckpoint 记录父 timeline 的 max id 并返回一个恢复函数，该
// 函数会截断 checkpoint 之后新增的所有条目。若父 invoker 没有可用的 timeline，
// 返回的恢复函数为 no-op。
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

// runSubLoopWithHandle 通过 CreateLoopByName 构建并执行一个子 loop，同时注册
// progress handle，使父 loop 的 stall heartbeat / verification watchdog 能观察
// 子 Agent 的活动。完成或出错时注销 handle。opts 为 loop 选项；configure 可在
// 执行前配置 loop。返回执行完成的子 loop 及其执行错误；loop 创建失败时
// createErr != nil 且 subLoop 为 nil。
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

// forkJobFromNested 将 SubAgentJob 转换为 fork 路径所用的 SubAgentJob（当前为
// 透传，保留以便未来字段裁剪）。
func forkJobFromNested(job SubAgentJob) SubAgentJob {
	return SubAgentJob{
		Order:      job.Order,
		Identifier: job.Identifier,
		Goal:       job.Goal,
		TaskName:   job.TaskName,
		UserInput:  job.UserInput,
	}
}

// nestedScopeName 从 SubAgentJob 推导 nested 子任务的 scope 名。
func nestedScopeName(job SubAgentJob) string {
	return deriveScopeName(job.LoopName, job.TaskName, job.Identifier)
}

// validateNestedJob 检查 nested 运行前需要满足的不变式：原地 trim
// job.LoopName 并验证 loop factory 存在。
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

// runNestedForked 在 fork 出的 timeline 分支中运行一个 SubAgentJob，并将其进度
// 注册到 registry。供 RunNestedJobWithProgress（ForkTimeline = true）使用。
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

// runNestedInPlace 在父 timeline 上（不 fork）运行一个 SubAgentJob，运行结束后
// 回滚期间新增的 timeline 条目，并将其进度注册到 registry。供
// RunNestedJobWithProgress（ForkTimeline = false）使用。
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
