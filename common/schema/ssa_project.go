package schema

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SSAProjectSourceKind string

const (
	SSAProjectSourceLocal       SSAProjectSourceKind = "local"       // 本地路径
	SSAProjectSourceCompression SSAProjectSourceKind = "compression" // 压缩包
	SSAProjectSourceJar         SSAProjectSourceKind = "jar"         // Jar文件
	SSAProjectSourceGit         SSAProjectSourceKind = "git"         // Git仓库
	SSAProjectSourceSvn         SSAProjectSourceKind = "svn"         // SVN仓库
)

// SSAProject 用于配置SSA的项目信息，包括项目名称、源码获取方式以及编译、扫描选项等
type SSAProject struct {
	gorm.Model
	ProjectName string `json:"project_name" gorm:"unique_index;not null;comment:项目名称"`

	// 源码获取方式配置
	SourceKind SSAProjectSourceKind `json:"source_kind" gorm:"not null;comment:源码获取方式"`
	LocalPath  string               `json:"local_path,omitempty" gorm:"comment:本地路径"`
	URL        string               `json:"url,omitempty" gorm:"comment:远程URL(适用于git/svn/压缩包等)"`
	Branch     string               `json:"branch,omitempty" gorm:"comment:Git或SVN分支"`
	GitPath    string               `json:"git_path,omitempty" gorm:"comment:Git仓库中的子路径"`

	// 认证信息
	AuthKind     string `json:"auth_kind,omitempty" gorm:"comment:认证方式:password/ssh_key"`
	AuthUsername string `json:"auth_username,omitempty" gorm:"comment:用户名"`
	AuthPassword string `json:"auth_password,omitempty" gorm:"comment:密码或token"`
	AuthKeyPath  string `json:"auth_key_path,omitempty" gorm:"comment:SSH私钥路径"`

	// 代理配置
	ProxyURL      string `json:"proxy_url,omitempty" gorm:"comment:代理URL"`
	ProxyUser     string `json:"proxy_user,omitempty" gorm:"comment:代理用户名"`
	ProxyPassword string `json:"proxy_password,omitempty" gorm:"comment:代理密码"`

	// 编译配置选项
	Description  string `json:"description,omitempty" gorm:"comment:项目描述"`
	StrictMode   bool   `json:"strict_mode" gorm:"default:false;comment:是否启用严格模式"`
	PeepholeSize int    `json:"peephole_size" gorm:"default:100;comment:窥孔编译大小"`
	ExcludeFiles string `json:"exclude_files,omitempty" gorm:"comment:排除文件列表,逗号分隔"`
	ReCompile    bool   `json:"re_compile" gorm:"default:false;comment:是否重新编译"`

	// 扫描配置选项
	ScanConcurrency uint32 `json:"scan_concurrency" gorm:"default:5;comment:扫描并发数"`
	MemoryScan      bool   `json:"memory_scan" gorm:"default:false;comment:是否使用内存扫描"`
	ScanRuleGroups  string `json:"scan_rule_groups,omitempty" gorm:"comment:扫描规则组,逗号分隔"`
	ScanRuleNames   string `json:"scan_rule_names,omitempty" gorm:"comment:扫描规则名称,逗号分隔"`
	IgnoreLanguage  bool   `json:"ignore_language" gorm:"default:false;comment:是否忽略语言检查"`
}

