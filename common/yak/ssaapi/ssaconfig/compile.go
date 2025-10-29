package ssaconfig

import (
	"time"
)

// 基础信息配置
type BaseInfo struct {
	ProgramNames       []string `json:"program_names"`
	ProjectID          uint64   `json:"project_id"`
	ProjectName        string   `json:"project_name"`
	ProjectDescription string   `json:"project_description"`
	Language           Language `json:"language"`
	Tags               []string `json:"tags"`
}

// --- 基础信息配置 Get/Set 方法 ---

func (c *Config) GetProjectID() uint64 {
	if c == nil || c.BaseInfo == nil {
		return 0
	}
	return c.BaseInfo.ProjectID
}

func (c *Config) GetProgramName() string {
	if c == nil || c.BaseInfo == nil || len(c.BaseInfo.ProgramNames) == 0 {
		return ""
	}
	return c.BaseInfo.ProgramNames[0]
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

func (c *Config) GetLanguage() Language {
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

func WithProjectID(projectId uint64) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Project ID"); err != nil {
			return err
		}
		c.BaseInfo.ProjectID = projectId
		return nil
	}
}

func WithProgramNames(programName ...string) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Program Name"); err != nil {
			return err
		}
		c.BaseInfo.ProgramNames = append(c.BaseInfo.ProgramNames, programName...)
		return nil
	}
}

func WithProgramDescription(description string) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Program Description"); err != nil {
			return err
		}
		c.BaseInfo.ProjectDescription = description
		return nil
	}
}

func WithProjectRawLanguage(language string) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Project Raw Language"); err != nil {
			return err
		}
		var err error
		if c.BaseInfo.Language, err = ValidateLanguage(language); err != nil {
			return err
		}
		return nil
	}
}

func WithProjectLanguage(language Language) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Program Language"); err != nil {
			return err
		}
		c.BaseInfo.Language = language
		return nil
	}
}

// SSACompileConfig 编译配置
type SSACompileConfig struct {
	StrictMode        bool          `json:"strict_mode"`
	PeepholeSize      int           `json:"peephole_size"`
	ExcludeFiles      []string      `json:"exclude_files"`
	ReCompile         bool          `json:"re_compile"`
	MemoryCompile     bool          `json:"memory_compile"`
	Concurrency       uint32        `json:"compile_concurrency"`
	CompileIrCacheTTL time.Duration `json:"compile_ir_cache_ttl"`
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
		c.SSACompile = defaultSSACompileConfig()
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
		c.SSACompile = defaultSSACompileConfig()
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
		c.SSACompile = defaultSSACompileConfig()
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
		c.SSACompile = defaultSSACompileConfig()
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
		c.SSACompile = defaultSSACompileConfig()
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
		c.SSACompile = defaultSSACompileConfig()
	}
	c.SSACompile.Concurrency = concurrency
}

// --- 编译配置 Options ---

// WithCompileStrictMode 设置严格模式
func WithCompileStrictMode(strictMode bool) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Strict Mode"); err != nil {
			return err
		}
		c.SSACompile.StrictMode = strictMode
		return nil
	}
}

// WithCompilePeepholeSize 设置窥视孔大小
func WithCompilePeepholeSize(peepholeSize int) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Peephole Size"); err != nil {
			return err
		}
		c.SSACompile.PeepholeSize = peepholeSize
		return nil
	}
}

// WithCompileExcludeFiles 设置排除文件
func WithCompileExcludeFiles(excludeFiles []string) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Exclude Files"); err != nil {
			return err
		}
		c.SSACompile.ExcludeFiles = excludeFiles
		return nil
	}
}

// WithCompileReCompile 设置重新编译
func WithCompileReCompile(reCompile bool) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Re Compile"); err != nil {
			return err
		}
		c.SSACompile.ReCompile = reCompile
		return nil
	}
}

// WithCompileMemoryCompile 设置内存编译
func WithCompileMemoryCompile(memoryCompile bool) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Memory Compile"); err != nil {
			return err
		}
		c.SSACompile.MemoryCompile = memoryCompile
		return nil
	}
}

// WithCompileConcurrency 设置编译并发数
func WithCompileConcurrency(concurrency uint32) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Concurrency"); err != nil {
			return err
		}
		c.SSACompile.Concurrency = concurrency
		return nil
	}
}
