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
}

const (
	pauseFuncKey          = "pauseFunc"
	resultCallbackKey     = "resultCallback"
	errorCallbackKey      = "errorCallback"
	processCallbackKey    = "processCallback"
	reporterKey           = "reporter"
	reporterWriterKey     = "reporterWriter"
	processRuleKey        = "processRuleDetail"
	rulePerformanceLogKey = "rulePerformanceLog"
)

func WithReporter(reporter sfreport.IReport) ssaconfig.Option {
	return func(sc *ssaconfig.Config) error {
		sc.SetExtraInfo(reporterKey, reporter)
		return nil
	}
}

func WithPauseFunc(pause func() bool) ssaconfig.Option {
	return func(sc *ssaconfig.Config) error {
		sc.SetExtraInfo(pauseFuncKey, pause)
		return nil
	}
}

func WithScanResultCallback(callback ScanResultCallback) ssaconfig.Option {
	return func(sc *ssaconfig.Config) error {
		sc.SetExtraInfo(resultCallbackKey, callback)
		return nil
	}
}

func WithErrorCallback(callback errorCallback) ssaconfig.Option {
	return func(sc *ssaconfig.Config) error {
		sc.SetExtraInfo(errorCallbackKey, callback)
		return nil
	}
}

// WithProcessCallback 设置扫描进度回调函数
func WithProcessCallback(callback ProcessCallback) ssaconfig.Option {
	return func(sc *ssaconfig.Config) error {
		sc.SetExtraInfo(processCallbackKey, callback)
		return nil
	}
}

// WithProcessRuleDetail 控制进度回调是否包含规则级别详情
func WithProcessRuleDetail(withRule bool) ssaconfig.Option {
	return func(sc *ssaconfig.Config) error {
		sc.SetExtraInfo(processRuleKey, withRule)
		return nil
	}
}

// WithRulePerformanceLog 控制是否开启规则级性能日志
func WithRulePerformanceLog(enable bool) ssaconfig.Option {
	return func(sc *ssaconfig.Config) error {
		sc.SetExtraInfo(rulePerformanceLogKey, enable)
		return nil
	}
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

	if f, ok := cfg.ExtraInfo[pauseFuncKey]; ok {
		if pauseFunc, ok := f.(func() bool); ok {
			cfg.pauseCheck = pauseFunc
		}
	}

	if f, ok := cfg.ExtraInfo[resultCallbackKey]; ok {
		if resultCallback, ok := f.(ScanResultCallback); ok {
			cfg.resultCallback = resultCallback
		}
	}

	if f, ok := cfg.ExtraInfo[errorCallbackKey]; ok {
		if errorCallback, ok := f.(errorCallback); ok {
			cfg.errorCallback = errorCallback
		}
	}

	if f, ok := cfg.ExtraInfo[processCallbackKey]; ok {
		if processCallback, ok := f.(ProcessCallback); ok {
			cfg.ProcessCallback = processCallback
		}
	}

	if f, ok := cfg.ExtraInfo[processRuleKey]; ok {
		if withRule, ok := f.(bool); ok {
			cfg.ProcessWithRule = withRule
		}
	}

	if f, ok := cfg.ExtraInfo[reporterKey]; ok {
		if reporter, ok := f.(sfreport.IReport); ok {
			cfg.Reporter = reporter
		}
	}

	if f, ok := cfg.ExtraInfo[rulePerformanceLogKey]; ok {
		if enable, ok := f.(bool); ok {
			cfg.EnableRulePerformanceLog = enable
			if cfg.ScanTaskCallback != nil {
				cfg.ScanTaskCallback.EnableRulePerformanceLog = enable
			}
		}
	}

	return cfg, nil
}
