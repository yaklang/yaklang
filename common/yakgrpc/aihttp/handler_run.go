package aihttp

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func (gw *AIAgentHTTPGateway) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	runID := req.RunID
	if runID == "" {
		runID = uuid.NewString()
	}

	if exist, ok := gw.runManager.Get(runID); ok {
		writeJSON(w, http.StatusOK, CreateSessionResponse{
			RunID:  exist.RunID,
			Status: exist.Status,
		})
		return
	}

	setting, err := gw.GetSettingFromDB()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
		return
	}
	params := mergeParams(req.Params, setting)

	session := gw.runManager.Create(runID, ConvertAIParamsToYPB(params, runID), params.AttachedFiles)
	if db := gw.getDB(); db != nil {
		if _, err := yakit.EnsureAISessionMeta(db, runID); err != nil {
			log.Warnf("ensure ai session meta failed for %s: %v", runID, err)
		}
	}

	writeJSON(w, http.StatusCreated, CreateSessionResponse{
		RunID:  session.RunID,
		Status: session.Status,
	})
}

func (gw *AIAgentHTTPGateway) handleRun(w http.ResponseWriter, r *http.Request) {
	gw.handleStreamInput(w, r, true)
}

func (gw *AIAgentHTTPGateway) handleStreamInput(w http.ResponseWriter, r *http.Request, allowStart bool) {
	runID := mux.Vars(r)["run_id"]
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	session, ok := gw.runManager.Get(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "run not found: "+runID)
		return
	}

	if session.Status == RunStatusCompleted || session.Status == RunStatusFailed || session.Status == RunStatusCancelled {
		writeError(w, http.StatusConflict, "run is not active, current status: "+string(session.Status))
		return
	}

	if !allowStart && session.Status != RunStatusRunning && session.Status != RunStatusPending {
		writeError(w, http.StatusConflict, "run is not active, current status: "+string(session.Status))
		return
	}

	event, err := readAIInputEventRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	startOnly := allowStart && event.GetIsStart() && !hasInputPayload(event)

	if allowStart && event.GetParams() != nil {
		setting, err := gw.GetSettingFromDB()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
			return
		}
		session.StartParams = mergeStartParams(event.GetParams(), setting, runID)
	}

	if !startOnly && !hasInputPayload(event) {
		writeError(w, http.StatusBadRequest, "input event is empty")
		return
	}

	if allowStart && session.MarkStreamStarted() {
		go gw.runGRPCStream(session)
	}

	if !startOnly {
		if event.GetIsStart() {
			cloned := proto.Clone(event).(*ypb.AIInputEvent)
			cloned.IsStart = false
			event = cloned
		}
		session.PushInput(event)
	}

	writeProtoJSON(w, http.StatusOK, newResultOutputEvent("accepted"))
}

func (gw *AIAgentHTTPGateway) handleCancelRun(w http.ResponseWriter, r *http.Request) {
	runID := mux.Vars(r)["run_id"]
	if runID == "" {
		writeError(w, http.StatusBadRequest, "run_id is required")
		return
	}

	if _, err := readOptionalAIInputEventRequest(r); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	session, ok := gw.runManager.Get(runID)
	if !ok {
		writeError(w, http.StatusNotFound, "run not found: "+runID)
		return
	}

	session.Cancel()

	writeProtoJSON(w, http.StatusOK, newResultOutputEvent(string(RunStatusCancelled)))
}

