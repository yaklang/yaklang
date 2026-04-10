package aihttp

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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

	session := gw.runManager.Create(runID, params)
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

	var req PushEventRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Params != nil {
		setting, err := gw.GetSettingFromDB()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
			return
		}
		session.Params = mergeParams(*req.Params, setting)
	}

	event := convertPushToInputEvent(req, runID)
	if !hasInputPayload(event) {
		writeError(w, http.StatusBadRequest, "input event is empty")
		return
	}

	if session.MarkStreamStarted() {
		go gw.runGRPCStream(session, session.Params)
	}

	session.PushInput(event)

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": runID,
		"status": "accepted",
	})
}

func (gw *AIAgentHTTPGateway) handleCancelRun(w http.ResponseWriter, r *http.Request) {
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

	session.Cancel()

	writeJSON(w, http.StatusOK, map[string]any{
		"run_id": runID,
		"status": "cancelled",
	})
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

func (gw *AIAgentHTTPGateway) runGRPCStream(session *RunSession, startParams AIParams) {
	session.Status = RunStatusRunning

	stream, err := gw.yakClient.StartAIReAct(session.ctx)
	if err != nil {
		log.Errorf("start AIReAct stream failed for run %s: %v", session.RunID, err)
		session.Complete(err)
		return
	}

	grpcStartParams := ConvertAIParamsToYPB(startParams, session.RunID)
	startMsg := &ypb.AIInputEvent{
		IsStart:          true,
		Params:           grpcStartParams,
		AttachedFilePath: startParams.AttachedFiles,
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

		event := convertOutputToRunEvent(resp)

		session.AddEvent(event)
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

func convertOutputToRunEvent(e *ypb.AIOutputEvent) RunEvent {
	event := RunEvent{
		ID:            uuid.New().String(),
		Type:          e.Type,
		CoordinatorID: e.CoordinatorId,
		AIModelName:   e.AIModelName,
		NodeID:        string(e.NodeId),
		IsSystem:      e.IsSystem,
		IsStream:      e.IsStream,
		IsReason:      e.IsReason,
		Timestamp:     e.Timestamp,
		TaskIndex:     e.TaskIndex,
		EventUUID:     e.EventUUID,
		TaskUUID:      e.TaskUUID,
	}

	if len(e.StreamDelta) > 0 {
		event.StreamDelta = string(e.StreamDelta)
	}

	if len(e.Content) > 0 {
		event.Content = string(e.Content)
	}

	if event.Timestamp <= 0 {
		event.Timestamp = time.Now().Unix()
	}

	return event
}
