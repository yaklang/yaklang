package yakgrpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
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
		log.Errorf("recv first msg failed: %v", err)
		return utils.Errorf("recv first msg failed: %v", err)
	}

	if !firstMsg.IsStart {
		log.Info("first msg is not start")
		return utils.Error("first msg is not start")
	}
	startParams := firstMsg.Params

	baseCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	inputEvent := chanx.NewUnlimitedChan[*ypb.AIInputEvent](baseCtx, 10)
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

	var hotpatchChan = chanx.NewUnlimitedChan[aicommon.ConfigOption](baseCtx, 10)
	var configOption = []aicommon.ConfigOption{
		aicommon.WithSaveEvent(true),
		aicommon.WithEventHandler(sendEvent),
		aicommon.WithEventInputChanx(inputEvent),
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithEnablePETaskAnalyze(true),
	}

	configOption = append(configOption, buildAIDOption(startParams)...)

	go func() {
		defer cancel()
		for {
			event, err := stream.Recv()
			if err != nil {
				log.Errorf("receive event failed (for sync messages): %v", err)
				return
			}

			inputEvent.SafeFeed(event)
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
		res, err = yak.ExecuteForge(forgeName, params, buildAIAgentOption(baseCtx, startParams.GetCoordinatorId(), sendEvent, configOption...)...)
		if err != nil {
			log.Errorf("run ai forge[%s] failed: %v", forgeName, err)
			return err
		}
		log.Infof("run ai forge[%s] success, result res: %v", forgeName, res)
	} else {
		log.Info("call without forgeName, use 'forge_triage' as default")

		cod, err := aid.NewCoordinatorContext(baseCtx, utils.InterfaceToString(params), append(configOption, aicommon.WithID(currentCoordinatorId))...)
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

func buildAIAgentOption(ctx context.Context, CoordinatorId string, agentEventHandler func(e *schema.AiOutputEvent), extendOption ...aicommon.ConfigOption) []any {
	agentOption := []any{
		yak.WithContext(ctx),
	}
	if CoordinatorId != "" {
		agentOption = append(agentOption, yak.WithCoordinatorId(CoordinatorId))
	}

	if len(extendOption) > 0 {
		agentOption = append(agentOption, yak.WithExtendAICommonOptions(extendOption...))
	}

	if agentEventHandler != nil {
		agentOption = append(agentOption, yak.WithAiAgentEventHandler(agentEventHandler))
	}

	return agentOption
}

func buildAIDOption(startParams *ypb.AIStartParams) []aicommon.ConfigOption {
	aidOption := make([]aicommon.ConfigOption, 0)

	if startParams.GetEnableSystemFileSystemOperator() {
		aidOption = append(aidOption, aicommon.WithSystemFileOperator())
		aidOption = append(aidOption, aicommon.WithJarOperator())
	}

	switch startParams.GetReviewPolicy() {
	case "yolo":
		aidOption = append(aidOption, aicommon.WithAgreeYOLO())
	case "ai":
		aidOption = append(aidOption, aicommon.WithAIAgree())
	case "manual":
		aidOption = append(aidOption, aicommon.WithAgreeManual())
	}

	if startParams.GetEnableQwenNoThinkMode() {
		aidOption = append(aidOption, aicommon.WithQwenNoThink())
	}

	if startParams.GetAllowPlanUserInteract() {
		aidOption = append(aidOption, aicommon.WithAllowPlanUserInteract(true))
	}

	if startParams.GetPlanUserInteractMaxCount() > 0 {
		aidOption = append(aidOption, aicommon.WithPlanUserInteractMaxCount(startParams.GetPlanUserInteractMaxCount()))
	}

	if startParams.GetAllowGenerateReport() {
		aidOption = append(aidOption, aicommon.WithGenerateReport(startParams.GetAllowGenerateReport()))
	}

	if startParams.GetUseDefaultAIConfig() {
		wrapperChat := aicommon.AIChatToAICallbackType(ai.Chat)
		aidOption = append(aidOption, aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			//fmt.Println(req.GetPrompt())
			//time.Sleep(100 * time.Millisecond)
			return wrapperChat(config, req)
		}))
	}

	if serviceName := startParams.GetAIService(); serviceName != "" {
		callback, err := localModelAICallbackByServiceName(serviceName)
		if err != nil {
			log.Errorf("load ai service failed: %v", err)
		} else {
			aidOption = append(aidOption, aicommon.WithAICallback(callback))
		}
	}

	if mockedAIChat != nil {
		aidOption = append(aidOption, aicommon.WithAICallback(aicommon.AIChatToAICallbackType(mockedAIChat)))
	}

	if startParams.GetDisallowRequireForUserPrompt() {
		aidOption = append(aidOption, aicommon.WithDisallowRequireForUserPrompt())
	}

	if startParams.GetDisableToolUse() {
		aidOption = append(aidOption, aicommon.WithDisableToolUse(true))
	}

	if startParams.GetAICallAutoRetry() > 0 {
		aidOption = append(aidOption, aicommon.WithAIAutoRetry(startParams.GetAICallAutoRetry()))
	}

	if startParams.GetAITransactionRetry() > 0 {
		aidOption = append(aidOption, aicommon.WithAITransactionRetry(startParams.GetAITransactionRetry()))
	}

	if startParams.GetEnableAISearchTool() {
		aidOption = append(aidOption, aid.WithAiToolsSearchTool())
	}

	if startParams.GetEnableAISearchInternet() {
		aidOption = append(aidOption, aicommon.WithOmniSearchTool())
	}

	if len(startParams.GetIncludeSuggestedToolKeywords()) > 0 {
		aidOption = append(aidOption, aicommon.WithKeywords(startParams.GetIncludeSuggestedToolKeywords()...))
	}

	if len(startParams.GetIncludeSuggestedToolNames()) > 0 {
		aidOption = append(aidOption, aicommon.WithEnableToolsName(startParams.GetIncludeSuggestedToolNames()...))
	}

	if len(startParams.GetExcludeToolNames()) > 0 {
		aidOption = append(aidOption, aicommon.WithDisableToolsName(startParams.GetExcludeToolNames()...))
	}

	if startParams.GetCoordinatorId() != "" {
		aidOption = append(aidOption, aicommon.WithID(startParams.GetCoordinatorId()))
	}

	if startParams.GetTaskMaxContinueCount() > 0 {
		aidOption = append(aidOption, aicommon.WithMaxTaskContinue(startParams.GetTaskMaxContinueCount()))
	}

	return aidOption
}

func localModelAICallbackByServiceName(serviceName string) (func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error), error) {
	// localmodelManager := localmodel.GetManager()
	// service, err := localmodelManager.GetServiceStatus(startParams.GetAIService())
	// if err != nil {
	// }
	chat, err := ai.LoadChater(serviceName)
	if err != nil {
		return nil, fmt.Errorf("load ai service failed: %v", err)
	}
	return aicommon.AIChatToAICallbackType(chat), nil
}
