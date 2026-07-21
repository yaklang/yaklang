package reactloops

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
	"encoding/json"
	"sort"
)

// PreparedSubAgent 是阶段 1（准备构建）的产物：一个已装配好、随时可执行的子
// Agent 运行体。它持有子 invoker / 子 task / timeline 容器 / progress handle，
// 调用方在阶段 2 启动 loop，在阶段 3 释放资源。
type PreparedSubAgent struct {
	Job       SubAgentJob
	Invoker   aicommon.AITaskInvokeRuntime
	Task      aicommon.AIStatefulTask
	Timeline  *TimelineHandle
	Handle    *SubAgentHandle // 已注册到 ProgressRegistry；nil 表示跳过注册
	Release   func()          // 取消 jobCtx（幂等）
	StartedAt time.Time
}

// ExecutedSubAgent 是阶段 2（执行任务）的产物：一个已执行（或执行失败）的子
// Agent。它内嵌 PreparedSubAgent，使阶段 3 能读取身份与运行体信息。
type ExecutedSubAgent struct {
	*PreparedSubAgent
	SubLoop  *ReActLoop // loop 创建失败时为 nil
	ExecErr  error
	Duration time.Duration
}

// DispatchSubAgents 是下发子 Agent 的唯一公共入口，涵盖原 dispatch / fork /
// nested 三条路径。它内部完成 准备→执行→结果统一 三阶段。
//
// 调用方只需提供 jobs 和 options，拿到 []*SubAgentResult。父 loop 的
// ProgressRegistry 在此自动 ensure + 注册，不再需要调用方手动管理。
//
// 关键词: DispatchSubAgents, sub agent 统一入口, 三阶段流水线
func DispatchSubAgents(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	jobs []SubAgentJob,
	opts SubAgentOptions,
) []*SubAgentResult {
	if len(jobs) == 0 {
		return nil
	}
	if parentInvoker == nil {
		return nil
	}
	if parentTask == nil {
		return nil
	}

	// ── 阶段 1：准备构建 ──
	prepared := prepareSubAgents(parentInvoker, parentTask, jobs, opts)

	// ── 阶段 2：执行任务 ──
	executed := executeSubAgents(prepared, opts)

	// ── 阶段 3：结果统一 ──
	results := finalizeSubAgents(executed, opts)

	return results
}

// ─────────────────────────────────────────────────────────────────────
// 阶段 1：准备构建 (Prepare)
// ─────────────────────────────────────────────────────────────────────

// prepareSubAgents 为每个 job 构建 PreparedSubAgent：timeline 容器、子 invoker、
// 子 task、progress handle。不做任何 LLM 调用，不启动 loop。
func prepareSubAgents(
	parentInvoker aicommon.AIInvokeRuntime,
	parentTask aicommon.AIStatefulTask,
	jobs []SubAgentJob,
	opts SubAgentOptions,
) []*PreparedSubAgent {
	parentCfg, ok := parentInvoker.GetConfig().(*aicommon.Config)
	if !ok || parentCfg == nil {
		log.Warnf("subagent: parent config is not *aicommon.Config, prepare failed; type=%T", parentInvoker.GetConfig())
		return nil
	}
	parentTimeline := parentCfg.GetTimeline()

	// ensure ProgressRegistry on parent loop（若提供），使 stall heartbeat /
	// verification watchdog 能观察子 Agent 活动。
	var registry *ProgressRegistry
	if opts.ParentLoop != nil {
		registry = ensureSubAgentProgressRegistry(opts.ParentLoop)
	}

	prepared := make([]*PreparedSubAgent, 0, len(jobs))
	for _, job := range jobs {
		startedAt := time.Now()
		handle, err := buildTimelineHandle(
			parentCfg, parentTimeline,
			BuildForkTaskID(parentTask, job), subAgentTaskName(job),
			opts.TimelineMode,
		)
		if err != nil {
			// timeline 容器构建失败：构造一个失败的 prepared，让阶段 2/3 兜底。
			log.Warnf("subagent: build timeline handle for %q failed: %v", job.Identifier, err)
			prepared = append(prepared, &PreparedSubAgent{
				Job: job, StartedAt: startedAt,
				Release: func() {},
			})
			continue
		}

		invoker, task, release, err := buildSubAgentRuntime(parentInvoker, parentTask, job, handle, opts)
		if err != nil {
			log.Warnf("subagent: build runtime for %q failed: %v", job.Identifier, err)
			handle.Release()
			prepared = append(prepared, &PreparedSubAgent{
				Job: job, Timeline: handle, StartedAt: startedAt,
				Release: func() {},
			})
			continue
		}

		// 注册 progress handle，使父 loop 的 stall heartbeat / watchdog 旁路生效。
		var progHandle *SubAgentHandle
		if registry != nil {
			progHandle = registerHandle(registry, task.GetId(), job.Identifier, task, startedAt)
		}

		prepared = append(prepared, &PreparedSubAgent{
			Job:      job,
			Invoker:  invoker,
			Task:     task,
			Timeline: handle,
			Handle:   progHandle,
			Release:  release,
			StartedAt: startedAt,
		})
	}
	return prepared
}

