package yakgrpc

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiconfig"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
	enableMultiAgent, goalModeEnabled, goalMinIterations, maxSubAgents := resolveAIExecutionStrategy(i)
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
		maxIterations := i.GetReActMaxIteration()
		// Goal-mode entry-point normalization: raise a too-small ceiling so the
		// finish gate (GoalMinIterations) can open before iterations run out.
		// loop_default.resolveMaxIterations applies the same bump idempotently
		// as a safety net for programmatic (non-gRPC) entry paths.
		if goalModeEnabled {
			maxIterations = aicommon.EnsureGoalModeMaxIterations(maxIterations, goalMinIterations)
		}
		opts = append(opts, aicommon.WithMaxIterationCount(maxIterations))
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
	if enableMultiAgent {
		opts = append(opts,
			aicommon.WithEnableMultiAgentMode(true),
			aicommon.WithMaxSubAgents(maxSubAgents),
		)
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
	if i.GetEnableDetachedPlan() {
		opts = append(opts, aicommon.WithEnableDetachedPlan(true))
	}

	if i.GetAICallTokenLimit() > 0 {
		opts = append(opts, aicommon.WithAiCallTokenLimit(int64(i.GetAICallTokenLimit())))
	}

	if i.GetDisableToolIntervalReview() {
		opts = append(opts, aicommon.WithDisableToolCallerIntervalReview(true))
	}
	if i.GetSyncPerceptionTrigger() {
		opts = append(opts, aicommon.WithSyncPerceptionTrigger(true))
	}
	if goalModeEnabled {
		opts = append(opts,
			aicommon.WithEnableGoalMode(true),
			aicommon.WithGoalMinIterations(goalMinIterations),
		)
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

	if i.GetDisableMemoryTriage() {
		opts = append(opts, aicommon.WithDisableMemoryTriage(true))
	}

	return opts
}

func resolveAIExecutionStrategy(i *ypb.AIStartParams) (enableMultiAgent bool, enableGoalMode bool, goalMinIterations int64, maxSubAgents int64) {
	if i == nil {
		return false, false, aicommon.DefaultGoalMinIterations, aicommon.DefaultMaxSubAgentConcurrency
	}
	if strategy := i.GetStrategy(); strategy != nil {
		enableMultiAgent = strategy.GetEnableMultiAgent()
		enableGoalMode = strategy.GetEnableGoalMode()
		goalMinIterations = strategy.GetGoalMinIterations()
		maxSubAgents = strategy.GetMaxSubAgents()
	}
	goalMinIterations = aicommon.NormalizeGoalMinIterations(goalMinIterations)
	return
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
	return s.startAIReActWithOptions(stream, true)
}

// startAIReActWithOptions keeps production behavior unchanged while allowing
// lifecycle tests to replace external AI dependencies. It is now only the gRPC
// protocol adapter; session arbitration, ownership and input delivery live in
// ReActSessionRuntime.
func (s *Server) startAIReActWithOptions(stream ypb.Yak_StartAIReActServer, loadBuiltinTools bool, additionalOptions ...aicommon.ConfigOption) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		log.Errorf("recv re-act first config msg failed: %v", err)
		return utils.Errorf("recv first mgs failed: %v", err)
	}

	if firstMsg == nil || !firstMsg.GetIsStart() {
		log.Errorf("recv re-act first config msg is invalid: %v", firstMsg)
		return utils.Error("first msg is not a start/config message, set IsStart to true")
	}
	startParams := firstMsg.GetParams()
	var sendMu sync.Mutex
	feedback := func(e *schema.AiOutputEvent) error {
		if e == nil {
			return nil
		}
		if stream.Context().Err() != nil {
			return nil
		}
		sendMu.Lock()
		defer sendMu.Unlock()
		return stream.Send(e.ToGRPC())
	}

	runtime := s.getReActSessionRuntime()
	if runtime == nil {
		return utils.Error("AI ReAct session runtime is not configured")
	}
	request := ConnectRequest{
		StartParams: startParams,
		options: &reActConnectOptions{
			loadBuiltinTools: loadBuiltinTools,
			configOptions:    additionalOptions,
			onEventError: func(err error) {
				// Keep the original streaming behavior: a failed subscriber
				// delivery is observable, but it does not fail the shared ReAct.
				log.Errorf("send re-act event to stream failed: %v", err)
			},
		},
	}
	connection, err := runtime.Connect(stream.Context(), request, feedback)
	if err != nil {
		return err
	}
	defer connection.Close()
	createdRuntime := connection.CreatedRuntime()
	type recvResult struct {
		event *ypb.AIInputEvent
		err   error
	}
	recvResults := make(chan recvResult, 1)
	go func() {
		for {
			event, err := stream.Recv()
			select {
			case recvResults <- recvResult{event: event, err: err}:
			case <-stream.Context().Done():
				return
			case <-connection.Done():
				return
			}
			if err != nil && !createdRuntime {
				return
			}
		}
	}()

	for {
		select {
		case <-stream.Context().Done():
			if createdRuntime {
				log.Info("AIReAct stream context done, stopping re-act")
			} else {
				log.Info("attached AIReAct stream context done")
			}
			return nil
		case <-connection.Done():
			return nil
		case result := <-recvResults:
			event, err := result.event, result.err
			if err != nil {
				if createdRuntime {
					log.Errorf("recv re-act msg failed: %v", err)
					continue
				}
				log.Infof("attached AIReAct stream recv ended: %v", err)
				return nil
			}
			if event == nil {
				if createdRuntime {
					log.Errorf("recv re-act msg failed: nil event")
					continue
				}
				log.Infof("attached AIReAct stream recv ended: nil event")
				return nil
			}
			if event.GetIsStart() {
				continue
			}
			if err := connection.Send(event); err != nil {
				if !createdRuntime && event.GetIsSyncMessage() && event.GetSyncType() == aicommon.SYNC_TYPE_RECOVERY_HISTORY {
					log.Warnf("send attached recovery history failed: %v", err)
					continue
				}
				if createdRuntime {
					// The old creator path admitted input asynchronously. Processing
					// failures were logged by the event loop and did not close the stream.
					log.Errorf("ReAct event processing failed: %v", err)
				} else {
					log.Warnf("forward input to running session failed: %v", err)
				}
			}
		}
	}
}

