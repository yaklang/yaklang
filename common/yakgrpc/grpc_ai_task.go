package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiforge"
	_ "github.com/yaklang/yaklang/common/aiforge/aibp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

type aiChatType func(string, ...aispec.AIConfigOption) (string, error)

var mockedAIChat aiChatType = nil

func RegisterMockAIChat(c aiChatType) {
	mockedAIChat = c
}

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

	var opts = []aid.Option{
		aid.WithEventHandler(func(e *aid.Event) {
			if e.Timestamp <= 0 {
				e.Timestamp = time.Now().Unix() // fallback
			}
			event := &ypb.AIOutputEvent{
				CoordinatorId: e.CoordinatorId,
				Type:          string(e.Type),
				NodeId:        utils.EscapeInvalidUTF8Byte([]byte(e.NodeId)),
				IsSystem:      e.IsSystem,
				IsStream:      e.IsStream,
				IsReason:      e.IsReason,
				StreamDelta:   e.StreamDelta,
				IsJson:        e.IsJson,
				Content:       e.Content,
				Timestamp:     e.Timestamp,
			}
			err := stream.Send(event)
			if err != nil {
				log.Errorf("send event failed: %v", err)
			}
		}),
		aid.WithEventInputChan(inputEvent),
	}

	if startParams.GetEnableSystemFileSystemOperator() {
		opts = append(opts, aid.WithSystemFileOperator())
		opts = append(opts, aid.WithJarOperator())
	}

	if startParams.GetUseDefaultAIConfig() && startParams.GetForgeName() != "" {
		opts = append(opts, aid.WithAICallback(aid.AIChatToAICallbackType(ai.Chat)))
	}

	if mockedAIChat != nil {
		opts = append(opts, aid.WithAICallback(aid.AIChatToAICallbackType(mockedAIChat)))
	}

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

	forgeName := startParams.GetForgeName()
	if forgeName != "" {
		log.Infof("==========AI Forge Loading for %v ======", forgeName)
		log.Infof("==========AI Forge Loading for %v ======", forgeName)
		log.Infof("==========AI Forge Loading for %v ======", forgeName)

		if forgeName == "netscan" {
			log.Info("use deepseek-r1 for netscan plan")
			opts = append(opts, aid.WithPlanAICallback(aiforge.GetQwenAICallback("deepseek-r1")))
			opts = append(opts, aid.WithAICallback(aiforge.GetOpenRouterAICallback()))
		} else {
			opts = append(opts, aid.WithAICallback(aiforge.GetOpenRouterAICallbackGemini2_5flash()))
		}

		_, err := aiforge.ExecuteForge(forgeName, stream.Context(), []*ypb.ExecParamItem{
			{Key: "query", Value: startParams.GetUserQuery()},
		}, opts...)
		if err != nil {
			log.Errorf("aiforge exec forge [%v] failed: %v", forgeName, err.Error())
			cancel()
			return nil
		}
	} else {
		engine, err := aid.NewCoordinatorContext(baseCtx, startParams.GetUserQuery(), opts...)
		if err != nil {
			return utils.Errorf("create coordinator failed: %v", err)
		}
		err = engine.Run()
		if err != nil {
			log.Errorf("run coordinator failed: %v", err)
			return utils.Errorf("run coordinator failed: %v", err)
		}
	}
	return nil
}
