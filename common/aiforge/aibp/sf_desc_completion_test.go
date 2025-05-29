package aibp

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"os"
	"testing"
)

func TestSFDescCompletion(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	fileName := `D:\GoProject\yaklang\common\syntaxflow\sfbuildin\buildin\java\cwe-22-path-travel\java-path-travel.sf`
	content, err := os.ReadFile(fileName)
	require.NoError(t, err)
	results, err := aiforge.ExecuteForge(
		"sf_desc_completion",
		context.Background(),
		[]*ypb.ExecParamItem{
			{
				Key: "file_name", Value: fileName,
			},
			{
				Key: "file_content", Value: string(content),
			},
		},
		aid.WithAgreeYOLO(true),
		aid.WithAICallback(aiforge.GetOpenRouterAICallbackWithProxy()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(results)
}
