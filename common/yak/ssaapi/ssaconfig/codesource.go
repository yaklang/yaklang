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
	Kind       string `json:"kind"`                  // 认证方式: password/ssh_key/token
	UserName   string `json:"user_name,omitempty"`   // 用户名
	Password   string `json:"password,omitempty"`    // 密码或token
	KeyPath    string `json:"key_path,omitempty"`    // SSH私钥路径（本地文件场景）
	KeyContent string `json:"key_content,omitempty"` // SSH私钥内容
}

type ProxyConfigInfo struct {
	URL      string `json:"url"`                // 代理URL
	User     string `json:"user,omitempty"`     // 代理用户名
	Password string `json:"password,omitempty"` // 代理密码
}

// CodeSourceInfo 代码源配置信息
type CodeSourceInfo struct {
	Kind              CodeSourceKind   `json:"kind"`                          // 源码获取方式: local/compression/jar/git/svn
	LocalFile         string           `json:"local_file,omitempty"`          // 本地路径
	URL               string           `json:"url,omitempty"`                 // 远程URL (git/svn/tar/jar)
	Branch            string           `json:"branch,omitempty"`              // git或svn分支
	Path              string           `json:"path,omitempty"`                // git仓库中的子路径
	Auth              *AuthConfigInfo  `json:"auth,omitempty"`                // 认证信息
	Proxy             *ProxyConfigInfo `json:"proxy,omitempty"`               // 代理配置
	JarRecursiveParse *bool            `json:"jar_recursive_parse,omitempty"` // Jar递归解析开关，nil或true表示启用，false表示禁用
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

func (c *Config) GetCodeSourceLocalFileOrURL() string {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return ""
	}
	if c.CodeSource.URL != "" {
		return c.CodeSource.URL
	} else {
		return c.CodeSource.LocalFile
	}
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

func (c *Config) GetCodeSourceJarRecursiveParse() bool {
	if c == nil || c.Mode&ModeCodeSource == 0 || c.CodeSource == nil {
		return true // 默认启用递归解析
	}
	if c.CodeSource.JarRecursiveParse == nil {
		return true // 默认启用递归解析
	}
	return *c.CodeSource.JarRecursiveParse
}

// ---代码源配置 Options ---

// WithCodeSourceKind 设置代码源类型（导出名为 ssa.withCodeSourceKind）
// 参数:
//   - kind: 代码源类型，如本地、git 等
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceKind("local")
// println(opt)
// ```
func WithCodeSourceKind(kind CodeSourceKind) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Kind"); err != nil {
			return err
		}
		c.CodeSource.Kind = kind
		return nil
	}
}

// WithCodeSourceLocalFile 设置代码源的本地文件/目录路径（导出名为 ssa.withCodeSourceLocalFile）
// 参数:
//   - localFile: 本地文件或目录路径
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceLocalFile("/tmp/project")
// println(opt)
// ```
func WithCodeSourceLocalFile(localFile string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Local File"); err != nil {
			return err
		}
		c.CodeSource.LocalFile = localFile
		return nil
	}
}

// WithCodeSourceURL 设置代码源的远程地址（导出名为 ssa.withCodeSourceURL）
// 参数:
//   - url: 代码仓库 URL
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceURL("https://github.com/yaklang/yaklang.git")
// println(opt)
// ```
func WithCodeSourceURL(url string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source URL"); err != nil {
			return err
		}
		c.CodeSource.URL = url
		return nil
	}
}

// WithCodeSourceBranch 设置代码源的分支（导出名为 ssa.withCodeSourceBranch）
// 参数:
//   - branch: 分支名
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceBranch("main")
// println(opt)
// ```
func WithCodeSourceBranch(branch string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Branch"); err != nil {
			return err
		}
		c.CodeSource.Branch = branch
		return nil
	}
}

// WithCodeSourcePath 设置代码源在仓库内的子路径（导出名为 ssa.withCodeSourcePath）
// 参数:
//   - path: 仓库内的子路径
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourcePath("backend/")
// println(opt)
// ```
func WithCodeSourcePath(path string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Path"); err != nil {
			return err
		}
		c.CodeSource.Path = path
		return nil
	}
}

// WithCodeSourceAuthKind 设置代码源认证方式（导出名为 ssa.withCodeSourceAuthKind）
// 参数:
//   - kind: 认证类型，如 "password"、"ssh_key"
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceAuthKind("password")
// println(opt)
// ```
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

// WithCodeSourceAuthUserName 设置代码源认证用户名（导出名为 ssa.withCodeSourceAuthUserName）
// 参数:
//   - userName: 用户名
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceAuthUserName("git")
// println(opt)
// ```
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

// WithCodeSourceAuthPassword 设置代码源认证密码或令牌（导出名为 ssa.withCodeSourceAuthPassword）
// 参数:
//   - password: 密码或访问令牌
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceAuthPassword("your-token")
// println(opt)
// ```
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

// WithSSAProjectCodeSourceAuthKeyPath 设置代码源 SSH 私钥文件路径（导出名为 ssa.withCodeSourceAuthKeyPath）
// 参数:
//   - keyPath: SSH 私钥文件路径
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceAuthKeyPath("/root/.ssh/id_rsa")
// println(opt)
// ```
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

// WithCodeSourceAuthKeyContent 设置 SSH 私钥内容（PEM 格式，适用于分布式场景，导出名为 ssa.withCodeSourceAuthKeyContent）
// 参数:
//   - keyContent: SSH 私钥的 PEM 文本内容
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceAuthKeyContent("-----BEGIN OPENSSH PRIVATE KEY-----...")
// println(opt)
// ```
func WithCodeSourceAuthKeyContent(keyContent string) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Auth Key Content"); err != nil {
			return err
		}
		if c.CodeSource.Auth == nil {
			c.CodeSource.Auth = &AuthConfigInfo{}
		}
		c.CodeSource.Auth.KeyContent = keyContent
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

// WithCodeSourceMap 以 map 形式批量设置代码源配置（导出名为 ssa.withConfigInfo）
// 参数:
//   - input: 代码源配置字典，如 {"kind": "local", "local_file": "/tmp/project"}
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withConfigInfo({"kind": "local", "local_file": "/tmp/project"})
// println(opt)
// ```
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

// WithCodeSourceJarRecursiveParse 设置是否对 Jar 包进行递归解析（导出名为 ssa.withCodeSourceJarRecursiveParse）
// 参数:
//   - enable: 是否递归解析 Jar 包
//
// 返回值:
//   - 代码源配置可选项
//
// Example:
// ```
// opt = ssa.withCodeSourceJarRecursiveParse(true)
// println(opt)
// ```
func WithCodeSourceJarRecursiveParse(enable bool) Option {
	return func(c *Config) error {
		if err := c.ensureCodeSource("Code Source Jar Recursive Parse"); err != nil {
			return err
		}
		c.CodeSource.JarRecursiveParse = &enable
		return nil
	}
}
