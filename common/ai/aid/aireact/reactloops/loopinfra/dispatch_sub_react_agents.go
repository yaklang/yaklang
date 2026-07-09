package loopinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	subAgentDepthLoopVar               = reactloops.SubAgentDepthLoopVar
	dispatchSubReactJobsLoopKey        = "dispatch_sub_react_jobs"
	dispatchSubReactConcurrencyLoopKey = "dispatch_sub_react_concurrency"

	maxDispatchSubReactJobs    = 30
	defaultDispatchConcurrency = 5
	maxDispatchConcurrency     = 10
)

type subReactDispatchJob struct {
	Order      int    `json:"order"`
	Identifier string `json:"identifier"`
	Goal       string `json:"goal"`
	TaskName   string `json:"task_name"`
	LoopName   string `json:"loop_name"`
}

type subReactProcessStats struct {
	Iterations      int    `json:"iterations"`
	Actions         int    `json:"actions"`
	ToolCalls       int    `json:"tool_calls"`
	TimelineItems   int    `json:"timeline_items"`
	BranchDiffBytes int    `json:"branch_diff_bytes"`
	FinalAction     string `json:"final_action,omitempty"`
}

type subReactAgentTimelineRecord struct {
	SubAgentID      string               `json:"sub_agent_id"`
	Order           int                  `json:"order"`
	LoopName        string               `json:"loop_name"`
	Goal            string               `json:"goal"`
	Status          string               `json:"status"`
	Error           string               `json:"error,omitempty"`
	DurationMs      int64                `json:"duration_ms"`
	Result          string               `json:"result,omitempty"`
	ResultReference string               `json:"result_reference,omitempty"`
	ProcessStats    subReactProcessStats `json:"process_stats"`
	TracePreview    string               `json:"trace_preview,omitempty"`
}

type subReactAgentJobResult struct {
	Order    int
	Job      subReactDispatchJob
	Record   subReactAgentTimelineRecord
	Feedback string
}

type subReactAgentJobRunner interface {
	Run(
		parentInvoker aicommon.AIInvokeRuntime,
		parentLoop *reactloops.ReActLoop,
		parentTask aicommon.AIStatefulTask,
		job subReactDispatchJob,
	) (*subReactAgentJobResult, error)
}

type forkedSubReactAgentRunner struct{}

var subReactAgentRunner subReactAgentJobRunner = forkedSubReactAgentRunner{}

func (forkedSubReactAgentRunner) Run(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
	job subReactDispatchJob,
) (*subReactAgentJobResult, error) {
	return runForkedSubReactAgentJob(parentInvoker, parentLoop, parentTask, job)
}

