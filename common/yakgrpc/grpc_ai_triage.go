package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
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
	"regexp"
	"strings"
	"time"
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

	inputEvent := make(chan *aid.InputEvent, 1000)
	var aidOption = []aid.Option{
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
				TaskIndex:     e.TaskIndex,
			}
			err := stream.Send(event)
			if err != nil {
				log.Errorf("send event failed: %v", err)
			}
		}),
		aid.WithEventInputChan(inputEvent),
	}
	aidOption = append(aidOption, buildAIDOption(startParams)...)
	aidOption = append(aidOption, aid.WithAICallback(aiforge.GetHoldAICallback()))

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
			forges = append(forges, forgeMap[result.Tool])
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

	reducer, err := aireducer.NewReducerEx(
		freeInputChan,
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			query := string(chunk.Data())
			go func() {
				subCtx, cancel := context.WithCancel(baseCtx)
				defer cancel()
				res, err := yak.ExecuteForge("intent_recognition",
					query,
					yak.WithAICallback(aiforge.GetHoldAICallback()),
					yak.WithDisallowRequireForUserPrompt(),
					yak.WithMemory(memory),
					yak.WithContext(subCtx),
				)
				if err != nil {
					log.Errorf("ExecuteForge: %v", err)
					return
				}

				resString := utils.InterfaceToString(res)
				//fmt.Println(resString)
				if resString != "" {
					forgeList, err := searchHandler(resString, getForge())
					if err != nil {
						log.Errorf("searchHandler: %v", err)
						return
					}
					var opts []*aid.RequireInteractiveRequestOption
					for idx, opt := range forgeList {
						//fmt.Printf("%d\t%s:[%s]\n", idx, opt.ForgeName, opt.Description)
						opts = append(opts, &aid.RequireInteractiveRequestOption{
							Index:       idx,
							PromptTitle: opt.ForgeName,
							Prompt:      opt.Description,
						})
					}
					param, _, err := cod.GetConfig().RequireUserPromptWithEndpointResultEx(subCtx, "suggest forge for you", opts...)
					if err != nil {
						return
					}
					_ = param // param is the selected forge option
					//spew.Dump(param)
				}
			}()
			memory.PushUserInteraction(aid.UserInteractionStage_FreeInput, cod.GetConfig().AcquireId(), "", query) // push user input timeline
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
