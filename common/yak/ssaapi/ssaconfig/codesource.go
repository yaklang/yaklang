package ssaconfig

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/utils"
)

// CodeSourceKind 源码获取方式枚举
type CodeSourceKind string

const (
	CodeSourceLocal       CodeSourceKind = "local"       // 本地文件/目录
	CodeSourceCompression CodeSourceKind = "compression" // 压缩文件
	CodeSourceJar         CodeSourceKind = "jar"         // Jar文件
	CodeSourceGit         CodeSourceKind = "git"         // Git仓库
	CodeSourceSvn         CodeSourceKind = "svn"         // SVN仓库
)

type AuthConfigInfo struct {
	Kind     string `json:"kind"`                // 认证方式: password/ssh_key/token
	UserName string `json:"user_name,omitempty"` // 用户名
	Password string `json:"password,omitempty"`  // 密码或token
	KeyPath  string `json:"key_path,omitempty"`  // SSH私钥路径
}

type ProxyConfigInfo struct {
	URL      string `json:"url"`                // 代理URL
	User     string `json:"user,omitempty"`     // 代理用户名
	Password string `json:"password,omitempty"` // 代理密码
}

// CodeSourceInfo 代码源配置信息
type CodeSourceInfo struct {
	Kind      CodeSourceKind   `json:"kind"`                 // 源码获取方式: local/compression/jar/git/svn
	LocalFile string           `json:"local_file,omitempty"` // 本地路径
	URL       string           `json:"url,omitempty"`        // 远程URL (git/svn/tar/jar)
	Branch    string           `json:"branch,omitempty"`     // git或svn分支
	Path      string           `json:"path,omitempty"`       // git仓库中的子路径
	Auth      *AuthConfigInfo  `json:"auth,omitempty"`       // 认证信息
	Proxy     *ProxyConfigInfo `json:"proxy,omitempty"`      // 代理配置
}

func (c *CodeSourceInfo) JsonString() string {
	json, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(json)
}

// ValidateSourceConfig 验证代码源配置的有效性
func (c *CodeSourceInfo) ValidateSourceConfig() error {
	if c.Kind == "" {
		return utils.Errorf("source kind is required")
	}

	switch c.Kind {
	case CodeSourceLocal:
		if c.LocalFile == "" {
			return utils.Errorf("local_file is required for local source")
		}
	case CodeSourceCompression, CodeSourceJar:
		if c.LocalFile == "" && c.URL == "" {
			return utils.Errorf("either local_file or url is required for %s source", c.Kind)
		}
	case CodeSourceGit, CodeSourceSvn:
		if c.URL == "" {
			return utils.Errorf("url is required for %s source", c.Kind)
		}
	default:
		return utils.Errorf("unsupported source kind: %s", c.Kind)
	}

	return nil
}