func runForkedSubReactAgentJob(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
	job subReactDispatchJob,
) (*subReactAgentJobResult, error) {
	startedAt := time.Now()

	parentCfg, ok := parentInvoker.GetConfig().(*aicommon.Config)
	if !ok || parentCfg == nil {
		return nil, utils.Error("dispatch_sub_react_agents requires parent config to be *aicommon.Config")
	}
	parentTimeline := parentCfg.GetTimeline()
	if parentTimeline == nil {
		return nil, utils.Error("parent timeline is nil")
	}

	subTaskID := buildSubReactSubTaskID(parentTask, job)
	// The sub agent's task name is the explicit task_name the caller gave (a
	// short, human-readable label), not the goal. The goal can be a long
	// sentence and reads poorly as a task name in the UI and timeline.
	subTaskName := strings.TrimSpace(job.TaskName)
	if subTaskName == "" {
		subTaskName = strings.TrimSpace(job.Identifier)
	}
	if subTaskName == "" {
		subTaskName = job.Goal
	}
	if subTaskName == "" {
		subTaskName = subTaskID
	}

	fork, err := parentTimeline.ForkForTask(subTaskID, subTaskName, parentCfg, parentCfg)
	if err != nil {
		return nil, err
	}
	if fork == nil || fork.Branch == nil {
		return nil, utils.Error("failed to create timeline fork for sub react agent")
	}

	// No per-job timeout: sub agents inherit the parent task context and run
	// until they finish naturally. Previously each job could carry a
	// timeout_seconds that the AI often underestimated, cutting sub agents off
	// mid-task; that param is gone, so there is nothing to clamp here.
	jobCtx, jobCancel := context.WithCancel(parentTask.GetContext())

	childInvoker, err := buildForkedSubReactInvoker(parentCfg, fork, jobCtx, subTaskID)
	if err != nil {
		return nil, err
	}

	// Elaborate the brief intent (job.Goal) into a complete, self-contained goal
	// plus a result contract right before the sub agent runs. This moves the
	// long-form generation out of the (linear) dispatch action call — where the
	// parent AI used to write every sub agent's full goal+contract up front —
	// into a per-sub-agent step that runs with the forked timeline context and
	// overlaps across the concurrently-dispatched sub agents. The elaborated
	// goal streams to the sub agent's own thread (sub_react_agent_goal node)
	// via the child invoker's forwarding emitter. On failure we fall back to the
	// brief intent so a generation hiccup never blocks the dispatch.
	subTask := aicommon.NewSubTaskBaseWithOptions(
		parentTask,
		subTaskID,
		job.Goal,
		aicommon.WithStatefulTaskBaseName(subTaskName),
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContextAndCancel(jobCtx, jobCancel),
	)

	parentInvoker.AddRuntimeTask(subTask)
	childInvoker.SetCurrentTask(subTask)

	// Restore sub-agent emit: derive the sub-task emitter from the parent config emitter
	// (via PushEventProcesser) so sub-agent events reach the frontend, stamped with the
	// sub-task id as the aggregation marker. This replaces the temporary discard emitter
	// that suppressed sub-agent output while waiting for the frontend to support
	// aggregating sub-agent messages.
	subTask.SetEmitter(buildSubReactForwardingEmitter(parentCfg.GetEmitter(), subTaskID))
	branchMarker := fmt.Sprintf("sub-react-branch-marker-%s", subTaskID)
	fork.Branch.PushText(parentCfg.AcquireId(), branchMarker)
	subTask.SetStatus(aicommon.AITaskState_Processing)

	// Elaborate the sub agent's brief intent into a complete, self-contained goal and an optional result contract.
	elaboratedGoal, resultContract, elabErr := elaborateSubReactAgentGoal(jobCtx, childInvoker, parentLoop, subTaskID, job)
	if elabErr != nil {
		log.Warnf("dispatch_sub_react_agents: elaborate goal for %s failed, falling back to brief intent: %v", subTaskID, elabErr)
		elaboratedGoal = job.Goal
		resultContract = ""
	}
	subTask.SetUserInput(buildSubAgentUserInput(elaboratedGoal, resultContract))

	subLoop, err := reactloops.CreateLoopByName(job.LoopName, childInvoker, buildSubReactLoopOptions()...)
	if err != nil {
		result, _ := buildSubReactJobResult(job, startedAt, subTask, nil, fork, err)
		return result, nil
	}

	execErr := subLoop.ExecuteWithExistedTask(subTask)
	result, _ := buildSubReactJobResult(job, startedAt, subTask, subLoop, fork, execErr)
	return result, nil
}

