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

func NewGLMMessage(msg string) *ModelAPI {
	var api = &ModelAPI{
		Model: "glm-4",
		Prompt: []map[string]any{
			{"role": "user", "content": msg},
		},
	}
	return api
}

const (
	BaseURL    = "https://open.bigmodel.cn/api/paas/v4/chat/completions"
	APITimeout = 300 * time.Second
)

func (m ModelAPI) Invoke(apiKey string) (map[string]interface{}, error) {
	token, err := generateToken(apiKey)
	if err != nil {
		return nil, err
	}
	return post(BaseURL, token, m.buildParams(), APITimeout)
}

func (m ModelAPI) buildParams() map[string]interface{} {
	params := make(map[string]interface{})
	if m.Model == "" {
		m.Model = "glm-4"
	}
	params["model"] = m.Model
	params["messages"] = m.Prompt
	return params
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
