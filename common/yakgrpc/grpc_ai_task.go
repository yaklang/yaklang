package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type aiChatType func(string, ...aispec.AIConfigOption) (string, error)

var mockedAIChat aiChatType = nil

func RegisterMockAIChat(c aiChatType) {
	mockedAIChat = c
}

var RedirectForge = "redirect_forge"

func (s *Server) StartAITask(stream ypb.Yak_StartAITaskServer) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		log.Infof("recv first msg failed: %v", err)
		return utils.Errorf("recv first msg failed: %v", err)
	}

	if !firstMsg.IsStart {
		log.Info("first msg is not start")
		return utils.Error("first msg is not start")
	}
	startParams := firstMsg.Params

	baseCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	inputEvent := make(chan *aid.InputEvent, 1000)
	var currentCoordinatorId = startParams.CoordinatorId
	var coordinatorIdOnce sync.Once
	var sendEvent = func(e *schema.AiOutputEvent) {
		if e.Timestamp <= 0 {
			e.Timestamp = time.Now().Unix() // fallback
		}
		coordinatorIdOnce.Do(func() {
			currentCoordinatorId = e.CoordinatorId
		})
		if e.CoordinatorId != currentCoordinatorId {
			fmt.Printf("e.CoordinatorId [%s] != currentCoordinatorId [%s]\n", e.CoordinatorId, currentCoordinatorId)
		}
		err := stream.Send(e.ToGRPC())
		if err != nil {
			log.Errorf("send event failed: %v", err)
		}
	}

	var aidOption = []aid.Option{
		aid.WithSaveEvent(true),
		aid.WithTaskAnalysis(true),
		aid.WithEventHandler(sendEvent),
		aid.WithEventInputChan(inputEvent),
	}

	aidOption = append(aidOption, buildAIDOption(startParams)...)

	var hotpatchBroadcaster = chanx.NewBroadcastChannel[aid.Option](baseCtx, 10)
	aidOption = append(aidOption, aid.WithHotpatchOptionChanFactory(hotpatchBroadcaster.Subscribe))
	go func() {
		defer cancel()
		for {
			event, err := stream.Recv()
			if err != nil {
				log.Errorf("receive event failed (for sync messages): %v", err)
				return
			}
			if event.IsSyncMessage {
				t, ok := aid.ParseSyncType(event.GetSyncType())
				if !ok {
					log.Errorf("parse sync type failed, got: %v", event.GetSyncType())
					continue
				}
				var params = make(aitool.InvokeParams)
				err := json.Unmarshal([]byte(event.GetSyncJsonInput()), &params)
				if err != nil {
					log.Errorf("unmarshal interactive json input failed: %v", err)
				}
				select {
				case inputEvent <- &aid.InputEvent{
					IsSyncInfo: true,
					SyncType:   t,
					Params:     params,
				}:
					continue
				case <-baseCtx.Done():
					return
				}
			}

			if event.IsInteractiveMessage {
				var params = make(aitool.InvokeParams)
				err := json.Unmarshal([]byte(event.InteractiveJSONInput), &params)
				if err != nil {
					log.Errorf("unmarshal interactive json input failed: %v", err)
					continue
				}
				inEvent := &aid.InputEvent{
					IsInteractive: true,
					Id:            event.InteractiveId,
					Params:        params,
				}
				select {
				case inputEvent <- inEvent:
					continue
				case <-baseCtx.Done():
					return
				}
			}

			if event.IsConfigHotpatch {
				params := event.GetParams()
				var updateOption aid.Option
				switch event.HotpatchType {
				case "ReviewPolicy":
					switch params.GetReviewPolicy() {
					case "yolo":
						updateOption = aid.WithAgreeYOLO(true)
					case "ai":
						updateOption = aid.WithAIAgree()
					case "manual":
						updateOption = aid.WithAgreeManual()
					}
				default:
					log.Errorf("unknown hotpatch type: %s", event.HotpatchType)
					continue
				}
				if updateOption == nil {
					hotpatchBroadcaster.Submit(updateOption)
				}
			}
		}
	}()

	var params any
	if startParams.GetForgeParams() != nil {
		params = startParams.GetForgeParams()
	} else {
		params = startParams.GetUserQuery()
	}

	forgeName := startParams.GetForgeName()

	var res any
	if forgeName != "" {
		log.Infof("forgeName is %v, start call yak.ExecuteForge", forgeName)
		res, err = yak.ExecuteForge(forgeName, params, buildAIAgentOption(baseCtx, startParams.GetCoordinatorId(), sendEvent, aidOption...)...)
		if err != nil {
			log.Errorf("run ai forge[%s] failed: %v", forgeName, err)
			return err
		}
		log.Infof("run ai forge[%s] success, result res: %v", forgeName, res)
	} else {
		log.Info("call without forgeName, use 'forge_triage' as default")

		cod, err := aid.NewCoordinatorContext(baseCtx, utils.InterfaceToString(params), append(aidOption, aid.WithCoordinatorId(currentCoordinatorId))...)
		if err != nil {
			log.Errorf("create ai coordinator failed: %v", err)
			return err
		}
		err = cod.Run()
		if err != nil {
			log.Errorf("run ai coordinator failed: %v", err)
			return err
		}
	}
	if res != nil {
		stream.Send(&ypb.AIOutputEvent{
			CoordinatorId: currentCoordinatorId,
			IsReason:      true,
			Content:       utils.InterfaceToBytes(res),
		})
	}
	return nil
}

