package chatglm

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"
)

func TestBasic(t *testing.T) {
	_ = yak.NewScriptEngine(0)
	yakit.LoadGlobalNetworkConfig()

	if utils.InGithubActions() {
		t.Skip()
		return
	}

	api := &ModelAPI{
		Model: "",
		Prompt: []map[string]any{
			{
				"你好": "世界",
			},
		},
	}
	key := consts.GetThirdPartyApplicationConfig("chatglm").APIKey
	spew.Dump(key)
	results, err := api.Invoke(key)
	if err != nil {
		panic(err)
	}
	spew.Dump(results)
}
