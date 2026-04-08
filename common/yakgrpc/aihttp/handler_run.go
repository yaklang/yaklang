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

	session, created, err := gw.ensureReusableSession(runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
		return
	}

	statusCode := http.StatusCreated
	if !created {
		statusCode = http.StatusOK
	}

	writeJSON(w, statusCode, CreateSessionResponse{
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

	event, err := readAIInputEventRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	startOnly := allowStart && event.GetIsStart() && !hasInputPayload(event)

	if !startOnly && !hasInputPayload(event) {
		writeError(w, http.StatusBadRequest, "input event is empty")
		return
	}

	session, ok := gw.runManager.Get(runID)
	if !ok {
		if !allowStart {
			writeError(w, http.StatusNotFound, "run not found: "+runID)
			return
		}
		session, _, err = gw.ensureReusableSession(runID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
			return
		}
	}

	if allowStart {
		setting, err := gw.GetSettingFromDB()
		if err != nil {
			writeError(w, http.StatusInternalServerError, "load setting failed: "+err.Error())
			return
		}
		session.StartParams = mergeStartInputEvent(event, mergeStartParams(event.GetParams(), setting, runID), runID)
	}

	if allowStart && session.MarkStreamStarted() {
		go gw.runGRPCStream(session)
	}

	if !startOnly {
		if allowStart && event.GetIsStart() {
			writeProtoJSON(w, http.StatusOK, newResultOutputEvent("accepted"))
			return
		}
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
	gw.runManager.Remove(runID)

	writeProtoJSON(w, http.StatusOK, newResultOutputEvent(string(RunStatusCancelled)))
}

func (gw *AIAgentHTTPGateway) ensureReusableSession(runID string) (*RunSession, bool, error) {
	if session, ok := gw.runManager.Get(runID); ok {
		if session.ctx.Err() == nil {
			return session, false, nil
		}
		gw.runManager.Remove(runID)
	}

	setting, err := gw.GetSettingFromDB()
	if err != nil {
		return nil, false, err
	}

	session, created := gw.runManager.GetOrCreate(runID, func() *RunSession {
		return NewRunSession(gw.runManager.ctx, runID, &ypb.AIInputEvent{
			Params: cloneStartParams(mergeStartParams(nil, setting, runID), runID),
		})
	})

	if created {
		if db := gw.getDB(); db != nil {
			if _, err := yakit.EnsureAISessionMeta(db, runID); err != nil {
				log.Warnf("ensure ai session meta failed for %s: %v", runID, err)
			}
		}
	}

	return session, created, nil
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

func cloneStartInputEvent(event *ypb.AIInputEvent, runID string) *ypb.AIInputEvent {
	if event == nil {
		return &ypb.AIInputEvent{Params: cloneStartParams(nil, runID)}
	}
	cloned := proto.Clone(event).(*ypb.AIInputEvent)
	cloned.Params = cloneStartParams(cloned.GetParams(), runID)
	return cloned
}

func mergeStartInputEvent(event *ypb.AIInputEvent, params *ypb.AIStartParams, runID string) *ypb.AIInputEvent {
	cloned := cloneStartInputEvent(event, runID)
	cloned.Params = cloneStartParams(params, runID)
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

	startMsg := cloneStartInputEvent(session.StartParams, session.RunID)
	startMsg.IsStart = true
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