func (p *SSAProject) GetSourceCodeInfo() (map[string]interface{}, error) {
	configInfo := map[string]interface{}{
		"kind": string(p.SourceKind),
	}

	switch p.SourceKind {
	case SSAProjectSourceLocal:
		if p.LocalPath == "" {
			return nil, fmt.Errorf("local path is required for local source kind")
		}
		configInfo["local_file"] = p.LocalPath

	case SSAProjectSourceCompression, SSAProjectSourceJar:
		if p.LocalPath != "" {
			configInfo["local_file"] = p.LocalPath
		} else if p.URL != "" {
			configInfo["url"] = p.URL
		} else {
			return nil, fmt.Errorf("either local_file or url is required for %s source kind", p.SourceKind)
		}

	case SSAProjectSourceGit, SSAProjectSourceSvn:
		if p.URL == "" {
			return nil, fmt.Errorf("url is required for %s source kind", p.SourceKind)
		}
		configInfo["url"] = p.URL
		if p.Branch != "" {
			configInfo["branch"] = p.Branch
		}
		if p.GitPath != "" {
			configInfo["path"] = p.GitPath
		}

		// 认证信息
		if p.AuthKind != "" {
			auth := map[string]interface{}{
				"kind": p.AuthKind,
			}
			if p.AuthUsername != "" {
				auth["user_name"] = p.AuthUsername
			}
			if p.AuthPassword != "" {
				auth["password"] = p.AuthPassword
			}
			if p.AuthKeyPath != "" {
				auth["key_path"] = p.AuthKeyPath
			}
			configInfo["auth"] = auth
		}

		// 代理信息
		if p.ProxyURL != "" {
			proxy := map[string]interface{}{
				"url": p.ProxyURL,
			}
			if p.ProxyUser != "" {
				proxy["user"] = p.ProxyUser
			}
			if p.ProxyPassword != "" {
				proxy["password"] = p.ProxyPassword
			}
			configInfo["proxy"] = proxy
		}

	default:
		return nil, fmt.Errorf("unsupported source kind: %s", p.SourceKind)
	}

	return configInfo, nil
}

func FromConfigInfoRaw(projectName, configRaw string) (*SSAProject, error) {
	var configInfo map[string]interface{}
	if err := json.Unmarshal([]byte(configRaw), &configInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config info: %v", err)
	}

	project := &SSAProject{
		ProjectName: projectName,
	}

	// 解析source kind
	if kind, ok := configInfo["kind"].(string); ok {
		project.SourceKind = SSAProjectSourceKind(kind)
	} else {
		return nil, fmt.Errorf("missing or invalid kind field")
	}

	if localFile, ok := configInfo["local_file"].(string); ok {
		project.LocalPath = localFile
	}
	if url, ok := configInfo["url"].(string); ok {
		project.URL = url
	}
	if branch, ok := configInfo["branch"].(string); ok {
		project.Branch = branch
	}
	if path, ok := configInfo["path"].(string); ok {
		project.GitPath = path
	}

	// 认证信息
	if authData, ok := configInfo["auth"].(map[string]interface{}); ok {
		if kind, ok := authData["kind"].(string); ok {
			project.AuthKind = kind
		}
		if username, ok := authData["user_name"].(string); ok {
			project.AuthUsername = username
		}
		if password, ok := authData["password"].(string); ok {
			project.AuthPassword = password
		}
		if keyPath, ok := authData["key_path"].(string); ok {
			project.AuthKeyPath = keyPath
		}
	}

	// 代理信息
	if proxyData, ok := configInfo["proxy"].(map[string]interface{}); ok {
		if url, ok := proxyData["url"].(string); ok {
			project.ProxyURL = url
		}
		if user, ok := proxyData["user"].(string); ok {
			project.ProxyUser = user
		}
		if password, ok := proxyData["password"].(string); ok {
			project.ProxyPassword = password
		}
	}

	return project, nil
}

// Validate 验证SSAProject配置的有效性
func (p *SSAProject) Validate() error {
	if p.ProjectName == "" {
		return fmt.Errorf("project name is required")
	}

	switch p.SourceKind {
	case SSAProjectSourceLocal:
		if p.LocalPath == "" {
			return fmt.Errorf("local path is required for local source")
		}
	case SSAProjectSourceCompression, SSAProjectSourceJar:
		if p.LocalPath == "" && p.URL == "" {
			return fmt.Errorf("either local path or URL is required for %s source", p.SourceKind)
		}
	case SSAProjectSourceGit, SSAProjectSourceSvn:
		if p.URL == "" {
			return fmt.Errorf("URL is required for %s source", p.SourceKind)
		}
	default:
		return fmt.Errorf("unsupported source kind: %s", p.SourceKind)
	}

	return nil
}

