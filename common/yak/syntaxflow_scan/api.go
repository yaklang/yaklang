package syntaxflow_scan

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
)

// ScanOption 定义扫描选项类型，用于配置SyntaxFlow扫描任务的各种参数
type ScanOption func(*scanInputConfig)

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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.ScanRequest.ProgramName = names
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.RuleNames = names
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.Language = languages
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.GroupNames = groups
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.Severity = severity
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.Purpose = purpose
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.Tag = tags
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.Keyword = keyword
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter.IncludeLibraryRule = include
	}
}

// withRuleInput 设置规则输入，用于调试模式时直接输入自定义规则内容（内部使用，不导出）
func withRuleInput(input *ypb.SyntaxFlowRuleInput) ScanOption {
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.ScanRequest.RuleInput = input
	}
}

// WithRuleFilter 设置规则过滤器，直接传入过滤器结构体（内部使用，保持向后兼容）
func WithRuleFilter(filter *ypb.SyntaxFlowRuleFilter) ScanOption {
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		if sc.ScanRequest.Filter == nil {
			sc.ScanRequest.Filter = &ypb.SyntaxFlowRuleFilter{}
		}
		sc.ScanRequest.Filter = filter
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.ScanRequest.IgnoreLanguage = ignore
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.ScanRequest.Concurrency = concurrency
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.ScanRequest.Memory = memory
	}
}

func WithReporter(reporter sfreport.IReport) ScanOption {
	return func(config *scanInputConfig) {
		config.Reporter = reporter
	}
}

func WithReporterWriter(writer io.Writer) ScanOption {
	return func(config *scanInputConfig) {
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
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.ScanRequest.ResumeTaskId = taskId
	}
}

