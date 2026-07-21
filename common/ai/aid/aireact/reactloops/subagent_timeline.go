package reactloops

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/schema"
)

// TimelineHandle 封装子 Agent 的 timeline 容器，屏蔽 Fork / Clean 差异。
// 阶段 1 创建，阶段 3 释放（目前靠 GC，Release 幂等）。MergeBack 永不调用：
// 子 Agent 的 timeline 一旦 merge 回父，隔离语义彻底失效，父将看到子的全部
// 中间条目——这是 sub agent 不变量所禁止的。
type TimelineHandle struct {
	mode   SubAgentTimelineMode
	fork   *aicommon.TimelineFork // Fork 模式非 nil，Clean 模式为 nil
	branch *aicommon.Timeline     // 子实际使用的 timeline（Fork.Branch 或 Clean 新建）
}

// Timeline 返回子 invoker 应使用的 timeline。
func (h *TimelineHandle) Timeline() *aicommon.Timeline {
	if h == nil {
		return nil
	}
	return h.branch
}

// Fork 返回 Fork 模式的 fork 句柄，Clean 模式返回 nil。
func (h *TimelineHandle) Fork() *aicommon.TimelineFork {
	if h == nil {
		return nil
	}
	return h.fork
}

// Mode 返回 timeline 容器模式。
func (h *TimelineHandle) Mode() SubAgentTimelineMode {
	if h == nil {
		return SubAgentTimelineFork
	}
	return h.mode
}

// DiffPreview 返回 Fork 模式的 fork diff 截断预览和字节计数；Clean 模式返回空。
func (h *TimelineHandle) DiffPreview() (preview string, bytes int) {
	if h == nil || h.fork == nil {
		return "", 0
	}
	return SummarizeForkDiff(h.fork)
}

// Release 释放 timeline 容器。幂等；目前仅标记，实际回收靠 GC。
// Fork 模式不调用 MergeBack——分支随 fork 句柄释放。
func (h *TimelineHandle) Release() {
	// no-op：timeline 随 handle 被 GC 回收；不 merge 回父。
}

// buildTimelineHandle 按模式构建子 Agent 的 timeline 容器。
//
//   - Fork：用父 timeline.ForkForTask 克隆快照，子在分支上运行。
//   - Clean：用 aicommon.NewTimeline 新建空 timeline，子从零开始。
func buildTimelineHandle(
	parentCfg *aicommon.Config,
	parentTimeline *aicommon.Timeline,
	subTaskID, subTaskName string,
	mode SubAgentTimelineMode,
) (*TimelineHandle, error) {
	switch mode {
	case SubAgentTimelineClean:
		branch := aicommon.NewTimeline(parentCfg, nil)
		return &TimelineHandle{mode: SubAgentTimelineClean, branch: branch}, nil
	default: // SubAgentTimelineFork
		if parentTimeline == nil {
			return nil, utils.Error("fork timeline mode requires parent timeline")
		}
		fork, err := parentTimeline.ForkForTask(subTaskID, subTaskName, parentCfg, parentCfg)
		if err != nil {
			return nil, err
		}
		if fork == nil || fork.Branch == nil {
			return nil, utils.Error("failed to create timeline fork for sub-agent")
		}
		return &TimelineHandle{mode: SubAgentTimelineFork, fork: fork, branch: fork.Branch}, nil
	}
}

