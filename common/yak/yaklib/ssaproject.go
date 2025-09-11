package yaklib

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// querySSAProjectByName 根据项目名查询SSA项目
func querySSAProjectByName(projectName string) (*schema.SSAProject, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Error("no database connection")
	}

	req := &ypb.QuerySSAProjectRequest{
		Filter: &ypb.SSAProjectFilter{
			ProjectNames: []string{projectName},
		},
		Pagination: &ypb.Paging{
			Limit: 1,
		},
	}

	_, projects, err := yakit.QuerySSAProject(db, req)
	if err != nil {
		return nil, utils.Errorf("query SSA project failed: %s", err)
	}

	if len(projects) == 0 {
		return nil, utils.Errorf("SSA project not found: %s", projectName)
	}

	return projects[0], nil
}

// querySSAProjectByID 根据ID查询SSA项目
func querySSAProjectByID(id int64) (*schema.SSAProject, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, utils.Error("no database connection")
	}

	req := &ypb.QuerySSAProjectRequest{
		Filter: &ypb.SSAProjectFilter{
			IDs: []int64{id},
		},
		Pagination: &ypb.Paging{
			Limit: 1,
		},
	}

	_, projects, err := yakit.QuerySSAProject(db, req)
	if err != nil {
		return nil, utils.Errorf("query SSA project failed: %s", err)
	}

	if len(projects) == 0 {
		return nil, utils.Errorf("SSA project not found with ID: %d", id)
	}

	return projects[0], nil
}

// SSAProject选项类型
type SSAProjectParamsOpt func(*schema.SSAProject)

// 创建SSAProject的选项函数
func WithSourceKind(kind string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.SourceKind = schema.SSAProjectSourceKind(kind)
	}
}

func WithLocalPath(path string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.LocalPath = path
	}
}

func WithURL(url string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.URL = url
	}
}

func WithBranch(branch string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.Branch = branch
	}
}

func WithGitPath(path string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.GitPath = path
	}
}

func WithDescription(desc string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.Description = desc
	}
}

func WithStrictMode(strict bool) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.StrictMode = strict
	}
}

func WithPeepholeSize(size int) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.PeepholeSize = size
	}
}

func WithExcludeFiles(files string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.ExcludeFiles = files
	}
}

func WithReCompile(recompile bool) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.ReCompile = recompile
	}
}

func WithScanConcurrency(concurrency int) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.ScanConcurrency = uint32(concurrency)
	}
}

func WithMemoryScan(memory bool) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.MemoryScan = memory
	}
}

func WithScanRuleGroups(groups string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.ScanRuleGroups = groups
	}
}

func WithScanRuleNames(names string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.ScanRuleNames = names
	}
}

func WithIgnoreLanguage(ignore bool) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.IgnoreLanguage = ignore
	}
}

func WithProxyURL(proxy string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.ProxyURL = proxy
	}
}

func WithProxyAuth(user, password string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.ProxyUser = user
		p.ProxyPassword = password
	}
}

func WithAuthKind(kind string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.AuthKind = kind
	}
}

func WithAuthUsername(username string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.AuthUsername = username
	}
}

func WithAuthPassword(password string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.AuthPassword = password
	}
}

func WithAuthKeyPath(keyPath string) SSAProjectParamsOpt {
	return func(p *schema.SSAProject) {
		p.AuthKeyPath = keyPath
	}
}

// _createSSAProject 创建SSAProject对象（不保存到数据库）
func _createSSAProject(projectName string, opts ...SSAProjectParamsOpt) *schema.SSAProject {
	project := &schema.SSAProject{
		ProjectName:     projectName,
		ScanConcurrency: 5,   // 默认并发数
		PeepholeSize:    100, // 默认窥孔大小
	}

	for _, opt := range opts {
		opt(project)
	}

	return project
}

// _saveSSAProject 保存SSAProject到数据库
func _saveSSAProject(project *schema.SSAProject) error {
	// 验证项目配置
	if err := project.Validate(); err != nil {
		return utils.Errorf("project validation failed: %s", err)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("no database connection")
	}

	// 如果有ID则更新，否则创建
	if project.ID > 0 {
		if err := db.Save(project).Error; err != nil {
			return utils.Errorf("update SSA project failed: %s", err)
		}
	} else {
		if err := db.Create(project).Error; err != nil {
			return utils.Errorf("create SSA project failed: %s", err)
		}
	}

	return nil
}

// NewSSAProject 创建新的SSA项目并保存到数据库
func NewSSAProject(projectName string, opts ...SSAProjectParamsOpt) (*schema.SSAProject, error) {
	project := _createSSAProject(projectName, opts...)
	return project, _saveSSAProject(project)
}

// SaveSSAProject 保存SSAProject到数据库
func SaveSSAProject(project *schema.SSAProject) error {
	return _saveSSAProject(project)
}

var SSAProjectExports = map[string]interface{}{
	"QuerySSAProjectByName": querySSAProjectByName,
	"QuerySSAProjectByID":   querySSAProjectByID,
	"NewSSAProject":         NewSSAProject,
	"SaveSSAProject":        SaveSSAProject,

	// 选项函数
	"withSourceKind":      WithSourceKind,
	"withLocalPath":       WithLocalPath,
	"withURL":             WithURL,
	"withBranch":          WithBranch,
	"withGitPath":         WithGitPath,
	"withDescription":     WithDescription,
	"withStrictMode":      WithStrictMode,
	"withPeepholeSize":    WithPeepholeSize,
	"withExcludeFiles":    WithExcludeFiles,
	"withReCompile":       WithReCompile,
	"withScanConcurrency": WithScanConcurrency,
	"withMemoryScan":      WithMemoryScan,
	"withScanRuleGroups":  WithScanRuleGroups,
	"withScanRuleNames":   WithScanRuleNames,
	"withIgnoreLanguage":  WithIgnoreLanguage,
	"withProxyURL":        WithProxyURL,
	"withProxyAuth":       WithProxyAuth,
	"withAuthKind":        WithAuthKind,
	"withAuthUsername":    WithAuthUsername,
	"withAuthPassword":    WithAuthPassword,
	"withAuthKeyPath":     WithAuthKeyPath,
}
