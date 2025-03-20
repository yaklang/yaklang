package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/taskstack"
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
	aiCallback := func(req *taskstack.AIRequest) (*taskstack.AIResponse, error) {
		rsp := taskstack.NewAIResponse()
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

	coordinator := taskstack.NewCoordinator(
		"查询北京天气，帮我规划今天一天去一个最推荐的地方旅游，两步规划",
		taskstack.WithPlan_AICallback(aiCallback),
		taskstack.WithCoordinator_Tool(taskstack.GetAllMockTools()...),
		taskstack.WithCoordinator_AICallback(aiCallback),
	)
	if err := coordinator.Run(); err != nil {
		panic(err)
	}
}
