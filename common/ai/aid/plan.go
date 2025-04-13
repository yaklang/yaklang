package aid

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"text/template"

	"github.com/yaklang/yaklang/common/utils"
)

// 常用元数据键名常量
const (
	MetaInfoKey       = "MetaInfo"       // 基本元信息
	CurrentTimeKey    = "CurrentTime"    // 当前时间
	FrameworkKey      = "Framework"      // 框架信息
	LanguageKey       = "Language"       // 编程语言
	EnvironmentKey    = "Environment"    // 环境信息
	TargetPlatformKey = "TargetPlatform" // 目标平台
	APIVersionKey     = "APIVersion"     // API版本
	DbTypeKey         = "DbType"         // 数据库类型
	SecurityLevelKey  = "SecurityLevel"  // 安全级别要求
	PerformanceKey    = "Performance"    // 性能要求
	DeadlineKey       = "Deadline"       // 截止日期
	BudgetKey         = "Budget"         // 预算
	UserLevelKey      = "UserLevel"      // 用户技术水平
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

type planResponse struct {
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

func (pr *Config) newPlanResponse(rootTask *aiTask) *planResponse {
	pr.SetSyncCallback(SYNC_TYPE_PLAN, func() any {
		return rootTask
	})
	return &planResponse{
		RootTask: rootTask,
	}
}

// Invoke 执行规划请求，调用AI生成任务列表并返回解析后的Task
func (pr *planRequest) Invoke() (*planResponse, error) {
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
	task, err := pr.extractTaskFromRawResponse(response)
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
