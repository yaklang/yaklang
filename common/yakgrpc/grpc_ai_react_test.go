package yakgrpc

import (
	"context"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/utils"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestConvertYPBAIStartParamsToReActConfig(t *testing.T) {
	rand.Seed(time.Now().UnixNano())

	// bool random
	disallowRequire := rand.Intn(2) == 1
	disableToolUse := rand.Intn(2) == 1

	// make sure risk > 0
	risk := utils.RandFloat64()
	maxIter := rand.Intn(10) + 1                 // >0
	timelineLimit := int32(rand.Intn(2000) + 1)  // >0
	userInteractLimit := int32(rand.Intn(5) + 1) // >0

	includeKeywords := []string{uuid.New().String(), uuid.New().String()}
	aiService := uuid.NewString()
	presetPrompt := uuid.New().String()
	disableToolIntervalReview := rand.Intn(2) == 1
	temperature := utils.RandFloat64()
	topP := utils.RandFloat64()
	topK := int64(rand.Intn(100) + 1)
	maxTokens := int64(rand.Intn(4096) + 1)
	presencePenalty := utils.RandFloat64()
	frequencyPenalty := utils.RandFloat64()

	start := &ypb.AIStartParams{
		DisallowRequireForUserPrompt: disallowRequire,
		ReviewPolicy:                 "yolo",
		AIReviewRiskControlScore:     risk,
		ReActMaxIteration:            int64(maxIter),
		TimelineContentSizeLimit:     int64(timelineLimit),
		UserInteractLimit:            int64(userInteractLimit),
		DisableToolUse:               disableToolUse,
		IncludeSuggestedToolKeywords: includeKeywords,
		AIService:                    aiService,
		DisableAISearchForge:         true, // just to test that this field is ignored, not start embedding server in ci test
		AICallTokenLimit:             100 * 1024,
		UserPresetPrompt:             presetPrompt,
		DisableToolIntervalReview:    disableToolIntervalReview,
		Temperature:                  proto.Float64(temperature),
		TopP:                         proto.Float64(topP),
		TopK:                         proto.Int64(topK),
		MaxTokens:                    proto.Int64(maxTokens),
		PresencePenalty:              proto.Float64(presencePenalty),
		FrequencyPenalty:             proto.Float64(frequencyPenalty),
	}

	opts := ConvertYPBAIStartParamsToReActConfig(start)
	require.NotEmpty(t, opts)

	cfg := aicommon.NewConfig(context.Background(), append(opts,
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return &aicommon.AIResponse{}, nil
		}))...)

	// Assertions for core mappings
	expectedAllow := !start.DisallowRequireForUserPrompt
	require.Equal(t, expectedAllow, cfg.AllowRequireForUserInteract)
	require.Equal(t, aicommon.AgreePolicyYOLO, cfg.AgreePolicy)
	require.Equal(t, start.AIReviewRiskControlScore, cfg.AgreeAIScoreMiddle)
	require.Equal(t, int64(start.ReActMaxIteration), cfg.MaxIterationCount)
	require.Equal(t, int(start.TimelineContentSizeLimit), cfg.TimelineContentSizeLimit)
	require.Equal(t, int64(start.UserInteractLimit), cfg.PlanUserInteractMaxCount)
	require.Equal(t, start.DisableToolUse, cfg.DisableToolUse)
	require.ElementsMatch(t, start.IncludeSuggestedToolKeywords, cfg.Keywords)
	require.Equal(t, start.AICallTokenLimit, cfg.AiCallTokenLimit)
	require.Equal(t, start.UserPresetPrompt, cfg.UserPresetPrompt)
	require.Equal(t, start.DisableToolIntervalReview, cfg.DisableIntervalReview)
	require.NotNil(t, cfg.Temperature)
	require.Equal(t, temperature, *cfg.Temperature)
	require.NotNil(t, cfg.TopP)
	require.Equal(t, topP, *cfg.TopP)
	require.NotNil(t, cfg.TopK)
	require.Equal(t, topK, *cfg.TopK)
	require.NotNil(t, cfg.MaxTokens)
	require.Equal(t, maxTokens, *cfg.MaxTokens)
	require.NotNil(t, cfg.PresencePenalty)
	require.Equal(t, presencePenalty, *cfg.PresencePenalty)
	require.NotNil(t, cfg.FrequencyPenalty)
	require.Equal(t, frequencyPenalty, *cfg.FrequencyPenalty)
	// AiServerName is no longer set from frontend params (WithAIChatInfo deprecated),
	// it is now auto-detected via ModelInfoCallback during actual AI gateway calls.
}