const attachedRecoveryHistoryBlockLimit = 20

func sendAttachedRecoveryHistory(
	ctx context.Context,
	db *gorm.DB,
	send func(*schema.AiOutputEvent) error,
	persistentSession string,
	event *ypb.AIInputEvent,
) error {
	if db == nil {
		return sendAttachedSyncError(send, "recovery_history", utils.Errorf("db is nil"), event.GetSyncID())
	}

	sessionID := persistentSession
	startID := int64(0)
	limit := attachedRecoveryHistoryBlockLimit

	if raw := event.GetSyncJsonInput(); raw != "" {
		var params map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &params); err != nil {
			return sendAttachedSyncError(send, "recovery_history", utils.Errorf("failed to parse recovery history params: %v", err), event.GetSyncID())
		}
		if sid := utils.InterfaceToString(params["session_id"]); sid != "" {
			sessionID = sid
		}
		if rawStartID, ok := params["start_id"]; ok {
			startID = int64(utils.InterfaceToInt(rawStartID))
		}
		if rawLimit, ok := params["limit"]; ok {
			if parsedLimit := utils.InterfaceToInt(rawLimit); parsedLimit > 0 {
				limit = parsedLimit
			}
		}
	}

	if sessionID == "" {
		return sendAttachedSyncError(send, "recovery_history", utils.Errorf("session_id is empty"), event.GetSyncID())
	}

	eventCh, result, err := yakit.YieldAIEventRecoveryHistory(ctx, db, sessionID, startID, limit)
	if err != nil {
		return sendAttachedSyncError(send, "recovery_history", err, event.GetSyncID())
	}
	for recoveredEvent := range eventCh {
		if recoveredEvent == nil {
			continue
		}
		recoveredEvent.IsSync = true
		if err := send(recoveredEvent); err != nil {
			return err
		}
	}

	return sendAttachedSyncJSON(send, schema.EVENT_TYPE_STRUCTURED, "recovery_history", map[string]interface{}{
		"session_id":         sessionID,
		"requested_start_id": startID,
		"block_count":        result.BlockCount,
		"event_count":        result.EventCount,
		"next_start_id":      result.NextStartID,
		"has_more":           result.HasMore,
	}, event.GetSyncID())
}

func sendAttachedSyncError(send func(*schema.AiOutputEvent) error, nodeID string, err error, syncID string) error {
	if err == nil {
		return nil
	}
	return sendAttachedSyncJSON(send, schema.EVENT_TYPE_STRUCTURED, nodeID, map[string]any{
		"error": err.Error(),
	}, syncID)
}

func sendAttachedSyncJSON(send func(*schema.AiOutputEvent) error, eventType schema.EventType, nodeID string, payload any, syncID string) error {
	return send(&schema.AiOutputEvent{
		Type:      eventType,
		NodeId:    nodeID,
		IsJson:    true,
		IsSync:    true,
		Content:   utils.Jsonify(payload),
		Timestamp: time.Now().Unix(),
		SyncID:    syncID,
	})
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
