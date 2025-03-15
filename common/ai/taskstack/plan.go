package taskstack

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"text/template"
	"time"

	"github.com/yaklang/yaklang/common/ai"
)

// AICallback 定义AI调用回调函数类型，输入为提示语字符串，输出为响应reader和错误
type AICallback func(prompt string) (io.Reader, error)

//go:embed prompts/generate-tasklist.txt
var generateTaskListPrompt string

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

type PlanRequest struct {
	MetaData   map[string]any
	Query      string
	AICallback AICallback // AI回调函数
}

type PlanResponse struct {
	RootTask *Task
}

type PlanOption func(*PlanRequest)

// 添加查询选项
func WithPlan_Query(query string) PlanOption {
	return func(pr *PlanRequest) {
		pr.Query = query
	}
}

// 添加元数据选项
func WithPlan_MetaData(key string, value any) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[key] = value
	}
}

// 添加当前时间
func WithPlan_CurrentTime() PlanOption {
	return func(pr *PlanRequest) {
		now := time.Now()
		pr.MetaData[CurrentTimeKey] = now.Format("2006-01-02 15:04:05")
	}
}

// 添加基本元信息
func WithPlan_MetaInfo(info string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[MetaInfoKey] = info
	}
}

// 添加框架信息
func WithPlan_Framework(framework string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[FrameworkKey] = framework
	}
}

// 添加编程语言
func WithPlan_Language(language string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[LanguageKey] = language
	}
}

// 添加环境信息
func WithPlan_Environment(env string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[EnvironmentKey] = env
	}
}

// 添加目标平台
func WithPlan_TargetPlatform(platform string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[TargetPlatformKey] = platform
	}
}

// 添加API版本
func WithPlan_APIVersion(version string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[APIVersionKey] = version
	}
}

// 添加数据库类型
func WithPlan_DbType(dbType string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[DbTypeKey] = dbType
	}
}

// 添加安全级别要求
func WithPlan_SecurityLevel(level string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[SecurityLevelKey] = level
	}
}

// 添加性能要求
func WithPlan_Performance(performance string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[PerformanceKey] = performance
	}
}

// 添加截止日期
func WithPlan_Deadline(deadline string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[DeadlineKey] = deadline
	}
}

// 添加预算
func WithPlan_Budget(budget string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[BudgetKey] = budget
	}
}

// 添加用户技术水平
func WithPlan_UserLevel(level string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[UserLevelKey] = level
	}
}

// WithPlan_AICallback 设置AI回调函数
func WithPlan_AICallback(callback AICallback) PlanOption {
	return func(pr *PlanRequest) {
		pr.AICallback = callback
	}
}

