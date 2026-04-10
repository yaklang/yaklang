package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/yaklang/yaklang/common/ai/rag"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func ConvertYPBAIStartParamsToReActConfig(i *ypb.AIStartParams) []aicommon.ConfigOption {
	opts := make([]aicommon.ConfigOption, 0)
	if i == nil {
		return opts
	}
	if i.DisallowRequireForUserPrompt {
		opts = append(opts, aicommon.WithAllowRequireForUserInteract(false))
	} else {
		opts = append(opts, aicommon.WithAllowRequireForUserInteract(true))
	}

	if i.ReviewPolicy != "" {
		opts = append(opts, aicommon.WithAgreePolicy(aicommon.AgreePolicyType(i.ReviewPolicy)))
	}

	if i.GetAIReviewRiskControlScore() > 0 {
		opts = append(opts, aicommon.WithAgreeAIRiskCtrlScore(i.GetAIReviewRiskControlScore()))
	}

	if i.ReActMaxIteration > 0 {
		opts = append(opts, aicommon.WithMaxIterationCount(int64(int(i.ReActMaxIteration))))
	}

	if i.GetTimelineContentSizeLimit() > 0 {
		opts = append(opts, aicommon.WithTimelineContentLimit(int(i.GetTimelineContentSizeLimit())))
	}

	if i.UserInteractLimit > 0 {
		opts = append(opts, aicommon.WithPlanUserInteractMaxCount(i.UserInteractLimit))
	}

	if i.GetDisableToolUse() {
		opts = append(opts, aicommon.WithDisableToolUse(true))
	}
	if i.GetEnableAISearchTool() {
		opts = append(opts, aid.WithAiToolsSearchTool())
	}
	if len(i.GetExcludeToolNames()) > 0 {
		opts = append(opts, aicommon.WithDisableToolsName(i.GetExcludeToolNames()...))
	}
	if len(i.GetIncludeSuggestedToolNames()) > 0 {
		opts = append(opts, aicommon.WithEnableToolsName(i.GetIncludeSuggestedToolNames()...))
	}
	if len(i.GetIncludeSuggestedToolKeywords()) > 0 {
		opts = append(opts, aicommon.WithKeywords(i.GetIncludeSuggestedToolKeywords()...))
	}
	if i.GetAIService() != "" {
		serviceName := i.GetAIService()
		aiCb, err := aicommon.CreateCallbackFromConfig(aiconfig.GetGlobalManager().GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, serviceName, ""))
		if err != nil {
			log.Errorf("load ai service failed: %v", err)
		} else {
			opts = append(opts, aicommon.WithAICallback(aiCb))
		}
		log.Warnf("AIStartParams.AIService/AIModelName for WithAIChatInfo is deprecated, " +
			"model info is now auto-detected from the actual AI gateway call")
	}

	if !i.GetDisableAISearchForge() {
		opts = append(opts, aid.WithAiForgeSearchTool())
	}

	if i.GetAICallTokenLimit() > 0 {
		opts = append(opts, aicommon.WithAiCallTokenLimit(int64(i.GetAICallTokenLimit())))
	}

	if i.GetDisableToolIntervalReview() {
		opts = append(opts, aicommon.WithDisableToolCallerIntervalReview(true))
	}

	if i.GetUserPresetPrompt() != "" {
		opts = append(opts, aicommon.WithUserPresetPrompt(i.GetUserPresetPrompt()))
	}

	return opts
}