func (p *SSAProject) GetCompileOptions() (map[string]interface{}, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("project validation failed: %v", err)
	}

	configInfo, err := p.GetSourceCodeInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to generate config info: %v", err)
	}

	optionsMap := map[string]interface{}{
		"configInfo":   configInfo,
		"programName":  p.ProjectName,
		"description":  p.Description,
		"strictMode":   p.StrictMode,
		"peepholeSize": p.PeepholeSize,
		"reCompile":    p.ReCompile,
		"excludeFiles": p.GetExcludeFilesList(),
	}

	return optionsMap, nil
}

func (p *SSAProject) GetScanOptions() (map[string]interface{}, error) {
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("project validation failed: %v", err)
	}
	// 扫描相关的选项
	scanOptions := map[string]interface{}{
		"programName":    []string{p.ProjectName},
		"concurrency":    p.ScanConcurrency,
		"memory":         p.MemoryScan,
		"ignoreLanguage": p.IgnoreLanguage,
	}

	if p.ScanRuleGroups != "" {
		scanOptions["ruleGroups"] = p.GetScanRuleGroupsList()
	}

	if p.ScanRuleNames != "" {
		scanOptions["ruleNames"] = p.GetScanRuleNamesList()
	}

	return scanOptions, nil
}

func (p *SSAProject) GetExcludeFilesList() []string {
	if p.ExcludeFiles == "" {
		return nil
	}
	files := make([]string, 0)
	for _, file := range strings.Split(p.ExcludeFiles, ",") {
		file = strings.TrimSpace(file)
		if file != "" {
			files = append(files, file)
		}
	}
	return files
}

func (p *SSAProject) SetExcludeFilesList(files []string) {
	p.ExcludeFiles = strings.Join(files, ",")
}

func (p *SSAProject) GetScanRuleGroupsList() []string {
	if p.ScanRuleGroups == "" {
		return nil
	}
	groups := make([]string, 0)
	for _, group := range strings.Split(p.ScanRuleGroups, ",") {
		group = strings.TrimSpace(group)
		if group != "" {
			groups = append(groups, group)
		}
	}
	return groups
}

func (p *SSAProject) SetScanRuleGroupsList(groups []string) {
	p.ScanRuleGroups = strings.Join(groups, ",")
}

func (p *SSAProject) GetScanRuleNamesList() []string {
	if p.ScanRuleNames == "" {
		return nil
	}
	names := make([]string, 0)
	for _, name := range strings.Split(p.ScanRuleNames, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (p *SSAProject) SetScanRuleNamesList(names []string) {
	p.ScanRuleNames = strings.Join(names, ",")
}

func (p *SSAProject) HasScanRules() bool {
	return p.ScanRuleGroups != "" || p.ScanRuleNames != ""
}

func (p *SSAProject) GetScanConcurrency() uint32 {
	if p.ScanConcurrency == 0 {
		return 5 // 默认并发数
	}
	return p.ScanConcurrency
}

func (p *SSAProject) ToGRPCModel() *ypb.SSAProject {
	return &ypb.SSAProject{
		ID:              int64(p.ID),
		CreatedAt:       p.CreatedAt.Unix(),
		UpdatedAt:       p.UpdatedAt.Unix(),
		ProjectName:     p.ProjectName,
		SourceKind:      string(p.SourceKind),
		LocalPath:       p.LocalPath,
		URL:             p.URL,
		Branch:          p.Branch,
		GitPath:         p.GitPath,
		AuthKind:        p.AuthKind,
		AuthUsername:    p.AuthUsername,
		AuthPassword:    p.AuthPassword,
		AuthKeyPath:     p.AuthKeyPath,
		ProxyURL:        p.ProxyURL,
		ProxyUser:       p.ProxyUser,
		ProxyPassword:   p.ProxyPassword,
		Description:     p.Description,
		StrictMode:      p.StrictMode,
		PeepholeSize:    int32(p.PeepholeSize),
		ExcludeFiles:    p.ExcludeFiles,
		ReCompile:       p.ReCompile,
		ScanConcurrency: p.ScanConcurrency,
		MemoryScan:      p.MemoryScan,
		ScanRuleGroups:  p.ScanRuleGroups,
		ScanRuleNames:   p.ScanRuleNames,
		IgnoreLanguage:  p.IgnoreLanguage,
	}
}