// GeneratePrompt 根据PlanRequest生成prompt
func (pr *PlanRequest) GeneratePrompt() (string, error) {
	tmpl, err := template.New("generateTaskList").Parse(generateTaskListPrompt)
	if err != nil {
		return "", err
	}

	// 准备模板数据
	data := map[string]interface{}{
		"TaskJsonSchema": taskJsonSchema,
		"Query":          pr.Query,
		"MetaInfo":       "",
	}

	// 构建综合元信息字符串
	metaInfoBuilder := &bytes.Buffer{}

	// 如果元数据中有MetaInfo，首先添加它
	if metaInfo, ok := pr.MetaData[MetaInfoKey]; ok {
		if metaInfoStr, isStr := metaInfo.(string); isStr && metaInfoStr != "" {
			metaInfoBuilder.WriteString(metaInfoStr)
			metaInfoBuilder.WriteString("\n\n")
		}
	}

	// 添加当前时间
	if currentTime, ok := pr.MetaData[CurrentTimeKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("当前时间: %v\n", currentTime))
	}

	// 添加框架信息
	if framework, ok := pr.MetaData[FrameworkKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("使用框架: %v\n", framework))
	}

	// 添加编程语言
	if language, ok := pr.MetaData[LanguageKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("编程语言: %v\n", language))
	}

	// 添加环境信息
	if env, ok := pr.MetaData[EnvironmentKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("运行环境: %v\n", env))
	}

	// 添加目标平台
	if platform, ok := pr.MetaData[TargetPlatformKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("目标平台: %v\n", platform))
	}

	// 添加API版本
	if version, ok := pr.MetaData[APIVersionKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("API版本: %v\n", version))
	}

	// 添加数据库类型
	if dbType, ok := pr.MetaData[DbTypeKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("数据库类型: %v\n", dbType))
	}

	// 添加安全级别要求
	if level, ok := pr.MetaData[SecurityLevelKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("安全级别要求: %v\n", level))
	}

	// 添加性能要求
	if performance, ok := pr.MetaData[PerformanceKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("性能要求: %v\n", performance))
	}

	// 添加截止日期
	if deadline, ok := pr.MetaData[DeadlineKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("截止日期: %v\n", deadline))
	}

	// 添加预算
	if budget, ok := pr.MetaData[BudgetKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("预算: %v\n", budget))
	}

	// 添加用户技术水平
	if level, ok := pr.MetaData[UserLevelKey]; ok {
		metaInfoBuilder.WriteString(fmt.Sprintf("用户技术水平: %v\n", level))
	}

	// 添加其他自定义元数据
	for key, value := range pr.MetaData {
		// 跳过已处理的预定义元数据键
		if key == MetaInfoKey || key == CurrentTimeKey || key == FrameworkKey ||
			key == LanguageKey || key == EnvironmentKey || key == TargetPlatformKey ||
			key == APIVersionKey || key == DbTypeKey || key == SecurityLevelKey ||
			key == PerformanceKey || key == DeadlineKey || key == BudgetKey || key == UserLevelKey {
			continue
		}

		metaInfoBuilder.WriteString(fmt.Sprintf("%s: %v\n", key, value))
	}

	// 设置合并后的元信息
	data["MetaInfo"] = metaInfoBuilder.String()

	// 渲染模板
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// Invoke 执行规划请求，调用AI生成任务列表并返回解析后的Task
func (pr *PlanRequest) Invoke() (*PlanResponse, error) {
	// 检查回调函数是否设置
	if pr.AICallback == nil {
		return nil, fmt.Errorf("未设置AI回调函数")
	}

	// 生成 Prompt
	prompt, err := pr.GeneratePrompt()
	if err != nil {
		return nil, fmt.Errorf("生成规划 prompt 失败: %v", err)
	}

	// 调用 AI 回调函数
	responseReader, err := pr.AICallback(prompt)
	if err != nil {
		return nil, fmt.Errorf("调用 AI 服务失败: %v", err)
	}

	// 读取响应内容
	responseBytes, err := io.ReadAll(responseReader)
	if err != nil {
		return nil, fmt.Errorf("读取 AI 响应失败: %v", err)
	}
	response := string(responseBytes)

	// 从响应中提取任务
	task, err := ExtractTaskFromRawResponse(response)
	if err != nil {
		return nil, fmt.Errorf("从 AI 响应中提取任务失败: %v", err)
	}

	// 将AICallback转换为TaskAICallback并设置给Task
	taskCallback := TaskAICallback(pr.AICallback)
	task.SetAICallback(taskCallback)

	return &PlanResponse{
		RootTask: task,
	}, nil
}

// DefaultAICallback 默认的AI回调函数，使用ai.Chat实现
func DefaultAICallback(prompt string) (io.Reader, error) {
	response, err := ai.Chat(prompt)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(response), nil
}

func CreatePlanRequest(query string, opts ...PlanOption) (*PlanRequest, error) {
	request := &PlanRequest{
		MetaData:   map[string]any{},
		Query:      query,
		AICallback: DefaultAICallback, // 设置默认的AI回调函数
	}

	// 添加默认元数据 - 当前时间
	request.MetaData[CurrentTimeKey] = time.Now().Format("2006-01-02 15:04:05")

	// 应用其他选项
	for _, opt := range opts {
		opt(request)
	}

	return request, nil
}
