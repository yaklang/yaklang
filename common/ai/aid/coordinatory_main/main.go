package main

import (
	"bytes"
	"fmt"
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
	aiCallback := func(req *aid.AIRequest) (*aid.AIResponse, error) {
		rsp := aid.NewAIResponse()
		defer rsp.Close()
		fmt.Println(req.GetPrompt())
		_, err := ai.Chat(
			req.GetPrompt(),
			aispec.WithStreamHandler(func(c io.Reader) {
				var buf bytes.Buffer
				c = io.TeeReader(c, &buf)
				go func() {
					io.Copy(os.Stdout, c)
				}()
				rsp.EmitOutputStream(&buf)
			}),
			aispec.WithReasonStreamHandler(func(c io.Reader) {
				var buf bytes.Buffer
				c = io.TeeReader(c, &buf)
				go func() {
					io.Copy(os.Stdout, c)
				}()
				rsp.EmitReasonStream(&buf)
			}),
			aispec.WithType("tongyi"),
			aispec.WithModel("qwen-plus"),
			aispec.WithAPIKey(string(apikey)),
			// aispec.WithDomain("api.siliconflow.cn"),
		)
		if err != nil {
			return nil, err
		}
		return rsp, nil
	}

	coordinator, err := aid.NewCoordinator(
		"找出 /tmp 目录中最大的文件",
		aid.WithAICallback(aiCallback),
		aid.WithTools(aid.GetAllMockTools()...),
		aid.WithSystemFileOperator(),
	)
	if err := coordinator.Run(); err != nil {
		panic(err)
	}
}
