package aiforge

import (
	"io"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

func getTestSuiteAICallback(fileName string, opts []aispec.AIConfigOption, typeName string, modelName ...string) aid.AICallbackType {
	var model string
	if len(modelName) > 0 {
		model = modelName[0]
	}

	if model == "" {
		log.Errorf("getTestSuiteAICallback: model name is empty")
	}

	keyPath := filepath.Join(consts.GetDefaultYakitBaseDir(), fileName)
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
			opts = append(opts, aispec.WithStreamHandler(func(c io.Reader) {
				rsp.EmitOutputStream(c)
			}),
				aispec.WithReasonStreamHandler(func(c io.Reader) {
					rsp.EmitReasonStream(c)
				}),
				aispec.WithType(typeName),
				aispec.WithModel(model),
				aispec.WithAPIKey(string(apikey)))
			_, err := ai.Chat(req.GetPrompt(), opts...)
			if err != nil {
				log.Errorf("chat error: %v", err)
			}
		}()
		return rsp, nil
	}
	return aiCallback
}

func GetOpenRouterAICallback(modelName ...string) aid.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.0-flash-001"}
	}
	return getTestSuiteAICallback("openrouter.txt", nil, "openrouter", modelName...)
}

func GetOpenRouterAICallbackGemini2_5flash(modelName ...string) aid.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.5-flash-preview"}
	}
	return getTestSuiteAICallback("openrouter.txt", nil, "openrouter", modelName...)
}

func GetOpenRouterAICallbackFree(modelName ...string) aid.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.0-flash-exp:free"}
	}
	return getTestSuiteAICallback("openrouter.txt", nil, "openrouter", modelName...)
}

func GetHoldAICallback(modelName ...string) aid.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"gemini-2.0-flash"}
	}
	return getTestSuiteAICallback("holdai.txt", []aispec.AIConfigOption{aispec.WithDomain("api.holdai.top")}, "openai", modelName...)
}

func GetOpenRouterAICallbackWithProxy(modelName ...string) aid.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.0-flash-001"}
	}
	return getTestSuiteAICallback("openrouter.txt", []aispec.AIConfigOption{
		aispec.WithProxy("http://127.0.0.1:10808"),
	}, "openrouter", modelName...)
}

func GetGLMAICallback(modelName ...string) aid.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"glm-4-flash"}
	}
	return getTestSuiteAICallback("chatglm.txt", nil, "chatglm", modelName...)
}

func GetQwenAICallback(modelName ...string) aid.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"qwq-32b"}
	}
	return getTestSuiteAICallback("tongyi-apikey.txt", nil, "tongyi", modelName...)
}