// subReactGoalElaborationPrompt is rendered with the parent loop's base frame
// context (CurrentTime/OSArch/WorkingDir/Timeline) plus the sub agent's name,
// identifier and brief intent, and asks the model to produce a complete,
// self-contained task goal and an optional result contract for the sub agent.
const subReactGoalElaborationPrompt = `You are preparing a task brief for an autonomous sub ReAct agent that will run in an isolated timeline fork, inheriting the parent agent's current context snapshot.

Parent context (the sub agent will see this snapshot):
- Current time: {{.CurrentTime}}
- OS/Arch: {{.OSArch}}{{ if .WorkingDir }}
- Working directory: {{.WorkingDir}}{{end}}

Parent timeline snapshot (the sub agent inherits this as its starting context):
{{ if .Timeline }}{{.Timeline}}{{else}}<empty>{{end}}

The parent agent has decided to dispatch a sub agent with the following brief intent. Your job is to elaborate that brief intent into a COMPLETE, self-contained task goal the sub agent can execute without re-reading the parent's reasoning, plus a result contract describing the output format / acceptance criteria the sub agent's final answer should satisfy.

Sub agent name: {{ if .SubTaskName }}{{.SubTaskName}}{{else}}<unspecified>{{end}}
Sub agent identifier: {{ if .SubTaskIdentifier }}{{.SubTaskIdentifier}}{{else}}<unspecified>{{end}}
Brief intent: {{ if .BriefGoal }}{{.BriefGoal}}{{else}}<unspecified>{{end}}

Write the elaborated goal so it stands alone (the sub agent does not see this prompt). Keep it focused and actionable; do not invent scope beyond the intent. The result contract is optional — omit it if no specific output format is needed.`

// elaborateSubReactAgentGoal expands a sub agent's brief intent into a complete,
// self-contained goal and an optional result contract via a QualityPriority
// LiteForge call. The "goal" field is streamed to the sub agent's own thread
// (sub_react_agent_goal node) through the child invoker's forwarding emitter,
// which stamps events with the sub-task id so the frontend aggregates them
// under this sub agent. The returned goal/contract become the sub task's input.
func elaborateSubReactAgentGoal(
	ctx context.Context,
	childInvoker aicommon.AITaskInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	subTaskId string,
	job subReactDispatchJob,
) (goal, resultContract string, err error) {
	if childInvoker == nil {
		return "", "", utils.Error("child invoker is nil")
	}
	templateData := map[string]any{}
	if parentLoop != nil {
		for k, v := range parentLoop.GetBaseFrameContext() {
			templateData[k] = v
		}
	}
	templateData["SubTaskName"] = strings.TrimSpace(job.TaskName)
	templateData["SubTaskIdentifier"] = strings.TrimSpace(job.Identifier)
	templateData["BriefGoal"] = strings.TrimSpace(job.Goal)

	prompt, err := utils.RenderTemplate(subReactGoalElaborationPrompt, templateData)
	if err != nil {
		return "", "", utils.Wrap(err, "render sub react goal elaboration prompt failed")
	}

	action, err := childInvoker.InvokeQualityPriorityLiteForge(
		ctx,
		"sub_react_agent_goal_elaboration",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("goal",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Elaborated, self-contained task goal for the sub agent."),
			),
			aitool.WithStringParam("result_contract",
				aitool.WithParam_Description("Optional acceptance criteria / output format for the sub agent result."),
			),
		},
		aicommon.WithGeneralConfigStreamableFieldEmitterCallback(
			[]string{"goal"},
			func(key string, r io.Reader, emitter *aicommon.Emitter) {
				r = utils.JSONStringReader(r)
				if emitter == nil {
					io.Copy(io.Discard, r)
					return
				}
				emitter.EmitTextPlainTextStreamEvent(loopInfraNodeSubReactGoal, r, subTaskId)
			},
		),
	)
	if err != nil {
		return "", "", err
	}
	if action == nil {
		return "", "", utils.Error("sub react goal elaboration returned nil action")
	}
	goal = strings.TrimSpace(action.GetString("goal"))
	resultContract = strings.TrimSpace(action.GetString("result_contract"))
	if goal == "" {
		return "", "", utils.Error("sub react goal elaboration returned empty goal")
	}
	return goal, resultContract, nil
}