// subAgentTaskName 从 job 推导子 task 的展示名。
func subAgentTaskName(job SubAgentJob) string {
	if name := strings.TrimSpace(job.TaskName); name != "" {
		return name
	}
	if id := strings.TrimSpace(job.Identifier); id != "" {
		return id
	}
	if g := strings.TrimSpace(job.Goal); g != "" {
		return g
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────
// 阶段 2：执行任务 (Execute)
// ─────────────────────────────────────────────────────────────────────

// executeSubAgents 通过 worker 池并发执行 prepared 子 Agent。每个 worker 内
// 串行完成 [可选润色 → 构建 loop → 执行 loop]。
func executeSubAgents(prepared []*PreparedSubAgent, opts SubAgentOptions) []*ExecutedSubAgent {
	if len(prepared) == 0 {
		return nil
	}
	concurrency := normalizeSubAgentConcurrency(opts.ExecuteConcurrency, len(prepared))

	ctx := context.Background()
	// 复用 prepared 自身没有 ctx；用 background，executeSubAgent 内部从 task 取 ctx。
	runSingle := func(p *PreparedSubAgent) *ExecutedSubAgent {
		return runExecuteSingleWithRecover(p, opts)
	}

	// runJobsConcurrently 接受 []*SubAgentResult 以内嵌 SubAgentJob 携带身份；
	// 这里临时把 prepared 包装成 executed（零值 SubLoop），执行后回填。
	// 为复用现有 worker 池，直接用一个本地 worker 池实现。
	executed := make([]*ExecutedSubAgent, 0, len(prepared))
	if concurrency <= 1 {
		for _, p := range prepared {
			executed = append(executed, runSingle(p))
		}
		return executed
	}

	swg := utils.NewSizedWaitGroup(concurrency)
	var mu sync.Mutex
	for _, p := range prepared {
		if err := swg.AddWithContext(ctx, 1); err != nil {
			executed = append(executed, failedExecuted(p, "cancelled", err))
			continue
		}
		p := p
		go func() {
			defer swg.Done()
			res := runSingle(p)
			mu.Lock()
			executed = append(executed, res)
			mu.Unlock()
		}()
	}
	swg.Wait()
	return executed
}

// runExecuteSingleWithRecover 在 recover 中执行 executeSubAgent，把 panic 转写成
// 失败结果，避免单个 job 崩溃静默吞掉整批任务。返回 nil 时兜底成失败结果。
func runExecuteSingleWithRecover(p *PreparedSubAgent, opts SubAgentOptions) (result *ExecutedSubAgent) {
	if p == nil {
		return nil
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Errorf("subagent: executeSubAgent panic for job %q (order=%d): %v", p.Job.Identifier, p.Job.Order, rec)
			result = failedExecuted(p, "panic", utils.Errorf("subagent panic: %v", rec))
			return
		}
		if result == nil {
			log.Errorf("subagent: executeSubAgent returned nil for job %q (order=%d)", p.Job.Identifier, p.Job.Order)
			result = failedExecuted(p, "panic", utils.Error("executeSubAgent returned nil result"))
		}
	}()
	result = executeSubAgent(p, opts)
	return
}

// executeSubAgent 执行单个子 Agent：可选润色 → 构建 loop → 执行 loop。
// 润色与子 loop 共享同一 worker 槽位、同一并发度，不在单独的集中批次中执行。
func executeSubAgent(prepared *PreparedSubAgent, opts SubAgentOptions) *ExecutedSubAgent {
	// timeline/invoker 构建失败时 prepared.Invoker 为 nil，直接返回失败。
	if prepared.Invoker == nil || prepared.Task == nil {
		return failedExecuted(prepared, "failed",
			utils.Errorf("subagent runtime not built: %s", prepared.Job.Identifier))
	}

	// 在润色与 loop 执行之前就把子任务标记为 Processing，使 UI 在润色 LLM
	// 调用进行期间就显示"处理中"，而非仅在 loop 执行后翻转。
	prepared.Task.SetStatus(aicommon.AITaskState_Processing)

	// 2a. 可选润色 goal（单 job 内前置步骤）。
	if opts.ElaborateGoals {
		elaborator := opts.GoalElaborator
		if elaborator == nil {
			elaborator = defaultGoalElaborator{}
		}
		ctx := prepared.Task.GetContext()
		if ctx == nil {
			ctx = context.Background()
		}
		goal, contract, elabErr := elaborator.Elaborate(ctx, prepared)
		if elabErr != nil {
			log.Warnf("subagent: elaborate goal for %s failed, falling back to brief intent: %v", prepared.Task.GetId(), elabErr)
			goal = prepared.Job.Goal
			contract = ""
		}
		prepared.Task.SetUserInput(buildUserInput(goal, contract))
	}

	// 2b. 构建 loop。
	builder := opts.LoopBuilder
	if builder == nil {
		builder = nameLoopBuilder{} // 默认：按 job.LoopName 走 CreateLoopByName
	}
	loop, buildErr := builder.Build(prepared)
	if buildErr != nil {
		return failedExecuted(prepared, "failed", utils.Wrap(buildErr, "build sub-loop"))
	}
	if prepared.Handle != nil {
		prepared.Handle.SubLoop = loop
	}

	// 2c. 执行 loop。
	if opts.ConfigureLoop != nil {
		opts.ConfigureLoop(loop)
	}
	execErr := loop.ExecuteWithExistedTask(prepared.Task)

	return &ExecutedSubAgent{
		PreparedSubAgent: prepared,
		SubLoop:          loop,
		ExecErr:          execErr,
		Duration:         time.Since(prepared.StartedAt),
	}
}

// nameLoopBuilder 是默认 LoopBuilder，按 job.LoopName 走 CreateLoopByName。
type nameLoopBuilder struct{}

func (nameLoopBuilder) Build(prepared *PreparedSubAgent) (*ReActLoop, error) {
	loopName := strings.TrimSpace(prepared.Job.LoopName)
	if loopName == "" {
		return nil, utils.Error("subagent job loop_name is required when no LoopBuilder is provided")
	}
	// 统一追加子 Agent 默认选项（depth / 关闭 dispatch / plan / forge / loading）。
	opts := append([]ReActLoopOption{}, DefaultSubAgentLoopOptions()...)
	// 调用方额外的 loop 选项在此追加。
	return CreateLoopByName(loopName, prepared.Invoker, opts...)
}

// failedExecuted 构建一个失败的 ExecutedSubAgent，统一 cancelled / panic /
// build-failed 等失败场景。status 标识失败类别，err 为失败原因。
func failedExecuted(prepared *PreparedSubAgent, status string, err error) *ExecutedSubAgent {
	return &ExecutedSubAgent{
		PreparedSubAgent: prepared,
		SubLoop:          nil,
		ExecErr:          err,
		Duration:         time.Since(prepared.StartedAt),
	}
}

// ─────────────────────────────────────────────────────────────────────
// 阶段 3：结果统一 (Finalize)
// ─────────────────────────────────────────────────────────────────────

// finalizeSubAgents 统一构造 SubAgentResult，注销 handle，释放资源。
func finalizeSubAgents(executed []*ExecutedSubAgent, opts SubAgentOptions) []*SubAgentResult {
	results := make([]*SubAgentResult, 0, len(executed))
	for _, e := range executed {
		if e == nil {
			continue
		}
		results = append(results, BuildSubAgentResult(e, opts))
		// 注销 progress handle（幂等）。
		if e.Handle != nil && opts.ParentLoop != nil {
			if reg := opts.ParentLoop.GetSubAgentProgressRegistry(); reg != nil {
				reg.Unregister(e.Task.GetId(), e.ExecErr)
			}
		}
		// 释放 jobCtx / timeline 容器。
		if e.Release != nil {
			e.Release()
		}
		if e.Timeline != nil {
			e.Timeline.Release()
		}
	}
	sortSubAgentResultsByOrder(results)
	return results
}

// BuildSubAgentResult 是阶段 3 的唯一结果构造函数，覆盖所有路径。它从 executed
// 中读取子 task / 子 loop / timeline 容器 / 执行错误，统一填充 SubAgentResult
// 的所有字段——不再有"按路径取字段"的问题。
func BuildSubAgentResult(executed *ExecutedSubAgent, opts SubAgentOptions) *SubAgentResult {
	if executed == nil {
		return nil
	}
	job := executed.Job
	subTask := executed.Task
	subLoop := executed.SubLoop
	execErr := executed.ExecErr
	duration := executed.Duration
	if duration == 0 {
		duration = time.Since(executed.StartedAt)
	}

	record := TimelineRecord{
		SubAgentID: subTaskIDOr(subTask, job),
		Order:      job.Order,
		LoopName:   job.LoopName,
		Goal:       job.Goal,
		DurationMs: duration.Milliseconds(),
	}
	status := "completed"
	if execErr != nil {
		status = "failed"
		record.Error = execErr.Error()
	}
	record.Status = status

	// 结果文本：优先 task.GetResult，fallback subLoop 的 directly_answer_payload。
	resultText := ""
	if subTask != nil {
		resultText = strings.TrimSpace(subTask.GetResult())
	}
	if resultText == "" && subLoop != nil {
		resultText = strings.TrimSpace(subLoop.Get("directly_answer_payload"))
	}
	record.Result = utils.ShrinkTextBlock(resultText, 4000)

	// 统一落盘 reference：所有路径都享受，不再只有 dispatch 路径。
	if strings.TrimSpace(record.Result) != "" && opts.ParentLoop != nil {
		ref, preview := SaveContentReference(opts.ParentLoop, "sub_react_agent_"+record.SubAgentID, record.Result, 800)
		if ref != "" {
			record.ResultReference = ref
			record.Result = preview
		}
	}

	// ProcessStats + TracePreview。
	branchDiffBytes := 0
	if executed.Timeline != nil {
		preview, bytes := executed.Timeline.DiffPreview()
		record.TracePreview = preview
		branchDiffBytes = bytes
	}
	record.ProcessStats = CollectProcessStats(subLoop, executed.Timeline.Fork(), branchDiffBytes)

	feedback := fmt.Sprintf("[%d] %s (%s): %s", job.Order, job.Identifier, record.Status, utils.ShrinkString(record.Result, 240))
	if record.Error != "" {
		feedback = fmt.Sprintf("[%d] %s (%s): %s", job.Order, job.Identifier, record.Status, record.Error)
	}

	return &SubAgentResult{
		SubAgentJob: job,
		SubTaskID:   record.SubAgentID,
		SubTask:     subTask,
		SubLoop:     subLoop,
		Fork:        executed.Timeline.Fork(),
		ExecErr:     execErr,
		Duration:    duration,
		Record:      record,
		Feedback:    feedback,
	}
}

func subTaskIDOr(subTask aicommon.AIStatefulTask, job SubAgentJob) string {
	if subTask != nil && subTask.GetId() != "" {
		return subTask.GetId()
	}
	return job.Identifier
}

// ─────────────────────────────────────────────────────────────────────
// 默认 GoalElaborator
// ─────────────────────────────────────────────────────────────────────

// defaultGoalElaborator 封装现有 elaborateGoal（prompt 模板 +
// InvokeQualityPriorityLiteForge）。它是 SubAgentOptions.GoalElaborator 为 nil
// 时的默认实现。
type defaultGoalElaborator struct{}

func (defaultGoalElaborator) Elaborate(ctx context.Context, prepared *PreparedSubAgent) (goal, resultContract string, err error) {
	if prepared == nil || prepared.Invoker == nil || prepared.Task == nil {
		return "", "", utils.Error("prepared sub-agent is not ready for elaboration")
	}
	// 默认使用父 loop 的 base frame context 构造润色 prompt；若无父 loop，传 nil。
	var parentLoop *ReActLoop
	// prepared 不持有 parentLoop，这里用 task context 中的 invoker 已足够——
	// elaborateGoal 内部读 parentLoop.GetBaseFrameContext()，parentLoop 为 nil
	// 时 templateData 仅含 job 字段，仍可工作。
	return elaborateGoal(ctx, prepared.Invoker, parentLoop, prepared.Task.GetId(), prepared.Job)
}

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

// --- 统计 ---

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

// sortSubAgentResultsByOrder 按 Order 升序原地排序结果。
func sortSubAgentResultsByOrder(results []*SubAgentResult) {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Order < results[j].Order
	})
}

