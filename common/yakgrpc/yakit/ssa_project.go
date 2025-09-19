package yakit

import (
	"encoding/json"
	"errors"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func CreateSSAProject(db *gorm.DB, project *ypb.SSAProject) (*schema.SSAProject, error) {
	if project == nil {
		return nil, utils.Errorf("create SSA project failed: project is nil")
	}

	projectBuilder := newSSAProjectBuilderByProto(project)
	if err := projectBuilder.Validate(); err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	schemaProject, err := projectBuilder.toSchemaSSAProject()
	if err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	if err := db.Create(schemaProject).Error; err != nil {
		return nil, utils.Errorf("create SSA project failed: %s", err)
	}
	return schemaProject, nil
}

func UpdateSSAProject(db *gorm.DB, project *ypb.SSAProject) (*schema.SSAProject, error) {
	if project == nil {
		return nil, utils.Errorf("update SSA project failed: project is nil")
	}

	if project.ID <= 0 {
		return nil, utils.Errorf("update SSA project failed: project ID is required")
	}

	projectBuilder := newSSAProjectBuilderByProto(project)
	if err := projectBuilder.Validate(); err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}
	schemaProject, err := projectBuilder.toSchemaSSAProject()
	if err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}

	var existingProject schema.SSAProject
	if err := db.First(&existingProject, schemaProject.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.Errorf("project with ID %d not found", schemaProject.ID)
		}
		return nil, utils.Errorf("check project existence failed: %s", err)
	}

	if err := db.Model(&existingProject).Updates(schemaProject).Error; err != nil {
		return nil, utils.Errorf("update SSA project failed: %s", err)
	}
	return schemaProject, nil
}

func DeleteSSAProject(db *gorm.DB, req *ypb.DeleteSSAProjectRequest) (int64, error) {
	if req == nil || req.Filter == nil {
		return 0, utils.Errorf("delete SSA project failed: filter is nil")
	}

	db = db.Model(&schema.SSAProject{})
	query := FilterSSAProject(db, req.Filter)

	result := query.Unscoped().Delete(&schema.SSAProject{})
	if result.Error != nil {
		return 0, utils.Errorf("delete SSA projects failed: %s", result.Error)
	}
	return result.RowsAffected, nil
}

func QuerySSAProject(db *gorm.DB, req *ypb.QuerySSAProjectRequest) (*bizhelper.Paginator, []*schema.SSAProject, error) {
	if req == nil {
		req = &ypb.QuerySSAProjectRequest{}
	}
	db = db.Model(&schema.SSAProject{})
	p := req.Pagination
	if p == nil {
		p = &ypb.Paging{
			Page:    1,
			Limit:   30,
			OrderBy: "updated_at",
			Order:   "desc",
		}
	}

	db = bizhelper.OrderByPaging(db, p)
	db = FilterSSAProject(db, req.GetFilter())
	var projects []*schema.SSAProject
	paging, db := bizhelper.Paging(db, int(p.Page), int(p.Limit), &projects)
	if db.Error != nil {
		return nil, nil, utils.Errorf("paging failed: %s", db.Error)
	}
	return paging, projects, nil
}

func FilterSSAProject(db *gorm.DB, filter *ypb.SSAProjectFilter) *gorm.DB {
	if filter == nil {
		return db
	}

	db = db.Model(&schema.SSAProject{})

	db = bizhelper.ExactQueryInt64ArrayOr(db, "id", filter.GetIDs())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "project_name", filter.GetProjectNames())
	db = bizhelper.ExactOrQueryStringArrayOr(db, "language", filter.GetLanguages())

	if filter.GetSearchKeyword() != "" {
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{
			"project_name", "description", "tags",
		}, []string{filter.GetSearchKeyword()}, false)
	}
	return db
}