func mergeParams(req AIParams, defaults aiAgentChatSettingPayload) AIParams {
	if req.UseDefaultAI || defaults.UseDefaultAIConfig {
		req.UseDefaultAI = true
		if req.AIService == "" {
			req.AIService = defaults.AIService
		}
		if req.AIModelName == "" {
			req.AIModelName = defaults.AIModelName
		}
		if req.ForgeName == "" {
			req.ForgeName = defaults.ForgeName
		}
		if req.ReviewPolicy == "" {
			req.ReviewPolicy = defaults.ReviewPolicy
		}
		if req.ReActMaxIteration == 0 {
			req.ReActMaxIteration = defaults.ReActMaxIteration
		}
		if req.MaxIteration == 0 {
			if defaults.ReActMaxIteration > 0 {
				req.MaxIteration = int32(defaults.ReActMaxIteration)
			}
		}
		if !req.DisableToolUse {
			req.DisableToolUse = defaults.DisableToolUse
		}
		if !req.EnableSystemFileSystemOperator {
			req.EnableSystemFileSystemOperator = defaults.EnableSystemFileSystemOperator
		}
		if !req.DisallowRequireForUserPrompt {
			req.DisallowRequireForUserPrompt = defaults.DisallowRequireForUserPrompt
		}
		if req.AIReviewRiskControlScore == 0 {
			req.AIReviewRiskControlScore = defaults.AIReviewRiskControlScore
		}
		if req.AICallAutoRetry == 0 {
			req.AICallAutoRetry = defaults.AICallAutoRetry
		}
		if req.AITransactionRetry == 0 {
			req.AITransactionRetry = defaults.AITransactionRetry
		}
		if !req.EnableAISearchTool {
			req.EnableAISearchTool = defaults.EnableAISearchTool
		}
		if !req.EnableAISearchInternet {
			req.EnableAISearchInternet = defaults.EnableAISearchInternet
		}
		if !req.EnableQwenNoThinkMode {
			req.EnableQwenNoThinkMode = defaults.EnableQwenNoThinkMode
		}
		if !req.AllowPlanUserInteract {
			req.AllowPlanUserInteract = defaults.AllowPlanUserInteract
		}
		if req.PlanUserInteractMaxCount == 0 {
			req.PlanUserInteractMaxCount = defaults.PlanUserInteractMaxCount
		}
		if req.TimelineItemLimit == 0 {
			req.TimelineItemLimit = defaults.TimelineItemLimit
		}
		if req.TimelineContentSizeLimit == 0 {
			req.TimelineContentSizeLimit = defaults.TimelineContentSizeLimit
		}
		if req.UserInteractLimit == 0 {
			req.UserInteractLimit = defaults.UserInteractLimit
		}
		if req.TimelineSessionID == "" {
			req.TimelineSessionID = defaults.TimelineSessionID
		}
	}
	return req
}

func mergeStartParams(req *ypb.AIStartParams, defaults aiAgentChatSettingPayload, runID string) *ypb.AIStartParams {
	params := cloneStartParams(req, runID)
	if params.GetUseDefaultAIConfig() || defaults.UseDefaultAIConfig {
		params.UseDefaultAIConfig = true
		if params.GetAIService() == "" {
			params.AIService = defaults.AIService
		}
		if params.GetAIModelName() == "" {
			params.AIModelName = defaults.AIModelName
		}
		if params.GetForgeName() == "" {
			params.ForgeName = defaults.ForgeName
		}
		if params.GetReviewPolicy() == "" {
			params.ReviewPolicy = defaults.ReviewPolicy
		}
		if params.GetReActMaxIteration() == 0 {
			params.ReActMaxIteration = defaults.ReActMaxIteration
		}
		if !params.GetDisableToolUse() {
			params.DisableToolUse = defaults.DisableToolUse
		}
		if !params.GetEnableSystemFileSystemOperator() {
			params.EnableSystemFileSystemOperator = defaults.EnableSystemFileSystemOperator
		}
		if !params.GetDisallowRequireForUserPrompt() {
			params.DisallowRequireForUserPrompt = defaults.DisallowRequireForUserPrompt
		}
		if params.GetAIReviewRiskControlScore() == 0 {
			params.AIReviewRiskControlScore = defaults.AIReviewRiskControlScore
		}
		if params.GetAICallAutoRetry() == 0 {
			params.AICallAutoRetry = defaults.AICallAutoRetry
		}
		if params.GetAITransactionRetry() == 0 {
			params.AITransactionRetry = defaults.AITransactionRetry
		}
		if !params.GetEnableAISearchTool() {
			params.EnableAISearchTool = defaults.EnableAISearchTool
		}
		if !params.GetEnableAISearchInternet() {
			params.EnableAISearchInternet = defaults.EnableAISearchInternet
		}
		if !params.GetEnableQwenNoThinkMode() {
			params.EnableQwenNoThinkMode = defaults.EnableQwenNoThinkMode
		}
		if !params.GetAllowPlanUserInteract() {
			params.AllowPlanUserInteract = defaults.AllowPlanUserInteract
		}
		if params.GetPlanUserInteractMaxCount() == 0 {
			params.PlanUserInteractMaxCount = defaults.PlanUserInteractMaxCount
		}
		if params.GetTimelineItemLimit() == 0 {
			params.TimelineItemLimit = defaults.TimelineItemLimit
		}
		if params.GetTimelineContentSizeLimit() == 0 && defaults.TimelineContentSizeLimit > 0 {
			params.TimelineContentSizeLimit = defaults.TimelineContentSizeLimit * 1024
		}
		if params.GetUserInteractLimit() == 0 {
			params.UserInteractLimit = defaults.UserInteractLimit
		}
		if params.GetTimelineSessionID() == runID && defaults.TimelineSessionID != "" {
			params.TimelineSessionID = defaults.TimelineSessionID
		}
	}
	return params
}

