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
	require.Equal(t, start.AIService, cfg.AiServerName)
}
