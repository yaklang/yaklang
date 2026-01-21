package aihttp

import (
	"io"
	"net/http"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// handleCreateRun handles POST /agent/run
func (gw *AIAgentHTTPGateway) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var req CreateRunRequest
	if err := readJSON(r, &req); err != nil {
		log.Debugf("Failed to parse create run request: %v", err)
		writeError(w, http.StatusBadRequest, "bad_request", "invalid request body: "+err.Error())
		return
	}

	if req.Query == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "query is required")
		return
	}

	if req.TaskID == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "task_id is required")
		return
	}

	// Create session
	session := gw.runManager.CreateSession(r.Context(), req.TaskID)
	session.SetStatus(RunStatusRunning)

	// Prepare AI start params
	defaultSetting := gw.GetDefaultSetting()
	startParams := ConvertAIParamsToYPB(req.Params, defaultSetting)
	startParams.UserQuery = req.Query

	// Start the AI ReAct via gRPC in background
	go gw.runReActViaGRPC(session, startParams, req.AttachedFiles)

	// Return immediately with run info
	writeJSON(w, CreateRunResponse{
		RunID:     session.RunID,
		TaskID:    session.TaskID,
		StartTime: session.StartTime,
		Status:    session.Status,
	})
}

// runReActViaGRPC runs the AI ReAct process via gRPC client
func (gw *AIAgentHTTPGateway) runReActViaGRPC(session *RunSession, startParams *ypb.AIStartParams, attachedFiles []string) {
	ctx := session.Context()

	// Get gRPC client
	client := gw.GetGRPCClient()
	if client == nil {
		log.Error("gRPC client is nil")
		session.SetStatus(RunStatusFailed)
		session.SetError("internal error: gRPC client not initialized")
		return
	}

	// Create bidirectional stream via gRPC
	stream, err := client.StartAIReAct(ctx)
	if err != nil {
		log.Errorf("Failed to start AI ReAct stream: %v", err)
		session.SetStatus(RunStatusFailed)
		session.SetError("failed to start AI session: " + err.Error())
		return
	}

	// Send the initial start message
	inputEvent := &ypb.AIInputEvent{
		IsStart: true,
		Params:  startParams,
	}
	if len(attachedFiles) > 0 {
		inputEvent.AttachedFilePath = attachedFiles
	}

	if err := stream.Send(inputEvent); err != nil {
		log.Errorf("Failed to send start event: %v", err)
		session.SetStatus(RunStatusFailed)
		session.SetError("failed to send start event: " + err.Error())
		return
	}

	log.Infof("Started AI ReAct via gRPC for run: %s", session.RunID)

	var coordinatorIdOnce sync.Once

	// Start goroutine to receive events from gRPC stream
	go func() {
		defer func() {
			if session.Status == RunStatusRunning {
				session.SetStatus(RunStatusCompleted)
			}
		}()

		for {
			select {
			case <-ctx.Done():
				log.Infof("Context cancelled for run: %s", session.RunID)
				return
			default:
			}

			event, err := stream.Recv()
			if err != nil {
				if err == io.EOF {
					log.Infof("AI ReAct stream completed for run: %s", session.RunID)
					return
				}
				// Check if context was cancelled
				if ctx.Err() != nil {
					return
				}
				log.Errorf("Error receiving from AI ReAct stream: %v", err)
				session.SetStatus(RunStatusFailed)
				session.SetError("stream error: " + err.Error())
				return
			}

			// Update coordinator ID
			if event.CoordinatorId != "" {
				coordinatorIdOnce.Do(func() {
					session.SetCoordinatorID(event.CoordinatorId)
				})
			}

			// Add event to session for subscribers
			session.AddEvent(event)
		}
	}()

	// Start goroutine to forward input events from session to gRPC stream
	go func() {
		inputChan := session.GetInputChan().OutputChannel()
		for {
			select {
			case <-ctx.Done():
				return
			case inputEvent, ok := <-inputChan:
				if !ok {
					return
				}
				// Skip the initial start event as it's already sent
				if inputEvent.IsStart {
					continue
				}
				if err := stream.Send(inputEvent); err != nil {
					log.Errorf("Failed to forward input event: %v", err)
					return
				}
			}
		}
	}()
}