func buildAIAgentOption(ctx context.Context, CoordinatorId string, agentEventHandler func(e *schema.AiOutputEvent), extendOption ...aid.Option) []any {
	agentOption := []any{
		yak.WithContext(ctx),
	}
	if CoordinatorId != "" {
		agentOption = append(agentOption, yak.WithCoordinatorId(CoordinatorId))
	}

	if len(extendOption) > 0 {
		agentOption = append(agentOption, yak.WithExtendAIDOptions(extendOption...))
	}

	if agentEventHandler != nil {
		agentOption = append(agentOption, yak.WithAiAgentEventHandler(agentEventHandler))
	}

	return agentOption
}

func buildAIDOption(startParams *ypb.AIStartParams) []aid.Option {
	aidOption := make([]aid.Option, 0)

	if startParams.GetEnableSystemFileSystemOperator() {
		aidOption = append(aidOption, aid.WithSystemFileOperator())
		aidOption = append(aidOption, aid.WithJarOperator())
	}

	switch startParams.GetReviewPolicy() {
	case "yolo":
		aidOption = append(aidOption, aid.WithAgreeYOLO(true))
	case "ai":
		aidOption = append(aidOption, aid.WithAIAgree())
	case "manual":
		aidOption = append(aidOption, aid.WithAgreeManual())
	}

	if startParams.GetEnableQwenNoThinkMode() {
		aidOption = append(aidOption, aid.WithQwenNoThink())
	}

	if startParams.GetAllowPlanUserInteract() {
		aidOption = append(aidOption, aid.WithAllowPlanUserInteract())
	}

	if startParams.GetPlanUserInteractMaxCount() > 0 {
		aidOption = append(aidOption, aid.WithPlanUserInteractMaxCount(startParams.GetPlanUserInteractMaxCount()))
	}

	if startParams.GetAllowGenerateReport() {
		aidOption = append(aidOption, aid.WithGenerateReport(startParams.GetAllowGenerateReport()))
	}

	if startParams.GetUseDefaultAIConfig() {
		wrapperChat := aid.AIChatToAICallbackType(ai.Chat)
		aidOption = append(aidOption, aid.WithAICallback(func(config *aid.Config, req *aid.AIRequest) (*aid.AIResponse, error) {
			//fmt.Println(req.GetPrompt())
			//time.Sleep(100 * time.Millisecond)
			return wrapperChat(config, req)
		}))
	}

	if mockedAIChat != nil {
		aidOption = append(aidOption, aid.WithAICallback(aid.AIChatToAICallbackType(mockedAIChat)))
	}

	if startParams.GetDisallowRequireForUserPrompt() {
		aidOption = append(aidOption, aid.WithDisallowRequireForUserPrompt())
	}

	if startParams.GetDisableToolUse() {
		aidOption = append(aidOption, aid.WithDisableToolUse())
	}

	if startParams.GetAICallAutoRetry() > 0 {
		aidOption = append(aidOption, aid.WithAIAutoRetry(int(startParams.GetAICallAutoRetry())))
	}

	if startParams.GetAITransactionRetry() > 0 {
		aidOption = append(aidOption, aid.WithAITransactionRetry(int(startParams.GetAITransactionRetry())))
	}

	if startParams.GetEnableAISearchTool() {
		aidOption = append(aidOption, aid.WithAiToolsSearchTool())
	}

	if startParams.GetEnableAISearchInternet() {
		aidOption = append(aidOption, aid.WithOmniSearchTool())
	}

	if len(startParams.GetIncludeSuggestedToolKeywords()) > 0 {
		aidOption = append(aidOption, aid.WithToolKeywords(startParams.GetIncludeSuggestedToolKeywords()...))
	}

	if len(startParams.GetIncludeSuggestedToolNames()) > 0 {
		aidOption = append(aidOption, aid.WithEnableToolsName(startParams.GetIncludeSuggestedToolNames()...))
	}

	if len(startParams.GetExcludeToolNames()) > 0 {
		aidOption = append(aidOption, aid.WithDisableToolsName(startParams.GetExcludeToolNames()...))
	}

	if startParams.GetCoordinatorId() != "" {
		aidOption = append(aidOption, aid.WithCoordinatorId(startParams.GetCoordinatorId()))
	}

	if startParams.GetTaskMaxContinueCount() > 0 {
		aidOption = append(aidOption, aid.WithMaxTaskContinue(startParams.GetTaskMaxContinueCount()))
	}

	return aidOption
}
