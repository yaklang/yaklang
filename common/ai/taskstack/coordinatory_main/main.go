package main

import (
	"bytes"
	"fmt"
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
			aispec.WithModel("qwen-max"),
			aispec.WithAPIKey(string(apikey)),
		)
		if err != nil {
			return nil, err
		}
		fmt.Println(result)
		return bytes.NewBufferString(result), nil
	}

	coordinator := taskstack.NewCoordinator(
		"帮我规划一个一家三口北京的一日游，子任务控制三步内",
		taskstack.WithPlan_AICallback(aiCallback),
		taskstack.WithCoordinator_Tool(taskstack.GetAllMockTools()...),
	)
	if err := coordinator.Run(); err != nil {
		panic(err)
	}
}
