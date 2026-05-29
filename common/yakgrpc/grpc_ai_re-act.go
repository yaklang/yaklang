package yakgrpc

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/utils/chanx"

	"github.com/yaklang/yaklang/common/ai/rag"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/reactloops_yak"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func fixOptionsWithServiceName(serviceName string, opts ...aicommon.ConfigOption) []aicommon.ConfigOption {
	aiCb, err := aicommon.CreateCallbackFromConfig(aiconfig.GetGlobalManager().GetFirstConfigByTierAndProviderAndModel(consts.TierIntelligent, serviceName, ""))
	if err != nil {
		log.Errorf("load ai service failed: %v", err)
	} else {
		opts = append(opts, aicommon.WithAutoTieredAICallback(aiCb))
	}
	log.Warnf("AIStartParams.AIService/AIModelName for WithAIChatInfo is deprecated, " +
		"model info is now auto-detected from the actual AI gateway call")
	return opts
}

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
		opts = fixOptionsWithServiceName(i.GetAIService(), opts...)
	}

	if !i.GetDisableAISearchForge() {
		opts = append(opts, aid.WithAiForgeSearchTool())
	}

	// EnablePlan 晚于 DisableAISearchForge 应用，用于控制 PE / 蓝图动作；与 AI 搜索 Forge 工具独立。
	opts = append(opts, aicommon.WithEnablePlanAndExec(i.GetEnablePlan()))

	if i.GetAICallTokenLimit() > 0 {
		opts = append(opts, aicommon.WithAiCallTokenLimit(int64(i.GetAICallTokenLimit())))
	}

	if i.GetDisableToolIntervalReview() {
		opts = append(opts, aicommon.WithDisableToolCallerIntervalReview(true))
	}
	if i.GetSyncPerceptionTrigger() {
		opts = append(opts, aicommon.WithSyncPerceptionTrigger(true))
	}

	if i.GetUserPresetPrompt() != "" {
		opts = append(opts, aicommon.WithUserPresetPrompt(i.GetUserPresetPrompt()))
	}

	if i.GetPlanExecTaskConcurrency() > 0 {
		opts = append(opts, aicommon.WithPlanExecTaskConcurrency(int(i.GetPlanExecTaskConcurrency())))
	}

	if i.GetUserPlanPrompt() != "" {
		opts = append(opts, aicommon.WithPlanPrompt(i.GetUserPlanPrompt()))
	}

	if i.GetSource() != "" {
		opts = append(opts, aicommon.WithSessionSource(i.GetSource()))
	}

	if caps := aicommon.ParseEnabledCapabilitiesFromProto(i); len(caps) > 0 {
		opts = append(opts, aicommon.WithEnabledCapabilities(caps...))
	}

	return opts
}

