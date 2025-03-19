package main

import (
	"bytes"
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/taskstack"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if utils.InGithubActions() {
		return
	}

	keyPath := filepath.Join(consts.GetDefaultYakitBaseDir(), "tongyi-apikey.txt")
	apikey, err := os.ReadFile(keyPath)
	if err != nil {
		panic(err)
		return
	}
	if string(apikey) == "" {
		panic("apikey is empty")
		return
	}
	consts.InitializeYakitDatabase("", "")
	log.Infof("apikey for tongyi: %v", string(apikey))
	log.Infof("primary ai engien: %v", consts.GetAIPrimaryType())
	aiCallback := func(prompt string) (io.Reader, error) {
		log.Infof("start to chat with AI with input: %v", prompt)
		result, err := ai.Chat(
			prompt,
			aispec.WithDebugStream(true),
			aispec.WithType("tongyi"),
			aispec.WithModel("qwq-plus"),
			aispec.WithAPIKey(string(apikey)),
		)
		if err != nil {
			return nil, err
		}
		return bytes.NewBufferString(result), nil
	}

	coordinator := taskstack.NewCoordinator(
		"查询北京天气，帮我规划今天一天去一个最推荐的地方旅游，两步规划",
		taskstack.WithPlan_AICallback(aiCallback),
		taskstack.WithCoordinator_Tool(taskstack.GetAllMockTools()...),
	)
	if err := coordinator.Run(); err != nil {
		panic(err)
	}
}
