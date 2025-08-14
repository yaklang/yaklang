package aiforge

import (
	"bytes"
	"encoding/json"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
)

func MockAICallbackByRecord(content []byte) aicommon.AICallbackType {
	var chatRecord []string
	json.Unmarshal(content, &chatRecord)
	currentPairId := 0
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()
		reqPrompt := chatRecord[currentPairId]
		rspPrompt := chatRecord[currentPairId+1]
		_ = reqPrompt
		rsp.EmitOutputStream(strings.NewReader(rspPrompt))
		currentPairId += 2
		rsp.Close()
		return rsp, nil
	}
}
func AICallbackRecorder(callback aicommon.AICallbackType, fileName string) (aicommon.AICallbackType, func()) {
	chatRecord := []string{}
	saveToFile := func() {
		f, err := os.Create(fileName)
		if err != nil {
			log.Errorf("aiCallbackRecorder: create file error: %v", err)
			return
		}
		defer f.Close()
		json.NewEncoder(f).Encode(chatRecord)
	}
	return func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()
		reader, writer := io.Pipe()
		rsp.EmitOutputStream(reader)
		go func() {
			defer func() {
				writer.Close()
				rsp.Close()
			}()
			originRsp, err := callback(config, req)
			if err != nil {
				log.Errorf("aiCallbackRecorder: callback error: %v", err)
			}
			outputReader := originRsp.GetOutputStreamReader("output", false, config.GetEmitter())
			outputBuf := bytes.Buffer{}
			io.Copy(&outputBuf, io.TeeReader(outputReader, writer))
			chatRecord = append(chatRecord, req.GetPrompt(), outputBuf.String())
		}()
		return rsp, nil
	}, saveToFile
}

func getTestSuiteAICallback(fileName string, opts []aispec.AIConfigOption, typeName string, modelName ...string) aicommon.AICallbackType {
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
	aiCallback := func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		rsp := config.NewAIResponse()
		go func() {
			defer rsp.Close()
			//fmt.Println(req.GetPrompt())
			for _, data := range req.GetImageList() {
				if data.IsBase64 {
					opts = append(opts, aispec.WithImageBase64(string(data.Data)))
				} else {
					opts = append(opts, aispec.WithImageRaw(data.Data))
				}
			}
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

func GetOpenRouterAICallback(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.0-flash-001"}
	}
	return getTestSuiteAICallback("openrouter.txt", nil, "openrouter", modelName...)
}

func GetOpenRouterAICallbackGemini2_5flash(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.5-flash-preview"}
	}
	return getTestSuiteAICallback("openrouter.txt", nil, "openrouter", modelName...)
}

func GetOpenRouterAICallbackFree(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.0-flash-exp:free"}
	}
	return getTestSuiteAICallback("openrouter.txt", nil, "openrouter", modelName...)
}

func GetHoldAICallback(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"gemini-2.0-flash"}
	}
	return getTestSuiteAICallback("holdai.txt", []aispec.AIConfigOption{aispec.WithDomain("api.holdai.top")}, "openai", modelName...)
}

func GetOpenRouterAICallbackWithProxy(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"google/gemini-2.0-flash-001"}
	}
	return getTestSuiteAICallback("openrouter.txt", []aispec.AIConfigOption{
		aispec.WithProxy("http://127.0.0.1:10808"),
	}, "openrouter", modelName...)
}

func GetGLMAICallback(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"glm-4-flash"}
	}
	return getTestSuiteAICallback("chatglm.txt", nil, "chatglm", modelName...)
}

func GetQwenAICallback(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"qwq-32b"}
	}
	return getTestSuiteAICallback("tongyi-apikey.txt", nil, "tongyi", modelName...)
}

func GetAIBalance(modelName ...string) aicommon.AICallbackType {
	if len(modelName) == 0 {
		modelName = []string{"deepseek-v3"}
	}
	return getTestSuiteAICallback("aibalance.txt", nil, "aibalance", modelName...)
}

func GetCliValueByKey(key string, items []*ypb.ExecParamItem) string {
	for _, i := range items {
		if i.Key == key {
			return i.Value
		}
	}
	return ""
}

func Any2ExecParams(i any) []*ypb.ExecParamItem {
	// covert params to ypb.ExecParamItem
	var params []*ypb.ExecParamItem
	if p, ok := i.([]*ypb.ExecParamItem); ok {
		return p
	} else if utils.IsMap(i) {
		for k, v := range utils.InterfaceToGeneralMap(i) {
			params = append(params, &ypb.ExecParamItem{
				Key:   k,
				Value: utils.InterfaceToString(v),
			})
		}
	} else {
		params = append(params, &ypb.ExecParamItem{
			Key:   "query",
			Value: utils.InterfaceToString(i),
		})
	}
	return params
}