func resolveAISessionStartParams(db *gorm.DB, sessionID string, request *ypb.AIStartParams, preferCached bool) (*ypb.AIStartParams, error) {
	if request == nil {
		request = &ypb.AIStartParams{}
	}
	if !preferCached || db == nil || strings.TrimSpace(sessionID) == "" {
		return request, nil
	}

	if _, err := yakit.GetAISessionMetaBySessionID(db, sessionID); err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return request, nil
		}
		return nil, err
	}

	cached, err := yakit.GetAISessionMetaStartParamsBySessionID(db, sessionID)
	if err != nil {
		return nil, err
	}
	if cached == nil {
		return request, nil
	}
	return yakit.MergeCachedAISessionStartParams(cached, request), nil
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

	// 启动 ReAct 之前懒扫描用户的 ~/yakit-projects/ai-focus/，
	// 防止客户端跳过 QueryAIFocus 直接发带 FocusModeLoop 的 free input 时
	// 找不到注册项。冷却由 EnsureUserFocusModesLoaded 内部控制，失败只 log。
	// 关键词: start ai re-act ensure user focus modes
	if err := reactloops_yak.EnsureUserFocusModesLoaded(); err != nil {
		log.Warnf("ensure user yak focus modes failed: %v", err)
	}

	startParams := firstMsg.Params

	baseCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	inputEvent := chanx.NewUnlimitedChan[*ypb.AIInputEvent](baseCtx, 10)

	var currentCoordinatorId = startParams.CoordinatorId
	_ = currentCoordinatorId
	var coordinatorIdOnce = new(sync.Once)
	var sendMu sync.Mutex
	// debugStreamPrinter 在 DEBUG=1 时把流式 delta 合并到单行，避免每个
	// token 单独换行造成的刷屏；非流事件来临时先 FlushIfActive 收尾，让
	// 后续 log / 普通事件都从新行开始，消除"夹心"现象。
	// 关键词: DEBUG=1 流式输出体验, AI stream delta debug print
	debugStreamPrinter := aicommon.GetDefaultDebugStreamPrinter()
	// 同步把 common/log 默认输出包装上一层 flush, 让任何日志写入前先把
	// 流缓冲刷出, 彻底消灭日志被夹在流中间的视觉混乱。
	// 关键词: EnsureLogFlushWrapperInstalled grpc_ai_react entry
	aicommon.EnsureLogFlushWrapperInstalled()

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
			if e.IsStream {
				debugStreamPrinter.PrintStreamDelta(e)
			} else {
				debugStreamPrinter.FlushIfActive()
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

	if runningReAct, ok := aireact.GetRunningSession(persistentSession); ok {
		return s.attachToRunningAIReActSession(stream, baseCtx, runningReAct, firstMsg, persistentSession, startParams)
	}

	resolvedStartParams, err := resolveAISessionStartParams(
		s.GetProjectDatabase(),
		persistentSession,
		startParams,
		startParams.GetPreferSessionCachedConfig(),
	)
	if err != nil {
		return utils.Errorf("resolve session cached config failed: %v", err)
	}
	startParams = resolvedStartParams
	firstMsg.Params = resolvedStartParams

	if _, err := yakit.CreateOrUpdateAISessionMetaOnStart(s.GetProjectDatabase(), persistentSession, startParams, time.Now()); err != nil {
		log.Warnf("persist ai session start meta failed for %s: %v", persistentSession, err)
	}

	optsFromStartParams := ConvertYPBAIStartParamsToReActConfig(startParams)
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

func (s *Server) attachToRunningAIReActSession(
	stream ypb.Yak_StartAIReActServer,
	baseCtx context.Context,
	runningReAct *aireact.ReAct,
	firstMsg *ypb.AIInputEvent,
	persistentSession string,
	startParams *ypb.AIStartParams,
) error {
	log.Infof("attach grpc stream to running aireact session: %s", persistentSession)

	if _, err := yakit.CreateOrUpdateAISessionMetaOnStart(s.GetProjectDatabase(), persistentSession, startParams, time.Now()); err != nil {
		log.Warnf("persist ai session start meta failed for %s: %v", persistentSession, err)
	}

	var sendMu sync.Mutex
	debugStreamPrinter := aicommon.GetDefaultDebugStreamPrinter()
	aicommon.EnsureLogFlushWrapperInstalled()

	feedback := func(e *schema.AiOutputEvent) {
		if e.Timestamp <= 0 {
			e.Timestamp = time.Now().Unix()
		}

		utils.Debug(func() {
			if e.IsStream {
				debugStreamPrinter.PrintStreamDelta(e)
			} else {
				debugStreamPrinter.FlushIfActive()
			}
		})

		if stream.Context().Err() != nil {
			return
		}
		sendMu.Lock()
		defer sendMu.Unlock()
		if err := stream.Send(e.ToGRPC()); err != nil {
			log.Errorf("send re-act event to attached stream failed: %v", err)
		}
	}

	unsubscribe, ok := aireact.SubscribeRunningSession(persistentSession, feedback)
	if !ok {
		return utils.Errorf("failed to subscribe running aireact session: %s", persistentSession)
	}
	defer unsubscribe()

	if firstMsg != nil && !firstMsg.GetIsStart() {
		if err := runningReAct.SendInputEvent(firstMsg); err != nil {
			log.Warnf("forward first input to running session failed: %v", err)
		}
	}

	for {
		select {
		case <-baseCtx.Done():
			log.Info("attached AIReAct stream context done")
			return nil
		default:
		}

		event, err := stream.Recv()
		if err != nil {
			log.Infof("attached AIReAct stream recv ended: %v", err)
			return nil
		}
		if event.GetIsStart() {
			continue
		}
		if err := runningReAct.SendInputEvent(event); err != nil {
			log.Warnf("forward input to running session failed: %v", err)
		}
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