// buildSubAgentRuntime 为一个 prepared 子 Agent 构建子 invoker 与子 task。
// 它收敛原 fork/nested 三处散落逻辑，统一在阶段 1 完成子运行体构建。
//
// 返回的 Release 必须由调用方在阶段 3 调用（取消 jobCtx + 注销 handle）。
func buildSubAgentRuntime(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	handle *TimelineHandle,
	opts SubAgentOptions,
) (invoker aicommon.AITaskInvokeRuntime, task aicommon.AIStatefulTask, release func(), err error) {
	parentCfg, ok := parentInvoker.GetConfig().(*aicommon.Config)
	if !ok || parentCfg == nil {
		return nil, nil, nil, utils.Error("sub-agent requires parent config to be *aicommon.Config")
	}
	if parentTask == nil {
		return nil, nil, nil, utils.Error("parent task is nil")
	}
	if handle == nil || handle.branch == nil {
		return nil, nil, nil, utils.Error("timeline handle is nil")
	}

	subTaskID := BuildForkTaskID(parentTask, job)
	subTaskName := strings.TrimSpace(job.TaskName)
	if subTaskName == "" {
		subTaskName = strings.TrimSpace(job.Identifier)
	}
	if subTaskName == "" {
		subTaskName = strings.TrimSpace(job.Goal)
	}
	if subTaskName == "" {
		subTaskName = subTaskID
	}

	jobCtx, jobCancel := context.WithCancel(parentTask.GetContext())

	userInput := strings.TrimSpace(job.UserInput)
	if userInput == "" {
		userInput = buildForkUserInput(job)
	}

	// 子 task 的 context 派生自 jobCtx，但子 task 完成不会取消 jobCtx
	//（避免一个子任务完成时连带取消同一 job scope 上的其他工作）。
	//
	// InheritEmitter=true：子 Agent 直接复用父任务的 emitter，共用父任务的
	// TaskId/TaskUUID，不发 react_task_created 卡片——前端表现为父任务自身
	// 的流，用户不会看到额外的子任务卡片（适用于 fast_context 等"子 Agent
	// 对用户不可见"的场景）。
	// InheritEmitter=false（默认）：派生转发 emitter，打上子 TaskId/TaskUUID，
	// 前端显示独立子 Agent 卡片（适用于 dispatch 等需要 UI 区分父子任务的
	// 场景）。
	subTaskOpts := []aicommon.StatefulTaskBaseOption{
		aicommon.WithStatefulTaskBaseName(subTaskName),
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(jobCtx),
	}
	if opts.InheritEmitter {
		// 继承 emitter：跳过 react_task_created 卡片，复用父任务 emitter。
		subTaskOpts = append(subTaskOpts, aicommon.WithStatefulTaskBaseSkipTaskStatusChangeEmit())
	}
	subTask := aicommon.NewSubTaskBaseWithOptions(parentTask, subTaskID, userInput, subTaskOpts...)
	var taskEmitter *aicommon.Emitter
	if opts.InheritEmitter {
		// 复用父任务的 emitter，事件保持父任务的 TaskId，前端不出现新卡片。
		taskEmitter = subTask.GetEmitter()
		if taskEmitter == nil {
			taskEmitter = parentCfg.GetEmitter()
		}
	} else {
		taskEmitter = BuildForwardingEmitterForTask(parentCfg.GetEmitter(), subTask)
	}
	subTask.SetEmitter(taskEmitter)

	childInvoker, err := buildSubAgentInvoker(parentCfg, handle, subTask.GetContext(), taskEmitter)
	if err != nil {
		jobCancel()
		return nil, nil, nil, utils.Wrap(err, "create sub react invoker failed")
	}

	parentInvoker.AddRuntimeTask(subTask)
	childInvoker.SetCurrentTask(subTask)

	if handle.Mode() == SubAgentTimelineFork {
		branchMarker := fmt.Sprintf("sub-react-branch-marker-%s", subTaskID)
		handle.branch.PushText(parentCfg.AcquireId(), branchMarker)
	}

	release = func() {
		if jobCancel != nil {
			jobCancel()
		}
	}
	return childInvoker, subTask, release, nil
}

