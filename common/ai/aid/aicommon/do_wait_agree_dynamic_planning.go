package aicommon

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
)

// PlanningReviewControl is a callback that reviews plan or task decisions
// and returns appropriate suggestion params (e.g. {"suggestion": "continue"}).
// Returning an error causes fallback to the default auto-continue behavior.
type PlanningReviewControl func(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error)

//go:embed prompts/review/ai-review-plan.txt
var aiPlanReviewPromptTemplate string

//go:embed prompts/review/ai-review-task.txt
var aiTaskReviewPromptTemplate string

type PlanReviewPromptData struct {
	CurrentTime      string
	OSArch           string
	WorkingDir       string
	WorkingDirGlance string
	Timeline         string
	Nonce            string
	PlanDetails      string
	Language         string
}

type TaskReviewPromptData struct {
	CurrentTime      string
	OSArch           string
	WorkingDir       string
	WorkingDirGlance string
	Timeline         string
	Nonce            string
	TaskDetails      string
	ShortSummary     string
	LongSummary      string
	Language         string
	Progress         string
	PendingTasks     string
}

func generatePlanReviewPrompt(config *Config, materials aitool.InvokeParams) (string, error) {
	data := &PlanReviewPromptData{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		OSArch:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Nonce:       utils.RandStringBytes(4),
		Language:    config.Language,
	}

	data.WorkingDir = config.Workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = filesys.Glance(data.WorkingDir)
	}

	if t := config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}

	if !utils.IsNil(materials) {
		materialsCopy := make(aitool.InvokeParams)
		for k, v := range materials {
			if k == "selectors" || k == "id" || k == "plans_id" {
				continue
			}
			materialsCopy[k] = v
		}
		raw, err := json.MarshalIndent(materialsCopy, "", "  ")
		if err != nil {
			data.PlanDetails = materials.Dump()
		} else {
			data.PlanDetails = string(raw)
		}
	}

	tmpl, err := template.New("plan-review").Parse(aiPlanReviewPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing plan review template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing plan review template: %w", err)
	}
	return buf.String(), nil
}

func generateTaskReviewPrompt(config *Config, materials aitool.InvokeParams) (string, error) {
	data := &TaskReviewPromptData{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		OSArch:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Nonce:       utils.RandStringBytes(4),
		Language:    config.Language,
	}

	data.WorkingDir = config.Workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = filesys.Glance(data.WorkingDir)
	}

	if t := config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}

	if !utils.IsNil(materials) {
		data.ShortSummary = materials.GetString("short_summary")
		data.LongSummary = materials.GetString("long_summary")
		data.Progress = materials.GetString("progress")
		data.PendingTasks = materials.GetString("pending_tasks")

		materialsCopy := make(aitool.InvokeParams)
		for k, v := range materials {
			if k == "selectors" || k == "id" || k == "short_summary" || k == "long_summary" || k == "progress" || k == "pending_tasks" {
				continue
			}
			materialsCopy[k] = v
		}
		raw, err := json.MarshalIndent(materialsCopy, "", "  ")
		if err != nil {
			data.TaskDetails = materials.Dump()
		} else {
			data.TaskDetails = string(raw)
		}
	}

	tmpl, err := template.New("task-review").Parse(aiTaskReviewPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing task review template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("error executing task review template: %w", err)
	}
	return buf.String(), nil
}

var validPlanSuggestions = map[string]bool{
	"continue":       true,
	"unclear":        true,
	"incomplete":     true,
	"create-subtask": true,
}

var validTaskSuggestions = map[string]bool{
	"continue":     true,
	"deeply_think": true,
	"inaccurate":   true,
	"adjust_plan":  true,
}

type reviewFieldStreamSpec struct {
	FieldKey  string
	NodeID    string
	Formatter func(string) string
}

func emitReviewStatus(config *Config, nodeID, message, taskIndex string) (*schema.AiOutputEvent, error) {
	message = strings.TrimSpace(message)
	if message == "" || config == nil || config.GetEmitter() == nil {
		return nil, nil
	}
	return config.GetEmitter().EmitDefaultStreamEvent(nodeID, strings.NewReader(message), taskIndex)
}

func emitReviewStructured(config *Config, nodeID string, payload map[string]any) {
	if config == nil || config.GetEmitter() == nil || len(payload) == 0 {
		return
	}
	_, _ = config.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, nodeID, payload)
}

