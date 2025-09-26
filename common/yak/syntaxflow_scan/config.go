package syntaxflow_scan

import (
	"context"
	"io"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type ScanStream interface {
	Recv() (*ypb.SyntaxFlowScanRequest, error)
	Send(*ypb.SyntaxFlowScanResponse) error
	Context() context.Context
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

type Option func(*Config) error

type Config struct {
	ProgramNames []string
	ProjectName  string

	Config *ssaconfig.Config

	ResultCallback  ScanCallback
	ProcessCallback func(progress float64)

	reporter       sfreport.IReport
	reporterWriter io.Writer
}

func WithSyntaxFlowScanConfig(config *ssaconfig.Config) Option {
	return func(c *Config) error {
		if !config.IsSyntaxFlowScanConfig() {
			return utils.Error("Invalid SyntaxFlow Scan Config: should use NewSyntaxFlowScanConfig to create config")
		}
		projectName, programNames := config.GetProjectName(), config.GetProgramNames()
		if projectName == "" || len(programNames) == 0 {
			return utils.Error("Invalid SyntaxFlow Scan Config: should set program names or project name")
		}
		c.ProjectName = projectName
		c.ProgramNames = programNames
		c.Config = config
		return nil
	}
}

func WithSyntaxFlowScanResultCallback(callback ScanCallback) Option {
	return func(c *Config) error {
		c.ResultCallback = callback
		return nil
	}
}

func WithSyntaxFlowScanProcessCallback(callback func(progress float64)) Option {
	return func(c *Config) error {
		c.ProcessCallback = callback
		return nil
	}
}

func WithSyntaxFlowScanReporter(reporter sfreport.IReport) Option {
	return func(c *Config) error {
		c.reporter = reporter
		return nil
	}
}

func WithSyntaxFlowScanReporterWriter(writer io.Writer) Option {
	return func(c *Config) error {
		c.reporterWriter = writer
		return nil
	}
}