// buildSubAgentInvoker 根据子 timeline 容器和父 config 构建子 invoker。
// timeline 来源由 TimelineHandle 决定（Fork.Branch 或 Clean 新建）。
func buildSubAgentInvoker(
	parentCfg *aicommon.Config,
	handle *TimelineHandle,
	taskCtx context.Context,
	taskEmitter *aicommon.Emitter,
) (aicommon.AITaskInvokeRuntime, error) {
	baseOpts := aicommon.ConvertConfigToOptions(parentCfg)
	baseOpts = append(baseOpts,
		aicommon.WithTimeline(handle.Timeline()),
		aicommon.WithContext(taskCtx),
		aicommon.WithAICallbacks(parentCfg.GetRawAICallbacks()),
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithEmitter(taskEmitter),
		aicommon.WithAgreeAuto(),
		aicommon.WithSessionPromptState(parentCfg.SessionPromptState.ForkForSubAgent()),
	)
	// 子 Agent 不得继承任何顶层执行策略。显式关闭 plan / goal mode / dispatch
	// / increase iteration，使子 Agent 契约自文档化，且在 ConvertConfigToOptions
	// 传播逻辑变化时不会静默回退。
	baseOpts = append(baseOpts, buildSubAgentStrategyOptions()...)

	childInvoker, err := aicommon.AIRuntimeInvokerGetter(taskCtx, baseOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "create sub react invoker failed")
	}
	return childInvoker, nil
}


// buildSubAgentStrategyOptions 返回子 Agent 强制关闭的顶层策略选项。子 Agent
// 不得继承 plan / goal mode / dispatch / increase iteration。
func buildSubAgentStrategyOptions() []aicommon.ConfigOption {
	return []aicommon.ConfigOption{
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithEnableGoalMode(false),
		aicommon.WithPreferDispatchSubReactAgents(false),
		aicommon.WithDisableIncreaseIteration(true),
	}
}

// BuildForwardingEmitter 通过 PushEventProcesser 从父 config 的 emitter（而非
// 父任务 emitter）派生子 Agent emitter。派生 emitter 共享父前端 sink，同时
// processor 会给每个事件的 TaskId 打上子任务 ID——前端据此聚合子 Agent 消息。
func BuildForwardingEmitter(parentEmitter *aicommon.Emitter, subTaskID string) *aicommon.Emitter {
	if parentEmitter == nil {
		return aicommon.NewDummyEmitter()
	}
	return parentEmitter.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil && subTaskID != "" {
			event.TaskId = subTaskID
		}
		return event
	})
}

// BuildForwardingEmitterForTask 同时打上 TaskId 和 TaskUUID，使工具卡片嵌套在
// react_task_created 创建的子 Agent 卡片下。
func BuildForwardingEmitterForTask(parentEmitter *aicommon.Emitter, task aicommon.AIStatefulTask) *aicommon.Emitter {
	if task == nil {
		return BuildForwardingEmitter(parentEmitter, "")
	}
	taskID := task.GetId()
	taskUUID := task.GetUUID()
	emitter := BuildForwardingEmitter(parentEmitter, taskID)
	if taskUUID == "" {
		return emitter
	}
	return emitter.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil {
			event.TaskUUID = taskUUID
		}
		return event
	})
}

// RunForkInvokerCallback 在 timeline fork 出的子 invoker 上执行 fn，但不启动
// ReAct loop。timeline 噪音留在分支上；父 timeline 不会被截断。
//
// 它是"借 fork invoker 跑一次 LiteForge / 任意逻辑"的轻量场景，不属于 sub
// agent 语义（不下发独立子 Agent 等待结果），故保留为独立入口，不并入
// DispatchSubAgents。
//
// 关键词: WithForkedInvoker, fork invoker callback, LiteForge 借用
func RunForkInvokerCallback(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	fn ForkInvokerCallback,
) error {
	if fn == nil {
		return utils.Error("fork invoker callback is nil")
	}
	child, err := prepareForkedChild(parentInvoker, parentTask, job)
	if err != nil {
		return err
	}
	defer child.release()
	return fn(child.invoker, child.task)
}

// prepareForkedChild fork 父 timeline 并返回打包好的 forkedChild。调用方 defer
// child.release() 即可。
func prepareForkedChild(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
) (*forkedChild, error) {
	invoker, task, fork, jobCancel, err := PrepareForkedSubAgent(parentInvoker, parentTask, job)
	if err != nil {
		if jobCancel != nil {
			jobCancel()
		}
		return nil, err
	}
	return &forkedChild{invoker: invoker, task: task, fork: fork, jobCancel: jobCancel}, nil
}