func waitForReviewStreams(config *Config) {
	if config == nil || config.GetEmitter() == nil {
		return
	}
	config.GetEmitter().WaitForStream()
}

func normalizeReviewFieldText(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	var decoded string
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return strings.TrimSpace(decoded)
	}
	return raw
}

func emitReviewFieldStream(emitter *Emitter, nodeID, taskIndex string, reader io.Reader, formatter func(string) string) {
	if emitter == nil || reader == nil {
		return
	}
	raw, err := io.ReadAll(utils.UTF8Reader(reader))
	if err != nil {
		return
	}
	content := normalizeReviewFieldText(string(raw))
	if formatter != nil {
		content = formatter(content)
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	_, _ = emitter.EmitDefaultStreamEvent(nodeID, strings.NewReader(content), taskIndex)
}

func reviewFieldStreamOptions(emitter *Emitter, taskIndex string, specs ...reviewFieldStreamSpec) []ActionMakerOption {
	if len(specs) == 0 {
		return nil
	}
	result := make([]ActionMakerOption, 0, len(specs))
	for _, spec := range specs {
		spec := spec
		if spec.FieldKey == "" || spec.NodeID == "" {
			continue
		}
		result = append(result, WithActionFieldStreamHandler([]string{spec.FieldKey}, func(_ string, reader io.Reader) {
			emitReviewFieldStream(emitter, spec.NodeID, taskIndex, reader, spec.Formatter)
		}))
	}
	return result
}

// DefaultAIPlanReviewControl calls AI to review the generated plan and decide
// whether to continue, mark it unclear, mark it incomplete, or request subtask creation.
func DefaultAIPlanReviewControl(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
	materials := ep.GetReviewMaterials()

	prompt, err := generatePlanReviewPrompt(config, materials)
	if err != nil {
		return nil, fmt.Errorf("generate plan review prompt failed: %w", err)
	}

	var suggestion string
	var reason string
	var rawResponse bytes.Buffer

	_, _ = emitReviewStatus(config, "plan-review-status", "正在审查计划", ep.GetId())

	err = CallAITransaction(config, prompt, config.CallQualityPriorityAI, func(rsp *AIResponse) error {
		stream := rsp.GetOutputStreamReader("plan-review", true, config.GetEmitter())
		stream = io.TeeReader(stream, &rawResponse)
		actionOpts := []ActionMakerOption{
			WithActionAlias("object"),
		}
		action, err := ExtractActionFromStream(ctx, stream, "plan_review", actionOpts...)
		if err != nil {
			return fmt.Errorf("extract plan_review action failed: %w", err)
		}
		suggestion = action.GetString("suggestion")
		reason = action.GetString("reason")
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("plan review AI transaction failed: %w", err)
	}

	if !validPlanSuggestions[suggestion] {
		log.Warnf("dynamic planning: AI returned invalid plan suggestion %q, defaulting to continue", suggestion)
		suggestion = "continue"
	}

	compactReason := compactPlanReviewReason(suggestion, reason)
	payload := map[string]any{
		"verdict":    planReviewVerdict(suggestion),
		"suggestion": suggestion,
		"reason":     compactReason,
	}
	reviewEvent, _ := emitReviewStatus(config, "plan-review", planReviewDisplayMessage(suggestion, compactReason), ep.GetId())
	waitForReviewStreams(config)
	if reviewEvent != nil {
		EmitAIRequestAndResponseReferenceMaterials(config.GetEmitter(), reviewEvent.GetStreamEventWriterId(), prompt, rawResponse.String())
	}
	emitReviewStructured(config, "plan-review-decision", payload)
	emitReviewStructured(config, "plan-review", payload)

	log.Infof("dynamic planning: plan review AI decision: %s (reason: %s)", suggestion, reason)
	return aitool.InvokeParams{"suggestion": suggestion, "reason": compactReason}, nil
}

// DefaultAITaskReviewControl calls AI to review the completed task and decide
// whether to continue, request deeper analysis, mark as inaccurate, or adjust the plan.
// When suggestion is "adjust_plan", the AI may also output task_deltas for incremental modifications.
func DefaultAITaskReviewControl(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
	materials := ep.GetReviewMaterials()

	prompt, err := generateTaskReviewPrompt(config, materials)
	if err != nil {
		return nil, fmt.Errorf("generate task review prompt failed: %w", err)
	}

	var suggestion string
	var reason string
	var taskDeltaSummary string
	var taskDeltasArray []aitool.InvokeParams
	var rawResponse bytes.Buffer

	_, _ = emitReviewStatus(config, "task-review-status", "正在审查任务，以便任务动态规划 / start to do task/plan review for dynamic plan", ep.GetId())

	err = CallAITransaction(config, prompt, config.CallQualityPriorityAI, func(rsp *AIResponse) error {
		boundEmitter := rsp.BindEmitter(config.GetEmitter())
		stream := rsp.GetOutputStreamReader("task-review", true, config.GetEmitter())
		stream = io.TeeReader(stream, &rawResponse)
		actionOpts := []ActionMakerOption{
			WithActionAlias("object"),
		}
		actionOpts = append(actionOpts, reviewFieldStreamOptions(boundEmitter, rsp.GetTaskIndex(),
			reviewFieldStreamSpec{FieldKey: "task_delta_summary", NodeID: "task-review-adjustment"},
		)...)
		action, err := ExtractActionFromStream(ctx, stream, "task_review", actionOpts...)
		if err != nil {
			return fmt.Errorf("extract task_review action failed: %w", err)
		}
		suggestion = action.GetString("suggestion")
		reason = action.GetString("reason")
		taskDeltaSummary = action.GetString("task_delta_summary")
		taskDeltasArray = action.GetInvokeParamsArray("task_deltas")
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("task review AI transaction failed: %w", err)
	}

	if !validTaskSuggestions[suggestion] {
		log.Warnf("dynamic planning: AI returned invalid task suggestion %q, defaulting to continue", suggestion)
		suggestion = "continue"
	}

	compactReason := compactTaskReviewReason(suggestion, reason)
	compactDeltaSummary := compactTaskDeltaSummary(taskDeltaSummary, taskDeltasArray)
	if config.GetEmitter() != nil {
		reviewEvent, _ := config.GetEmitter().EmitDefaultStreamEvent(
			"task-review",
			strings.NewReader(taskReviewDisplayMessage(suggestion, compactReason, compactDeltaSummary)),
			ep.GetId(),
		)
		if compactDeltaSummary != "" && strings.TrimSpace(normalizeReviewFieldText(taskDeltaSummary)) == "" {
			_, _ = config.GetEmitter().EmitDefaultStreamEvent(
				"task-review-adjustment",
				strings.NewReader(compactDeltaSummary),
				ep.GetId(),
			)
		}
		waitForReviewStreams(config)
		if reviewEvent != nil {
			EmitAIRequestAndResponseReferenceMaterials(config.GetEmitter(), reviewEvent.GetStreamEventWriterId(), prompt, rawResponse.String())
		}
		payload := map[string]any{
			"verdict":    taskReviewVerdict(suggestion),
			"suggestion": suggestion,
			"reason":     compactReason,
		}
		if compactDeltaSummary != "" {
			payload["task_delta_summary"] = compactDeltaSummary
		}
		if len(taskDeltasArray) > 0 {
			var rawDeltas []interface{}
			for _, d := range taskDeltasArray {
				rawDeltas = append(rawDeltas, map[string]interface{}(d))
			}
			payload["task_deltas"] = rawDeltas
		}
		emitReviewStructured(config, "task-review-decision", payload)
		_, _ = config.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "task-review", payload)
	}

	log.Infof("dynamic planning: task review AI decision: %s (reason: %s)", suggestion, reason)
	result := aitool.InvokeParams{"suggestion": suggestion, "reason": compactReason}
	if compactDeltaSummary != "" {
		result["task_delta_summary"] = compactDeltaSummary
	}
	if len(taskDeltasArray) > 0 {
		var rawDeltas []interface{}
		for _, d := range taskDeltasArray {
			rawDeltas = append(rawDeltas, map[string]interface{}(d))
		}
		result["task_deltas"] = rawDeltas
	}
	return result, nil
}

func planReviewVerdict(suggestion string) string {
	switch suggestion {
	case "continue":
		return "计划合理，继续执行"
	case "unclear":
		return "计划目标不清晰"
	case "incomplete":
		return "计划缺少关键步骤"
	case "create-subtask":
		return "计划需要进一步拆分"
	default:
		return "计划审查完成"
	}
}

func compactPlanReviewReason(suggestion, reason string) string {
	reason = strings.TrimSpace(reason)
	if suggestion == "continue" {
		return "继续执行"
	}
	if reason == "" {
		switch suggestion {
		case "unclear":
			return "目标还不够清晰"
		case "incomplete":
			return "缺少关键执行步骤"
		case "create-subtask":
			return "当前任务粒度过粗"
		default:
			return "计划需要进一步审查"
		}
	}
	return reason
}

func planReviewDisplayMessage(suggestion, reason string) string {
	switch suggestion {
	case "continue":
		return "继续执行"
	case "unclear":
		return "目标不够清晰"
	case "incomplete":
		if reason != "" {
			return reason
		}
		return "缺少关键步骤"
	case "create-subtask":
		if reason != "" {
			return reason
		}
		return "建议拆分子任务"
	default:
		if reason != "" {
			return reason
		}
		return "计划审查完成"
	}
}

func taskReviewVerdict(suggestion string) string {
	if suggestion == "adjust_plan" {
		return "需要修改后续任务"
	}
	return "无需修改后续任务"
}

func compactTaskReviewReason(suggestion, reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		if suggestion == "adjust_plan" {
			reason = "发现新证据，后续任务需调整"
		} else {
			reason = "当前结果支持继续按原计划执行"
		}
	}
	return fmt.Sprintf("%s：%s", taskReviewVerdict(suggestion), reason)
}

func taskReviewDisplayMessage(suggestion, reason, deltaSummary string) string {
	switch suggestion {
	case "continue":
		return "任务继续"
	case "deeply_think":
		return "需要深入分析"
	case "inaccurate":
		return "结果待重查"
	case "adjust_plan":
		if deltaSummary != "" {
			return "需要调整后续任务 / need to adjust subsequent tasks: " + deltaSummary
		}
		if reason != "" {
			return reason
		}
		return "需要调整后续任务 / need to adjust subsequent tasks"
	default:
		if reason != "" {
			return reason
		}
		return "任务审查完成"
	}
}

func compactTaskDeltaSummary(summary string, taskDeltasArray []aitool.InvokeParams) string {
	summary = normalizeReviewFieldText(summary)
	summary = strings.TrimSpace(summary)
	if summary != "" {
		return summary
	}
	return buildTaskDeltaSummary(taskDeltasArray)
}

func buildTaskDeltaSummary(taskDeltasArray []aitool.InvokeParams) string {
	if len(taskDeltasArray) == 0 {
		return ""
	}
	lines := make([]string, 0, len(taskDeltasArray))
	for _, delta := range taskDeltasArray {
		line := summarizeTaskDelta(delta)
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func summarizeTaskDelta(delta aitool.InvokeParams) string {
	op := delta.GetString("op")
	ref := delta.GetString("ref_task_index")
	tasks := summarizeTaskDeltaTasks(delta.GetObjectArray("tasks"))
	updatedName := delta.GetString("updated_name")
	updatedGoal := delta.GetString("updated_goal")

	switch op {
	case "insert_after":
		if ref != "" && tasks != "" {
			return fmt.Sprintf("在 %s 后新增：%s", ref, tasks)
		}
	case "append":
		if tasks != "" {
			return fmt.Sprintf("追加任务：%s", tasks)
		}
	case "remove":
		if ref != "" {
			return fmt.Sprintf("移除任务：%s", ref)
		}
	case "modify":
		parts := make([]string, 0, 2)
		if updatedName != "" {
			parts = append(parts, fmt.Sprintf("名称改为“%s”", updatedName))
		}
		if updatedGoal != "" {
			parts = append(parts, fmt.Sprintf("目标改为“%s”", updatedGoal))
		}
		if ref != "" && len(parts) > 0 {
			return fmt.Sprintf("调整任务 %s：%s", ref, strings.Join(parts, "，"))
		}
		if ref != "" {
			return fmt.Sprintf("调整任务：%s", ref)
		}
	case "replace_all":
		if tasks != "" {
			return fmt.Sprintf("替换剩余任务：%s", tasks)
		}
	}
	if ref != "" {
		return fmt.Sprintf("调整任务：%s", ref)
	}
	if tasks != "" {
		return fmt.Sprintf("调整任务：%s", tasks)
	}
	return ""
}

func summarizeTaskDeltaTasks(tasks []aitool.InvokeParams) string {
	if len(tasks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tasks))
	for _, task := range tasks {
		name := strings.TrimSpace(task.GetString("subtask_name"))
		goal := strings.TrimSpace(task.GetString("subtask_goal"))
		switch {
		case name != "":
			parts = append(parts, name)
		case goal != "":
			parts = append(parts, goal)
		default:
			parts = append(parts, "新任务")
		}
	}
	return strings.Join(parts, "、")
}
