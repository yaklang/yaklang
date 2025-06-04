package yak

import (
	"fmt"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/utils"
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
			action := res.(*aiforge.ForgeResult).Action
			memory.ModifyMemoryFromOpList(action.GetInvokeParams("params").GetObjectArray("memory_op")...)
			return nil
		}),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)
	fmt.Println(memory.PersistentMemory())
}
