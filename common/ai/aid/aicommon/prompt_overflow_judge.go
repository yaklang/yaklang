package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

const promptOverflowJudgeTimeout = 15 * time.Second

type promptOverflowJudgeState struct {
	mu        sync.Mutex
	depth     int
	decisions map[string]bool
}

func (s *promptOverflowJudgeState) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.depth > 0 {
		return false
	}
	s.depth++
	return true
}

func (s *promptOverflowJudgeState) end() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.depth > 0 {
		s.depth--
	}
}

func (s *promptOverflowJudgeState) lookup(key string) (bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decision, ok := s.decisions[key]
	return decision, ok
}

func (s *promptOverflowJudgeState) store(key string, overflow bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.decisions == nil {
		s.decisions = map[string]bool{}
	}
	s.decisions[key] = overflow
}

type ambiguousPromptOverflowJudge interface {
	JudgeAmbiguousPromptOverflow(context.Context, aispec.PromptOverflowAmbiguity) (bool, error)
}

// JudgeAmbiguousPromptOverflow asks the Speed model once. The prompt contains
// only the provider error fields. A re-entrant call from that judgment returns
// false without making another request. Failures are not cached.
func (c *Config) JudgeAmbiguousPromptOverflow(ctx context.Context, sample aispec.PromptOverflowAmbiguity) (bool, error) {
	if c == nil {
		return false, nil
	}
	if !c.ResolveAuxiliaryTask(CallerLabelPromptOverflowClassify).ShouldRun() {
		return false, nil
	}
	if ctx == nil {
		ctx = c.GetContext()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	key := promptOverflowJudgeKey(sample)
	if decision, ok := c.promptOverflowJudge.lookup(key); ok {
		return decision, nil
	}
	if !c.promptOverflowJudge.begin() {
		return false, nil
	}
	defer c.promptOverflowJudge.end()

	judgeCtx, cancel := context.WithTimeout(ctx, promptOverflowJudgeTimeout)
	defer cancel()
	result, err := c.invokeSpeedPriorityLiteForge(promptOverflowJudgePrompt(sample),
		&LiteForgeInvokeRequest{
			Context:    judgeCtx,
			ActionName: CallerLabelPromptOverflowClassify,
			Outputs: []aitool.ToolOption{
				aitool.WithBoolParam("is_prompt_overflow",
					aitool.WithParam_Description("true only when the provider rejected the request because its input exceeded the model context window"),
					aitool.WithParam_Required(true),
				),
			},
			Emitter: c.Emitter,
		},
		WithAgreeYOLO(),
		WithAITransactionAutoRetry(1),
		WithPersistentSessionId(c.PersistentSessionId),
	)
	if err != nil {
		return false, err
	}
	if result == nil || result.Action == nil {
		return false, utils.Error("prompt overflow judge returned no action")
	}
	if _, ok := result.Action.LookupParam("is_prompt_overflow"); !ok {
		return false, utils.Error("prompt overflow judge omitted is_prompt_overflow")
	}
	decision := result.Action.GetBool("is_prompt_overflow")
	c.promptOverflowJudge.store(key, decision)
	return decision, nil
}

func promptOverflowJudgeKey(sample aispec.PromptOverflowAmbiguity) string {
	return fmt.Sprintf("%d\n%s\n%s\n%s", sample.HTTPStatus, sample.Code, sample.Type, sample.Message)
}

func promptOverflowJudgePrompt(sample aispec.PromptOverflowAmbiguity) string {
	return fmt.Sprintf(`Classify this provider error. Set is_prompt_overflow true only when the provider rejected the request because the input prompt exceeded the model context window.
Set it false for rate limits, quota, authentication, safety filtering, invalid parameters, server failures, and any message that does not explicitly say the input or prompt is too long.
Do not infer overflow merely because the message is long. Use only the fields below.
HTTP status: %d
Code: %s
Type: %s
Message: %s`, sample.HTTPStatus, sample.Code, sample.Type, sample.Message)
}

func classifyTransactionPromptOverflow(ctx context.Context, c AICallerConfigIf, rsp *AIResponse, err error) error {
	if overflowErr := ClassifyPromptTooLong(rsp, err); overflowErr != nil {
		return overflowErr
	}
	if c == nil || rsp == nil {
		return nil
	}
	sample := aispec.DetectPromptOverflowAmbiguity(rsp.GetHTTPStatusCode(), rsp.GetHTTPResponseBody(), "")
	if sample == nil {
		return nil
	}
	judge, ok := c.(ambiguousPromptOverflowJudge)
	if !ok {
		return nil
	}
	overflow, judgeErr := judge.JudgeAmbiguousPromptOverflow(ctx, *sample)
	if judgeErr != nil || !overflow {
		return nil
	}
	cause := err
	if cause == nil {
		cause = rsp.GetError()
	}
	if cause == nil {
		cause = utils.Errorf("AI provider returned HTTP %d: %s", sample.HTTPStatus, sample.Message)
	}
	return &aispec.PromptOverflowError{
		PromptOverflowEvidence: aispec.PromptOverflowEvidence{
			HTTPStatus:     sample.HTTPStatus,
			Provider:       strings.TrimSpace(rsp.GetProviderName()),
			Model:          strings.TrimSpace(rsp.GetModelName()),
			Code:           sample.Code,
			Type:           sample.Type,
			Message:        sample.Message,
			MatchedPattern: "auxiliary_judge",
			Confidence:     aispec.PromptOverflowHigh,
			Source:         aispec.PromptOverflowSourceAuxiliary,
			Raw:            sample.Raw,
		},
		Cause: cause,
	}
}
