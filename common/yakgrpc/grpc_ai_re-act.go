package yakgrpc

import (
	"context"
	"sync"

	"github.com/samber/lo"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/sessionruntime"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func fixOptionsWithServiceName(serviceName string, opts ...aicommon.ConfigOption) []aicommon.ConfigOption {
	return sessionruntime.FixOptionsWithServiceName(serviceName, opts...)
}

func ConvertYPBAIStartParamsToReActConfig(i *ypb.AIStartParams) []aicommon.ConfigOption {
	return sessionruntime.ConvertStartParamsToReActConfig(i)
}

func resolveAISessionStartParams(db *gorm.DB, sessionID string, request *ypb.AIStartParams, preferCached bool) (*ypb.AIStartParams, error) {
	return sessionruntime.ResolveSessionStartParams(db, sessionID, request, preferCached)
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
	request := sessionruntime.ConnectRequest{
		StartParams: startParams,
		Options: &sessionruntime.ConnectOptions{
			LoadBuiltinTools: loadBuiltinTools,
			ConfigOptions:    additionalOptions,
			OnEventError: func(err error) {
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