func buildForkedSubReactInvoker(
	parentCfg *aicommon.Config,
	fork *aicommon.TimelineFork,
	jobCtx context.Context,
	subTaskId string,
) (aicommon.AITaskInvokeRuntime, error) {
	baseOpts := aicommon.ConvertConfigToOptions(parentCfg)
	baseOpts = append(baseOpts,
		aicommon.WithTimeline(fork.Branch),
		aicommon.WithContext(jobCtx),
		aicommon.WithAICallbacks(parentCfg.GetRawAICallbacks()),
		aicommon.WithEmitter(buildSubReactForwardingEmitter(parentCfg.GetEmitter(), subTaskId)),
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

// buildSubReactForwardingEmitter derives a sub-agent emitter from the parent emitter
// via PushEventProcesser (same pattern used by coordinator_invoker.go and taskif.go for
// stamping task identity onto events). The derived emitter shares the parent's frontend
// sink (its baseEmitter), so sub-agent status/action-log/answer events reach the frontend,
// while a processor stamps every event's TaskId with the sub-task id — the marker the
// frontend uses to aggregate sub-agent messages.
//
// It derives from the parent *config* emitter (empty processor stack) rather than the
// parent *task* emitter on purpose: ForeachStack runs top-to-bottom, so deriving from the
// task emitter would let the parent task's own TaskId stamp (pushed earlier, lower in the
// stack) overwrite this sub-task stamp. Deriving from the config emitter leaves only this
// stamp in the stack, so the sub-task id wins.
//
// A nil parentEmitter (e.g. some test configs) degrades to a no-op dummy emitter so
// callers never panic.
func buildSubReactForwardingEmitter(parentEmitter *aicommon.Emitter, subTaskId string) *aicommon.Emitter {
	if parentEmitter == nil {
		return aicommon.NewDummyEmitter()
	}
	return parentEmitter.PushEventProcesser(func(event *schema.AiOutputEvent) *schema.AiOutputEvent {
		if event != nil && subTaskId != "" {
			event.TaskId = subTaskId
		}
		return event
	})
}

// buildSubAgentStrategyOptions returns the config options that strip every
// top-level execution strategy from a forked sub agent. A sub agent must not:
//   - open a plan (plan is a top-level orchestration concern),
//   - be subject to the goal-mode minimum-iteration finish gate (it must be
//     free to finish as soon as its single goal is done),
//   - inherit the multi-agent dispatch preference (only the top-level loop
//     dispatches; sub agents are blocked from dispatching anyway via the action
//     filter + verifier depth check, but clearing the preference keeps the
//     prompt honest).
func buildSubAgentStrategyOptions() []aicommon.ConfigOption {
	return []aicommon.ConfigOption{
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithEnableGoalMode(false),
		aicommon.WithPreferDispatchSubReactAgents(false),
	}
}

// buildSubReactLoopOptions returns the loop options applied to every forked
// sub-react agent. Notably it no longer sets WithMaxIterations: sub agents
// inherit the ReActLoop's own default iteration ceiling (soft-interrupt,
// not a hard close) instead of a per-job cap the parent AI could
// misestimate. There is also no per-job timeout — see runForkedSubReactAgentJob.
func buildSubReactLoopOptions() []reactloops.ReActLoopOption {
	return []reactloops.ReActLoopOption{
		reactloops.WithVar(subAgentDepthLoopVar, 1),
		reactloops.WithNoEndLoadingStatus(true),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS
		}),
	}
}

func buildSubReactJobResult(
	job subReactDispatchJob,
	startedAt time.Time,
	subTask aicommon.AIStatefulTask,
	subLoop *reactloops.ReActLoop,
	fork *aicommon.TimelineFork,
	execErr error,
) (*subReactAgentJobResult, error) {
	record := subReactAgentTimelineRecord{
		SubAgentID: subTask.GetId(),
		Order:      job.Order,
		LoopName:   job.LoopName,
		Goal:       job.Goal,
		DurationMs: time.Since(startedAt).Milliseconds(),
	}

	if execErr != nil {
		record.Status = "failed"
		record.Error = execErr.Error()
	} else {
		record.Status = "completed"
	}

	resultText := strings.TrimSpace(subTask.GetResult())
	if resultText == "" && subLoop != nil {
		resultText = strings.TrimSpace(subLoop.Get("directly_answer_payload"))
	}
	record.Result = utils.ShrinkTextBlock(resultText, 4000)

	tracePreview, branchDiffBytes := summarizeForkDiff(fork)
	record.TracePreview = tracePreview
	record.ProcessStats = collectSubReactProcessStats(subLoop, fork, branchDiffBytes)

	feedback := fmt.Sprintf("[%d] %s (%s): %s", job.Order, job.Identifier, record.Status, utils.ShrinkString(record.Result, 240))
	if record.Error != "" {
		feedback = fmt.Sprintf("[%d] %s (%s): %s", job.Order, job.Identifier, record.Status, record.Error)
	}

	return &subReactAgentJobResult{
		Order:    job.Order,
		Job:      job,
		Record:   record,
		Feedback: feedback,
	}, nil
}