const (
	defaultSubAgentConcurrency = 5
	maxSubAgentConcurrency     = 10
)

// SubAgentTimelineMode 决定子 Agent 的 timeline 容器方式。
//
//   - SubAgentTimelineFork：从父 timeline 克隆一份快照作为子 timeline 起点，
//     子 Agent 在分支上运行，绝不 MergeBack。适合需要继承父上下文快照的子任务。
//   - SubAgentTimelineClean：新建一个全新的空 timeline，不继承父的任何条目。
//     子 Agent 完全从零开始，必要上下文通过 job.UserInput 显式传递。
//     适合"给个查询目标就行"的搜索式子任务（如 fast_context）。
//
// 不存在"in-place rollback"模式：子 Agent 永远在自己的 timeline 容器中运行，
// 绝不写回父 timeline。父只通过 SubAgentResult.Record 看到结果。
type SubAgentTimelineMode int

const (
	// SubAgentTimelineFork 是默认模式：fork 父 timeline 快照，分支隔离。
	SubAgentTimelineFork SubAgentTimelineMode = iota
	// SubAgentTimelineClean 新建空 timeline，彻底隔离，不继承父条目。
	SubAgentTimelineClean
)

// SubAgentOptions 是下发一组子 Agent 的统一选项，替代原 dispatch / fork /
// nested 三条路径各自的隐式默认值。零值是安全的（走默认）。
type SubAgentOptions struct {
	// TimelineMode 决定子 Agent 的 timeline 容器。默认 SubAgentTimelineFork。
	TimelineMode SubAgentTimelineMode

	// LoopBuilder 构建子 loop。nil 表示按 job.LoopName 走 CreateLoopByName。
	// 调用方可注入自定义实现（如 phase2 的 buildSingleCategoryScanLoop）。
	LoopBuilder LoopBuilder

	// ElaborateGoals 为 true 时，executeSubAgent 内部在构建 loop 之前先润色
	// goal。润色是单 job 内的可选前置步骤，与子 loop 共享同一 worker 槽位与
	// 同一并发度，不单独成批。默认 false。
	ElaborateGoals bool
	// GoalElaborator 注入自定义润色器。nil + ElaborateGoals=true 用默认实现
	// （defaultGoalElaborator，封装现有 elaborateGoal）。
	GoalElaborator GoalElaborator

	// ExecuteConcurrency 单 worker 内 [可选润色 → 子 loop 执行] 的并发度。
	// <=0 默认 5，上限 10。润色与子 loop 共享此并发度。
	ExecuteConcurrency int

	// ConfigureLoop 在 loop 创建后、执行前配置 loop（设置 loop 变量等）。
	// 替代现有 configure func(*ReActLoop) 参数。
	ConfigureLoop func(*ReActLoop)

	// ExtraLoopOpts 追加到 loop 创建选项（如 tool pool 限制）。
	ExtraLoopOpts []ReActLoopOption

	// InheritEmitter 控制子 Agent 的 emitter 是否直接继承父任务的 emitter。
	//
	// 为 true 时：子 Agent 直接复用父任务的 emitter，共用父任务的
	// TaskId/TaskUUID，不打子任务 ID、不发 react_task_created 卡片。前端表现
	// 为父任务自身的流，用户不会看到额外的子任务卡片——适用于希望"子 Agent
	// 对用户不可见"的场景（如 fast_context：它只是父任务内部的一个搜索步骤，
	// 不应让用户以为是两个任务在跑）。
	//
	// 为 false 时（默认）：子 Agent 的事件经转发 emitter（BuildForwardingEmitterForTask）
	// 打上子 TaskId/TaskUUID，前端会显示子 Agent 卡片（react_task_created），
	// 用户看到的是一个独立子任务在运行——适用于需要 UI 区分父子任务的场景
	// （如 dispatch）。
	InheritEmitter bool

	// ParentLoop 是父 loop，用于挂载 ProgressRegistry 使 stall heartbeat /
	// verification watchdog 能观察子 Agent 活动。可为 nil（跳过注册，但会
	// 丧失 stall heartbeat 旁路保护）。
	ParentLoop *ReActLoop
}