func QuerySSAProjectById(id uint64) (*schema.SSAProject, error) {
	if id == 0 {
		return nil, utils.Errorf("get SSA project failed: id is required")
	}
	db := consts.GetGormProfileDatabase()
	db = db.Model(&schema.SSAProject{})
	var project schema.SSAProject
	db = db.Where("id = ?", id).First(&project)
	if db.Error != nil {
		return nil, utils.Errorf("get SSA project failed: %s", db.Error)
	}
	return &project, nil
}

type SSAProjectBuilder struct {
	ID               uint
	ProjectName      string
	Description      string
	Tags             []string
	Language         string
	CodeSourceConfig *schema.CodeSourceInfo
	Config           *schema.SSAProjectConfig
}

func newSSAProjectBuilderByProto(proto *ypb.SSAProject) *SSAProjectBuilder {
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

func (s *SSAProjectBuilder) toSchemaSSAProject() (*schema.SSAProject, error) {
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
	schemaProject, err := s.toSchemaSSAProject()
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

type SSAProjectBuilderOption func(builder *SSAProjectBuilder)

func NewSSAProjectBuilder(projectName string, opts ...SSAProjectBuilderOption) *SSAProjectBuilder {
	builder := &SSAProjectBuilder{
		ProjectName:      projectName,
		CodeSourceConfig: &schema.CodeSourceInfo{},
		Config:           schema.NewSSAProjectConfig(),
	}
	for _, opt := range opts {
		opt(builder)
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
func WithSSAProjectStrictMode(strictMode bool) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.CompileConfig.StrictMode = strictMode
	}
}

func WithSSAProjectPeepholeSize(peepholeSize int) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.CompileConfig.PeepholeSize = peepholeSize
	}
}

func WithSSAProjectExcludeFiles(excludeFiles []string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.CompileConfig.ExcludeFiles = excludeFiles
	}
}

func WithSSAProjectReCompile(reCompile bool) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.CompileConfig.ReCompile = reCompile
	}
}

func WithSSAProjectMemoryCompile(memoryCompile bool) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.CompileConfig.MemoryCompile = memoryCompile
	}
}

func WithSSAProjectCompileConcurrency(concurrency int) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.CompileConfig.Concurrency = uint32(concurrency)
	}
}

// 扫描配置
func WithSSAProjectScanConcurrency(concurrency int) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.ScanConfig.Concurrency = uint32(concurrency)
	}
}

func WithSSAProjectMemoryScan(memoryScan bool) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.ScanConfig.Memory = memoryScan
	}
}

func WithSSAProjectIgnoreLanguage(ignoreLanguage bool) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.ScanConfig.IgnoreLanguage = ignoreLanguage
	}
}

// 基础信息配置
func WithSSAProjectTags(tags []string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Tags = tags
	}
}

func WithSSAProjectLanguage(language string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Language = language
	}
}

func WithSSAProjectDescription(description string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Description = description
	}
}

// 规则配置
func WithSSAProjectRuleFilterLanguage(language ...string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.Language = language
	}
}

func WithSSAProjectRuleFilterSeverity(severity ...string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.Severity = severity
	}
}

func WithSSAProjectRuleFilterKind(kind string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.FilterRuleKind = kind
	}
}

func WithSSAProjectRuleFilterPurpose(purpose ...string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.Purpose = purpose
	}
}

func WithSSAProjectRuleFilterKeyword(keyword string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.Keyword = keyword
	}
}

func WithSSAProjectRuleFilterGroupNames(groupNames ...string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.GroupNames = groupNames
	}
}

func WithSSAProjectRuleFilterRuleNames(ruleNames ...string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.RuleNames = ruleNames
	}
}

func WithSSAProjectRuleFilterTag(tag ...string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.Config.RuleConfig.RuleFilter.Tag = tag
	}
}

// 代码源配置
func WithSSAProjectCodeSourceKind(kind string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Kind = schema.CodeSourceKind(kind)
	}
}

func WithSSAProjectCodeSourceLocalFile(localFile string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.LocalFile = localFile
	}
}

