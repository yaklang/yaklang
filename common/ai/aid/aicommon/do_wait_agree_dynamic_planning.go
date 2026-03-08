package aicommon

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
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

		materialsCopy := make(aitool.InvokeParams)
		for k, v := range materials {
			if k == "selectors" || k == "id" || k == "short_summary" || k == "long_summary" {
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

	err = CallAITransaction(config, prompt, config.CallQualityPriorityAI, func(rsp *AIResponse) error {
		stream := rsp.GetOutputStreamReader("plan-review", true, config.GetEmitter())
		action, err := ExtractActionFromStream(
			ctx,
			stream, "plan_review",
			WithActionAlias("object"),
			WithActionFieldStreamHandler([]string{"reason"}, func(key string, reader io.Reader) {
				reader = utils.JSONStringReader(utils.UTF8Reader(reader))
				config.GetEmitter().EmitDefaultStreamEvent(
					"plan-review",
					reader,
					rsp.GetTaskIndex(),
				)
			}),
		)
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

	log.Infof("dynamic planning: plan review AI decision: %s (reason: %s)", suggestion, reason)
	return aitool.InvokeParams{"suggestion": suggestion, "reason": reason}, nil
}

// DefaultAITaskReviewControl calls AI to review the completed task and decide
// whether to continue, request deeper analysis, mark as inaccurate, or adjust the plan.
func DefaultAITaskReviewControl(ctx context.Context, config *Config, ep *Endpoint) (aitool.InvokeParams, error) {
	materials := ep.GetReviewMaterials()

	prompt, err := generateTaskReviewPrompt(config, materials)
	if err != nil {
		return nil, fmt.Errorf("generate task review prompt failed: %w", err)
	}

	var suggestion string
	var reason string

	err = CallAITransaction(config, prompt, config.CallQualityPriorityAI, func(rsp *AIResponse) error {
		stream := rsp.GetOutputStreamReader("task-review", true, config.GetEmitter())
		action, err := ExtractActionFromStream(
			ctx,
			stream, "task_review",
			WithActionAlias("object"),
			WithActionFieldStreamHandler([]string{"reason"}, func(key string, reader io.Reader) {
				reader = utils.JSONStringReader(utils.UTF8Reader(reader))
				config.GetEmitter().EmitDefaultStreamEvent(
					"task-review",
					reader,
					rsp.GetTaskIndex(),
				)
			}),
		)
		if err != nil {
			return fmt.Errorf("extract task_review action failed: %w", err)
		}
		suggestion = action.GetString("suggestion")
		reason = action.GetString("reason")
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("task review AI transaction failed: %w", err)
	}

	if !validTaskSuggestions[suggestion] {
		log.Warnf("dynamic planning: AI returned invalid task suggestion %q, defaulting to continue", suggestion)
		suggestion = "continue"
	}

	log.Infof("dynamic planning: task review AI decision: %s (reason: %s)", suggestion, reason)
	return aitool.InvokeParams{"suggestion": suggestion, "reason": reason}, nil
}