// WithProcessCallback 设置扫描进度回调函数
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withProcessCallback(func(progress ) {
//		println("扫描进度:", progress)
//	}),
//
// )
// ```
func WithProcessCallback(callback func(progress float64)) ScanOption {
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.ProcessCallback = callback
	}
}

// WithRuleProcessCallback 设置规则进度回调函数
// Example:
// ```
// syntaxflowscan.StartScan(context.New(), callback,
//
//	syntaxflowscan.withRuleProcessCallback(func(progName, ruleName , progress ) {
//		println("规则进度:", progName, ruleName, progress)
//	}),
//
// )
// ```
func WithRuleProcessCallback(callback func(progName, ruleName string, progress float64)) ScanOption {
	return func(sc *scanInputConfig) {
		if sc.ScanRequest == nil {
			sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
		}
		sc.RuleProcessCallback = callback
	}
}

// ScanResult 扫描结果结构体，包含扫描任务的所有结果信息
type ScanResult struct {
	TaskID     string                // 任务ID，用于唯一标识扫描任务
	Status     string                // 任务状态："executing"执行中, "done"完成, "paused"暂停, "error"错误
	ExecResult *ypb.ExecResult       // 执行结果，包含执行过程中的输出信息
	Result     *ypb.SyntaxFlowResult // SyntaxFlow扫描结果，包含规则匹配的详细信息
	Risks      []*ypb.Risk           // 风险列表，包含发现的安全风险
	SSARisks   []*ypb.SSARisk        // SSA风险列表，包含静态分析发现的风险
}

// ScanCallback 扫描回调函数类型，用于处理扫描过程中产生的结果
// 回调函数会在扫描过程中被多次调用，每当有新的结果产生时都会触发
// 返回非nil错误将中止扫描过程
type ScanCallback func(*ScanResult) error

// ScanStreamImpl 实现扫描流接口
type ScanStreamImpl struct {
	ctx          context.Context
	requestChan  chan *ypb.SyntaxFlowScanRequest
	callbackFunc ScanCallback
}

func newScanStreamImpl(ctx context.Context, callback ScanCallback) *ScanStreamImpl {
	return &ScanStreamImpl{
		ctx:          ctx,
		requestChan:  make(chan *ypb.SyntaxFlowScanRequest, 1),
		callbackFunc: callback,
	}
}

func (s *ScanStreamImpl) Recv() (*ypb.SyntaxFlowScanRequest, error) {
	select {
	case req := <-s.requestChan:
		return req, nil
	case <-s.ctx.Done():
		return nil, s.ctx.Err()
	}
}

func (s *ScanStreamImpl) Send(resp *ypb.SyntaxFlowScanResponse) error {
	if s.callbackFunc == nil {
		return nil
	}
	result := &ScanResult{
		TaskID:     resp.TaskID,
		Status:     resp.Status,
		ExecResult: resp.ExecResult,
		Result:     resp.Result,
		Risks:      resp.Risks,
		SSARisks:   resp.SSARisks,
	}

	return s.callbackFunc(result)
}

func (s *ScanStreamImpl) Context() context.Context {
	return s.ctx
}

// StartScan 启动新的SyntaxFlow扫描任务，使用options模式配置扫描参数
//
// 参数:
//   - ctx: 上下文，用于控制扫描任务的生命周期
//   - callback: 回调函数，用于处理扫描结果
//   - opts: 可变数量的选项函数，用于配置扫描参数
//
// 返回值:
//   - error: 如果启动失败则返回错误信息
//
// Example:
// ```
// // 基础扫描示例
//
//	err := syntaxflowscan.StartScan(context.New(), func(result) {
//	    println("任务ID:", result.TaskID)
//	    println("状态:", result.Status)
//	    if result.Risks && len(result.Risks) > 0 {
//	        for _, risk := range result.Risks {
//	            println("发现风险:", risk.Title)
//	        }
//	    }
//	    return nil
//	},
//
//	syntaxflowscan.withProgramNames("my-java-project"),
//	syntaxflowscan.withRuleNames("sql-injection", "xss"),
//	syntaxflowscan.withSeverity("high", "critical"),
//	syntaxflowscan.withConcurrency(10),
//
// )
// die(err)
//
// // 多程序扫描示例
//
//	err := syntaxflowscan.StartScan(context.New(), func(result) {
//	    yakit.Info("扫描进度: %s", result.Status)
//	    return nil
//	},
//
//	syntaxflowscan.withProgramNames("frontend", "backend", "api"),
//	syntaxflowscan.withLanguages("javascript", "java", "go"),
//	syntaxflowscan.withKeyword("security"),
//	syntaxflowscan.withMemory(true),
//
// )
// ```
func StartScan(ctx context.Context, callback ScanCallback, opts ...ScanOption) error {
	req := &ypb.SyntaxFlowScanRequest{
		ControlMode: "start",
		Concurrency: 5,
	}
	sc := &scanInputConfig{ScanRequest: req}
	for _, opt := range opts {
		opt(sc)
	}

	if len(sc.GetScanRequest().GetProgramName()) == 0 {
		return utils.Error("program names are required")
	}
	if sc.GetScanRequest().GetFilter() == nil && sc.GetScanRequest().GetRuleInput() == nil {
		return utils.Error("either rule filter or rule input must be provided")
	}
	stream := newScanStreamImpl(ctx, callback)
	stream.requestChan <- sc.GetScanRequest()
	close(stream.requestChan)
	return ScanWithConfig(stream, sc)
}

// ResumeScan 恢复之前暂停的扫描任务
//
// 参数:
//   - ctx: 上下文，用于控制扫描任务的生命周期
//   - taskId: 要恢复的任务ID
//   - callback: 回调函数，用于处理扫描结果
//
// 返回值:
//   - error: 如果恢复失败则返回错误信息
//
// Example:
// ```
// // 恢复之前暂停的扫描任务
// taskId := "previous-task-12345"
//
//	err := syntaxflowscan.ResumeScan(context.New(), taskId, func(result) {
//	    println("恢复扫描 - 任务ID:", result.TaskID)
//	    println("当前状态:", result.Status)
//	    if result.Status == "done" {
//	        println("扫描已完成！")
//	    }
//	    return nil
//	})
//
// die(err)
// ```
func ResumeScan(ctx context.Context, taskId string, callback ScanCallback) error {
	return StartScan(ctx, callback,
		func(sc *scanInputConfig) {
			if sc.ScanRequest == nil {
				sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
			}
			sc.ScanRequest.ControlMode = "resume"
			sc.ScanRequest.ResumeTaskId = taskId
		},
	)
}

// GetScanStatus 查询扫描任务的当前状态
//
// 参数:
//   - ctx: 上下文，用于控制查询的生命周期
//   - taskId: 要查询的任务ID
//   - callback: 回调函数，用于处理状态查询结果
//
// 返回值:
//   - error: 如果查询失败则返回错误信息
//
// Example:
// ```
// // 查询扫描任务状态
// taskId := "running-task-67890"
//
//	err := syntaxflowscan.GetScanStatus(context.New(), taskId, func(result) {
//	    println("任务状态查询:")
//	    println("  任务ID:", result.TaskID)
//	    println("  当前状态:", result.Status)
//	    if result.ExecResult {
//	        println("  执行信息:", result.ExecResult.Message)
//	    }
//	    return nil
//	})
//
// die(err)
// ```
func GetScanStatus(ctx context.Context, taskId string, callback ScanCallback) error {
	return StartScan(ctx, callback,
		func(sc *scanInputConfig) {
			if sc.ScanRequest == nil {
				sc.ScanRequest = &ypb.SyntaxFlowScanRequest{}
			}
			sc.ScanRequest.ControlMode = "status"
			sc.ScanRequest.ResumeTaskId = taskId
		},
	)
}