func WithSSAProjectCodeSourceURL(url string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.URL = url
	}
}

func WithSSAProjectCodeSourceBranch(branch string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Branch = branch
	}
}

func WithSSAProjectCodeSourcePath(path string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Path = path
	}
}

// 认证配置
func WithSSAProjectCodeSourceAuthKind(kind string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.Kind = kind
	}
}

func WithSSAProjectCodeSourceAuthUserName(userName string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.UserName = userName
	}
}

func WithSSAProjectCodeSourceAuthPassword(password string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.Password = password
	}
}

func WithSSAProjectCodeSourceAuthKeyPath(keyPath string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Auth.KeyPath = keyPath
	}
}

// 代理配置
func WithSSAProjectCodeSourceProxyURL(url string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Proxy.URL = url
	}
}

func WithSSAProjectCodeSourceProxyAuth(user string, password string) SSAProjectBuilderOption {
	return func(builder *SSAProjectBuilder) {
		builder.CodeSourceConfig.Proxy.User = user
		builder.CodeSourceConfig.Proxy.Password = password
	}
}

var SSAProjectExports = map[string]interface{}{
	"GetSSAProjectByName": LoadSSAProjectBuilderByName,
	"GetSSAProjectByID":   LoadSSAProjectBuilderByID,
	"NewSSAProject":       NewSSAProjectBuilder,

	"withStrictMode":         WithSSAProjectStrictMode,
	"withPeepholeSize":       WithSSAProjectPeepholeSize,
	"withExcludeFiles":       WithSSAProjectExcludeFiles,
	"withReCompile":          WithSSAProjectReCompile,
	"withMemoryCompile":      WithSSAProjectMemoryCompile,
	"withCompileConcurrency": WithSSAProjectCompileConcurrency,
	"withScanConcurrency":    WithSSAProjectScanConcurrency,
	"withMemoryScan":         WithSSAProjectMemoryScan,
	"withIgnoreLanguage":     WithSSAProjectIgnoreLanguage,

	"withTags":        WithSSAProjectTags,
	"withLanguage":    WithSSAProjectLanguage,
	"withDescription": WithSSAProjectDescription,

	"withRuleFilterLanguage":   WithSSAProjectRuleFilterLanguage,
	"withRuleFilterSeverity":   WithSSAProjectRuleFilterSeverity,
	"withRuleFilterKind":       WithSSAProjectRuleFilterKind,
	"withRuleFilterPurpose":    WithSSAProjectRuleFilterPurpose,
	"withRuleFilterKeyword":    WithSSAProjectRuleFilterKeyword,
	"withRuleFilterGroupNames": WithSSAProjectRuleFilterGroupNames,
	"withRuleFilterRuleNames":  WithSSAProjectRuleFilterRuleNames,
	"withRuleFilterTag":        WithSSAProjectRuleFilterTag,

	"withCodeSourceKind":         WithSSAProjectCodeSourceKind,
	"withCodeSourceLocalFile":    WithSSAProjectCodeSourceLocalFile,
	"withCodeSourceURL":          WithSSAProjectCodeSourceURL,
	"withCodeSourceBranch":       WithSSAProjectCodeSourceBranch,
	"withCodeSourcePath":         WithSSAProjectCodeSourcePath,
	"withCodeSourceAuthKind":     WithSSAProjectCodeSourceAuthKind,
	"withCodeSourceAuthUserName": WithSSAProjectCodeSourceAuthUserName,
	"withCodeSourceAuthPassword": WithSSAProjectCodeSourceAuthPassword,
	"withCodeSourceAuthKeyPath":  WithSSAProjectCodeSourceAuthKeyPath,
	"withCodeSourceProxyURL":     WithSSAProjectCodeSourceProxyURL,
	"withCodeSourceProxyAuth":    WithSSAProjectCodeSourceProxyAuth,
}
