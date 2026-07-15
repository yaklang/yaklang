package reactloops

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
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// DispatchSubReactJobsLoopKey stores the JSON-encoded dispatch jobs in loop vars.
	DispatchSubReactJobsLoopKey        = "dispatch_sub_react_jobs"
	DispatchSubReactConcurrencyLoopKey = "dispatch_sub_react_concurrency"

	MaxDispatchSubReactJobs    = 30
	DefaultDispatchConcurrency = 5
	MaxDispatchConcurrency     = 10
)

// DispatchJob describes one sub-agent dispatch entry parsed from the AI action.
type DispatchJob struct {
	Order      int    `json:"order"`
	Identifier string `json:"identifier"`
	Goal       string `json:"goal"`
	TaskName   string `json:"task_name"`
	LoopName   string `json:"loop_name"`
}

// ProcessStats summarizes the runtime activity of a completed sub-agent.
type ProcessStats struct {
	Iterations      int    `json:"iterations"`
	Actions         int    `json:"actions"`
	ToolCalls       int    `json:"tool_calls"`
	TimelineItems   int    `json:"timeline_items"`
	BranchDiffBytes int    `json:"branch_diff_bytes"`
	FinalAction     string `json:"final_action,omitempty"`
}

// TimelineRecord is the structured result written back to the parent timeline.
type TimelineRecord struct {
	SubAgentID      string       `json:"sub_agent_id"`
	Order           int          `json:"order"`
	LoopName        string       `json:"loop_name"`
	Goal            string       `json:"goal"`
	Status          string       `json:"status"`
	Error           string       `json:"error,omitempty"`
	DurationMs      int64        `json:"duration_ms"`
	Result          string       `json:"result,omitempty"`
	ResultReference string       `json:"result_reference,omitempty"`
	ProcessStats    ProcessStats `json:"process_stats"`
	TracePreview    string       `json:"trace_preview,omitempty"`
}

// JobResult is returned by a runner after a single sub-agent job completes.
type JobResult struct {
	Order    int
	Job      DispatchJob
	Record   TimelineRecord
	Feedback string
}

// JobRunner is the interface for executing a single sub-agent dispatch job.
// The default implementation (ForkedRunner) forks the parent timeline and runs
// a full ReAct loop in the child. Tests can provide a mock implementation.
type JobRunner interface {
	Run(
		parentInvoker aicommon.AIInvokeRuntime,
		parentLoop *ReActLoop,
		parentTask aicommon.AIStatefulTask,
		job DispatchJob,
		registry *ProgressRegistry,
	) (*JobResult, error)
}

// ForkedRunner is the default JobRunner that forks the parent timeline and
// runs a full ReAct loop in the child.
type ForkedRunner struct{}

// DefaultRunner is the package-level runner instance used by RunJobsConcurrently.
// Tests may swap this to inject mock behaviour.
var DefaultRunner JobRunner = ForkedRunner{}

// Run executes one sub-agent job by forking the parent timeline and running
// a full ReAct loop in the child.
func (ForkedRunner) Run(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	job DispatchJob,
	registry *ProgressRegistry,
) (*JobResult, error) {
	return RunForkedJob(parentInvoker, parentLoop, parentTask, job, registry)
}

