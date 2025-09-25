package ssaconfig

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type Option func(*Config) error

// --- 编译配置 Options ---

// WithCompileStrictMode 设置严格模式
func WithCompileStrictMode(strictMode bool) Option {
	return func(c *Config) error {
		if c.SSACompile == nil {
			return utils.Errorf("Config: Compile Strict Mode can only be set in Compile mode")
		}
		c.SSACompile.StrictMode = strictMode
		return nil
	}
}

// WithCompilePeepholeSize 设置窥视孔大小
func WithCompilePeepholeSize(peepholeSize int) Option {
	return func(c *Config) error {
		if c.SSACompile == nil {
			return utils.Errorf("Config: Compile Peephole Size can only be set in Compile mode")
		}
		c.SSACompile.PeepholeSize = peepholeSize
		return nil
	}
}

// WithCompileExcludeFiles 设置排除文件
func WithCompileExcludeFiles(excludeFiles []string) Option {
	return func(c *Config) error {
		if c.SSACompile == nil {
			return utils.Errorf("Config: Compile Exclude Files can only be set in Compile mode")
		}
		c.SSACompile.ExcludeFiles = excludeFiles
		return nil
	}
}

// WithCompileReCompile 设置重新编译
func WithCompileReCompile(reCompile bool) Option {
	return func(c *Config) error {
		if c.SSACompile == nil {
			return utils.Errorf("Config: Compile Re Compile can only be set in Compile mode")
		}
		c.SSACompile.ReCompile = reCompile
		return nil
	}
}

// WithCompileMemoryCompile 设置内存编译
func WithCompileMemoryCompile(memoryCompile bool) Option {
	return func(c *Config) error {
		if c.SSACompile == nil {
			return utils.Errorf("Config: Compile Memory Compile can only be set in Compile mode")
		}
		c.SSACompile.MemoryCompile = memoryCompile
		return nil
	}
}

// WithCompileConcurrency 设置编译并发数
func WithCompileConcurrency(concurrency uint32) Option {
	return func(c *Config) error {
		if c.SSACompile == nil {
			return utils.Errorf("Config: Compile Concurrency can only be set in Compile mode")
		}
		c.SSACompile.Concurrency = concurrency
		return nil
	}
}

// --- 扫描配置 Options ---

// WithScanConcurrency 设置扫描并发数
func WithScanConcurrency(concurrency uint32) Option {
	return func(c *Config) error {
		if c.SyntaxFlow == nil {
			return utils.Errorf("Config: Scan Concurrency can only be set in Scan mode")
		}
		c.SyntaxFlow.Concurrency = concurrency
		return nil
	}
}

// WithScanMemory 设置内存扫描
func WithScanMemory(memory bool) Option {
	return func(c *Config) error {
		if c.SyntaxFlow == nil {
			return utils.Errorf("Config: Scan Memory can only be set in Scan mode")
		}
		c.SyntaxFlow.Memory = memory
		return nil
	}
}

// WithScanIgnoreLanguage 设置忽略语言
func WithScanIgnoreLanguage(ignoreLanguage bool) Option {
	return func(c *Config) error {
		if c.SyntaxFlow == nil {
			return utils.Errorf("Config: Scan Ignore Language can only be set in Scan mode")
		}
		c.SyntaxFlow.IgnoreLanguage = ignoreLanguage
		return nil
	}
}

// WithScanProcessCallback 设置进度回调
func WithScanProcessCallback(callback func(progress float64)) Option {
	return func(c *Config) error {
		if c.SyntaxFlow == nil {
			return utils.Errorf("Config: Scan Process Callback can only be set in Scan mode")
		}
		c.SyntaxFlow.ProcessCallback = callback
		return nil
	}
}

// --- 规则配置 Options ---

// WithRuleFilter 设置规则过滤器
func WithRuleFilter(filter *ypb.SyntaxFlowRuleFilter) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter can only be set in Rule mode")
		}
		c.SyntaxFlowRule.RuleFilter = filter
		return nil
	}
}

// WithRuleFilterLanguage 设置规则过滤器语言
func WithRuleFilterLanguage(language ...string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Language can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Language = language
		return nil
	}
}

// WithRuleFilterSeverity 设置规则过滤器严重程度
func WithRuleFilterSeverity(severity ...string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Severity can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Severity = severity
		return nil
	}
}

// WithRuleFilterKind 设置规则过滤器类型
func WithRuleFilterKind(kind string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Kind can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.FilterRuleKind = kind
		return nil
	}
}

// WithRuleFilterPurpose 设置规则过滤器用途
func WithRuleFilterPurpose(purpose ...string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Purpose can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Purpose = purpose
		return nil
	}
}

// WithRuleFilterKeyword 设置规则过滤器关键字
func WithRuleFilterKeyword(keyword string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Keyword can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Keyword = keyword
		return nil
	}
}

// WithRuleFilterGroupNames 设置规则过滤器组名
func WithRuleFilterGroupNames(groupNames ...string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Group Names can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.GroupNames = groupNames
		return nil
	}
}

// WithRuleFilterRuleNames 设置规则过滤器规则名
func WithRuleFilterRuleNames(ruleNames ...string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Rule Names can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.RuleNames = ruleNames
		return nil
	}
}

// WithRuleFilterTag 设置规则过滤器标签
func WithRuleFilterTag(tag ...string) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Tag can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.Tag = tag
		return nil
	}
}

// WithRuleFilterIncludeLibraryRule 设置规则过滤器包含库规则
func WithRuleFilterIncludeLibraryRule(includeLibraryRule bool) Option {
	return func(c *Config) error {
		if c.SyntaxFlowRule == nil {
			return utils.Errorf("Config: Rule Filter Include Library Rule can only be set in Rule mode")
		}
		if c.SyntaxFlowRule.RuleFilter == nil {
			c.SyntaxFlowRule.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		c.SyntaxFlowRule.RuleFilter.IncludeLibraryRule = includeLibraryRule
		return nil
	}
}

// --- 基础信息配置 Options ---
func WithProgramNames(programName ...string) Option {
	return func(c *Config) error {
		if c.BaseInfo == nil {
			return utils.Errorf("Config: Program Name can only be set in Base mode")
		}
		c.BaseInfo.ProgramNames = append(c.BaseInfo.ProgramNames, programName...)
		return nil
	}
}

func WithProgramDescription(description string) Option {
	return func(c *Config) error {
		if c.BaseInfo == nil {
			return utils.Errorf("Config: Program Description can only be set in Base mode")
		}
		c.BaseInfo.ProgramDescription = description
		return nil
	}
}

func WithProgramLanguage(language string) Option {
	return func(c *Config) error {
		if c.BaseInfo == nil {
			return utils.Errorf("Config: Program Language can only be set in Base mode")
		}
		c.BaseInfo.ProgramLanguage = language
		return nil
	}
}

func WithProjectName(projectName string) Option {
	return func(c *Config) error {
		if c.BaseInfo == nil {
			return utils.Errorf("Config: Project Name can only be set in Base mode")
		}
		c.BaseInfo.ProjectName = projectName
		return nil
	}
}
