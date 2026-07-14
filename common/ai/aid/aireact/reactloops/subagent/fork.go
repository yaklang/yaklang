package subagent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils"
)

// KeepAliveFunc is called periodically while the parent loop is blocked waiting
// for forked sub-agents to finish. Its purpose is to refresh the parent loop's
// stall-heartbeat tick so that the stall detector does not misfire during
// legitimate long-running sub-agent waits. A nil KeepAliveFunc means no
// keep-alive signalling is needed (e.g. the caller has no stall heartbeat).
type KeepAliveFunc func()

// keepAliveInterval is the period between KeepAliveFunc calls while waiting for
// sub-agents. It is shorter than the stall-heartbeat interval (30s) so the
// parent tick stays fresh well before the 90s stuck threshold.
const keepAliveInterval = 15 * time.Second

// RunKeepAlive starts a goroutine that periodically calls keepAlive until
// the returned stop function is called. If keepAlive is nil it returns a
// no-op stop function. The stop function closes the internal stop channel
// and waits for the goroutine to exit.
//
// This is exported so that callers outside the subagent package (e.g.
// loopinfra's dispatch concurrency pool) can reuse the same keep-alive
// ticker pattern while blocking on sub-agent completion.
func RunKeepAlive(keepAlive KeepAliveFunc) func() {
	if keepAlive == nil {
		return func() {}
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		ticker := time.NewTicker(keepAliveInterval)
		defer ticker.Stop()
		keepAlive() // fire immediately so the tick is fresh from the start
		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				keepAlive()
			}
		}
	}()
	return func() {
		close(stopCh)
		<-doneCh
	}
}

// RunForkInvokerCallback executes fn on a timeline-forked child invoker.
func RunForkInvokerCallback(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job ForkJob,
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

// RunForkJob runs one ReAct loop inside a timeline-forked sub-agent.
func RunForkJob(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job ForkJob,
	factory ForkLoopFactory,
	keepAlive KeepAliveFunc,
) (*ForkResult, error) {
	startedAt := time.Now()
	if factory == nil {
		return nil, utils.Error("fork sub-loop factory is nil")
	}

	// Refresh the parent tick before entering the potentially long sub-loop.
	if keepAlive != nil {
		keepAlive()
	}

	childInvoker, subTask, fork, jobCancel, err := PrepareForkedSubAgent(parentInvoker, parentTask, job)
	if jobCancel != nil {
		defer jobCancel()
	}
	if err != nil {
		return &ForkResult{
			Order:      job.Order,
			Identifier: job.Identifier,
			ExecErr:    err,
			DurationMs: time.Since(startedAt).Milliseconds(),
		}, nil
	}

	subLoop, err := factory(childInvoker, job)
	if err != nil {
		return &ForkResult{
			Order:      job.Order,
			Identifier: job.Identifier,
			SubTaskID:  subTask.GetId(),
			SubTask:    subTask,
			Fork:       fork,
			ExecErr:    err,
			DurationMs: time.Since(startedAt).Milliseconds(),
		}, nil
	}

	subTask.SetStatus(aicommon.AITaskState_Processing)
	execErr := subLoop.ExecuteWithExistedTask(subTask)

	// Refresh again after the sub-loop returns so the parent tick is current.
	if keepAlive != nil {
		keepAlive()
	}

	return &ForkResult{
		Order:      job.Order,
		Identifier: job.Identifier,
		SubTaskID:  subTask.GetId(),
		SubTask:    subTask,
		SubLoop:    subLoop,
		Fork:       fork,
		ExecErr:    execErr,
		DurationMs: time.Since(startedAt).Milliseconds(),
	}, nil
}

// RunForkJobsConcurrently runs multiple forked sub-loops with a worker pool.
func RunForkJobsConcurrently(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	jobs []ForkJob,
	concurrency int,
	factory ForkLoopFactory,
	keepAlive KeepAliveFunc,
) []*ForkResult {
	if len(jobs) == 0 {
		return nil
	}
	concurrency = normalizeForkConcurrency(concurrency, len(jobs))

	if concurrency <= 1 {
		stopKeepAlive := RunKeepAlive(keepAlive)
		defer stopKeepAlive()
		results := make([]*ForkResult, 0, len(jobs))
		for _, job := range jobs {
			result, err := RunForkJob(parentInvoker, parentTask, job, factory, keepAlive)
			if err != nil && result == nil {
				result = &ForkResult{
					Order:      job.Order,
					Identifier: job.Identifier,
					ExecErr:    err,
				}
			}
			results = append(results, result)
		}
		return results
	}

	jobsCh := make(chan ForkJob)
	resultsCh := make(chan *ForkResult, len(jobs))
	var workers sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobsCh {
				result, err := RunForkJob(parentInvoker, parentTask, job, factory, nil)
				if err != nil && result == nil {
					result = &ForkResult{
						Order:      job.Order,
						Identifier: job.Identifier,
						ExecErr:    err,
					}
				}
				resultsCh <- result
			}
		}()
	}
	for _, job := range jobs {
		jobsCh <- job
	}
	close(jobsCh)

	// Start keep-alive ticker on the parent goroutine while it blocks on
	// workers.Wait(). Each worker calls RunForkJob with keepAlive=nil because
	// the ticker here already covers the whole wait.
	stopKeepAlive := RunKeepAlive(keepAlive)
	workers.Wait()
	stopKeepAlive()

	close(resultsCh)

	results := make([]*ForkResult, 0, len(jobs))
	for result := range resultsCh {
		results = append(results, result)
	}
	return results
}

// PrepareForkedSubAgent forks the parent timeline and returns a child invoker plus sub-task.
// Callers may mutate sub-task input (e.g. dispatch goal elaboration) before starting a loop.
func PrepareForkedSubAgent(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	job ForkJob,
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

	// Derive a per-sub-agent task context from jobCtx. Do not bind jobCancel directly to
	// the sub-task: nested loops (e.g. fast_context) may finish a child task while the
	// category scan must keep running phase A→B on the same job scope.
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

	childInvoker, err := BuildForkReactInvoker(parentCfg, fork, jobCtx, taskEmitter)
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

func buildForkUserInput(job ForkJob) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(job.Goal))
	if contract := strings.TrimSpace(job.ResultContract); contract != "" {
		sb.WriteString("\n\n## Result Contract\n\n")
		sb.WriteString(contract)
	}
	return sb.String()
}

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

	// Sub agents must not inherit any top-level execution strategy. Even though
	// ConvertConfigToOptions already omits EnableDispatchSubReactAgents /
	// PreferDispatchSubReactAgents / EnableGoalMode, we disable plan and goal
	// mode explicitly here so the sub-agent contract is self-documenting and does
	// not silently regress if ConvertConfigToOptions propagation changes.
	baseOpts = append(baseOpts, buildSubAgentStrategyOptions()...)

	childInvoker, err := aicommon.AIRuntimeInvokerGetter(jobCtx, baseOpts...)
	if err != nil {
		return nil, utils.Wrap(err, "create forked sub react invoker failed")
	}
	return childInvoker, nil
}

func buildSubAgentStrategyOptions() []aicommon.ConfigOption {
	return []aicommon.ConfigOption{
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithEnableGoalMode(false),
		aicommon.WithPreferDispatchSubReactAgents(false),
	}
}