func collectSubReactProcessStats(subLoop *reactloops.ReActLoop, fork *aicommon.TimelineFork, branchDiffBytes int) subReactProcessStats {
	stats := subReactProcessStats{
		BranchDiffBytes: branchDiffBytes,
		TimelineItems:   countBranchTimelineItems(fork),
	}
	if subLoop == nil {
		return stats
	}

	stats.Iterations = subLoop.GetCurrentIterationIndex()
	records := subLoop.GetAllExistedActionRecord()
	stats.Actions = len(records)
	stats.ToolCalls = countToolCallsFromActionRecords(records)
	if last := subLoop.GetLastAction(); last != nil {
		stats.FinalAction = last.ActionType
	}
	return stats
}

func countToolCallsFromActionRecords(records []*reactloops.ActionRecord) int {
	count := 0
	for _, record := range records {
		if record == nil {
			continue
		}
		switch record.ActionType {
		case schema.AI_REACT_LOOP_ACTION_REQUIRE_TOOL,
			schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL,
			schema.AI_REACT_LOOP_ACTION_TOOL_COMPOSE:
			count++
		}
	}
	return count
}

func countBranchTimelineItems(fork *aicommon.TimelineFork) int {
	if fork == nil || fork.Branch == nil {
		return 0
	}
	count := 0
	for _, id := range fork.Branch.GetTimelineItemIDs() {
		if id > fork.BaseMaxID {
			count++
		}
	}
	return count
}

func summarizeForkDiff(fork *aicommon.TimelineFork) (preview string, bytes int) {
	if fork == nil {
		return "", 0
	}
	diff, err := fork.Diff()
	if err != nil {
		return "", 0
	}
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return "", 0
	}
	return utils.ShrinkTextBlock(diff, 1200), len(diff)
}

func buildSubReactSubTaskID(parentTask aicommon.AIStatefulTask, job subReactDispatchJob) string {
	parentID := "sub-react"
	if parentTask != nil && parentTask.GetId() != "" {
		parentID = parentTask.GetId()
	}
	segment := sanitizeSubReactIDSegment(job.Identifier)
	if segment == "" {
		segment = fmt.Sprintf("job-%d", job.Order)
	}
	return fmt.Sprintf("%s-sub-%s-%s", parentID, segment, utils.RandStringBytes(4))
}

