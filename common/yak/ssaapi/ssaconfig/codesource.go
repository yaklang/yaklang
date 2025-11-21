package ssaconfig

import (
	"encoding/json"

	"github.com/yaklang/yaklang/common/utils"
)

// CodeSourceKind 源码获取方式枚举
type CodeSourceKind string

const (
	CodeSourceNone        CodeSourceKind = ""            // 未指定
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

func NewLocalFileCodeSourceConfig(localFile string) *CodeSourceInfo {
	return &CodeSourceInfo{
		Kind:      CodeSourceLocal,
		LocalFile: localFile,
	}
}

func (c *CodeSourceInfo) ToJSONString() string {
	if c == nil {
		return ""
	}
	jsonRaw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(jsonRaw)
}

func (c *CodeSourceInfo) GetCodeSourceURL() string {
	if c == nil {
		return ""
	}
	switch c.Kind {
	case CodeSourceLocal:
		return c.LocalFile
	default:
		return c.URL
	}
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

// --- 代码源配置 Get 方法 ---

func (c *Config) GetCodeSource() *CodeSourceInfo {
	if c == nil || c.Mode&ModeCodeSource == 0 {
		return nil
	}
	return c.CodeSource
}

func (c *Config) GetCodeSourceKind() CodeSourceKind {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Kind
}

func (c *Config) GetCodeSourceLocalFile() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.LocalFile
}

func (c *Config) GetCodeSourceURL() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.URL
}

func (c *Config) GetCodeSourceBranch() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Branch
}

func (c *Config) GetCodeSourcePath() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return ""
	}
	return c.CodeSource.Path
}

func (c *Config) GetCodeSourceAuthKind() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil || c.CodeSource.Auth == nil {
		return ""
	}
	return c.CodeSource.Auth.Kind
}

func (c *Config) GetCodeSourceAuthUserName() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil || c.CodeSource.Auth == nil {
		return ""
	}
	return c.CodeSource.Auth.UserName
}

func (c *Config) GetCodeSourceAuthPassword() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil || c.CodeSource.Auth == nil {
		return ""
	}
	return c.CodeSource.Auth.Password
}

func (c *Config) GetCodeSourceProxyURL() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil || c.CodeSource.Proxy == nil {
		return ""
	}
	return c.CodeSource.Proxy.URL
}

func (c *Config) GetCodeSourceProxyAuth() (string, string) {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil || c.CodeSource.Proxy == nil {
		return "", ""
	}
	return c.CodeSource.Proxy.User, c.CodeSource.Proxy.Password
}

func (c *Config) GetCodeSourceAuth() *AuthConfigInfo {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return nil
	}
	return c.CodeSource.Auth
}

// ---代码源配置 Options ---

// WithCodeSourceKind 设置代码源类型
func WithCodeSourceKind(kind CodeSourceKind) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Kind"); err != nil {
			return err
		}
		c.CodeSource.Kind = kind
		return nil
	}
}

func WithCodeSourceLocalFile(localFile string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Local File"); err != nil {
			return err
		}
		c.CodeSource.LocalFile = localFile
		return nil
	}
}

func WithCodeSourceURL(url string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source URL"); err != nil {
			return err
		}
		c.CodeSource.URL = url
		return nil
	}
}

func WithCodeSourceBranch(branch string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Branch"); err != nil {
			return err
		}
		c.CodeSource.Branch = branch
		return nil
	}
}

func WithCodeSourcePath(path string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Path"); err != nil {
			return err
		}
		c.CodeSource.Path = path
		return nil
	}
}

func WithCodeSourceAuthKind(kind string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Auth Kind"); err != nil {
			return err
		}
		if c.CodeSource.Auth == nil {
			c.CodeSource.Auth = &AuthConfigInfo{}
		}
		c.CodeSource.Auth.Kind = kind
		return nil
	}
}

func WithCodeSourceAuthUserName(userName string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Auth User Name"); err != nil {
			return err
		}
		if c.CodeSource.Auth == nil {
			c.CodeSource.Auth = &AuthConfigInfo{}
		}
		c.CodeSource.Auth.UserName = userName
		return nil
	}
}

func WithCodeSourceAuthPassword(password string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Auth Password"); err != nil {
			return err
		}
		if c.CodeSource.Auth == nil {
			c.CodeSource.Auth = &AuthConfigInfo{}
		}
		c.CodeSource.Auth.Password = password
		return nil
	}
}

func WithSSAProjectCodeSourceAuthKeyPath(keyPath string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Auth Key Path"); err != nil {
			return err
		}
		if c.CodeSource.Auth == nil {
			c.CodeSource.Auth = &AuthConfigInfo{}
		}
		c.CodeSource.Auth.KeyPath = keyPath
		return nil
	}
}

func WithCodeSourceProxyURL(url string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Proxy URL"); err != nil {
			return err
		}
		if c.CodeSource.Proxy == nil {
			c.CodeSource.Proxy = &ProxyConfigInfo{}
		}
		c.CodeSource.Proxy.URL = url
		return nil
	}
}

func WithCodeSourceProxyAuth(user string, password string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Proxy Auth"); err != nil {
			return err
		}
		if c.CodeSource.Proxy == nil {
			c.CodeSource.Proxy = &ProxyConfigInfo{}
		}
		c.CodeSource.Proxy.User = user
		c.CodeSource.Proxy.Password = password
		return nil
	}
}

func WithCodeSourceJson(raw string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source JSON"); err != nil {
			return err
		}
		codeSource := &CodeSourceInfo{}
		err := json.Unmarshal([]byte(raw), codeSource)
		c.CodeSource = codeSource
		if err != nil {
			return utils.Errorf("Config: Code Source JSON Unmarshal failed: %v", err)
		}
		if err := c.CodeSource.ValidateSourceConfig(); err != nil {
			return utils.Errorf("Config: Code Source JSON Validate failed: %v", err)
		}
		return nil
	}
}

func WithCodeSourceMap(input map[string]any) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Map"); err != nil {
			return err
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return utils.Errorf("Config: Code Source Map Marshal failed: %v", err)
		}
		err = json.Unmarshal(raw, c.CodeSource)
		if err != nil {
			return utils.Errorf("Config: Code Source Map Unmarshal failed: %v", err)
		}
		if err := c.CodeSource.ValidateSourceConfig(); err != nil {
			return utils.Errorf("Config: Code Source Map Validate failed: %v", err)
		}
		return nil
	}
}

func WithCodeSourceInfo(info *CodeSourceInfo) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Info"); err != nil {
			return err
		}
		c.CodeSource = info
		return nil
	}
}
