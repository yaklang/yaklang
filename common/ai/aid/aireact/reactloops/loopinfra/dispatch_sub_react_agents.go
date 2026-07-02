package loopinfra

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"io"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	subAgentDepthLoopVar               = "sub_agent_depth"
	dispatchSubReactJobsLoopKey        = "dispatch_sub_react_jobs"
	dispatchSubReactConcurrencyLoopKey = "dispatch_sub_react_concurrency"

	maxDispatchSubReactJobs       = 30
	defaultDispatchConcurrency    = 5
	maxDispatchConcurrency        = 10
	defaultSubAgentMaxIterations  = 50
	maxSubAgentMaxIterations      = 100
	defaultSubAgentTimeoutSeconds = 0
	maxSubAgentTimeoutSeconds     = 600
)

type subReactDispatchJob struct {
	Order          int    `json:"order"`
	Identifier     string `json:"identifier"`
	Goal           string `json:"goal"`
	LoopName       string `json:"loop_name"`
	ResultContract string `json:"result_contract"`
	MaxIterations  int    `json:"max_iterations"`
	TimeoutSeconds int    `json:"timeout_seconds"`
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
	subTaskName := job.Goal
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

	jobCtx := parentTask.GetContext()
	if job.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		jobCtx, cancel = context.WithTimeout(jobCtx, time.Duration(job.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	childInvoker, err := buildForkedSubReactInvoker(parentCfg, fork, jobCtx, subTaskID)
	if err != nil {
		return nil, err
	}

	subTask := aicommon.NewSubTaskBaseWithOptions(
		parentTask,
		subTaskID,
		buildSubAgentUserInput(job),
		aicommon.WithStatefulTaskBaseName(subTaskName),
		aicommon.WithStatefulTaskBaseSubAgent(),
		aicommon.WithStatefulTaskBaseContext(jobCtx),
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

	subLoop, err := reactloops.CreateLoopByName(job.LoopName, childInvoker, buildSubReactLoopOptions(job)...)
	if err != nil {
		result, _ := buildSubReactJobResult(job, startedAt, subTask, nil, fork, err)
		return result, nil
	}

	execErr := subLoop.ExecuteWithExistedTask(subTask)
	result, _ := buildSubReactJobResult(job, startedAt, subTask, subLoop, fork, execErr)
	return result, nil
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
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithEmitter(buildSubReactForwardingEmitter(parentCfg.GetEmitter(), subTaskId)),
		aicommon.WithHotPatchOptionChan(chanx.NewUnlimitedChan[aicommon.ConfigOption](jobCtx, 1)),
		aicommon.WithAgreeAuto(),
	)

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

func buildSubReactLoopOptions(job subReactDispatchJob) []reactloops.ReActLoopOption {
	maxIter := job.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultSubAgentMaxIterations
	}
	return []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(maxIter),
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

func buildSubAgentUserInput(job subReactDispatchJob) string {
	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(job.Goal))
	if contract := strings.TrimSpace(job.ResultContract); contract != "" {
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
			Identifier:     strings.TrimSpace(item.GetString("identifier")),
			Goal:           strings.TrimSpace(item.GetString("goal")),
			LoopName:       strings.TrimSpace(item.GetString("loop_name")),
			ResultContract: strings.TrimSpace(item.GetString("result_contract")),
			MaxIterations:  int(item.GetInt("max_iterations")),
			TimeoutSeconds: int(item.GetInt("timeout_seconds")),
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
		if jobs[i].MaxIterations <= 0 {
			jobs[i].MaxIterations = defaultSubAgentMaxIterations
		}
		if jobs[i].MaxIterations > maxSubAgentMaxIterations {
			return nil, utils.Errorf("dispatches[%d].max_iterations exceeds limit %d", i, maxSubAgentMaxIterations)
		}
		if jobs[i].TimeoutSeconds < 0 {
			return nil, utils.Errorf("dispatches[%d].timeout_seconds must be >= 0", i)
		}
		if jobs[i].TimeoutSeconds > maxSubAgentTimeoutSeconds {
			return nil, utils.Errorf("dispatches[%d].timeout_seconds exceeds limit %d", i, maxSubAgentTimeoutSeconds)
		}
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
	loopInfraActionFinish(loop, loopInfraNodeDispatchSubReact, summary)

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

func formatDispatchSubReactJobDisplayLine(job subReactDispatchJob) string {
	identifier := strings.TrimSpace(job.Identifier)
	goal := strings.TrimSpace(job.Goal)
	if identifier == "" && goal == "" {
		return ""
	}
	line := fmt.Sprintf("- %s", goal)
	if loopName := strings.TrimSpace(job.LoopName); loopName != "" && loopName != schema.AI_REACT_LOOP_NAME_DEFAULT {
		line += fmt.Sprintf(" (loop: %s)", loopName)
	}
	return line
}

func dispatchSubReactDispatchesStreamHandler(fieldReader io.Reader, emitWriter io.Writer) {
	if err := writeDispatchSubReactDispatchesDisplayStream(fieldReader, emitWriter); err != nil {
		log.Debugf("dispatch_sub_react_agents: dispatches display stream failed: %v", err)
		_, _ = io.Copy(io.Discard, fieldReader)
	}
}

func writeDispatchSubReactDispatchesDisplayStream(reader io.Reader, writer io.Writer) error {
	// Stream each dispatch object as soon as it completes in the JSON array,
	// instead of buffering the whole array via encoding/json. The structured
	// streaming extractor invokes the object callback inline in parse order, so
	// goals are emitted one-by-one as they arrive.
	firstLine := true
	order := 0
	emitLine := func(line string) error {
		if strings.TrimSpace(line) == "" {
			return nil
		}
		if !firstLine {
			if _, err := writer.Write([]byte("\n")); err != nil {
				return err
			}
		}
		firstLine = false
		_, err := io.WriteString(writer, line)
		return err
	}

	err := jsonextractor.ExtractStructuredJSONFromStream(reader,
		jsonextractor.WithObjectCallback(func(data map[string]any) {
			params := aitool.InvokeParams(data)
			goal := strings.TrimSpace(params.GetString("goal"))
			// Skip objects without a goal: they are either incomplete stream
			// fragments or nested maps that are not dispatch jobs.
			if goal == "" {
				return
			}
			order++
			job := subReactDispatchJob{
				Order:      order,
				Identifier: strings.TrimSpace(params.GetString("identifier")),
				Goal:       goal,
				LoopName:   strings.TrimSpace(params.GetString("loop_name")),
			}
			if e := emitLine(formatDispatchSubReactJobDisplayLine(job)); e != nil {
				log.Debugf("dispatch_sub_react_agents: dispatches display stream write failed: %v", e)
			}
		}),
		jsonextractor.WithStreamErrorCallback(func(err error) {
			log.Debugf("dispatch_sub_react_agents: dispatches display stream parse error: %v", err)
		}),
	)
	return err
}

var loopAction_DispatchSubReactAgents = &reactloops.LoopAction{
	ActionType: schema.AI_REACT_LOOP_ACTION_DISPATCH_SUB_REACT_AGENTS,
	Description: "Dispatch multiple independent sub ReAct agents in parallel. Each sub agent inherits the current timeline snapshot as context, " +
		"runs in an isolated timeline fork, and returns one structured result record back to the parent agent. " +
		"Use this when a task can be split into parallel sub-goals that should not pollute the parent timeline with their full execution traces. " +
		"Sub agents cannot dispatch more sub agents.",
	Options: []aitool.ToolOption{
		aitool.WithStructArrayParam("dispatches",
			[]aitool.PropertyOption{
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Sub agent jobs to dispatch in parallel. Each item runs in an isolated timeline fork and returns one structured result back to the parent."),
			},
			nil,
			aitool.WithStringParam("identifier",
				aitool.WithParam_Description("Optional stable label for this sub agent. Auto-generated from array index when omitted."),
			),
			aitool.WithStringParam("goal",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Task goal for the sub ReAct agent."),
			),
			aitool.WithStringParam("loop_name",
				aitool.WithParam_Description(fmt.Sprintf("Target ReAct loop name. Defaults to %q.", schema.AI_REACT_LOOP_NAME_DEFAULT)),
			),
			aitool.WithStringParam("result_contract",
				aitool.WithParam_Description("Optional output format or acceptance criteria for the sub agent result."),
			),
			aitool.WithIntegerParam("max_iterations",
				aitool.WithParam_Description(fmt.Sprintf("Maximum sub loop iterations. Default %d, max %d.", defaultSubAgentMaxIterations, maxSubAgentMaxIterations)),
			),
			aitool.WithIntegerParam("timeout_seconds",
				aitool.WithParam_Description(fmt.Sprintf("Per-job timeout in seconds. 0 inherits parent task context. Max %d.", maxSubAgentTimeoutSeconds)),
			),
		),
		aitool.WithIntegerParam(
			"concurrency",
			aitool.WithParam_Description(fmt.Sprintf("Parallelism for sub agent execution. Default min(len(dispatches), %d), max %d.", defaultDispatchConcurrency, maxDispatchConcurrency)),
		),
	},
	StreamFields: []*reactloops.LoopStreamField{
		{
			FieldName:     "dispatches",
			AINodeId:      loopInfraNodeDispatchSubReact,
			StreamHandler: dispatchSubReactDispatchesStreamHandler,
			ContentType:   aicommon.TypeTextMarkdown,
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
