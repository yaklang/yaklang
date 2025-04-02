package main

import (
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func main() {
	if utils.InGithubActions() {
		return
	}

	apikey, proxy := tryLoadApiKeyWithProxy()
	if apikey == "" {
		panic("not found apikey")
	}
	consts.InitializeYakitDatabase("", "")
	log.Infof("apikey for tongyi: %v", string(apikey))
	log.Infof("primary ai engien: %v", consts.GetAIPrimaryType())
	aiCallback := func(req *aid.AIRequest) (*aid.AIResponse, error) {
		rsp := aid.NewAIResponse()
		go func() {
			defer rsp.Close()
			//fmt.Println(req.GetPrompt())
			_, err := ai.Chat(
				req.GetPrompt(),
				aispec.WithStreamHandler(func(c io.Reader) {
					rsp.EmitOutputStream(c)
				}),
				aispec.WithReasonStreamHandler(func(c io.Reader) {
					rsp.EmitReasonStream(c)
				}),
				aispec.WithType("openai"),
				aispec.WithModel("gpt-4o-mini"),
				aispec.WithAPIKey(string(apikey)),
				aispec.WithProxy(proxy),
				// aispec.WithDomain("api.siliconflow.cn"),
			)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
		}()
		return rsp, nil
	}

	coordinator, err := aid.NewCoordinator(
		"帮我找出/Users/z3/Downloads/h5-graph-lite.jar文件中的MANIFEST.MF文件的内容，并帮我总结一下这个jar包的关键信息",
		aid.WithAICallback(aiCallback),
		aid.WithTools(aid.GetAllMockTools()...),
		aid.WithSystemFileOperator(),
		aid.WithJarOperator(),
		aid.WithDebugPrompt(),
	)
	if err != nil {
		panic(err)
	}
	if err := coordinator.Run(); err != nil {
		panic(err)
	}
}

func tryLoadApiKeyWithProxy() (string, string) {
	keyPath := filepath.Join(consts.GetDefaultYakitBaseDir(), "tongyi-apikey.txt")
	apikeyBytes, err := os.ReadFile(keyPath)
	if err == nil {
		return string(apikeyBytes), ""
	}
	yakit.LoadGlobalNetworkConfig()
	cfg := aispec.NewDefaultAIConfig(aispec.WithType("openai"))
	return cfg.APIKey, cfg.Proxy
}