// PrepareForkedSubAgent fork 父 timeline 并返回子 invoker 和子任务。调用方可在
// 启动 loop 之前修改子任务输入（如 dispatch goal 润色）。
func PrepareForkedSubAgent(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
) (aicommon.AITaskInvokeRuntime, aicommon.AIStatefulTask, *aicommon.TimelineFork, context.CancelFunc, error) {
	parentCfg, ok := parentInvoker.GetConfig().(*aicommon.Config)
	if !ok || parentCfg == nil {
		return nil, nil, nil, nil, utils.Error("forked sub-agent requires parent config to be *aicommon.Config")
	}
	parentTimeline := parentCfg.GetTimeline()
	if parentTimeline == nil {
		return nil, nil, nil, nil, utils.Error("parent timeline is nil")
	}
	if parentTask == nil {
		return nil, nil, nil, nil, utils.Error("parent task is nil")
	}

	subTaskID := BuildForkTaskID(parentTask, job)
	subTaskName := strings.TrimSpace(job.TaskName)
	if subTaskName == "" {
		subTaskName = strings.TrimSpace(job.Identifier)
	}
	if subTaskName == "" {
		subTaskName = strings.TrimSpace(job.Goal)
	}
	if subTaskName == "" {
		subTaskName = subTaskID
	}

	fork, err := parentTimeline.ForkForTask(subTaskID, subTaskName, parentCfg, parentCfg)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if fork == nil || fork.Branch == nil {
		return nil, nil, nil, nil, utils.Error("failed to create timeline fork for sub-agent")
	}

	jobCtx, jobCancel := context.WithCancel(parentTask.GetContext())

	userInput := strings.TrimSpace(job.UserInput)
	if userInput == "" {
		userInput = buildForkUserInput(job)
	}

	// PrepareForkedSubAgent 路径始终使用转发 emitter（InheritEmitter=false 语义，
	// 即子 Agent 拥有独立 TaskId/TaskUUID，前端显示子卡片）。它服务于
	// RunForkInvokerCallback 的轻量 fork 场景。
	subTaskOpts := []aicommon.StatefulTaskBaseOption{
		aicommon.WithStatefulTaskBaseName(subTaskName),
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(jobCtx),
	}
	subTask := aicommon.NewSubTaskBaseWithOptions(parentTask, subTaskID, userInput, subTaskOpts...)
	taskEmitter := BuildForwardingEmitterForTask(parentCfg.GetEmitter(), subTask)
	subTask.SetEmitter(taskEmitter)

	childInvoker, err := buildSubAgentInvoker(parentCfg, &TimelineHandle{mode: SubAgentTimelineFork, fork: fork, branch: fork.Branch}, subTask.GetContext(), taskEmitter)
	if err != nil {
		jobCancel()
		return nil, nil, nil, nil, err
	}

	parentInvoker.AddRuntimeTask(subTask)
	childInvoker.SetCurrentTask(subTask)

	branchMarker := "sub-react-branch-marker-" + subTaskID
	fork.Branch.PushText(parentCfg.AcquireId(), branchMarker)

	return childInvoker, subTask, fork, jobCancel, nil
}

// buildForkUserInput 拼接 fork 子 Agent 的用户输入：Goal + 可选的 ResultContract。
func buildForkUserInput(job SubAgentJob) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(job.Goal))
	if contract := strings.TrimSpace(job.ResultContract); contract != "" {
		sb.WriteString("\n\n## Result Contract\n\n")
		sb.WriteString(contract)
	}
	return sb.String()
}

// forkedChild 把 PrepareForkedSubAgent 的返回值打包，并接管 jobCancel 的释放。
type forkedChild struct {
	invoker   aicommon.AITaskInvokeRuntime
	task      aicommon.AIStatefulTask
	fork      *aicommon.TimelineFork
	jobCancel context.CancelFunc
}

func (c *forkedChild) release() {
	if c == nil || c.jobCancel == nil {
		return
	}
	c.jobCancel()
}
