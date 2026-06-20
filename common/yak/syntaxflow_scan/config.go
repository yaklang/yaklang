package syntaxflow_scan

import (
	"io"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type Config struct {
	*ssaconfig.Config
	*ScanTaskCallback `json:"-"`
}
type errorCallback func(string, string, string, ...any)

type ProcessCallback func(taskID, status string, progress float64, info *RuleProcessInfoList)

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

type ScanTaskCallback struct {
	ProcessCallback ProcessCallback `json:"-"`

	errorCallback  errorCallback      `json:"-"`
	resultCallback ScanResultCallback `json:"-"`
	// this function check if need pauseCheck,
	// /return true to pauseCheck, and no-blocking

	pauseCheck func() bool `json:"-"`

	Reporter       sfreport.IReport `json:"-"`
	ReporterWriter io.Writer        `json:"-"`

	// EnableRulePerformanceLog 是否启用规则级别的详细性能日志
	// 默认为 false，只显示任务级别的性能统计（编译时间等）
	// 设置为 true 时，会显示每个规则在每个程序上的详细执行时间
	EnableRulePerformanceLog bool `json:"-"`
	ProcessWithRule          bool `json:"-"`

	Programs []*ssaapi.Program `json:"-"`
}

const (
	pauseFuncKey       = "syntaxflow-scan/pauseFunc"
	resultCallbackKey  = "syntaxflow-scan/resultCallback"
	errorCallbackKey   = "syntaxflow-scan/errorCallback"
	processCallbackKey = "syntaxflow-scan/processCallback"
	reporterKey        = "syntaxflow-scan/reporter"
)

var WithReporter = ssaconfig.SetOption(reporterKey, func(c *Config, reporter sfreport.IReport) {
	c.Reporter = reporter
})

var WithPauseFunc = ssaconfig.SetOption(pauseFuncKey, func(c *Config, pause func() bool) {
	c.pauseCheck = pause
})

var withScanResultCallbackOption = ssaconfig.SetOption(resultCallbackKey, func(c *Config, callback ScanResultCallback) {
	c.resultCallback = callback
})

// WithScanResultCallback 设置扫描结果回调（导出名为 syntaxflow.withScanResultCallback）
// 每产生一个扫描结果时触发回调，作为 syntaxflow.StartScan 的可选项使用
//
// 参数:
//   - callback: 每产生一个扫描结果时触发的回调函数
//
// 返回值:
//   - 扫描配置可选项，可传入 syntaxflow.StartScan
//
// Example:
// ```
// opt = syntaxflow.withScanResultCallback((result) => { dump(result) })
// assert opt != nil, "withScanResultCallback should return a scan option"
// ```
func WithScanResultCallback(callback ScanResultCallback) ssaconfig.Option {
	return withScanResultCallbackOption(callback)
}

var WithErrorCallback = ssaconfig.SetOption(errorCallbackKey, func(c *Config, callback errorCallback) {
	c.errorCallback = callback
})

var withProcessCallbackOption = ssaconfig.SetOption(processCallbackKey, func(c *Config, callback ProcessCallback) {
	c.ProcessCallback = callback
})

// WithProcessCallback 设置扫描进度回调（导出名为 syntaxflow.withScanProcessCallback）
// 扫描进度变化时触发回调，作为 syntaxflow.StartScan 的可选项使用
//
// 参数:
//   - callback: 进度回调函数，参数含任务 ID、状态、进度比例与额外信息
//
// 返回值:
//   - 扫描配置可选项，可传入 syntaxflow.StartScan
//
// Example:
// ```
// opt = syntaxflow.withScanProcessCallback((taskID, status, progress, info) => { println(status) })
// assert opt != nil, "withScanProcessCallback should return a scan option"
// ```
func WithProcessCallback(callback ProcessCallback) ssaconfig.Option {
	return withProcessCallbackOption(callback)
}

var WithProcessRuleDetail = ssaconfig.SetOption("syntaxflow-scan/processRuleDetail", func(c *Config, withDetail bool) {
	c.ProcessWithRule = withDetail
})

var WithRulePerformanceLog = ssaconfig.SetOption("syntaxflow-scan/enableRulePerformanceLog", func(c *Config, enable bool) {
	c.EnableRulePerformanceLog = enable
})

var withProgramsOption = ssaconfig.SetOption("syntaxflow-scan/programs", func(c *Config, progs ssaapi.Programs) {
	c.ScanTaskCallback.Programs = progs
})

// withPrograms 指定本次扫描要覆盖的程序集合（导出名为 syntaxflow.withScanPrograms）
// 作为 syntaxflow.StartScan 的可选项，限定扫描在指定的已编译程序上进行
//
// 参数:
//   - progs: 待扫描的程序集合（已编译程序的列表）
//
// 返回值:
//   - 扫描配置可选项，可传入 syntaxflow.StartScan
//
// Example:
// ```
// prog = ssa.Parse(`a = 1; println(a)`)~
// opt = syntaxflow.withScanPrograms([prog])
// assert opt != nil, "withScanPrograms should return a scan option"
// ```
func withPrograms(progs ssaapi.Programs) ssaconfig.Option {
	return withProgramsOption(progs)
}

func WithPrograms(programs ...*ssaapi.Program) ssaconfig.Option {
	p := ssaapi.Programs(programs)
	return withProgramsOption(p)
}

func NewConfig(opts ...ssaconfig.Option) (*Config, error) {
	cfg := &Config{
		ScanTaskCallback: &ScanTaskCallback{},
	}
	var err error
	cfg.Config, err = ssaconfig.New(ssaconfig.ModeSyntaxFlowScan, opts...)
	if err != nil {
		return nil, err
	}

	ssaconfig.ApplyExtraOptions(cfg, cfg.Config)
	return cfg, nil
}

func (c *Config) IsEnableRulePerformanceLog() bool {
	if c == nil {
		return false
	}
	if c.ScanTaskCallback != nil {
		return c.ScanTaskCallback.EnableRulePerformanceLog
	}
	return false
}