func sanitizeSubReactIDSegment(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else if r == ' ' || r == '/' {
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

func buildSubAgentUserInput(goal, resultContract string) string {
	goal = strings.TrimSpace(goal)
	var sb strings.Builder
	sb.WriteString(goal)
	if contract := strings.TrimSpace(resultContract); contract != "" {
		sb.WriteString("\n\n## Result Contract\n\n")
		sb.WriteString(contract)
	}
	return sb.String()
}

func parseSubReactDispatchJobs(action *aicommon.Action) ([]subReactDispatchJob, error) {
	jobs, err := parseSubReactDispatchJobsFromArray(action.GetInvokeParamsArray("dispatches"))
	if err != nil {
		return nil, err
	}
	if len(jobs) > 0 {
		return jobs, nil
	}

	raw := strings.TrimSpace(action.GetString("dispatches"))
	if raw == "" {
		return nil, utils.Error("dispatches is required and must be a non-empty array")
	}
	if err := json.Unmarshal([]byte(raw), &jobs); err != nil {
		return nil, utils.Wrap(err, "dispatches must be a valid array")
	}
	return normalizeSubReactDispatchJobs(jobs)
}

func parseSubReactDispatchJobsFromArray(raw []aitool.InvokeParams) ([]subReactDispatchJob, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	jobs := make([]subReactDispatchJob, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		jobs = append(jobs, subReactDispatchJob{
			Identifier: strings.TrimSpace(item.GetString("identifier")),
			Goal:       strings.TrimSpace(item.GetString("goal")),
			TaskName:   strings.TrimSpace(item.GetString("task_name")),
			LoopName:   strings.TrimSpace(item.GetString("loop_name")),
		})
	}
	return normalizeSubReactDispatchJobs(jobs)
}

func normalizeSubReactDispatchJobs(jobs []subReactDispatchJob) ([]subReactDispatchJob, error) {
	if len(jobs) == 0 {
		return nil, utils.Error("dispatches must contain at least one sub agent job")
	}
	if len(jobs) > maxDispatchSubReactJobs {
		return nil, utils.Errorf("dispatches supports at most %d sub agents per call", maxDispatchSubReactJobs)
	}

	for i := range jobs {
		jobs[i].Order = i + 1
		jobs[i].Goal = strings.TrimSpace(jobs[i].Goal)
		if jobs[i].Goal == "" {
			return nil, utils.Errorf("dispatches[%d].goal is required", i)
		}
		jobs[i].LoopName = strings.TrimSpace(jobs[i].LoopName)
		if jobs[i].LoopName == "" {
			jobs[i].LoopName = schema.AI_REACT_LOOP_NAME_DEFAULT
		}
		if _, ok := reactloops.GetLoopFactory(jobs[i].LoopName); !ok {
			return nil, utils.Errorf("dispatches[%d].loop_name %q is not registered", i, jobs[i].LoopName)
		}
		jobs[i].Identifier = strings.TrimSpace(jobs[i].Identifier)
		if jobs[i].Identifier == "" {
			jobs[i].Identifier = fmt.Sprintf("sub_agent_%d", jobs[i].Order)
		}
		jobs[i].TaskName = strings.TrimSpace(jobs[i].TaskName)
	}
	return jobs, nil
}

func parseDispatchConcurrency(action *aicommon.Action, jobCount int) int {
	concurrency := action.GetInt("concurrency")
	if concurrency <= 0 {
		concurrency = defaultDispatchConcurrency
		if jobCount < concurrency {
			concurrency = jobCount
		}
	}
	if concurrency > maxDispatchConcurrency {
		concurrency = maxDispatchConcurrency
	}
	if concurrency > jobCount {
		concurrency = jobCount
	}
	return concurrency
}

func getSubAgentDepth(loop *reactloops.ReActLoop) int {
	if loop == nil {
		return 0
	}
	return loop.GetInt(subAgentDepthLoopVar)
}

func verifyDispatchSubReactAgents(loop *reactloops.ReActLoop, action *aicommon.Action) error {
	if getSubAgentDepth(loop) > 0 {
		return utils.Error("dispatch_sub_react_agents is only available in top-level agent; sub agents cannot dispatch more sub agents")
	}

	jobs, err := parseSubReactDispatchJobs(action)
	if err != nil {
		return err
	}

	concurrency := parseDispatchConcurrency(action, len(jobs))
	encoded, err := json.Marshal(jobs)
	if err != nil {
		return err
	}
	loop.Set(dispatchSubReactJobsLoopKey, string(encoded))
	loop.Set(dispatchSubReactConcurrencyLoopKey, concurrency)
	return nil
}

func handleDispatchSubReactAgents(
	loop *reactloops.ReActLoop,
	action *aicommon.Action,
	operator *reactloops.LoopActionHandlerOperator,
) {
	invoker := loop.GetInvoker()
	parentTask := operator.GetTask()
	if parentTask == nil {
		parentTask = loop.GetCurrentTask()
	}

	rawJobs := loop.Get(dispatchSubReactJobsLoopKey)
	if strings.TrimSpace(rawJobs) == "" {
		operator.Fail(utils.Error("dispatch_sub_react_agents verifier state missing; retry the action"))
		return
	}
	var jobs []subReactDispatchJob
	if err := json.Unmarshal([]byte(rawJobs), &jobs); err != nil {
		operator.Fail(err)
		return
	}

	concurrency := loop.GetInt(dispatchSubReactConcurrencyLoopKey)
	if concurrency <= 0 {
		concurrency = parseDispatchConcurrency(action, len(jobs))
	}

	loopInfraStatus(loop, "子 Agent 执行中/ Sub Agents Running...")

	results := runDispatchSubReactJobsConcurrently(invoker, loop, parentTask, jobs, concurrency)

	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})

	var feedbackLines []string
	successCount := 0
	for _, result := range results {
		if result == nil {
			continue
		}
		if result.Record.Status == "completed" {
			successCount++
		}
		writeSubReactAgentTimelineRecord(invoker, loop, result.Record)
		feedbackLines = append(feedbackLines, result.Feedback)
	}

	summary := fmt.Sprintf(
		"Dispatched %d sub react agents: %d succeeded, %d failed.",
		len(results), successCount, len(results)-successCount,
	)
	invoker.AddToTimeline("[DISPATCH_SUB_REACT_AGENTS_DONE]", summary)
	loopInfraActionFinish(loop, loopInfraNodeSubReactReport, summary)

	operator.Feedback(summary + "\n\n" + strings.Join(feedbackLines, "\n"))
	operator.Continue()
}

