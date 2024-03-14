package chatglm

import (
	"fmt"
	"os"
	"time"
)

type ModelAPI struct {
	Model       string
	Prompt      []map[string]interface{}
	TopP        float64
	Temperature float64
}

const (
	BaseURL         = "https://open.bigmodel.cn/api/paas/v3/model-api"
	InvokeTypeSync  = "invoke"
	InvokeTypeAsync = "async-invoke"
	InvokeTypeSSE   = "sse-invoke"
	APITimeout      = 300 * time.Second
	ChatGLMLite     = "chatglm_lite"
	ChatGLMStd      = "chatglm_std"
	ChatGLMPro      = "chatglm_pro"
)

func (m ModelAPI) Invoke(apiKey string) (map[string]interface{}, error) {
	token, err := generateToken(apiKey)
	if err != nil {
		return nil, err
	}
	return post(buildAPIURL(m.Model, InvokeTypeSync), token, m.buildParams(), APITimeout)
}

func (m ModelAPI) AsyncInvoke(apiKey string) (map[string]interface{}, error) {
	token, err := generateToken(apiKey)
	if err != nil {
		return nil, err
	}
	return post(buildAPIURL(m.Model, InvokeTypeAsync), token, m.buildParams(), APITimeout)
}

func QueryAsyncInvokeResult(apiKey, taskID string) (map[string]interface{}, error) {
	token, err := generateToken(apiKey)
	if err != nil {
		return nil, err
	}
	return get(buildGetAPIURL(InvokeTypeAsync, taskID), token, APITimeout)
}

func (m ModelAPI) buildParams() map[string]interface{} {
	params := make(map[string]interface{})
	params["prompt"] = m.Prompt
	params["top_p"] = m.TopP
	params["temperature"] = m.Temperature
	return params
}

func buildAPIURL(module, invokeMethod string) string {
	url := getBaseURL()
	return fmt.Sprintf("%s/%s/%s", url, module, invokeMethod)
}

func buildGetAPIURL(invokeMethod, taskID string) string {
	url := getBaseURL()
	return fmt.Sprintf("%s/-/%s/%s", url, invokeMethod, taskID)
}

func getBaseURL() string {
	var url string
	url = os.Getenv("ZHIPUAI_MODEL_API_URL")
	if url == "" {
		url = BaseURL
	}
	return url
}
