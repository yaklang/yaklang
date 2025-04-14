package aid

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"text/template"

	"github.com/yaklang/yaklang/common/utils"
)

type planRequest struct {
	config   *Config
	rawInput string
}

func (pr *planRequest) callAI(request *AIRequest) (*AIResponse, error) {
	for _, cb := range []AICallbackType{
		pr.config.planAICallback,
		pr.config.coordinatorAICallback,
		pr.config.taskAICallback,
	} {
		if cb == nil {
			continue
		}
		return cb(pr.config, request)
	}
	return nil, utils.Error("no any ai callback is set, cannot found ai config")
}

type PlanResponse struct {
	RootTask *aiTask `json:"root_task"`
}

// GenerateFirstPlanPrompt 根据PlanRequest生成prompt
func (pr *planRequest) GenerateFirstPlanPrompt() (string, error) {
	tmpl, err := template.New("generateTaskList").Parse(__prompt_GenerateTaskListPrompt)
	if err != nil {
		return "", err
	}

	// 准备模板数据
	data := map[string]interface{}{
		"Memory": pr.config.memory,
	}

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func (pr *Config) newPlanResponse(rootTask *aiTask) *PlanResponse {
	pr.SetSyncCallback(SYNC_TYPE_PLAN, func() any {
		return rootTask
	})
	return &PlanResponse{
		RootTask: rootTask,
	}
}

// Invoke 执行规划请求，调用AI生成任务列表并返回解析后的Task
func (pr *planRequest) Invoke() (*PlanResponse, error) {
	if pr.config.planMocker != nil {
		planRes := pr.config.planMocker(pr.config)
		if utils.IsNil(planRes) {
			return nil, utils.Error("planMocker returns nil, unknown error")
		}
		return planRes, nil
	}
	// 生成 Prompt
	prompt, err := pr.GenerateFirstPlanPrompt()
	if err != nil {
		return nil, fmt.Errorf("生成规划 prompt 失败: %v", err)
	}

	// 调用 AI 回调函数
	responseReader, err := pr.callAI(NewAIRequest(prompt))
	if err != nil {
		return nil, fmt.Errorf("调用 AI 服务失败: %v", err)
	}

	// 读取响应内容
	responseBytes, err := io.ReadAll(responseReader.GetOutputStreamReader("plan", false, pr.config))
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败: %v", err)
	}
	response := string(responseBytes)

	// 从响应中提取任务
	task, err := ExtractTaskFromRawResponse(pr.config, response)
	if err != nil {
		return nil, fmt.Errorf("从 AI 响应中提取任务失败: %v", err)
	}
	return pr.config.newPlanResponse(task), nil
}

func (c *Coordinator) createPlanRequest(rawUserInput string) (*planRequest, error) {
	req, err := createPlanRequest(rawUserInput)
	if err != nil {
		return nil, err
	}
	req.config = c.config
	req.rawInput = rawUserInput
	return req, nil
}

func createPlanRequest(rawUserInput string) (*planRequest, error) {
	request := &planRequest{
		rawInput: rawUserInput,
	}
	return request, nil
}