// LoopBuilder 构建子 Agent 的 ReActLoop。默认实现按 job.LoopName 查注册表
// （CreateLoopByName）；调用方可注入自定义实现（如 phase2 的
// buildSingleCategoryScanLoop）。
type LoopBuilder interface {
	Build(prepared *PreparedSubAgent) (*ReActLoop, error)
}

// GoalElaborator 把一个 SubAgentJob 的 brief intent 润色成完整 goal +
// result_contract。它是 executeSubAgent 内部的可选前置步骤，由
// SubAgentOptions.ElaborateGoals 开启。
//
// 润色在 worker 内、子 loop 执行之前被调用（串行），不在单独的集中批次中
// 执行。因此润色与子 loop 共享 ExecuteConcurrency，无需独立并发度参数；润色
// 失败只回退该 job 的 goal，不影响其他 job。
type GoalElaborator interface {
	Elaborate(ctx context.Context, prepared *PreparedSubAgent) (goal, resultContract string, err error)
}

// DefaultSubAgentLoopOptions 返回子 Agent 强制应用的 loop 选项：记录 sub agent
// 深度、关闭 plan / forge / dispatch、隐藏 loading 状态尾、过滤掉 dispatch
// action。
func DefaultSubAgentLoopOptions() []ReActLoopOption {
	return []ReActLoopOption{
		WithVar(SubAgentDepthLoopVar, 1),
		WithNoEndLoadingStatus(true),
		WithAllowPlanAndExec(false),
		WithAllowAIForge(false),
		WithActionFilter(func(action *LoopAction) bool {
			return action.ActionType != schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS
		}),
	}
}