func runDispatchSubReactJobsConcurrently(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	parentTask aicommon.AIStatefulTask,
	jobs []subReactDispatchJob,
	concurrency int,
) []*subReactAgentJobResult {
	if concurrency <= 1 {
		results := make([]*subReactAgentJobResult, 0, len(jobs))
		for _, job := range jobs {
			result, err := subReactAgentRunner.Run(parentInvoker, parentLoop, parentTask, job)
			if err != nil {
				result = &subReactAgentJobResult{
					Order: job.Order,
					Job:   job,
					Record: subReactAgentTimelineRecord{
						SubAgentID: buildSubReactSubTaskID(parentTask, job),
						Order:      job.Order,
						LoopName:   job.LoopName,
						Goal:       job.Goal,
						Status:     "failed",
						Error:      err.Error(),
					},
					Feedback: fmt.Sprintf("[%d] %s (failed): %s", job.Order, job.Identifier, err.Error()),
				}
			}
			results = append(results, result)
		}
		return results
	}

	jobsCh := make(chan subReactDispatchJob)
	resultsCh := make(chan *subReactAgentJobResult, len(jobs))
	var workers sync.WaitGroup

	workerCount := concurrency
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobsCh {
				result, err := subReactAgentRunner.Run(parentInvoker, parentLoop, parentTask, job)
				if err != nil {
					result = &subReactAgentJobResult{
						Order: job.Order,
						Job:   job,
						Record: subReactAgentTimelineRecord{
							SubAgentID: buildSubReactSubTaskID(parentTask, job),
							Order:      job.Order,
							LoopName:   job.LoopName,
							Goal:       job.Goal,
							Status:     "failed",
							Error:      err.Error(),
						},
						Feedback: fmt.Sprintf("[%d] %s (failed): %s", job.Order, job.Identifier, err.Error()),
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
	workers.Wait()
	close(resultsCh)

	results := make([]*subReactAgentJobResult, 0, len(jobs))
	for result := range resultsCh {
		results = append(results, result)
	}
	return results
}

func writeSubReactAgentTimelineRecord(
	invoker aicommon.AIInvokeRuntime,
	parentLoop *reactloops.ReActLoop,
	record subReactAgentTimelineRecord,
) {
	if invoker == nil {
		return
	}

	payload := record
	if strings.TrimSpace(payload.Result) != "" {
		if parentLoop != nil {
			ref, preview := loopInfraSaveReference(parentLoop, "sub_react_agent_"+record.SubAgentID, payload.Result, 800)
			if ref != "" {
				payload.ResultReference = ref
				payload.Result = preview
			}
		} else {
			payload.Result = utils.ShrinkTextBlock(payload.Result, 800)
		}
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		log.Warnf("dispatch_sub_react_agents: marshal timeline record failed: %v", err)
		invoker.AddToTimeline(schema.AI_TIMELINE_ITEM_TYPE_SUB_REACT_AGENT_RESULT, utils.InterfaceToString(record))
		return
	}
	invoker.AddToTimeline(schema.AI_TIMELINE_ITEM_TYPE_SUB_REACT_AGENT_RESULT, string(raw))
}

var loopAction_DispatchSubReactAgents = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS,
	Description: "Dispatch multiple INDEPENDENT sub ReAct agents in parallel. Each sub agent inherits the current timeline snapshot as context, " +
		"runs in an isolated timeline fork, and returns one structured result record back to the parent agent. " +
		"Sub agents cannot dispatch more sub agents, open plans, or be subject to goal-mode limits.\n" +
		"WHAT THIS IS FOR: parallelizing genuinely independent workstreams that can run at the same time without talking to each other (e.g. scan host A and scan host B).\n" +
		"WHAT THIS IS NOT FOR: (1) offloading a single sequential task you should do yourself — if the whole task is one chain of steps, do NOT dispatch it; (2) dumping every imaginable subtask into one call to avoid thinking. Only dispatch subtasks you have actually confirmed are independent.\n" +
		"DEPENDENCY RULE: every sub agent in ONE dispatch MUST be mutually independent — none may depend on another's input or result. If B depends on A's result, do NOT batch them: dispatch A (or do A yourself) now, wait for its result to land in the timeline, then in a LATER loop iteration dispatch B once the prior result is available. Group only no-dependency subtasks into the same dispatch.\n" +
		"GOAL QUALITY: give each sub agent a crisp, self-contained goal and a result_contract whenever possible, so it can finish and return a structured result without re-reading your reasoning.",
	Options: []aitool.ToolOption{
		aitool.WithStructArrayParam("dispatches",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Sub agent jobs to dispatch in parallel. Each item runs in an isolated timeline fork and returns one structured result back to the parent. " +
					"All jobs in one dispatch MUST be mutually independent — none may depend on another job's input or result. Dependent sub agents must be split across separate loop iterations: dispatch the first batch, wait for completion, then dispatch the dependent batch in the next iteration."),
			},
			nil,
			aitool.WithStringParam("identifier",
				aitool.WithParam_Description("Optional stable label for this sub agent. Auto-generated from array index when omitted."),
			),
			aitool.WithStringParam("goal",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Short one-line intent for this sub agent — a noun phrase or single sentence, STRICTLY within 15 characters (English) / 15 字以内 (Chinese). Keep it brief; a complete, self-contained goal and result contract are elaborated automatically before the sub agent runs — do not write the full goal here."),
			),
			aitool.WithStringParam("task_name",
				aitool.WithParam_Description("Short, human-readable name for this sub agent's task, shown as the task title in the UI and timeline. Falls back to identifier, then goal when omitted. Prefer a concise noun phrase here rather than reusing the full goal sentence."),
			),
			aitool.WithStringParam("loop_name",
				aitool.WithParam_Description(fmt.Sprintf("Target ReAct loop name. Defaults to %q.", schema.AI_REACT_LOOP_NAME_DEFAULT)),
			),
		),
		aitool.WithIntegerParam(
			"concurrency",
			aitool.WithParam_Description(fmt.Sprintf("Parallelism for sub agent execution. Default min(len(dispatches), %d), max %d.", defaultDispatchConcurrency, maxDispatchConcurrency)),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName: "goal",
			AINodeId:  loopInfraNodeDispatchSubReact,
		},
		{
			FieldName: "concurrency",
			AINodeId:  loopInfraNodeDispatchConcurrency,
			IsSystem:  true,
		},
	},
	ActionVerifier: verifyDispatchSubReactAgents,
	ActionHandler:  handleDispatchSubReactAgents,
}
