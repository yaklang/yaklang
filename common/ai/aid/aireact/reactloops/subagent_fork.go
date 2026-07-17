package reactloops

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// RunForkInvokerCallback 在 timeline fork 出的子 invoker 上执行 fn。
func RunForkInvokerCallback(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	fn ForkInvokerCallback,
) error {
	if fn == nil {
		return utils.Error("fork invoker callback is nil")
	}
	childInvoker, childTask, fork, jobCancel, err := PrepareForkedSubAgent(parentInvoker, parentTask, job)
	if jobCancel != nil {
		defer jobCancel()
	}
	if err != nil {
		return err
	}
	_ = fork
	return fn(childInvoker, childTask)
}

// RunForkJob 在 timeline fork 出的子 Agent 中运行一个 ReAct loop。
func RunForkJob(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	factory ForkLoopFactory,
) (*SubAgentResult, error) {
	startedAt := time.Now()
	if factory == nil {
		return nil, utils.Error("fork sub-loop factory is nil")
	}

	childInvoker, subTask, fork, jobCancel, err := PrepareForkedSubAgent(parentInvoker, parentTask, job)
	if jobCancel != nil {
		defer jobCancel()
	}
	if err != nil {
		return &SubAgentResult{SubAgentJob: job, ExecErr: err, DurationMs: time.Since(startedAt).Milliseconds()}, nil
	}

	subLoop, err := factory(childInvoker, job)
	if err != nil {
		return &SubAgentResult{
			SubAgentJob: job,
			SubTaskID:   subTask.GetId(),
			SubTask:     subTask,
			Fork:        fork,
			ExecErr:     err,
			DurationMs:  time.Since(startedAt).Milliseconds(),
		}, nil
	}

	subTask.SetStatus(aicommon.AITaskState_Processing)
	execErr := subLoop.ExecuteWithExistedTask(subTask)
	return &SubAgentResult{
		SubAgentJob: job,
		SubTaskID:   subTask.GetId(),
		SubTask:     subTask,
		SubLoop:     subLoop,
		Fork:        fork,
		ExecErr:     execErr,
		DurationMs:  time.Since(startedAt).Milliseconds(),
	}, nil
}

// RunForkJobsConcurrently 通过 worker 池并发运行多个 fork 子 loop。
//
// 由于 runJobsConcurrently 直接操作统一的 SubAgentResult 类型，这里先把每个
// SubAgentJob 包装成 SubAgentResult（经内嵌 SubAgentJob 携带任务身份），再由
// runSingle 执行 fork 并填入结果。这样公共 API（[]SubAgentJob -> []SubAgentResult）
// 保持不变，同时让并发辅助只面向单一类型。
func RunForkJobsConcurrently(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	jobs []SubAgentJob,
	concurrency int,
	factory ForkLoopFactory,
) []*SubAgentResult {
	if len(jobs) == 0 {
		return nil
	}
	concurrency = normalizeForkConcurrency(concurrency, len(jobs))

	wrapped := make([]*SubAgentResult, len(jobs))
	for i, job := range jobs {
		wrapped[i] = &SubAgentResult{SubAgentJob: job}
	}
	runSingle := func(r *SubAgentResult) *SubAgentResult {
		result, err := RunForkJob(parentInvoker, parentTask, r.SubAgentJob, factory)
		if err != nil && result == nil {
			return &SubAgentResult{SubAgentJob: r.SubAgentJob, ExecErr: err}
		}
		return result
	}
	return runJobsConcurrently(wrapped, concurrency, runSingle)
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

	// 从 jobCtx 派生子 Agent 的任务 context。不要把 jobCancel 直接绑定到子任务：
	// nested loop（如 fast_context）可能在某子任务结束时让该 child task 完成，
	// 但类别扫描仍需在同一 job scope 上继续运行 phase A→B。
	subTask := aicommon.NewSubTaskBaseWithOptions(
		parentTask,
		subTaskID,
		userInput,
		aicommon.WithStatefulTaskBaseName(subTaskName),
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(jobCtx),
	)
	taskEmitter := BuildForwardingEmitterForTask(parentCfg.GetEmitter(), subTask)
	subTask.SetEmitter(taskEmitter)

	// 使用 subTask.GetContext()（而非 jobCtx）构造子 config，使 subTask.Cancel()
	// 能直接取消子 config 的 context——确保取消时底层 AI HTTP 请求立即中止。
	childInvoker, err := BuildForkReactInvoker(parentCfg, fork, subTask.GetContext(), taskEmitter)
	if err != nil {
		jobCancel()
		return nil, nil, nil, nil, err
	}

	parentInvoker.AddRuntimeTask(subTask)
	childInvoker.SetCurrentTask(subTask)

	branchMarker := fmt.Sprintf("sub-react-branch-marker-%s", subTaskID)
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

// BuildForkReactInvoker 根据 fork 分支和父 config 构建子 Agent 的 invoker。
func BuildForkReactInvoker(
	parentCfg *aicommon.Config,
	fork *aicommon.TimelineFork,
	jobCtx context.Context,
	taskEmitter *aicommon.Emitter,
) (aicommon.AITaskInvokeRuntime, error) {
	baseOpts := aicommon.ConvertConfigToOptions(parentCfg)
	baseOpts = append(baseOpts,
		aicommon.WithTimeline(fork.Branch),
		aicommon.WithContext(jobCtx),
		aicommon.WithAICallbacks(parentCfg.GetRawAICallbacks()),
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithEmitter(taskEmitter),
		aicommon.WithAgreeAuto(),
		aicommon.WithSessionPromptState(parentCfg.SessionPromptState.ForkForSubAgent()),
	)

	// 子 Agent 不得继承任何顶层执行策略。尽管 ConvertConfigToOptions 已省略
	// EnableDispatchSubReactAgents / PreferDispatchSubReactAgents /
	// EnableGoalMode，这里仍显式关闭 plan 和 goal mode，使子 Agent 契约自文档
	// 化，且在 ConvertConfigToOptions 传播逻辑变化时不会静默回退。
	baseOpts = append(baseOpts, buildSubAgentStrategyOptions()...)

	childInvoker, err := aicommon.AIRuntimeInvokerGetter(jobCtx, baseOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "create forked sub react invoker failed")
	}
	return childInvoker, nil
}

// buildSubAgentStrategyOptions 返回子 Agent 强制关闭的顶层策略选项。
func buildSubAgentStrategyOptions() []aicommon.ConfigOption {
	return []aicommon.ConfigOption{
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithEnableGoalMode(false),
		aicommon.WithPreferDispatchSubReactAgents(false),
		aicommon.WithDisableIncreaseIteration(true),
	}
}
