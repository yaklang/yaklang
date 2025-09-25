package ssaproject

import (
	"encoding/json"
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSAProjectBuilder struct {
	ID               uint
	ProjectName      string
	Description      string
	Tags             []string
	Language         string
	CodeSourceConfig *schema.CodeSourceInfo
	Config           *schema.SSAProjectConfig
}

func NewSSAProjectBuilderByProto(proto *ypb.SSAProject) *SSAProjectBuilder {
	if proto == nil {
		return nil
	}
	builder := &SSAProjectBuilder{
		ID:               uint(proto.ID),
		ProjectName:      proto.ProjectName,
		Description:      proto.Description,
		Tags:             proto.Tags,
		Language:         proto.Language,
		CodeSourceConfig: &schema.CodeSourceInfo{},
		Config:           schema.NewSSAProjectConfig(),
	}
	if proto.CodeSourceConfig != "" {
		json.Unmarshal([]byte(proto.CodeSourceConfig), builder.CodeSourceConfig)
	}
	if cc := proto.CompileConfig; cc != nil {
		builder.Config.CompileConfig = &schema.SSACompileConfig{
			StrictMode:    cc.StrictMode,
			PeepholeSize:  int(cc.PeepholeSize),
			ExcludeFiles:  cc.ExcludeFiles,
			ReCompile:     cc.ReCompile,
			MemoryCompile: cc.Memory,
			Concurrency:   cc.Concurrency,
		}
	}
	if sc := proto.ScanConfig; sc != nil {
		builder.Config.ScanConfig = &schema.SSAScanConfig{
			Concurrency:    sc.Concurrency,
			Memory:         sc.Memory,
			IgnoreLanguage: sc.IgnoreLanguage,
		}
	}
	if rc := proto.RuleConfig; rc != nil && rc.RuleFilter != nil {
		builder.Config.RuleConfig.RuleFilter = rc.RuleFilter
	}
	return builder
}

func (s *SSAProjectBuilder) ToSchemaSSAProject() (*schema.SSAProject, error) {
	if s == nil {
		return nil, utils.Errorf("to schema SSA project failed: ssa project builder is nil")
	}
	var result schema.SSAProject
	result.ID = s.ID
	result.ProjectName = s.ProjectName
	result.Description = s.Description
	result.Language = s.Language
	result.SetTagsList(s.Tags)
	result.CodeSourceConfig = s.CodeSourceConfig.JsonString()
	err := result.SetConfig(s.Config)
	if err != nil {
		return nil, utils.Errorf("to schema SSA project failed: %s", err)
	}
	return &result, nil
}

func (s *SSAProjectBuilder) Save() error {
	if s == nil {
		return utils.Errorf("save SSA project failed: ssa project builder is nil")
	}
	schemaProject, err := s.ToSchemaSSAProject()
	if err != nil {
		return err
	}

	db := consts.GetGormProfileDatabase()
	var existingProject schema.SSAProject
	err = db.Where("project_name = ?", schemaProject.ProjectName).First(&existingProject).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return db.Create(schemaProject).Error
		} else {
			return utils.Errorf("check project existence failed: %s", err)
		}
	}
	err = db.Model(&existingProject).Updates(schemaProject).Error
	if err != nil {
		return utils.Errorf("update SSA project failed: %s", err)
	}
	return nil
}

func (s *SSAProjectBuilder) GetRuleFilter() *ypb.SyntaxFlowRuleFilter {
	return s.Config.GetRuleFilter()
}

func (s *SSAProjectBuilder) Validate() error {
	if s == nil {
		return utils.Errorf("validate SSA project failed: ssa project builder is nil")
	}
	if s.ProjectName == "" {
		return utils.Errorf("validate SSA project failed: project name is required")
	}
	if s.Language == "" {
		return utils.Errorf("validate SSA project failed: language is required")
	}
	if s.CodeSourceConfig == nil {
		return utils.Errorf("validate SSA project failed: code source config is required")
	}
	if err := s.CodeSourceConfig.ValidateSourceConfig(); err != nil {
		return utils.Errorf("validate SSA project failed: %s", err)
	}
	return nil
}

func (s *SSAProjectBuilder) GetCompileConfig() *schema.SSACompileConfig {
	if s == nil {
		return nil
	}
	if s.Config == nil {
		return nil
	}
	return s.Config.CompileConfig
}

func (s *SSAProjectBuilder) GetScanConfig() *schema.SSAScanConfig {
	if s == nil {
		return nil
	}
	if s.Config == nil {
		return nil
	}
	return s.Config.ScanConfig
}

type SSAProjectOption func(builder *SSAProjectBuilder)
type SSAProjectScanOption func(config *schema.SSAScanConfig)
type SSAProjectRuleOption func(config *schema.SSARuleConfig)
type SSAProjectCompileOption func(config *schema.SSACompileConfig)

func NewSSAProjectBuilder(projectName string, opts ...any) *SSAProjectBuilder {
	builder := &SSAProjectBuilder{
		ProjectName:      projectName,
		CodeSourceConfig: &schema.CodeSourceInfo{},
		Config:           schema.NewSSAProjectConfig(),
	}
	for _, opt := range opts {
		switch opt := opt.(type) {
		case SSAProjectOption:
			opt(builder)
		case SSAProjectScanOption:
			opt(builder.Config.ScanConfig)
		case SSAProjectRuleOption:
			opt(builder.Config.RuleConfig)
		case SSAProjectCompileOption:
			opt(builder.Config.CompileConfig)
		}
	}
	return builder
}

