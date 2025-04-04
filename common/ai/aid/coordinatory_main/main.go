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
)

func main() {
	if utils.InGithubActions() {
		return
	}

	keyPath := filepath.Join(consts.GetDefaultYakitBaseDir(), "tongyi-apikey.txt")
	apikey, err := os.ReadFile(keyPath)
	if err != nil {
		panic(err)
	}
	if string(apikey) == "" {
		panic("apikey is empty")
	}
	consts.InitializeYakitDatabase("", "")
	log.Infof("apikey for tongyi: %v", string(apikey))
	log.Infof("primary ai engien: %v", consts.GetAIPrimaryType())
	aiCallback := func(config *aid.Config, req *aid.AIRequest) (*aid.AIResponse, error) {
		rsp := config.NewAIResponse()
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
				aispec.WithType("tongyi"),
				aispec.WithModel("qwen-plus"),
				aispec.WithAPIKey(string(apikey)),
				// aispec.WithDomain("api.siliconflow.cn"),
			)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
		}()
		return rsp, nil
	}

	coordinator, err := aid.NewCoordinator(
		"找出 /Users/v1ll4n/Projects/yaklang 目录中最大的文件",
		aid.WithAICallback(aiCallback),
		aid.WithTools(aid.GetAllMockTools()...),
		aid.WithSystemFileOperator(),
		aid.WithDebugPrompt(),
		aid.WithAutoAgree(true),
	)
	if err != nil {
		panic(err)
	}
	if err := coordinator.Run(); err != nil {
		panic(err)
	}
}
