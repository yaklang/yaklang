package yaklib

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

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

type ssaProjectConfigBuilder struct {
	codeSource *schema.CodeSourceInfo
	project    *schema.SSAProject
}

type SSAProjectParamsOpt func(*ssaProjectConfigBuilder)

func WithSourceKind(kind string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		b.codeSource.Kind = schema.CodeSourceKind(kind)
	}
}

func WithLocalFile(path string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		b.codeSource.LocalFile = path
	}
}

func WithURL(url string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		b.codeSource.URL = url
	}
}

func WithBranch(branch string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		b.codeSource.Branch = branch
	}
}

func WithPath(path string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		b.codeSource.Path = path
	}
}

func WithAuthKind(kind string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		if b.codeSource.Auth == nil {
			b.codeSource.Auth = &schema.AuthConfigInfo{}
		}
		b.codeSource.Auth.Kind = kind
	}
}

func WithAuthUsername(username string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		if b.codeSource.Auth == nil {
			b.codeSource.Auth = &schema.AuthConfigInfo{}
		}
		b.codeSource.Auth.UserName = username
	}
}

func WithAuthPassword(password string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		if b.codeSource.Auth == nil {
			b.codeSource.Auth = &schema.AuthConfigInfo{}
		}
		b.codeSource.Auth.Password = password
	}
}

func WithAuthKeyPath(keyPath string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		if b.codeSource.Auth == nil {
			b.codeSource.Auth = &schema.AuthConfigInfo{}
		}
		b.codeSource.Auth.KeyPath = keyPath
	}
}

func WithProxyURL(proxy string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		if b.codeSource.Proxy == nil {
			b.codeSource.Proxy = &schema.ProxyConfigInfo{}
		}
		b.codeSource.Proxy.URL = proxy
	}
}

func WithProxyAuth(user, password string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.codeSource == nil {
			b.codeSource = &schema.CodeSourceInfo{}
		}
		if b.codeSource.Proxy == nil {
			b.codeSource.Proxy = &schema.ProxyConfigInfo{}
		}
		b.codeSource.Proxy.User = user
		b.codeSource.Proxy.Password = password
	}
}

func WithDescription(desc string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.Description = desc
	}
}

func WithStrictMode(strict bool) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.StrictMode = strict
	}
}

func WithPeepholeSize(size int) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.PeepholeSize = size
	}
}

func WithExcludeFiles(files []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.ExcludeFiles = strings.Join(files, ",")
	}
}

func WithReCompile(recompile bool) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.ReCompile = recompile
	}
}

func WithScanConcurrency(concurrency int) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.ScanConcurrency = uint32(concurrency)
	}
}

func WithMemoryScan(memory bool) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.MemoryScan = memory
	}
}
func WithIgnoreLanguage(ignore bool) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.IgnoreLanguage = ignore
	}
}

func _createSSAProject(projectName string, opts ...SSAProjectParamsOpt) *schema.SSAProject {
	project := &schema.SSAProject{
		ProjectName:     projectName,
		ScanConcurrency: 5,
		PeepholeSize:    100,
	}

	builder := &ssaProjectConfigBuilder{
		project: project,
	}

	for _, opt := range opts {
		opt(builder)
	}

	if builder.codeSource != nil {
		configBytes, err := json.Marshal(builder.codeSource)
		if err == nil {
			project.CodeSourceConfig = string(configBytes)
		}
	}

	return project
}

func _saveSSAProject(project *schema.SSAProject) error {
	if err := project.Validate(); err != nil {
		return utils.Errorf("project validation failed: %s", err)
	}

	db := consts.GetGormProfileDatabase()
	if db == nil {
		return utils.Error("no database connection")
	}

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

func NewSSAProject(projectName string, opts ...SSAProjectParamsOpt) (*schema.SSAProject, error) {
	project := _createSSAProject(projectName, opts...)
	return project, _saveSSAProject(project)
}

func SaveSSAProject(project *schema.SSAProject) error {
	return _saveSSAProject(project)
}

var SSAProjectExports = map[string]interface{}{
	"QuerySSAProjectByName": querySSAProjectByName,
	"QuerySSAProjectByID":   querySSAProjectByID,
	"NewSSAProject":         NewSSAProject,
	"SaveSSAProject":        SaveSSAProject,

	"withSourceKind":      WithSourceKind,
	"withLocalFile":       WithLocalFile,
	"withURL":             WithURL,
	"withBranch":          WithBranch,
	"withPath":            WithPath,
	"withDescription":     WithDescription,
	"withStrictMode":      WithStrictMode,
	"withPeepholeSize":    WithPeepholeSize,
	"withExcludeFiles":    WithExcludeFiles,
	"withReCompile":       WithReCompile,
	"withScanConcurrency": WithScanConcurrency,
	"withMemoryScan":      WithMemoryScan,
	"withIgnoreLanguage":  WithIgnoreLanguage,
	"withProxyURL":        WithProxyURL,
	"withProxyAuth":       WithProxyAuth,
	"withAuthKind":        WithAuthKind,
	"withAuthUsername":    WithAuthUsername,
	"withAuthPassword":    WithAuthPassword,
	"withAuthKeyPath":     WithAuthKeyPath,
}
