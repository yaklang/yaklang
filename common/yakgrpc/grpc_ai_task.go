package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/reducer"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
	"sync"
	"time"
)

type aiChatType func(string, ...aispec.AIConfigOption) (string, error)

var mockedAIChat aiChatType = nil

func RegisterMockAIChat(c aiChatType) {
	mockedAIChat = c
}

var triageCache = reducer.NewReducer(10, func(data []string) string {
	result, err := yak.ExecuteForge("fragment_summarizer", map[string]any{
		"textSnippet": strings.Join(data, "\n"),
	}, aid.WithAgreeYOLO(true))
	if err != nil {
		return ""
	}
	return utils.InterfaceToString(result)
})

var RedirectForge = "redirect_forge"

func (s *Server) StartAITask(stream ypb.Yak_StartAITaskServer) error {
	firstMsg, err := stream.Recv()
	if err != nil {
		return utils.Errorf("recv first msg failed: %v", err)
	}

	if !firstMsg.IsStart {
		return utils.Error("first msg is not start")
	}
	startParams := firstMsg.Params

	baseCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	inputEvent := make(chan *aid.InputEvent, 1000)

	var currentCoordinatorId = startParams.CoordinatorId
	var coordinatorIdOnce sync.Once
	var aidOption = []aid.Option{
		aid.WithEventHandler(func(e *aid.Event) {
			if e.Timestamp <= 0 {
				e.Timestamp = time.Now().Unix() // fallback
			}
			coordinatorIdOnce.Do(func() {
				currentCoordinatorId = e.CoordinatorId
			})
			event := &ypb.AIOutputEvent{
				CoordinatorId:   e.CoordinatorId,
				Type:            string(e.Type),
				NodeId:          utils.EscapeInvalidUTF8Byte([]byte(e.NodeId)),
				IsSystem:        e.IsSystem,
				IsStream:        e.IsStream,
				IsReason:        e.IsReason,
				StreamDelta:     e.StreamDelta,
				IsJson:          e.IsJson,
				Content:         e.Content,
				Timestamp:       e.Timestamp,
				TaskIndex:       e.TaskIndex,
				DisableMarkdown: e.DisableMarkdown,
			}
			err := stream.Send(event)
			if err != nil {
				log.Errorf("send event failed: %v", err)
			}
		}),
		aid.WithEventInputChan(inputEvent),
	}
	aidOption = append(aidOption, buildAIDOption(startParams)...)

	go func() {
		defer cancel()
		for {
			event, err := stream.Recv()
			if err != nil {
				log.Errorf("receive event failed: %v", err)
				return
			}
			if event.IsSyncMessage {
				t, ok := aid.ParseSyncType(event.GetSyncType())
				if !ok {
					log.Errorf("parse sync type failed, got: %v", event.GetSyncType())
					continue
				}
				select {
				case inputEvent <- &aid.InputEvent{
					IsSyncInfo: true,
					SyncType:   t,
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
		res, err = yak.ExecuteForge(forgeName, params, buildAIAgentOption(baseCtx, startParams.GetCoordinatorId(), aidOption...)...)
		if err != nil {
			log.Errorf("run ai forge[%s] failed: %v", forgeName, err)
			return err
		}
	} else {
		triageCache.Push(utils.InterfaceToString(params))
		res, err = yak.ExecuteForge("forge_triage", map[string]any{
			"query":   params,
			"context": triageCache.Dump(),
		}, buildAIAgentOption(baseCtx, startParams.GetCoordinatorId(), aidOption...)...)
		if err != nil {
			log.Errorf("run ai forge[%s] failed: %v", forgeName, err)
			return err
		}
		if res != nil {
			var redirectParam = &ypb.AIStartParams{
				ForgeName: strings.ToLower(utils.InterfaceToString(res)),
			}
			redirectParamJson, err := json.Marshal(redirectParam)
			if err != nil {
				return err
			}
			err = stream.Send(&ypb.AIOutputEvent{
				CoordinatorId: currentCoordinatorId,
				Type:          RedirectForge,
				Content:       redirectParamJson,
				Timestamp:     time.Now().Unix(),
				IsJson:        true,
			})
		}
	}
	return nil
}

func buildAIAgentOption(ctx context.Context, CoordinatorId string, extendOption ...aid.Option) []any {
	agentOption := []any{
		yak.WithContext(ctx),
	}
	if CoordinatorId != "" {
		agentOption = append(agentOption, yak.WithCoordinatorId(CoordinatorId))
	}

	if len(extendOption) > 0 {
		agentOption = append(agentOption, yak.WithExtendAIDOptions(extendOption...))
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
		aidOption = append(aidOption, aid.WithAICallback(aid.AIChatToAICallbackType(ai.Chat)))
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
