package reactloops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// DispatchSubReactJobsLoopKey 在 loop vars 中存储 JSON 编码的 dispatch 任务。
	DispatchSubReactJobsLoopKey        = "dispatch_sub_react_jobs"
	DispatchSubReactConcurrencyLoopKey = "dispatch_sub_react_concurrency"

	MaxDispatchSubReactJobs    = 30
	DefaultDispatchConcurrency = 5
	MaxDispatchConcurrency     = 10
)

// ProcessStats 汇总已完成子 Agent 的运行期活动数据。
type ProcessStats struct {
	Iterations      int    `json:"iterations"`
	Actions         int    `json:"actions"`
	ToolCalls       int    `json:"tool_calls"`
	TimelineItems   int    `json:"timeline_items"`
	BranchDiffBytes int    `json:"branch_diff_bytes"`
	FinalAction     string `json:"final_action,omitempty"`
}

// TimelineRecord 是写回父 timeline 的结构化结果。
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

// JobRunner 是执行单个子 Agent dispatch 任务的接口。默认实现（ForkedRunner）
// 会 fork 父 timeline 并在子分支中运行完整 ReAct loop。测试可提供 mock 实现。
type JobRunner interface {
	Run(
		parentInvoker aicommon.AIInvokeRuntime,
		parentLoop *ReActLoop,
		parentTask aicommon.AIStatefulTask,
		job SubAgentJob,
		registry *ProgressRegistry,
	) (*SubAgentResult, error)
}

// ForkedRunner 是默认的 JobRunner，fork 父 timeline 并在子分支中运行完整
// ReAct loop。
type ForkedRunner struct{}

// DefaultRunner 是 RunJobsConcurrently 使用的包级 runner 实例。测试可替换
// 此变量以注入 mock 行为。
var DefaultRunner JobRunner = ForkedRunner{}

// Run 执行单个子 Agent 任务：fork 父 timeline 并在子分支中运行完整 ReAct loop。
func (ForkedRunner) Run(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	registry *ProgressRegistry,
) (*SubAgentResult, error) {
	return RunForkedJob(parentInvoker, parentLoop, parentTask, job, registry)
}