func cloneStartParams(params *ypb.AIStartParams, runID string) *ypb.AIStartParams {
	if params == nil {
		return &ypb.AIStartParams{TimelineSessionID: runID}
	}
	cloned := proto.Clone(params).(*ypb.AIStartParams)
	if cloned.GetTimelineSessionID() == "" {
		cloned.TimelineSessionID = runID
	}
	return cloned
}

func (gw *AIAgentHTTPGateway) runGRPCStream(session *RunSession) {
	session.Status = RunStatusRunning

	stream, err := gw.yakClient.StartAIReAct(session.ctx)
	if err != nil {
		log.Errorf("start AIReAct stream failed for run %s: %v", session.RunID, err)
		session.Complete(err)
		return
	}

	startMsg := &ypb.AIInputEvent{
		IsStart:          true,
		Params:           cloneStartParams(session.StartParams, session.RunID),
		AttachedFilePath: append([]string(nil), session.StartAttachedFiles...),
	}
	if err := stream.Send(startMsg); err != nil {
		log.Errorf("send start message failed for run %s: %v", session.RunID, err)
		session.Complete(err)
		return
	}

	// forward input events from HTTP to gRPC
	go func() {
		for {
			select {
			case <-session.ctx.Done():
				stream.CloseSend()
				return
			case event, ok := <-session.inputChan.OutputChannel():
				if !ok {
					return
				}
				if err := stream.Send(event); err != nil {
					log.Errorf("forward input event failed for run %s: %v", session.RunID, err)
					return
				}
			}
		}
	}()

	// receive output events from gRPC and broadcast
	for {
		resp, err := stream.Recv()
		if err != nil {
			if session.ctx.Err() != nil {
				if session.Status != RunStatusCancelled {
					session.Complete(nil)
				}
			} else {
				log.Errorf("recv AIReAct event failed for run %s: %v", session.RunID, err)
				session.Complete(err)
			}
			return
		}

		session.AddEvent(normalizeOutputEvent(resp))
	}
}

func ConvertAIParamsToYPB(p AIParams, runID string) *ypb.AIStartParams {
	params := &ypb.AIStartParams{
		EnableSystemFileSystemOperator: p.EnableSystemFileSystemOperator,
		UseDefaultAIConfig:             p.UseDefaultAI,
		DisallowRequireForUserPrompt:   p.DisallowRequireForUserPrompt,
		ReviewPolicy:                   p.ReviewPolicy,
		AIReviewRiskControlScore:       p.AIReviewRiskControlScore,
		DisableToolUse:                 p.DisableToolUse,
		AICallAutoRetry:                p.AICallAutoRetry,
		AITransactionRetry:             p.AITransactionRetry,
		EnableAISearchTool:             p.EnableAISearchTool,
		EnableAISearchInternet:         p.EnableAISearchInternet,
		EnableQwenNoThinkMode:          p.EnableQwenNoThinkMode,
		AllowPlanUserInteract:          p.AllowPlanUserInteract,
		PlanUserInteractMaxCount:       p.PlanUserInteractMaxCount,
		AIService:                      p.AIService,
		AIModelName:                    p.AIModelName,
		TimelineItemLimit:              p.TimelineItemLimit,
		UserInteractLimit:              p.UserInteractLimit,
		TimelineSessionID:              runID,
		DisableToolIntervalReview:      p.DisableToolIntervalReview,
	}

	if p.ForgeName != "" {
		params.ForgeName = p.ForgeName
	}
	if p.ReActMaxIteration > 0 {
		params.ReActMaxIteration = p.ReActMaxIteration
	} else if p.MaxIteration > 0 {
		params.ReActMaxIteration = int64(p.MaxIteration)
	}
	if p.TimelineContentSizeLimit > 0 {
		params.TimelineContentSizeLimit = p.TimelineContentSizeLimit * 1024
	}
	if p.TimelineSessionID != "" {
		params.TimelineSessionID = p.TimelineSessionID
	}
	return params
}
