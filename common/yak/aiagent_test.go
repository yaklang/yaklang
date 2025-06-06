package yak

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBuildInForge(t *testing.T) {
	yakit.InitialDatabase()
	res, err := ExecuteForge("long_text_summarizer",
		[]*ypb.ExecParamItem{
			//{Key: "text", Value: monOncleJules},
			{Key: "filePath", Value: "我的叔叔于勒.txt"},
		},
		WithAICallback(aiforge.GetHoldAICallback()),
		WithExtendAIDOptions(
			aid.WithDebugPrompt(true),
		),
	)
	require.NoError(t, err)
	spew.Dump(res)

	//res, err := ExecuteForge("yaklang_writer",
	//	"写一个目录扫描脚本",
	//	aid.WithDebugPrompt(true),
	//	aid.WithAICallback(aiforge.GetHoldAICallback()),
	//	aid.WithAgreeYOLO(true),
	//)
	//require.NoError(t, err)
	//spew.Dump(res)
}

func TestReducerAI(t *testing.T) {
	yakit.InitialDatabase()
	raw, err := os.ReadFile("我的叔叔于勒.txt")
	require.NoError(t, err)
	memory := aid.GetDefaultMemory()

	key := "前情提要"
	reducer, err := aireducer.NewReducerFromString(
		string(raw),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			textSnippet := string(chunk.Data())
			preData, _ := memory.GetPersistentData(key)
			if preData != "" {
				textSnippet = key + " : " + preData + "\n" + textSnippet
			}
			res, err := ExecuteForge("fragment_summarizer",
				[]*ypb.ExecParamItem{
					{
						Key: "textSnippet", Value: textSnippet,
					}, {
						Key: "limit", Value: "1000",
					},
				},
				WithAICallback(aiforge.GetHoldAICallback()),
				WithExtendAIDOptions(
					aid.WithDebugPrompt(true),
				),
				WithDisallowRequireForUserPrompt(),
			)
			if err != nil {
				return err
			}
			memory.SetPersistentData(key, utils.InterfaceToString(res))
			spew.Dump(res)
			return nil
		}),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)

	spew.Dump(memory.GetPersistentData(key))

}

func TestReducerAI2(t *testing.T) {
	yakit.InitialDatabase()
	raw, err := os.ReadFile("我的叔叔于勒.txt")
	require.NoError(t, err)
	memory := aid.GetDefaultMemory()

	reducer, err := aireducer.NewReducerFromString(
		string(raw),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			textSnippet := string(chunk.Data())
			res, err := ExecuteForge("biography",
				[]*ypb.ExecParamItem{
					{
						Key: "text", Value: textSnippet,
					},
				},
				WithAICallback(aiforge.GetHoldAICallback()),
				WithExtendAIDOptions(
					aid.WithDebugPrompt(true),
				),
				WithDisallowRequireForUserPrompt(),
				WithMemory(memory.CopyReducibleMemory()),
				WithDebug(true),
			)
			if err != nil {
				return err
			}
			memory.ApplyOp(res.(*aiforge.ForgeResult).Action)
			return nil
		}),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)
	fmt.Println(memory.PersistentMemory())
}

func TestReducerIntentRecognition(t *testing.T) {
	yakit.InitialDatabase()
	raw := "我想做渗透测试\n\n可能需要用到xss攻击。\n\nsql注入\n\n还是不测xss了"

	ctx := context.Background()
	cod, err := aid.NewCoordinatorContext(ctx, "", aid.WithAICallback(aiforge.GetHoldAICallback()))
	require.NoError(t, err)
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

	reducer, err := aireducer.NewReducerFromString(
		raw,
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.Memory, chunk chunkmaker.Chunk) error {
			query := string(chunk.Data())
			go func() {
				//subCtx, cancel := context.WithCancel(ctx)
				//defer cancel()
				res, err := ExecuteForge("intent_recognition",
					query,
					WithAICallback(aiforge.GetHoldAICallback()),
					WithDisallowRequireForUserPrompt(),
					WithMemory(memory),
				)
				if err != nil {
					log.Errorf("ExecuteForge: %v", err)
					return
				}

				resString := utils.InterfaceToString(res)
				fmt.Println(resString)
				if resString != "" {
					forgeList, err := searchHandler(resString, getForge())
					if err != nil {
						log.Errorf("searchHandler: %v", err)
						return
					}
					//var opts []*aid.RequireInteractiveRequestOption
					for idx, opt := range forgeList {
						_ = idx
						fmt.Printf("%d\t%s:[%s]\n", idx, opt.ForgeName, opt.Description)
						//opts = append(opts, &aid.RequireInteractiveRequestOption{
						//	Index:  idx,
						//	Prompt: opt.ForgeName,
						//})
					}
					//param, _, err := cod.GetConfig().RequireUserPromptWithEndpointResultEx(subCtx, "")
					//if err != nil {
					//	return
					//}
					//spew.Dump(param)
				}
			}()
			memory.PushUserInteraction(aid.UserInteractionStage_FreeInput, cod.GetConfig().AcquireId(), "", query) // push user input timeline
			return nil
		}),
		aireducer.WithSeparatorTrigger("\n\n"),
		aireducer.WithContext(ctx),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)
	select {}
}
