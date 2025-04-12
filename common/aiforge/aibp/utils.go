package aibp

import (
	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"os"
	"path/filepath"
)

func GetTestSuiteAICallback(modelName ...string) aid.AICallbackType {
	var model string = "qwq-plus"
	if len(modelName) > 0 {
		model = modelName[0]
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
				aispec.WithModel(model),
				aispec.WithAPIKey(string(apikey)),
				// aispec.WithDomain("api.siliconflow.cn"),
			)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
		}()
		return rsp, nil
	}
	return aiCallback
}