// RunForkedJob fork 父 timeline，润色简要 goal，注册 progress handle，运行子
// loop，并返回 SubAgentResult。
func RunForkedJob(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	registry *ProgressRegistry,
) (*SubAgentResult, error) {
	startedAt := time.Now()
	forkJob := SubAgentJob{
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

	// 在子 Agent 运行前，把简要意图（job.Goal）润色成完整、自包含的 goal 加上
	// result contract。
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

	subLoop, execErr := runSubLoopWithHandle(
		childInvoker, job.LoopName, subTask, job.Identifier, registry, startedAt,
		DefaultForkOptions(), nil,
	)
	result, _ := BuildJobResult(job, startedAt, subTask, subLoop, fork, execErr)
	return result, nil
}

// RunJobsConcurrently 通过 worker 池并发运行多个子 Agent dispatch 任务。
//
// 由于 runJobsConcurrently 直接操作统一的 SubAgentResult 类型，这里先把每个
// SubAgentJob 包装成 SubAgentResult（经内嵌 SubAgentJob 携带任务身份），再由
// runSingle 执行 runner 并填入结果。
func RunJobsConcurrently(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	jobs []SubAgentJob,
	concurrency int,
	registry *ProgressRegistry,
) []*SubAgentResult {
	runner := DefaultRunner
	wrapped := make([]*SubAgentResult, len(jobs))
	for i, job := range jobs {
		wrapped[i] = &SubAgentResult{SubAgentJob: job}
	}
	runSingle := func(r *SubAgentResult) *SubAgentResult {
		result, err := runner.Run(parentInvoker, parentLoop, parentTask, r.SubAgentJob, registry)
		if err != nil {
			return failedJobResult(parentTask, r.SubAgentJob, err)
		}
		return result
	}
	return runJobsConcurrently(wrapped, concurrency, runSingle)
}

// failedJobResult 构建一个描述任务在产出正常结果前就已失败的 SubAgentResult
//（如 fork 准备阶段出错）。供 RunJobsConcurrently 及需要暴露 setup 级错误的
// runner 共用。
func failedJobResult(parentTask aicommon.AIStatefulTask, job SubAgentJob, err error) *SubAgentResult {
	return &SubAgentResult{
		SubAgentJob: job,
		Record: TimelineRecord{
			SubAgentID: BuildForkTaskID(parentTask, SubAgentJob{
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

// BuildJobResult 根据子任务和子 loop 状态构造 SubAgentResult。
func BuildJobResult(
	job SubAgentJob,
	startedAt time.Time,
	subTask aicommon.AIStatefulTask,
	subLoop *ReActLoop,
	fork *aicommon.TimelineFork,
	execErr error,
) (*SubAgentResult, error) {
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

	return &SubAgentResult{
		SubAgentJob: job,
		Record:   record,
		Feedback: feedback,
	}, nil
}

// CollectProcessStats 从子 loop 中收集 iteration / action / tool-call 统计数据。
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

// CountBranchTimelineItems 统计 fork 分支中新增的 timeline 条目数。
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

// SummarizeForkDiff 返回 fork diff 的截断预览和字节计数。
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

// --- goal 润色 ---

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
	job SubAgentJob,
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

// --- 解析 ---

// ParseDispatchJobs 从 AI action 的 "dispatches" 参数中提取 dispatch 任务。
func ParseDispatchJobs(action *aicommon.Action) ([]SubAgentJob, error) {
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

func parseDispatchJobsFromArray(raw []aitool.InvokeParams) ([]SubAgentJob, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	jobs := make([]SubAgentJob, 0, len(raw))
	for _, item := range raw {
		if item == nil {
			continue
		}
		jobs = append(jobs, SubAgentJob{
			Identifier: strings.TrimSpace(item.GetString("identifier")),
			Goal:       strings.TrimSpace(item.GetString("goal")),
			TaskName:   strings.TrimSpace(item.GetString("task_name")),
			LoopName:   strings.TrimSpace(item.GetString("loop_name")),
		})
	}
	return NormalizeDispatchJobs(jobs)
}

// NormalizeDispatchJobs 校验并规范化 dispatch 任务。
func NormalizeDispatchJobs(jobs []SubAgentJob) ([]SubAgentJob, error) {
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

// ParseConcurrency 从 AI action 中提取并发参数并限制到合法范围。
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

// SortJobResults 按 Order 升序原地排序 job 结果。
func SortJobResults(results []*SubAgentResult) {
	sortSubAgentResultsByOrder(results)
}

// ─────────────────────────────────────────────────────────────────────
// 编程式 nested 子 Agent dispatch（按 loop name，可选 fork）
//
// 与 AI 驱动的 dispatch_sub_react_agents action 不同，这些辅助函数由
// orchestrator loop（如 code audit phase2 fast_context）直接调用，以运行一个
// *指定* 的已注册 ReAct loop 作为子 Agent。调用方自行选择是 fork 父 timeline
//（timeline 隔离）还是原地运行（运行结束后回滚 timeline 条目）。
//
// 关键点：每次运行都会向父 loop 的 ProgressRegistry 注册一个 SubAgentHandle，
// 使 stall-heartbeat 子 Agent 旁路（见 loop_stall_heartbeat.go:141）将父 loop 的
// 阻塞等待视为"仍在推进"。若不注册，父 loop 在阻塞等待 nested 子 loop
//（如 fast_context）时 registry 中将没有活跃子 Agent，IsAnyActive() 返回 false，
// stall heartbeat 会误报 [LOOP_STALL_DETECTED] / hard abort。
// ─────────────────────────────────────────────────────────────────────

// ensureSubAgentProgressRegistry 返回父 loop 已有的 ProgressRegistry，若未设置
// 则创建并安装一个。使 stall heartbeat / verification watchdog 能观察子 Agent 活动。
func ensureSubAgentProgressRegistry(parentLoop *ReActLoop) *ProgressRegistry {
	if parentLoop == nil {
		return nil
	}
	registry := parentLoop.GetSubAgentProgressRegistry()
	if registry == nil {
		registry = NewProgressRegistry()
		parentLoop.SetSubAgentProgressRegistry(registry)
	}
	return registry
}

// RunNestedJobWithProgress 运行单个 nested 子 Agent loop（按已注册 loop name），
// 并将其进度注册到 parentLoop 的 ProgressRegistry，使 stall-heartbeat 子 Agent
// 旁路将父 loop 的阻塞等待视为"仍在推进"。
//
// 当 job.ForkTimeline 为 true 时 fork 父 timeline（timeline 隔离，分支 diff 可用）。
// 为 false 时子 loop 在父 timeline 上原地运行，运行期间产生的 timeline 条目会在
// 结束后回滚（截断）——与 RunNestedLoop 语义一致。
//
// 返回的 SubAgentResult.SubLoop 始终是执行过的 ReActLoop（即使出错也保留，以便
// 调用方读取 loop 变量 / 交付物）。
func RunNestedJobWithProgress(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	job SubAgentJob,
	configure func(subLoop *ReActLoop),
	opts ...ReActLoopOption,
) (*SubAgentResult, error) {
	startedAt := time.Now()

	if parentInvoker == nil {
		return nil, utils.Error("parent invoker is nil")
	}
	if parentTask == nil {
		return nil, utils.Error("parent task is nil")
	}
	if err := validateNestedJob(&job); err != nil {
		return nil, err
	}

	// 确保父 loop 有 progress registry，使 stall heartbeat 子 Agent 旁路能在父
	// loop 阻塞期间观察此子 Agent。
	registry := ensureSubAgentProgressRegistry(parentLoop)

	if job.ForkTimeline {
		return runNestedForked(parentInvoker, parentTask, job, registry, configure, opts, startedAt)
	}
	return runNestedInPlace(parentInvoker, parentTask, job, registry, configure, opts, startedAt)
}
// RunNestedJobsConcurrentlyWithProgress 通过 worker 池并发运行多个 nested 子 Agent
// 任务。每个任务经 RunNestedJobWithProgress 执行。结果按 Order 升序排序。
//
// 由于 runJobsConcurrently 直接操作统一的 SubAgentResult 类型，这里先把每个
// SubAgentJob 包装成 SubAgentResult（经内嵌 SubAgentJob 携带任务身份），再由
// runSingle 执行 nested 运行并填入结果。
func RunNestedJobsConcurrentlyWithProgress(
	parentInvoker aicommon.AIInvokeRuntime,
	parentLoop *ReActLoop,
	parentTask aicommon.AIStatefulTask,
	jobs []SubAgentJob,
	concurrency int,
	configure func(subLoop *ReActLoop),
	opts ...ReActLoopOption,
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
		result, err := RunNestedJobWithProgress(parentInvoker, parentLoop, parentTask, r.SubAgentJob, configure, opts...)
		if err != nil && result == nil {
			return &SubAgentResult{SubAgentJob: r.SubAgentJob, ExecErr: err}
		}
		return result
	}

	results := runJobsConcurrently(wrapped, concurrency, runSingle)
	sortSubAgentResultsByOrder(results)
	return results
}
