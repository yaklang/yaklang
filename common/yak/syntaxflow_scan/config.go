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

var WithScanResultCallback = ssaconfig.SetOption(resultCallbackKey, func(c *Config, callback ScanResultCallback) {
	c.resultCallback = callback
})

var WithErrorCallback = ssaconfig.SetOption(errorCallbackKey, func(c *Config, callback errorCallback) {
	c.errorCallback = callback
})

var WithProcessCallback = ssaconfig.SetOption(processCallbackKey, func(c *Config, callback ProcessCallback) {
	c.ProcessCallback = callback
})

var withPrograms = ssaconfig.SetOption("syntaxflow-scan/programs", func(c *Config, progs ssaapi.Programs) {
	c.ScanTaskCallback.Programs = progs
})

func WithPrograms(programs ...*ssaapi.Program) ssaconfig.Option {
	p := ssaapi.Programs(programs)
	return withPrograms(p)
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

