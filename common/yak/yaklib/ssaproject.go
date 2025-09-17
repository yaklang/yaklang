package yaklib

import (
	"encoding/json"
	"strings"

	"github.com/yaklang/yaklang/common/log"

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
	ruleFilter *ypb.SyntaxFlowRuleFilter
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

func WithCompileConcurrency(concurrency int) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.CompileConcurrency = uint32(concurrency)
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

func WithTags(tags []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.Tags = strings.Join(tags, ",")
	}
}

func WithLanguage(language string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		b.project.Language = language
	}
}

func WithRuleFilterLanguage(languages []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		b.ruleFilter.Language = languages
	}
}

func WithRuleFilterSeverity(severities []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		b.ruleFilter.Severity = severities
	}
}

func WithRuleFilterKind(types []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		// FilterRuleKind 字段是单个字符串，需要转换
		if len(types) > 0 {
			b.ruleFilter.FilterRuleKind = strings.Join(types, ",")
		}
	}
}

func WithRuleFilterPurpose(purposes []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		b.ruleFilter.Purpose = purposes
	}
}

func WithRuleFilterKeyword(keywords []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		// Keyword 字段是单个字符串，不是数组
		if len(keywords) > 0 {
			b.ruleFilter.Keyword = strings.Join(keywords, " ")
		}
	}
}

func WithRuleFilterGroupNames(groupNames []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		b.ruleFilter.GroupNames = groupNames
	}
}

func WithRuleFilterRuleNames(ruleNames []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		b.ruleFilter.RuleNames = ruleNames
	}
}

func WithRuleFilterTag(tags []string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		if b.ruleFilter == nil {
			b.ruleFilter = &ypb.SyntaxFlowRuleFilter{}
		}
		b.ruleFilter.Tag = tags
	}
}

func WithCompileConfigInfo(configInfo string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		var compileConfig schema.SSACompileConfig
		json.Unmarshal([]byte(configInfo), &compileConfig)
		b.project.Language = compileConfig.Language
		b.project.StrictMode = compileConfig.StrictMode
		b.project.PeepholeSize = compileConfig.PeepholeSize
		b.project.SetExcludeFilesList(compileConfig.ExcludeFiles)
		b.project.ReCompile = compileConfig.ReCompile
		b.project.MemoryCompile = compileConfig.MemoryCompile
	}
}

func WithScanConfigInfo(configInfo string) SSAProjectParamsOpt {
	return func(b *ssaProjectConfigBuilder) {
		var scanConfig schema.SSAScanConfig
		json.Unmarshal([]byte(configInfo), &scanConfig)
		b.project.ScanConcurrency = scanConfig.Concurrency
		b.project.MemoryScan = scanConfig.Memory
		b.project.IgnoreLanguage = scanConfig.IgnoreLanguage
	}
}

func _createSSAProject(projectName string, opts ...SSAProjectParamsOpt) *schema.SSAProject {
	project := &schema.SSAProject{
		ProjectName: projectName,
	}

	builder := &ssaProjectConfigBuilder{
		project:    project,
		ruleFilter: &ypb.SyntaxFlowRuleFilter{},
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
	if builder.ruleFilter != nil {
		err := project.SetRuleFilter(builder.ruleFilter)
		if err != nil {
			log.Errorf("set rule filter failed: %s", err)
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

	"withLanguage":           WithLanguage,
	"withTags":               WithTags,
	"withSourceKind":         WithSourceKind,
	"withLocalFile":          WithLocalFile,
	"withURL":                WithURL,
	"withBranch":             WithBranch,
	"withPath":               WithPath,
	"withDescription":        WithDescription,
	"withStrictMode":         WithStrictMode,
	"withPeepholeSize":       WithPeepholeSize,
	"withExcludeFiles":       WithExcludeFiles,
	"withReCompile":          WithReCompile,
	"withCompileConcurrency": WithCompileConcurrency,
	"withScanConcurrency":    WithScanConcurrency,
	"withMemoryScan":         WithMemoryScan,
	"withIgnoreLanguage":     WithIgnoreLanguage,
	"withProxyURL":           WithProxyURL,
	"withProxyAuth":          WithProxyAuth,
	"withAuthKind":           WithAuthKind,
	"withAuthUsername":       WithAuthUsername,
	"withAuthPassword":       WithAuthPassword,
	"withAuthKeyPath":        WithAuthKeyPath,

	// 规则过滤器选项
	"withRuleFilterLanguage":   WithRuleFilterLanguage,
	"withRuleFilterSeverity":   WithRuleFilterSeverity,
	"withRuleFilterKind":       WithRuleFilterKind,
	"withRuleFilterPurpose":    WithRuleFilterPurpose,
	"withRuleFilterKeyword":    WithRuleFilterKeyword,
	"withRuleFilterGroupNames": WithRuleFilterGroupNames,
	"withRuleFilterRuleNames":  WithRuleFilterRuleNames,
	"withRuleFilterTag":        WithRuleFilterTag,

	"withCompileConfigInfo": WithCompileConfigInfo,
	"withScanConfigInfo":    WithScanConfigInfo,
}
