package ssaconfig

import (
	"strings"
	"time"

	"github.com/samber/lo"
)

// 基础信息配置
type BaseInfo struct {
	ProgramNames []string `json:"program_names"`
	// TODO: set project ID should update config with this project??
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

func (c *Config) GetLatestProgramName() string {
	if c == nil || c.BaseInfo == nil || len(c.BaseInfo.ProgramNames) == 0 {
		return ""
	}
	return c.BaseInfo.ProgramNames[len(c.BaseInfo.ProgramNames)-1]
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

// SetProgramName 设置程序名（单个程序名）
func (c *Config) SetProgramName(name string) {
	if c == nil {
		return
	}
	if c.BaseInfo == nil {
		c.BaseInfo = defaultBaseInfo()
	}
	c.BaseInfo.ProgramNames = []string{name}
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

func WithProjectName(name string) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Project Name"); err != nil {
			return err
		}
		c.BaseInfo.ProjectName = name
		return nil
	}
}

func WithProjectTags(tags []string) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Project Tags"); err != nil {
			return err
		}
		c.BaseInfo.Tags = tags
		return nil
	}
}

func WithProjectDescription(s string) Option {
	return func(c *Config) error {
		if err := c.ensureBase("Project Description"); err != nil {
			return err
		}
		c.BaseInfo.ProjectDescription = s
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
	StrictMode               bool          `json:"strict_mode"`
	PeepholeSize             int           `json:"peephole_size"`
	ExcludeFiles             []string      `json:"exclude_files"`
	ReCompile                bool          `json:"re_compile"`
	MemoryCompile            bool          `json:"memory_compile"`
	Concurrency              int           `json:"compile_concurrency"`
	CompileIrCacheTTL        time.Duration `json:"compile_ir_cache_ttl"`
	FilePerformanceLog       bool          `json:"file_performance_log"`
	StopOnCliCheck           bool          `json:"stop_on_cli_check"`
	EnableIncrementalCompile bool          `json:"enable_incremental_compile"`
	BaseProgramName          string        `json:"base_program_name"`
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
	// 支持逗号分隔多个文件模式
	allFiles := lo.FlatMap(excludeFiles, func(item string, index int) []string {
		return strings.Split(item, ",")
	})
	c.SSACompile.ExcludeFiles = allFiles
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

func (c *Config) GetCompileConcurrency() int {
	if c == nil || c.SSACompile == nil {
		return 0
	}
	return c.SSACompile.Concurrency
}

func (c *Config) SetCompileConcurrency(concurrency int) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = defaultSSACompileConfig()
	}
	c.SSACompile.Concurrency = concurrency
}

func (c *Config) GetCompileFilePerformanceLog() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.FilePerformanceLog
}

func (c *Config) SetCompileFilePerformanceLog(enable bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = defaultSSACompileConfig()
	}
	c.SSACompile.FilePerformanceLog = enable
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
func WithCompileExcludeFiles(excludeFiles ...string) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Exclude Files"); err != nil {
			return err
		}
		// 支持逗号分隔多个文件模式
		allFiles := lo.FlatMap(excludeFiles, func(item string, index int) []string {
			return strings.Split(item, ",")
		})
		c.SSACompile.ExcludeFiles = append(c.SSACompile.ExcludeFiles, allFiles...)
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
func WithCompileMemoryCompile(memoryCompile ...bool) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Memory Compile"); err != nil {
			return err
		}
		enable := true
		if len(memoryCompile) > 0 {
			enable = memoryCompile[0]
		}
		c.SSACompile.MemoryCompile = enable
		return nil
	}
}

// WithCompileConcurrency 设置编译并发数
func WithCompileConcurrency(concurrency int) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile Concurrency"); err != nil {
			return err
		}
		c.SSACompile.Concurrency = concurrency
		return nil
	}
}

// WithCompileFilePerformanceLog 设置文件级别性能日志
func WithCompileFilePerformanceLog(enable bool) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Compile File Performance Log"); err != nil {
			return err
		}
		c.SSACompile.FilePerformanceLog = enable
		return nil
	}
}

// WithStopOnCliCheck 设置当检测到 cli.check() 时快速停止构建
func WithStopOnCliCheck(stopOnCliCheck bool) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Stop On Cli Check"); err != nil {
			return err
		}
		c.SSACompile.StopOnCliCheck = stopOnCliCheck
		return nil
	}
}

// --- 增量编译配置 Get/Set 方法 ---

func (c *Config) GetEnableIncrementalCompile() bool {
	if c == nil || c.SSACompile == nil {
		return false
	}
	return c.SSACompile.EnableIncrementalCompile
}

func (c *Config) SetEnableIncrementalCompile(enable bool) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = defaultSSACompileConfig()
	}
	c.SSACompile.EnableIncrementalCompile = enable
}

func (c *Config) GetBaseProgramName() string {
	if c == nil || c.SSACompile == nil {
		return ""
	}
	return c.SSACompile.BaseProgramName
}

func (c *Config) SetBaseProgramName(baseProgramName string) {
	if c == nil {
		return
	}
	if c.SSACompile == nil {
		c.SSACompile = defaultSSACompileConfig()
	}
	c.SSACompile.BaseProgramName = baseProgramName
	// 如果设置了 baseProgramName，自动启用增量编译
	if baseProgramName != "" {
		c.SSACompile.EnableIncrementalCompile = true
	}
}

// --- 增量编译配置 Options ---

// WithEnableIncrementalCompile 启用增量编译
// 如果启用增量编译但 BaseProgramName 为空，表示这是第一次增量编译（base program）
func WithEnableIncrementalCompile(enable bool) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Enable Incremental Compile"); err != nil {
			return err
		}
		c.SSACompile.EnableIncrementalCompile = enable
		return nil
	}
}

// WithBaseProgramName 设置基础程序名称（用于差量编译）
func WithBaseProgramName(baseProgramName string) Option {
	return func(c *Config) error {
		if err := c.ensureSSACompile("Base Program Name"); err != nil {
			return err
		}
		c.SSACompile.BaseProgramName = baseProgramName
		// 如果设置了 baseProgramName，自动启用增量编译
		if baseProgramName != "" {
			c.SSACompile.EnableIncrementalCompile = true
		}
		return nil
	}
}
