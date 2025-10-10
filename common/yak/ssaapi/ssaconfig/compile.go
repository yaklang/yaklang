package ssaconfig

import (
	"github.com/yaklang/yaklang/common/utils"
)

// 基础信息配置
type BaseInfo struct {
	ProgramNames       []string `json:"program_names"`
	ProjectID          uint64   `json:"project_id"`
	ProjectName        string   `json:"project_name"`
	ProjectDescription string   `json:"project_description"`
	Language           string   `json:"language"`
	Tags               []string `json:"tags"`
}

// --- 基础信息配置 Get/Set 方法 ---

func (c *Config) GetProjectID() uint64 {
	if c == nil || c.BaseInfo == nil {
		return 0
	}
	return c.BaseInfo.ProjectID
}

func (c *Config) GetProgramNames() []string {
	if c == nil || c.BaseInfo == nil {
		return nil
	}
	return c.BaseInfo.ProgramNames
}

func (c *Config) GetProjectName() string {
	if c == nil || c.BaseInfo == nil {
		return ""
	}
	return c.BaseInfo.ProjectName
}

func (c *Config) GetProjectDescription() string {
	if c == nil || c.BaseInfo == nil {
		return ""
	}
	return c.BaseInfo.ProjectDescription
}

func (c *Config) GetLanguage() string {
	if c == nil || c.BaseInfo == nil {
		return ""
	}
	return c.BaseInfo.Language
}

func (c *Config) GetTags() []string {
	if c == nil || c.BaseInfo == nil {
		return nil
	}
	return c.BaseInfo.Tags
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
		c.BaseInfo.ProjectDescription = description
		return nil
	}
}

func WithProgramLanguage(language string) Option {
	return func(c *Config) error {
		if c.BaseInfo == nil {
			return utils.Errorf("Config: Program Language can only be set in Base mode")
		}
		c.BaseInfo.Language = language
		return nil
	}
}

// SSACompileConfig 编译配置
type SSACompileConfig struct {
	StrictMode    bool     `json:"strict_mode"`
	PeepholeSize  int      `json:"peephole_size"`
	ExcludeFiles  []string `json:"exclude_files"`
	ReCompile     bool     `json:"re_compile"`
	MemoryCompile bool     `json:"memory_compile"`
	Concurrency   uint32   `json:"compile_concurrency"`
}

// --- 编译配置 Get/Set 方法 ---

func (c *Config) GetCompileStrictMode() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.StrictMode
}

func (c *Config) SetCompileStrictMode(strictMode bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.StrictMode = strictMode
}

func (c *Config) GetCompilePeepholeSize() int {
	if c == nil || c.SSACompile == nil {
		return 0
	}
	return c.SSACompile.PeepholeSize
}

func (c *Config) SetCompilePeepholeSize(peepholeSize int) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.PeepholeSize = peepholeSize
}

func (c *Config) GetCompileExcludeFiles() []string {
	if c == nil || c.SSACompile == nil {
		return nil
	}
	return c.SSACompile.ExcludeFiles
}

func (c *Config) SetCompileExcludeFiles(excludeFiles []string) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.ExcludeFiles = excludeFiles
}

func (c *Config) GetCompileReCompile() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.ReCompile
}

func (c *Config) SetCompileReCompile(reCompile bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.ReCompile = reCompile
}

func (c *Config) GetCompileMemory() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.MemoryCompile
}

func (c *Config) SetCompileMemory(memory bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.MemoryCompile = memory
}

func (c *Config) GetCompileConcurrency() uint32 {
	if c == nil || c.SSACompile == nil {
		return 0
	}
	return c.SSACompile.Concurrency
}

func (c *Config) SetCompileConcurrency(concurrency uint32) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = &SSACompileConfig{}
	}
	c.SSACompile.Concurrency = concurrency
}

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