func (s *Server) StartAIReAct(stream ypb.Yak_StartAIReActServer) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		log.Errorf("recv re-act first config msg failed: %v", err)
		return utils.Errorf("recv first mgs failed: %v", err)
	}

	if !firstMsg.IsStart {
		log.Errorf("recv re-act first config msg is invalid: %v", firstMsg)
		return utils.Error("first msg is not a start/config message, set IsStart to true")
	}

	startParams := firstMsg.Params

	baseCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	inputEvent := chanx.NewUnlimitedChan[*ypb.AIInputEvent](baseCtx, 10)

	optsFromStartParams := ConvertYPBAIStartParamsToReActConfig(startParams)

	var currentCoordinatorId = startParams.CoordinatorId
	_ = currentCoordinatorId
	var coordinatorIdOnce = new(sync.Once)
	var sendMu sync.Mutex
	feedback := func(e *schema.AiOutputEvent) {
		if e.Timestamp <= 0 {
			e.Timestamp = time.Now().Unix() // fallback
		}
		if e.CoordinatorId != "" {
			coordinatorIdOnce.Do(func() {
				currentCoordinatorId = e.CoordinatorId
			})
		}

		utils.Debug(func() {
			if res := e.ToGRPC(); res != nil {
				if res.IsStream {
					fmt.Println(string(res.GetStreamDelta()))
				}
			}
		})

		if stream.Context().Err() != nil {
			return
		}
		sendMu.Lock()
		defer sendMu.Unlock()
		err := stream.Send(e.ToGRPC())
		if err != nil {
			log.Errorf("send re-act event to stream failed: %v", err)
		}
	}

	persistentSession := startParams.GetTimelineSessionID()
	if persistentSession == "" {
		persistentSession = "default"
	}
	var hotpatchChan = chanx.NewUnlimitedChan[aicommon.ConfigOption](baseCtx, 10)

	if aiconfig.IsTieredAIConfig() {
		log.Info("tiered ai config is enabled. the old-styled ai config is override")
	}

	defaultAI, err := aicommon.GetDefaultAIModelCallback()
	if err != nil {
		defaultAI, _ = aicommon.GetDefaultAIModelCallback()
		log.Warnf("get default AI model callback failed: %v", err)
	}

	var configOptions = []aicommon.ConfigOption{
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			feedback(e)
		}),
		aicommon.WithEventInputChanx(inputEvent),
		aicommon.WithContext(baseCtx),
		aireact.WithBuiltinTools(),
		aicommon.WithEnhanceKnowledgeManager(rag.NewRagEnhanceKnowledgeManager()),
		aicommon.WithPersistentSessionId(persistentSession),
		aicommon.WithEnableSelfReflection(true),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithEnablePETaskAnalyze(true),
	}
	// optsFromStartParams (containing WithAICallback) must be applied BEFORE
	// tiered overrides, otherwise WithAICallback overwrites all three callbacks
	// (Original, Quality, Speed) to the same frontend-selected model.
	configOptions = append(configOptions, optsFromStartParams...)
	if aiconfig.IsTieredAIConfig() {
		configOptions = append(configOptions, aicommon.WithAutoTieredAICallback(defaultAI))
	}

	reAct, err := aireact.NewReAct(configOptions...)
	if err != nil {
		log.Errorf("create re-act failed: %v", err)
		return utils.Errorf("create re-act instance failed: %v", err)
	}

	reAct.GetConfig().SetConfig("MustProcessAttachedData", true)

	_ = reAct // ensure reAct is not nil
	for {
		select {
		case <-baseCtx.Done():
			log.Info("AIReAct stream context done, stopping re-act")
			return nil
		default:
			// continue processing
		}

		event, err := stream.Recv()
		if err != nil {
			log.Errorf("recv re-act msg failed: %v", err)
			continue
		}

		inputEvent.SafeFeed(event)
	}
}

func (s *Server) GetRandomAIMaterials(ctx context.Context, req *ypb.GetRandomAIMaterialsRequest) (*ypb.GetRandomAIMaterialsResponse, error) {
	limit := 3
	if req.GetLimit() > 0 {
		limit = int(req.GetLimit())
	}

	tools, kbes, forges, err := yakit.GetRandomAIMaterials(s.GetProfileDatabase(), limit)
	if err != nil {
		return nil, err
	}
	return &ypb.GetRandomAIMaterialsResponse{
		AITools: lo.Map(tools, func(item *schema.AIYakTool, _ int) *ypb.AITool {
			return item.ToGRPC()
		}),
		KnowledgeBaseEntries: lo.Map(kbes, func(item *schema.KnowledgeBaseEntry, _ int) *ypb.KnowledgeBaseEntry {
			return KnowledgeBaseEntryToGrpcModel(item)
		}),
		AIForges: lo.Map(forges, func(item *schema.AIForge, _ int) *ypb.AIForge {
			return item.ToGRPC()
		}),
	}, nil
}
