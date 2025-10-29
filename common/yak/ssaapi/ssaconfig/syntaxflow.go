package ssaconfig

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// SyntaxFlow结果保存类型
type SFResultSaveKind string

const (
	SFResultSaveNone     SFResultSaveKind = "none"     // no save
	SFResultSaveMemory   SFResultSaveKind = "memory"   // in cache
	SFResultSaveDatabase SFResultSaveKind = "database" // in database
)

type SyntaxFlowConfig struct {
	Memory          bool                  `json:"memory"`
	ResultSaveKind  SFResultSaveKind      `json:"result_save_kind"`
	ProcessCallback func(float64, string) `json:"-"`
}

type ControlMode string

const (
	ControlModeStart  ControlMode = "start"
	ControlModeStatus ControlMode = "status"
	ControlModeResume ControlMode = "resume"
)

type SyntaxFlowScanConfig struct {
	IgnoreLanguage bool       `json:"ignore_language"`
	Language       []Language `json:"language"`
	Concurrency    uint32     `json:"concurrency"`
	ControlMode    string     `json:"control_mode"`   // 控制模式 "start" "pause" "resume" "status"
	ResumeTaskId   string     `json:"resume_task_id"` // 恢复任务ID
}

// --- SyntaxFlow 配置 Get/Set 方法 ---

func (c *Config) GetSyntaxFlowResultKind() SFResultSaveKind {
	if c == nil || c.SyntaxFlow == nil {
		return SFResultSaveNone
	}
	return c.SyntaxFlow.ResultSaveKind
}

func (c *Config) SetSyntaxFlowResultKind(resultKind SFResultSaveKind) {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		return
	}
	c.SyntaxFlow.ResultSaveKind = resultKind
}

func (c *Config) SetSyntaxFlowResultSaveDataBase() {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		c.SyntaxFlow = defaultSyntaxFlowConfig()
	}
	c.SyntaxFlow.ResultSaveKind = SFResultSaveDatabase
}

func (c *Config) SetSyntaxFlowResultSaveMemory() {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		c.SyntaxFlow = defaultSyntaxFlowConfig()
	}
	c.SyntaxFlow.ResultSaveKind = SFResultSaveMemory
}

func (c *Config) GetSyntaxFlowProcessCallback() func(float64, string) {
	if c.SyntaxFlow == nil {
		c.SyntaxFlow = defaultSyntaxFlowConfig()
	}
	return c.SyntaxFlow.ProcessCallback
}

func (c *Config) SetSyntaxFlowProcessCallback(processCallback func(float64, string)) {
	if c == nil {
		return
	}
	if c.SyntaxFlow == nil {
		c.SyntaxFlow = defaultSyntaxFlowConfig()
	}
	c.SyntaxFlow.ProcessCallback = processCallback
}

// --- 扫描配置 Get 方法 ---

func (c *Config) GetSyntaxFlowMemory() bool {
	if c == nil || (c.Mode&ModeSyntaxFlow == 0 && c.Mode&ModeSyntaxFlowScanManager == 0) {
		return false
	}
	if c.SyntaxFlow == nil {
		c.SyntaxFlow = defaultSyntaxFlowConfig()
	}
	return c.SyntaxFlow.Memory
}

// GetScanMemory is a compatibility wrapper used by tests and callers.
func (c *Config) GetScanMemory() bool {
	if c == nil || (c.Mode&ModeSyntaxFlow == 0 && c.Mode&ModeSyntaxFlowScanManager == 0) {
		return false
	}
	if c.SyntaxFlow == nil {
		c.SyntaxFlow = defaultSyntaxFlowConfig()
	}
	return c.SyntaxFlow.Memory
}

func (c *Config) GetScanConcurrency() uint32 {
	if c == nil || c.SyntaxFlowScan == nil {
		return 0
	}
	return c.SyntaxFlowScan.Concurrency
}

func (c *Config) GetScanIgnoreLanguage() bool {
	if c == nil || c.SyntaxFlowScan == nil {
		return false
	}
	return c.SyntaxFlowScan.IgnoreLanguage
}