// normalizeSubAgentConcurrency 将并发数归一化到合法范围。原
// 
func normalizeSubAgentConcurrency(concurrency, jobCount int) int {
	if concurrency <= 0 {
		concurrency = defaultSubAgentConcurrency
		if jobCount < concurrency {
			concurrency = jobCount
		}
	}
	if concurrency > maxSubAgentConcurrency {
		concurrency = maxSubAgentConcurrency
	}
	if concurrency > jobCount {
		concurrency = jobCount
	}
	return concurrency
}

// ─────────────────────────────────────────────────────────────────────
// 兼容别名（迁移期间临时保留，旧入口删除后一并移除）
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

// SubAgentJob 是子 Agent 运行的统一描述符。它涵盖了原先 DispatchJob /
// ForkJob / NestedJob 三种类型所携带的全部字段；不同的行为差异（fork 与
// in-place、AI 润色 goal 与原始 goal、loop-factory 与 loop-name）由调用方 /
// runner 选择，而非由类型区分。
type SubAgentJob struct {
	// Order 是 1 起的序号，用于结果排序 / 展示。
	Order int `json:"order"`
	// Identifier 是子 Agent 的稳定标签（如 "scan_host_a"）。
	Identifier string `json:"identifier"`
	// Goal 是简要意图；当 UserInput 为空时作为子任务的输入，或在 dispatch
	// 路径中传给 goal 润色步骤。
	Goal string `json:"goal"`
	// TaskName 是展示在 timeline / UI 中的人类可读任务名。
	TaskName string `json:"task_name"`
	// UserInput 非空时覆盖 Goal，作为子任务的用户输入。
	UserInput string `json:"user_input,omitempty"`
	// ResultContract 是可选的验收标准 / 输出格式提示，在 fork / dispatch 路径
	// 中追加到用户输入后面。
	ResultContract string `json:"result_contract,omitempty"`
	// LoopName 是已注册的 ReAct loop factory 名称。为空时在 dispatch / nested
	// 路径中默认取 schema.AI_REACT_LOOP_NAME_DEFAULT。
	LoopName string `json:"loop_name"`

}

