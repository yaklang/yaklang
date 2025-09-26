package syntaxflow_scan

import (
	"io"
	"time"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type errorCallback func(string, ...any)

// ScanResult 扫描结果结构体，包含扫描任务的所有结果信息
type ScanResult struct {
	TaskID string                   // 任务ID，用于唯一标识扫描任务
	Status string                   // 任务状态："executing"执行中, "done"完成, "paused"暂停, "error"错误
	Result *ssaapi.SyntaxFlowResult // SyntaxFlow扫描结果，包含规则匹配的详细信息
}

// ScanResultCallback 扫描回调函数类型，用于处理扫描过程中产生的结果
// 回调函数会在扫描过程中被多次调用，每当有新的结果产生时都会触发
// 返回非nil错误将中止扫描过程
type ScanResultCallback func(*ScanResult)

type ScanTaskConfig struct {
	*ypb.SyntaxFlowScanRequest
	RuleNames []string `json:"rule_names"`

	// ttl
	ProcessMonitorTTL time.Duration

	ProcessCallback RuleProcessCallback `json:"-"`
	errorCallback   errorCallback       `json:"-"`
	resultCallback  ScanResultCallback  `json:"-"`
	// this function check if need pauseCheck,
	// /return true to pauseCheck, and no-blocking
	pauseCheck func() bool `json:"-"`

	Reporter       sfreport.IReport `json:"-"`
	ReporterWriter io.Writer        `json:"-"`
}

func NewScanConfig(options ...ScanOption) *ScanTaskConfig {
	config := &ScanTaskConfig{
		ProcessMonitorTTL: 30 * time.Second,
	}
	for _, option := range options {
		option(config)
	}
	return config
}

func WithPauseFunc(pause func() bool) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.pauseCheck = pause
	}
}

func WithScanResultCallback(callback ScanResultCallback) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.resultCallback = callback
	}
}

func WithErrorCallback(callback errorCallback) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.errorCallback = callback
	}
}

// ScanOption 定义扫描选项类型，用于配置SyntaxFlow扫描任务的各种参数
type ScanOption func(*ScanTaskConfig)

type ControlMode string

const (
	ControlModeStart  ControlMode = "start"
	ControlModeStatus ControlMode = "status"
	ControlModeResume ControlMode = "resume"
)

func WithControlMode(id ControlMode) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.ControlMode = string(id)
	}
}

func WithProjectId(id uint64) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.SSAProjectId = id
	}
}

func WithRawConfig(req *ypb.SyntaxFlowScanRequest) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.SyntaxFlowScanRequest = req
	}
}

// WithProgramNames 设置要扫描的程序名称，可以指定一个或多个程序进行扫描
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//
// )
// // 或扫描多个程序
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("program1", "program2"),
//
// )
// ```
func WithProgramNames(names ...string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.ProgramName = names
	}
}

// WithRuleNames 设置要使用的规则名称列表
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withRuleNames("sql-injection", "xss", "path-traversal"),
//
// )
// ```
func WithRuleNames(names ...string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.RuleNames = names
	}
}

// WithLanguages 设置要扫描的编程语言列表
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withLanguages("java", "javascript", "go"),
//
// )
// ```
func WithLanguages(languages ...string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.Language = languages
	}
}

// WithGroupNames 设置要使用的规则组名称列表
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withGroupNames("web-security", "api-security"),
//
// )
// ```
func WithGroupNames(groups ...string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.GroupNames = groups
	}
}

// WithSeverity 设置要扫描的漏洞严重程度列表
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withSeverity("high", "critical"),
//
// )
// ```
func WithSeverity(severity ...string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.Severity = severity
	}
}

// WithPurpose 设置规则用途列表，用于指定扫描的目的
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withPurpose("vulnerability", "audit"),
//
// )
// ```
func WithPurpose(purpose ...string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.Purpose = purpose
	}
}

// WithTags 设置规则标签列表
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withTags("owasp", "cwe-89", "sqli"),
//
// )
// ```
func WithTags(tags ...string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.Tag = tags
	}
}

// WithKeyword 设置关键词过滤，用于模糊匹配规则
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withKeyword("injection"),
//
// )
// ```
func WithKeyword(keyword string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.Keyword = keyword
	}
}

// WithIncludeLibraryRule 设置是否包含库规则，这些规则通常被其他规则引用
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withIncludeLibraryRule(true),
//
// )
// ```
func WithIncludeLibraryRule(include bool) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter.IncludeLibraryRule = include
	}
}

// withRuleInput 设置规则输入，用于调试模式时直接输入自定义规则内容（内部使用，不导出）
func withRuleInput(input *ypb.SyntaxFlowRuleInput) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.RuleInput = input
	}
}

// WithRuleFilter 设置规则过滤器，直接传入过滤器结构体（内部使用，保持向后兼容）
func WithRuleFilter(filter *ypb.SyntaxFlowRuleFilter) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.SyntaxFlowScanRequest.Filter == nil {
			sc.SyntaxFlowScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.SyntaxFlowScanRequest.Filter = filter
	}
}

// WithIgnoreLanguage 设置是否忽略语言匹配，当设置为true时，将运行所有规则而不考虑程序语言
// Example:
// ```
// // 忽略语言匹配，运行所有规则
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("my-program"),
//	syntaxflowscan.withIgnoreLanguage(true),
//	syntaxflowscan.withRuleFilter(filter),
//
// )
// ```
func WithIgnoreLanguage(ignore bool) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.IgnoreLanguage = ignore
	}
}

// WithConcurrency 设置并发数，控制同时执行的扫描任务数量，默认为5
// Example:
// ```
// // 设置高并发扫描
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("large-program"),
//	syntaxflowscan.withConcurrency(20),
//	syntaxflowscan.withRuleFilter(filter),
//
// )
// ```
func WithConcurrency(concurrency uint32) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.Concurrency = concurrency
	}
}

// WithMemory 设置是否在内存中编译数据，当设置为true时，程序数据将仅在内存中处理，不持久化到数据库
// Example:
// ```
// // 临时扫描，不保存到数据库
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProgramNames("temp-program"),
//	syntaxflowscan.withMemory(true),
//	syntaxflowscan.withRuleFilter(filter),
//
// )
// ```
func WithMemory(memory bool) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.Memory = memory
	}
}

func WithReporter(reporter sfreport.IReport) ScanOption {
	return func(config *ScanTaskConfig) {
		config.Reporter = reporter
	}
}

func WithReporterWriter(writer io.Writer) ScanOption {
	return func(config *ScanTaskConfig) {
		config.ReporterWriter = writer
	}
}

// WithResumeTaskId 设置要恢复的任务ID，用于恢复之前暂停的扫描任
// WithResumeTaskId 设置要恢复的任务ID，用于恢复之前暂停的扫描任务
// Example:
// ```
// taskId := "task-123-456"
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withResumeTaskId(taskId),
//
// )
// ```
func WithResumeTaskId(taskId string) ScanOption {
	return func(sc *ScanTaskConfig) {
		if sc.SyntaxFlowScanRequest == nil {
			sc.SyntaxFlowScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.SyntaxFlowScanRequest.ResumeTaskId = taskId
	}
}

// WithProcessCallback 设置扫描进度回调函数
func WithProcessCallback(callback RuleProcessCallback) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.ProcessCallback = callback
	}
}

func WithProcessMonitorTTL(ttl time.Duration) ScanOption {
	return func(sc *ScanTaskConfig) {
		sc.ProcessMonitorTTL = ttl
	}
}