// RunForkedJob forks the parent timeline, elaborates the brief goal, registers
// a progress handle, runs the sub-loop, and returns a JobResult.
func RunForkedJob(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	job DispatchJob,
	registry *ProgressRegistry,
) (*JobResult, error) {
	startedAt := time.Now()
	loopName := job.LoopName
	forkJob := ForkJob{
		Order:      job.Order,
		Identifier: job.Identifier,
		Goal:       job.Goal,
		TaskName:   job.TaskName,
	}

	childInvoker, subTask, fork, jobCancel, err := PrepareForkedSubAgent(parentInvoker, parentTask, forkJob)
	if jobCancel != nil {
		defer jobCancel()
	}
	if err != nil {
		return nil, err
	}

	// Elaborate the brief intent (job.Goal) into a complete, self-contained goal
	// plus a result contract right before the sub agent runs.
	subTask.SetStatus(aicommon.AITaskState_Processing)
	elaboratedGoal, resultContract, elabErr := elaborateGoal(
		subTask.GetContext(), childInvoker, parentLoop, subTask.GetId(), job,
	)
	if elabErr != nil {
		log.Warnf("dispatch_sub_react_agents: elaborate goal for %s failed, falling back to brief intent: %v", subTask.GetId(), elabErr)
		elaboratedGoal = job.Goal
		resultContract = ""
	}
	subTask.SetUserInput(buildUserInput(elaboratedGoal, resultContract))

	// Register sub-agent progress handle so stall heartbeat / verification
	// watchdog can observe sub-agent activity while the parent loop blocks.
	var handle *SubAgentHandle
	if registry != nil {
		handle = registry.Register(NewSubAgentHandle(
			subTask.GetId(), job.Identifier, subTask, startedAt,
		))
	}

	subLoop, err := CreateLoopByName(loopName, childInvoker, DefaultForkOptions()...)
	if err != nil {
		if handle != nil && registry != nil {
			registry.Unregister(subTask.GetId(), err)
		}
		result, _ := BuildJobResult(job, startedAt, subTask, nil, fork, err)
		return result, nil
	}

	if handle != nil {
		handle.SubLoop = subLoop
	}

	execErr := subLoop.ExecuteWithExistedTask(subTask)
	result, _ := BuildJobResult(job, startedAt, subTask, subLoop, fork, execErr)
	if handle != nil && registry != nil {
		registry.Unregister(subTask.GetId(), execErr)
	}
	return result, nil
}

