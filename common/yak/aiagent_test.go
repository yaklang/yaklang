package yak

import (
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
			{Key: "filePath", Value: "C:\\Users\\Rookie\\home\\code\\yaklang\\common\\aiforge\\aisecretary\\long_text_summarizer_data\\我的叔叔于勒.txt"},
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