// SubAgentResult 是子 Agent 运行的统一结果。它内嵌 SubAgentJob，使身份字段
//（Order / Identifier / ...）自动提升，且非泛型的 runJobsConcurrently 可以
// 把同一个切片既当作 job 载体又当作结果。结果字段的并集（SubLoop / SubTask /
// Record / Feedback / ...）覆盖了原先所有结果类型；未使用的字段保持零值即可。
type SubAgentResult struct {
	SubAgentJob

	// SubTaskID 是子 Agent 任务 ID（fork 路径设置）。
	SubTaskID string
	// SubTask 是子 Agent 的 stateful task（fork / nested-in-place 路径）。
	SubTask aicommon.AIStatefulTask
	// SubLoop 是执行完成的 ReActLoop；成功时必定设置，创建失败时可能为 nil。
	SubLoop *ReActLoop
	// Fork 是 timeline fork 句柄（仅 fork 路径）。
	Fork *aicommon.TimelineFork
	// ExecErr 是执行错误，成功时为 nil。
	ExecErr error
	// Duration 是运行时长。
	Duration time.Duration
	// Record 是结构化 timeline 记录（dispatch 路径）。
	Record TimelineRecord
	// Feedback 是简短的人类可读摘要（dispatch 路径）。
	Feedback string
}


// ForkInvokerCallback 在 fork 出的子 invoker 上执行任意逻辑，但不启动 ReAct
// loop。timeline 噪音留在分支上；父 timeline 不会被截断。
type ForkInvokerCallback func(childInvoker aicommon.AIInvokeRuntime, childTask aicommon.AIStatefulTask) error

// BuildForkTaskID 根据父任务和 job identifier 构建稳定的子 Agent 任务 ID。
func BuildForkTaskID(parentTask aicommon.AIStatefulTask, job SubAgentJob) string {
	parentID := "sub-agent"
	if parentTask != nil && parentTask.GetId() != "" {
		parentID = parentTask.GetId()
	}
	segment := SanitizeIDSegment(job.Identifier)
	if segment == "" {
		segment = fmt.Sprintf("job-%d", job.Order)
	}
	return fmt.Sprintf("%s-sub-%s-%s", parentID, segment, utils.RandStringBytes(4))
}

// SanitizeIDSegment 将 job identifier 规范化，使其可用于任务 ID。
func SanitizeIDSegment(s string) string {
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