// RunJobsConcurrently runs multiple sub-agent dispatch jobs with a worker pool.
func RunJobsConcurrently(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	jobs []DispatchJob,
	concurrency int,
	registry *ProgressRegistry,
) []*JobResult {
	runner := DefaultRunner
	if concurrency <= 1 {
		results := make([]*JobResult, 0, len(jobs))
		for _, job := range jobs {
			result, err := runner.Run(parentInvoker, parentLoop, parentTask, job, registry)
			if err != nil {
				result = &JobResult{
					Order: job.Order,
					Job:   job,
					Record: TimelineRecord{
						SubAgentID: BuildForkTaskID(parentTask, ForkJob{
							Order:      job.Order,
							Identifier: job.Identifier,
						}),
						Order:    job.Order,
						LoopName: job.LoopName,
						Goal:     job.Goal,
						Status:   "failed",
						Error:    err.Error(),
					},
					Feedback: fmt.Sprintf("[%d] %s (failed): %s", job.Order, job.Identifier, err.Error()),
				}
			}
			results = append(results, result)
		}
		return results
	}

	jobsCh := make(chan DispatchJob)
	resultsCh := make(chan *JobResult, len(jobs))
	var workers sync.WaitGroup

	workerCount := concurrency
	for i := 0; i < workerCount; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobsCh {
				result, err := runner.Run(parentInvoker, parentLoop, parentTask, job, registry)
				if err != nil {
					result = &JobResult{
						Order: job.Order,
						Job:   job,
						Record: TimelineRecord{
							SubAgentID: BuildForkTaskID(parentTask, ForkJob{
								Order:      job.Order,
								Identifier: job.Identifier,
							}),
							Order:    job.Order,
							LoopName: job.LoopName,
							Goal:     job.Goal,
							Status:   "failed",
							Error:    err.Error(),
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

	results := make([]*JobResult, 0, len(jobs))
	for result := range resultsCh {
		results = append(results, result)
	}
	return results
}

// BuildJobResult constructs a JobResult from the sub-task and sub-loop state.
func BuildJobResult(
	job DispatchJob,
	startedAt time.Time,
	subTask aicommon.AIStatefulTask,
	subLoop *ReActLoop,
	fork *aicommon.TimelineFork,
	execErr error,
) (*JobResult, error) {
	record := TimelineRecord{
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

	tracePreview, branchDiffBytes := SummarizeForkDiff(fork)
	record.TracePreview = tracePreview
	record.ProcessStats = CollectProcessStats(subLoop, fork, branchDiffBytes)

	feedback := fmt.Sprintf("[%d] %s (%s): %s", job.Order, job.Identifier, record.Status, utils.ShrinkString(record.Result, 240))
	if record.Error != "" {
		feedback = fmt.Sprintf("[%d] %s (%s): %s", job.Order, job.Identifier, record.Status, record.Error)
	}

	return &JobResult{
		Order:    job.Order,
		Job:      job,
		Record:   record,
		Feedback: feedback,
	}, nil
}

// CollectProcessStats gathers iteration / action / tool-call stats from the sub-loop.
func CollectProcessStats(subLoop *ReActLoop, fork *aicommon.TimelineFork, branchDiffBytes int) ProcessStats {
	stats := ProcessStats{
		BranchDiffBytes: branchDiffBytes,
		TimelineItems:   CountBranchTimelineItems(fork),
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

func countToolCallsFromActionRecords(records []*ActionRecord) int {
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

// CountBranchTimelineItems counts timeline items added to the fork branch.
func CountBranchTimelineItems(fork *aicommon.TimelineFork) int {
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

// SummarizeForkDiff returns a trimmed preview and byte count of the fork diff.
func SummarizeForkDiff(fork *aicommon.TimelineFork) (preview string, bytes int) {
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

func buildUserInput(goal, resultContract string) string {
	goal = strings.TrimSpace(goal)
	var sb strings.Builder
	sb.WriteString(goal)
	if contract := strings.TrimSpace(resultContract); contract != "" {
		sb.WriteString("\n\n## Result Contract\n\n")
		sb.WriteString(contract)
	}
	return sb.String()
}

// --- goal elaboration ---

const goalElaborationPrompt = `You are preparing a task brief for an autonomous sub ReAct agent that will run in an isolated timeline fork, inheriting the parent agent's current context snapshot.

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

func elaborateGoal(
	ctx context.Context,
	childInvoker aicommon.AITaskInvokeRuntime,
	parentLoop *ReActLoop,
	subTaskId string,
	job DispatchJob,
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

	prompt, err := utils.RenderTemplate(goalElaborationPrompt, templateData)
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
				emitter.EmitTextPlainTextStreamEvent("sub_react_agent_goal", r, subTaskId)
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

// --- parsing ---

// ParseDispatchJobs extracts dispatch jobs from an AI action's "dispatches" parameter.
func ParseDispatchJobs(action *aicommon.Action) ([]DispatchJob, error) {
	jobs, err := parseDispatchJobsFromArray(action.GetInvokeParamsArray("dispatches"))
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
	return NormalizeDispatchJobs(jobs)
}

func parseDispatchJobsFromArray(raw []aitool.InvokeParams) ([]DispatchJob, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	jobs := make([]DispatchJob, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		jobs = append(jobs, DispatchJob{
			Identifier: strings.TrimSpace(item.GetString("identifier")),
			Goal:       strings.TrimSpace(item.GetString("goal")),
			TaskName:   strings.TrimSpace(item.GetString("task_name")),
			LoopName:   strings.TrimSpace(item.GetString("loop_name")),
		})
	}
	return NormalizeDispatchJobs(jobs)
}

// NormalizeDispatchJobs validates and normalizes dispatch jobs.
func NormalizeDispatchJobs(jobs []DispatchJob) ([]DispatchJob, error) {
	if len(jobs) == 0 {
		return nil, utils.Error("dispatches must contain at least one sub agent job")
	}
	if len(jobs) > MaxDispatchSubReactJobs {
		return nil, utils.Errorf("dispatches supports at most %d sub agents per call", MaxDispatchSubReactJobs)
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
		if _, ok := GetLoopFactory(jobs[i].LoopName); !ok {
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

// ParseConcurrency extracts and clamps the concurrency parameter from an AI action.
func ParseConcurrency(action *aicommon.Action, jobCount int) int {
	concurrency := action.GetInt("concurrency")
	if concurrency <= 0 {
		concurrency = DefaultDispatchConcurrency
		if jobCount < concurrency {
			concurrency = jobCount
		}
	}
	if concurrency > MaxDispatchConcurrency {
		concurrency = MaxDispatchConcurrency
	}
	if concurrency > jobCount {
		concurrency = jobCount
	}
	return concurrency
}

// SortJobResults sorts job results by Order ascending (in-place).
func SortJobResults(results []*JobResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})
}