func (c *Config) GetScanControlMode() ControlMode {
	if c == nil || c.SyntaxFlowScan == nil {
		return ""
	}
	return ControlMode(c.SyntaxFlowScan.ControlMode)
}

func (c *Config) GetScanResumeTaskId() string {
	if c == nil || c.SyntaxFlowScan == nil {
		return ""
	}
	return c.SyntaxFlowScan.ResumeTaskId
}

func (c *Config) GetScanLanguage() []Language {
	if c == nil || c.SyntaxFlowScan == nil {
		return nil
	}
	return c.SyntaxFlowScan.Language
}

// --- 扫描配置 Options ---

func WithSyntaxFlowMemory(memory bool) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlow("Scan Memory"); err != nil {
			return err
		}
		c.SyntaxFlow.Memory = memory
		return nil
	}
}

// WithScanConcurrency 设置扫描并发数
func WithScanConcurrency(concurrency uint32) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowScan("Scan Concurrency"); err != nil {
			return err
		}
		c.SyntaxFlowScan.Concurrency = concurrency
		return nil
	}
}

// WithScanIgnoreLanguage 设置忽略语言
func WithScanIgnoreLanguage(ignoreLanguage bool) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowScan("Scan Ignore Language"); err != nil {
			return err
		}
		c.SyntaxFlowScan.IgnoreLanguage = ignoreLanguage
		return nil
	}
}

func WithScanControlMode(mode ControlMode) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowScan("Scan Control Mode"); err != nil {
			return err
		}
		c.SyntaxFlowScan.ControlMode = string(mode)
		return nil
	}
}

// WithScanRaw 从 ypb.SyntaxFlowScanRequest 提取配置
func WithScanRaw(req *ypb.SyntaxFlowScanRequest) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowScan("Scan Raw"); err != nil {
			return err
		}
		if req == nil {
			return nil
		}

		// 提取扫描配置
		c.SyntaxFlowScan.ControlMode = req.ControlMode
		c.SyntaxFlowScan.IgnoreLanguage = req.IgnoreLanguage
		c.SyntaxFlowScan.ResumeTaskId = req.ResumeTaskId
		c.SyntaxFlowScan.Concurrency = req.Concurrency

		// SyntaxFlow
		if err := c.ensureSyntaxFlow("Scan Raw"); err == nil {
			c.SyntaxFlow.Memory = req.Memory
		}

		// 提取基础信息 (只有当 Base 模式启用时)
		if c != nil && c.Mode&ModeProjectBase != 0 {
			if c.BaseInfo == nil {
				c.BaseInfo = defaultBaseInfo()
			}
			if len(req.ProgramName) > 0 {
				c.BaseInfo.ProgramNames = req.ProgramName
			}
			if len(req.ProjectName) > 0 {
				c.BaseInfo.ProjectName = req.ProjectName[0]
			}
		}

		// 提取规则配置
		if c != nil && c.Mode&ModeSyntaxFlowRule != 0 {
			if c.SyntaxFlowRule == nil {
				c.SyntaxFlowRule = defaultSyntaxFlowRuleConfig()
			}
			if req.Filter != nil {
				c.SyntaxFlowRule.RuleFilter = req.Filter
			}
			if req.RuleInput != nil {
				c.SyntaxFlowRule.RuleInput = req.RuleInput
			}
		}

		return nil
	}
}

// WithResumeTaskId 设置要恢复的任务ID，用于恢复之前暂停的扫描任务
func WithScanResumeTaskId(taskId string) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowScan("Resume Task Id"); err != nil {
			return err
		}
		c.SyntaxFlowScan.ResumeTaskId = taskId
		return nil
	}
}

// WithScanLanguage 设置扫描语言
func WithScanLanguage(language ...Language) Option {
	return func(c *Config) error {
		if err := c.ensureSyntaxFlowScan("Scan Language"); err != nil {
			return err
		}
		c.SyntaxFlowScan.Language = language
		return nil
	}
}
