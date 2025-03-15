package taskstack

import (
	"bytes"
	_ "embed"
	"fmt"
	"text/template"
	"time"
)

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
	MetaData map[string]any
	Query    string
}

type PlanResponse struct {
	RootTask *Task
}

type PlanOption func(*PlanRequest)

// 添加查询选项
func WithQuery(query string) PlanOption {
	return func(pr *PlanRequest) {
		pr.Query = query
	}
}

// 添加元数据选项
func WithMetaData(key string, value any) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[key] = value
	}
}

// 添加当前时间
func WithCurrentTime() PlanOption {
	return func(pr *PlanRequest) {
		now := time.Now()
		pr.MetaData[CurrentTimeKey] = now.Format("2006-01-02 15:04:05")
	}
}

// 添加基本元信息
func WithMetaInfo(info string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[MetaInfoKey] = info
	}
}

// 添加框架信息
func WithFramework(framework string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[FrameworkKey] = framework
	}
}

// 添加编程语言
func WithLanguage(language string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[LanguageKey] = language
	}
}

// 添加环境信息
func WithEnvironment(env string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[EnvironmentKey] = env
	}
}

// 添加目标平台
func WithTargetPlatform(platform string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[TargetPlatformKey] = platform
	}
}

// 添加API版本
func WithAPIVersion(version string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[APIVersionKey] = version
	}
}

// 添加数据库类型
func WithDbType(dbType string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[DbTypeKey] = dbType
	}
}

// 添加安全级别要求
func WithSecurityLevel(level string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[SecurityLevelKey] = level
	}
}

// 添加性能要求
func WithPerformance(performance string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[PerformanceKey] = performance
	}
}

// 添加截止日期
func WithDeadline(deadline string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[DeadlineKey] = deadline
	}
}

// 添加预算
func WithBudget(budget string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[BudgetKey] = budget
	}
}

// 添加用户技术水平
func WithUserLevel(level string) PlanOption {
	return func(pr *PlanRequest) {
		pr.MetaData[UserLevelKey] = level
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

func CreatePlanRequest(query string, opts ...PlanOption) (*PlanRequest, error) {
	request := &PlanRequest{
		MetaData: map[string]any{},
		Query:    query,
	}

	// 添加默认元数据 - 当前时间
	request.MetaData[CurrentTimeKey] = time.Now().Format("2006-01-02 15:04:05")

	// 应用其他选项
	for _, opt := range opts {
		opt(request)
	}

	return request, nil
}