func loadSSAProjectBySchema(project *schema.SSAProject) (*SSAProjectBuilder, error) {
	builder := &SSAProjectBuilder{
		ID:               project.ID,
		ProjectName:      project.ProjectName,
		Description:      project.Description,
		Tags:             project.GetTagsList(),
		Language:         project.Language,
		CodeSourceConfig: project.GetSourceConfig(),
		Config:           schema.NewSSAProjectConfig(),
	}
	config, err := project.GetConfig()
	if err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	builder.Config = config
	return builder, nil
}

func LoadSSAProjectBuilderByName(projectName string) (*SSAProjectBuilder, error) {
	db := consts.GetGormProfileDatabase()

	var project schema.SSAProject
	if err := db.Where("project_name = ?", projectName).First(&project).Error; err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return loadSSAProjectBySchema(&project)
}

func LoadSSAProjectBuilderByID(id uint) (*SSAProjectBuilder, error) {
	db := consts.GetGormProfileDatabase()
	var project schema.SSAProject
	if err := db.Where("id = ?", id).First(&project).Error; err != nil {
		return nil, utils.Errorf("load SSA project failed: %s", err)
	}
	return loadSSAProjectBySchema(&project)
}

// 编译配置
func WithSSAProjectStrictMode(strictMode bool) SSAProjectCompileOption {
	return func(config *schema.SSACompileConfig) {
		config.StrictMode = strictMode
	}
}

func WithSSAProjectPeepholeSize(peepholeSize int) SSAProjectCompileOption {
	return func(config *schema.SSACompileConfig) {
		config.PeepholeSize = peepholeSize
	}
}

func WithSSAProjectExcludeFiles(excludeFiles []string) SSAProjectCompileOption {
	return func(config *schema.SSACompileConfig) {
		config.ExcludeFiles = excludeFiles
	}
}

func WithSSAProjectReCompile(reCompile bool) SSAProjectCompileOption {
	return func(config *schema.SSACompileConfig) {
		config.ReCompile = reCompile
	}
}

func WithSSAProjectMemoryCompile(memoryCompile bool) SSAProjectCompileOption {
	return func(config *schema.SSACompileConfig) {
		config.MemoryCompile = memoryCompile
	}
}

func WithSSAProjectCompileConcurrency(concurrency int) SSAProjectCompileOption {
	return func(config *schema.SSACompileConfig) {
		config.Concurrency = uint32(concurrency)
	}
}

// 扫描配置
func WithSSAProjectScanConcurrency(concurrency int) SSAProjectScanOption {
	return func(config *schema.SSAScanConfig) {
		config.Concurrency = uint32(concurrency)
	}
}

func WithSSAProjectMemoryScan(memoryScan bool) SSAProjectScanOption {
	return func(config *schema.SSAScanConfig) {
		config.Memory = memoryScan
	}
}

func WithSSAProjectIgnoreLanguage(ignoreLanguage bool) SSAProjectScanOption {
	return func(config *schema.SSAScanConfig) {
		config.IgnoreLanguage = ignoreLanguage
	}
}

// 基础信息配置
func WithSSAProjectTags(tags []string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.Tags = tags
	}
}

func WithSSAProjectLanguage(language string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.Language = language
	}
}

func WithSSAProjectDescription(description string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.Description = description
	}
}

// 规则配置
func WithSSAProjectRuleFilterLanguage(language ...string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.Language = language
	}
}

func WithSSAProjectRuleFilterSeverity(severity ...string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.Severity = severity
	}
}

func WithSSAProjectRuleFilterKind(kind string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.FilterRuleKind = kind
	}
}

func WithSSAProjectRuleFilterPurpose(purpose ...string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.Purpose = purpose
	}
}

func WithSSAProjectRuleFilterKeyword(keyword string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.Keyword = keyword
	}
}

func WithSSAProjectRuleFilterGroupNames(groupNames ...string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.GroupNames = groupNames
	}
}

func WithSSAProjectRuleFilterRuleNames(ruleNames ...string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.RuleNames = ruleNames
	}
}

func WithSSAProjectRuleFilterTag(tag ...string) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.Tag = tag
	}
}

func WithSSAProjectRuleFilterIncludeLibraryRule(includeLibraryRule bool) SSAProjectRuleOption {
	return func(config *schema.SSARuleConfig) {
		if config.RuleFilter == nil {
			config.RuleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		config.RuleFilter.IncludeLibraryRule = includeLibraryRule
	}
}

func WithSSAProjectProcessCallback(callback func(progress float64)) SSAProjectScanOption {
	return func(config *schema.SSAScanConfig) {
		config.ProcessCallback = callback
	}
}

// 代码源配置
func WithSSAProjectCodeSourceKind(kind string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Kind = schema.CodeSourceKind(kind)
	}
}

func WithSSAProjectCodeSourceLocalFile(localFile string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.LocalFile = localFile
	}
}

func WithSSAProjectCodeSourceURL(url string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.URL = url
	}
}

func WithSSAProjectCodeSourceBranch(branch string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Branch = branch
	}
}

func WithSSAProjectCodeSourcePath(path string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Path = path
	}
}

// 认证配置
func WithSSAProjectCodeSourceAuthKind(kind string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.Kind = kind
	}
}

func WithSSAProjectCodeSourceAuthUserName(userName string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.UserName = userName
	}
}

func WithSSAProjectCodeSourceAuthPassword(password string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.Password = password
	}
}

func WithSSAProjectCodeSourceAuthKeyPath(keyPath string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.KeyPath = keyPath
	}
}

// 代理配置
func WithSSAProjectCodeSourceProxyURL(url string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Proxy.URL = url
	}
}

func WithSSAProjectCodeSourceProxyAuth(user string, password string) SSAProjectOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Proxy.User = user
		builder.CodeSourceConfig.Proxy.Password = password
	}
}
