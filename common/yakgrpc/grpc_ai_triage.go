package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

var (
	Triage_Event_Log        string = "triage_log"
	Triage_Event_Forge_List string = "triage_forge_list"
	Triage_Event_Finish     string = "triage_finish"
)

func (s *Server) StartAITriage(stream ypb.Yak_StartAITriageServer) error {
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

	sendEvent := func(e *schema.AiOutputEvent) {
		if e.Timestamp <= 0 {
			e.Timestamp = time.Now().Unix() // fallback
		}
		err := stream.Send(e.ToGRPC())
		if err != nil {
			log.Errorf("send event failed: %v", err)
		}
	}

	inputEvent := make(chan *aid.InputEvent, 1000)
	var aidOption = []aid.Option{
		aid.WithEventHandler(func(e *schema.AiOutputEvent) {
			sendEvent(e)
		}),
		aid.WithEventInputChan(inputEvent),
	}
	aidOption = append(aidOption, buildAIDOption(startParams)...)

	freeInputChan := chanx.NewUnlimitedChan[chunkmaker.Chunk](baseCtx, 1000)
	seqClear := regexp.MustCompile("\n\n+")

	go func() {
		defer cancel()
		for {
			event, err := stream.Recv()
			if err != nil {
				log.Errorf("receive event failed: %v", err)
				return
			}
			if event.IsFreeInput {
				content := strings.TrimSpace(event.FreeInput)
				if content == "" {
					continue
				}
				content = seqClear.ReplaceAllString(content, "\n")
				content = content + "\n\n"
				chunk := chunkmaker.NewBufferChunk([]byte(content))
				freeInputChan.SafeFeed(chunk)
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

	cod, err := aid.NewCoordinatorContext(baseCtx, "", aidOption...)
	if err != nil {
		return err
	}

	memory := cod.GetConfig().GetMemory()
	searchHandler := func(query string, searchList []*schema.AIForge) ([]*schema.AIForge, error) {
		keywords := omap.NewOrderedMap[string, []string](nil)
		forgeMap := map[string]*schema.AIForge{}
		for _, forge := range searchList {
			keywords.Set(forge.GetName(), forge.GetKeywords())
			forgeMap[forge.GetName()] = forge
		}
		searchResults, err := cod.GetConfig().HandleSearch(query, keywords)
		if err != nil {
			return nil, err
		}
		forges := []*schema.AIForge{}
		for _, result := range searchResults {
			forges = append(forges, forgeMap[result.Key])
		}
		return forges, nil
	}

	getForge := func() []*schema.AIForge {
		forgeList, err := yakit.GetAllAIForge(consts.GetGormProfileDatabase())
		if err != nil {
			log.Errorf("yakit.GetAllAIForge: %v", err)
			return nil
		}
		return forgeList
	}

	emitEvent := func(nodeId string, content any) {
		sendEvent(&schema.AiOutputEvent{
			Type:     schema.EVENT_TYPE_STREAM,
			NodeId:   nodeId,
			IsSystem: true,
			Content:  utils.InterfaceToBytes(content),
		})
	}
	reducer, err := aireducer.NewReducerFromInputChunk(
		freeInputChan,
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			query := strings.TrimSpace(string(chunk.Data()))
			memory.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, cod.GetConfig().AcquireId(), "", query) // push user input timeline
			defer emitEvent(Triage_Event_Finish, []byte("意图识别完成"))
			emitEvent(Triage_Event_Log, []byte(fmt.Sprintf("正在识别意图：%s", query)))
			res, err := yak.ExecuteForge("intent_recognition",
				query,
				append(
					buildAIAgentOption(baseCtx, startParams.GetCoordinatorId(), sendEvent, aidOption...),
					yak.WithMemory(memory),
					yak.WithDisallowRequireForUserPrompt(),
					yak.WithContext(baseCtx))...)
			if err != nil {
				log.Errorf("ExecuteForge: %v", err)
				return nil
			}

			intent, ok := res.(aitool.InvokeParams)
			if !ok {
				log.Errorf("yak.ExecuteForge: %v", res)
				return nil
			}
			detailIntention := intent.GetString("detail_intention")
			intentAssertion := intent.GetString("assertion")
			keywords := intent.GetString("keywords")
			emitEvent(Triage_Event_Log, []byte(fmt.Sprintf("当前意图：%s\n理由：%s\n关键词：%s", intentAssertion, detailIntention, keywords)))

			if intentAssertion != "" {
				emitEvent(Triage_Event_Log, []byte(fmt.Sprintf("搜索关联aiforge")))
				forgeList, err := searchHandler(intentAssertion, getForge())
				if err != nil {
					log.Errorf("searchHandler: %v", err)
					return nil
				}

				forgeName := lo.Map(forgeList, func(item *schema.AIForge, _ int) string {
					return item.GetName()
				})

				if len(forgeName) > 0 {
					emitEvent(Triage_Event_Forge_List, map[string]any{
						"forge_list": forgeName,
						"keywords":   keywords,
					})
				}
			}
			return nil
		}),
		aireducer.WithSeparatorTrigger("\n\n"),
		aireducer.WithContext(baseCtx),
		aireducer.WithMemory(memory),
	)
	if err != nil {
		return err
	}
	err = reducer.Run()
	if err != nil {
		return err
	}
	return nil
}
