package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func TestTaskStack(t *testing.T) {
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
	aiCallback := func(req *aid.AIRequest) (*aid.AIResponse, error) {
		rsp := aid.NewAIResponse()
		defer rsp.Close()

		fmt.Println("-----------Response------------")
		_, err := ai.Chat(
			req.GetPrompt(),
			aispec.WithDebugStream(true),
			aispec.WithReasonStreamHandler(func(c io.Reader) {
				var buf bytes.Buffer
				c = io.TeeReader(c, &buf)
				go func() {
					io.Copy(os.Stdout, c)
				}()
				rsp.EmitReasonStream(&buf)
			}),
			aispec.WithStreamHandler(func(c io.Reader) {
				var buf bytes.Buffer
				c = io.TeeReader(c, &buf)
				go func() {
					io.Copy(os.Stdout, c)
				}()
				rsp.EmitOutputStream(&buf)
			}),
			aispec.WithType("openai"),
			aispec.WithModel("Pro/deepseek-ai/DeepSeek-V3"),
			aispec.WithAPIKey(string(apikey)),
			aispec.WithDomain("api.siliconflow.cn"),
		)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	}

	coordinator, err := aid.NewCoordinator(
		"帮我规划一个一家三口北京的一日游，子任务控制三步内",
		aid.WithAICallback(aiCallback),
		aid.WithTools(aid.GetAllMockTools()...),
	)
	if err != nil {
		panic(err)
	}
	if err := coordinator.Run(); err != nil {
		panic(err)
	}
}
