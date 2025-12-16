package yakgrpc

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/yaklang/yaklang/common/ai/rag"

	"github.com/yaklang/yaklang/common/ai"
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
		aiConfig, err := ai.LoadAiGatewayConfig(serviceName)
		if err != nil {
			log.Errorf("ai service %s not found", serviceName)
		} else {
			chat, err := ai.LoadChater(i.GetAIService())
			if err != nil {
				log.Errorf("load ai service failed: %v", err)
			} else {
				opts = append(opts, aicommon.WithAICallback(aicommon.AIChatToAICallbackType(chat)))
				opts = append(opts, aicommon.WithAIChatInfo(serviceName, aiConfig.Model))
			}
		}
	}

	if !i.GetDisableAISearchForge() {
		opts = append(opts, aid.WithAiForgeSearchTool())
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
	var configOptions = []aicommon.ConfigOption{
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			feedback(e)
		}),
		aicommon.WithEventInputChanx(inputEvent),
		aicommon.WithContext(baseCtx),
		aireact.WithBuiltinTools(),
		aicommon.WithAICallback(aicommon.AIChatToAICallbackType(ai.Chat)),
		aicommon.WithEnhanceKnowledgeManager(rag.NewRagEnhanceKnowledgeManager()),
		aicommon.WithPersistentSessionId(persistentSession),
		aicommon.WithEnableSelfReflection(true),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithEnablePETaskAnalyze(true),
	}
	configOptions = append(configOptions, optsFromStartParams...)

	reAct, err := aireact.NewReAct(configOptions...)
	if err != nil {
		log.Errorf("create re-act failed: %v", err)
		return utils.Errorf("create re-act instance failed: %v", err)
	}
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
