package aicommon

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"io"
	"runtime"
	"text/template"
	"time"
)

func (c *Config) DoWaitAgreeWithPolicy(ctx context.Context, policy AgreePolicyType, endpoint *Endpoint) {
	if utils.IsNil(ctx) {
		ctx = c.Ctx
		if ctx == nil {
			ctx = context.Background()
		}
	}

	skipAIReview := utils.GetContextKeyBool(ctx, "skip_ai_review")

	interactiveId := endpoint.GetId()

	switch policy {
	case AgreePolicyYOLO, AgreePolicyAuto:
		c.Emitter.EmitInfo("yolo policy auto agree all")
		log.Infof("Auto-approving tool usage (non-interactive mode)")
		// Set default continue response
		endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
		endpoint.Release()
		return
	case AgreePolicyAI, AgreePolicyAIAuto:
		if skipAIReview {
			// Default behavior: wait for user interaction
			endpoint.Wait()
			return
		}
		go func() {
			go func() {
				c.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_START, "ai-reviewer", map[string]any{
					"interactive_id": interactiveId,
				})
				endOnce := utils.NewOnce()
				endNormally := func(score float64, level string, reason string) {
					endOnce.Do(func() {
						c.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_END, "ai-reviewer", map[string]any{
							"score":          score,
							"reason":         reason,
							"interactive_id": interactiveId,
							"level":          level,
						})
					})
				}
				defer func() {
					endNormally(1.0, "high", "review interrupted")
				}()

				riskResult, err := c.AiAgreeRiskControl(ctx, c, endpoint)
				if err != nil {
					endNormally(1, "high", "review failed: "+err.Error())
					log.Errorf("error during auto-review: %v", err)
					return
				}
				score := riskResult.GetFloat("risk_score")
				if score <= c.AgreeAIScoreLow {
					var duSec time.Duration = 1
					c.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN, "ai-reviewer", map[string]any{
						"seconds":        int(duSec),
						"interactive_id": interactiveId,
						"score":          score,
						"level":          "low",
					})
					c.Emitter.EmitInfo("Auto-review score is low, suggesting to continue in " + fmt.Sprint(int(duSec)) + " seconds...")
					// reason := action.WaitString("reason")
					endNormally(score, "low", "")
					time.Sleep(duSec * time.Second) // Simulate a delay for user to read the message
					endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
					endpoint.Release()
				} else if score > c.AgreeAIScoreLow && score <= c.AgreeAIScoreMiddle {
					var duSec time.Duration = 6
					c.Emitter.EmitJSON(schema.EVENT_TYPE_AI_REVIEW_COUNTDOWN, "ai-reviewer", map[string]any{
						"seconds":        int(duSec),
						"interactive_id": interactiveId,
						"score":          score,
						"level":          "middle",
					})
					// reason := action.WaitString("reason")
					endNormally(score, "middle", "")
					c.Emitter.EmitInfo("Auto-review score is middle, suggesting to continue in " + fmt.Sprint(int(duSec)) + " seconds...")
					time.Sleep(duSec * time.Second) // Simulate a delay for user to read the message
					endpoint.SetParams(aitool.InvokeParams{"suggestion": "continue"})
					endpoint.Release()
				} else {
					c.Emitter.EmitInfo("Auto-review score is high, suggesting to handled by user")
					reason := riskResult.GetString("reason")
					endNormally(score, "high", reason)
				}
			}()
		}()
		endpoint.Wait()
	case AgreePolicyManual:
		fallthrough
	default:
		manualCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		if c.AgreeManualCallback != nil { // if agreeManualCallback is not nil, use it help manual agree
			go func() {
				res, err := c.AgreeManualCallback(manualCtx, c)
				if err != nil {
					log.Errorf("agree assistant callback error: %v", err)
				} else {
					endpoint.SetParams(res)
					for i := 0; i < 3; i++ {
						endpoint.Release()
						time.Sleep(time.Second)
					}
				}
			}()
		}
		endpoint.WaitContext(ctx)
	}
}

func (c *Config) DoWaitAgree(ctx context.Context, endpoint *Endpoint) {
	c.DoWaitAgreeWithPolicy(ctx, c.AgreePolicy, endpoint)
}

type RiskControl func(ctx context.Context, config *Config, ep *Endpoint) (*Action, error)

//go:embed prompts/review/ai-review-tool-call.txt
var aiReviewPromptTemplate string

// AIReviewPromptData contains data for AI tool call review prompt
type AIReviewPromptData struct {
	CurrentTime        string
	OSArch             string
	WorkingDir         string
	WorkingDirGlance   string
	ConversationMemory string
	Timeline           string
	Nonce              string
	UserQuery          string
	Title              string
	Details            string
	Language           string
}

func GenerateAIReviewPrompt(config *Config, userQuery, toolOrTitle, params string) (string, error) {
	data := &AIReviewPromptData{
		CurrentTime: time.Now().Format("2006-01-02 15:04:05"),
		OSArch:      fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		UserQuery:   userQuery,
		Title:       toolOrTitle,
		Details:     params,
		Nonce:       utils.RandStringBytes(4),
		Language:    config.Language,
	}

	// Set working directory
	data.WorkingDir = config.Workdir
	if data.WorkingDir != "" {
		data.WorkingDirGlance = filesys.Glance(data.WorkingDir)
	}

	if t := config.GetTimeline(); t != nil {
		data.Timeline = t.Dump()
	}
	name := "ai-review"
	tmpl, err := template.New(name).Parse(aiReviewPromptTemplate)
	if err != nil {
		return "", fmt.Errorf("error parsing %s template: %w", name, err)
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("error executing %s template: %w", name, err)
	}

	return buf.String(), nil
}

func DefaultAIAssistantRiskControl(ctx context.Context, config *Config, ep *Endpoint) (*Action, error) {
	// In auto-review mode, automatically approve the request
	materials := ep.GetReviewMaterials()
	params := materials.GetObject("params")
	title := materials.GetString("tool")
	if title != "" {
		if desc := materials.GetString("tool_description"); desc != "" {
			title = fmt.Sprintf("[%s]:%s", title, desc)
		}
	} else {
		if materials.Has("selectors") {
			delete(materials, "selectors")
		}
		params = materials
	}

	prompt, err := GenerateAIReviewPrompt(
		config,
		"ai request tool-call, start to review",
		title,
		params.Dump(),
	)
	if err != nil {
		log.Errorf("error generating AI review prompt: %v", err)
		return nil, err
	}
	var score float64
	var action *Action
	err = CallAITransaction(config, prompt, config.CallAI, func(rsp *AIResponse) error {
		stream := rsp.GetOutputStreamReader("review", true, config.GetEmitter())
		// stream = io.TeeReader(stream, os.Stdout)
		var err error
		action, err = ExtractActionFromStream(
			ctx,
			stream, "risk_assessment",
			WithActionAlias("object"),
			WithActionFieldStreamHandler([]string{"reason"}, func(key string, reader io.Reader) {
				reader = utils.JSONStringReader(utils.UTF8Reader(reader))
				config.GetEmitter().EmitDefaultStreamEvent(
					"review",
					reader,
					rsp.GetTaskIndex(),
				)
			}),
		)
		if err != nil {
			return utils.Errorf("error extracting action from stream: %v", err)
		}
		score = action.GetFloat("risk_score")
		if score < 0 {
			score = 0.0
		}
		if score > 1 {
			score = 1.0
		}
		return nil
	})
	return action, err
}
