package totpproxy

import (
	"errors"
	"time"
)

const (
	// DefaultTOTPHeader 默认的 TOTP 验证头
	DefaultTOTPHeader = "Y-T-Verify-Code"
	// DefaultTimeout 默认请求超时时间
	DefaultTimeout = 120 * time.Second
)

var (
	// ErrMissingListenAddr 缺少监听地址
	ErrMissingListenAddr = errors.New("missing listen address: listenAddr is required")
	// ErrMissingTargetAddr 缺少目标地址
	ErrMissingTargetAddr = errors.New("missing target address: targetAddr is required")
	// ErrMissingTOTPSecret 缺少 TOTP 密钥
	ErrMissingTOTPSecret = errors.New("missing TOTP secret: totpSecret is required")
	// ErrMissingTLSConfig TLS 已启用但缺少证书配置
	ErrMissingTLSConfig = errors.New("TLS enabled but certificate/key not provided")
)

// ServerConfig 反向代理服务器配置
type ServerConfig struct {
	// 核心配置（必须显式设置）
	ListenAddr string // 监听地址，例如 "0.0.0.0:8443"
	TargetAddr string // 后端服务地址，例如 "127.0.0.1:21002"
	TOTPSecret string // TOTP 密钥

	// TLS 配置
	EnableTLS bool   // 是否启用 TLS
	TLSCert   []byte // TLS 证书内容
	TLSKey    []byte // TLS 私钥内容

	// TOTP 配置
	TOTPHeader string // TOTP 验证头名称

	// 后端配置
	TargetTLS bool // 后端是否使用 TLS

	// 可选配置
	AllowedPaths []string      // 允许的 API 路径前缀列表
	Timeout      time.Duration // 请求超时时间
	Debug        bool          // 调试模式
}

// NewDefaultServerConfig 创建默认配置
// 注意：ListenAddr、TargetAddr、TOTPSecret 必须显式设置
func NewDefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		// 核心配置 - 必须显式设置，不提供默认值
		ListenAddr: "",
		TargetAddr: "",
		TOTPSecret: "",

		// 可选配置 - 提供合理的默认值
		EnableTLS:    false,
		TOTPHeader:   DefaultTOTPHeader,
		TargetTLS:    false,
		AllowedPaths: nil, // nil 表示允许所有路径
		Timeout:      DefaultTimeout,
		Debug:        false,
	}
}

// Validate 验证配置是否完整
func (c *ServerConfig) Validate() error {
	if c.ListenAddr == "" {
		return ErrMissingListenAddr
	}
	if c.TargetAddr == "" {
		return ErrMissingTargetAddr
	}
	if c.TOTPSecret == "" {
		return ErrMissingTOTPSecret
	}
	return nil
}

// ServerOption 配置选项函数类型
type ServerOption func(*ServerConfig)

// WithListenAddr 设置监听地址
func WithListenAddr(addr string) ServerOption {
	return func(c *ServerConfig) {
		c.ListenAddr = addr
	}
}

// WithEnableTLS 启用 TLS
func WithEnableTLS(enable bool) ServerOption {
	return func(c *ServerConfig) {
		c.EnableTLS = enable
	}
}

// WithTLSCert 设置 TLS 证书
func WithTLSCert(cert, key []byte) ServerOption {
	return func(c *ServerConfig) {
		c.TLSCert = cert
		c.TLSKey = key
	}
}

// WithTOTPSecret 设置 TOTP 密钥
func WithTOTPSecret(secret string) ServerOption {
	return func(c *ServerConfig) {
		c.TOTPSecret = secret
	}
}

// WithTOTPHeader 设置 TOTP 验证头名称
func WithTOTPHeader(header string) ServerOption {
	return func(c *ServerConfig) {
		if header != "" {
			c.TOTPHeader = header
		}
	}
}

// WithTargetAddr 设置后端服务地址
func WithTargetAddr(addr string) ServerOption {
	return func(c *ServerConfig) {
		c.TargetAddr = addr
	}
}

// WithTargetTLS 设置后端是否使用 TLS
func WithTargetTLS(enable bool) ServerOption {
	return func(c *ServerConfig) {
		c.TargetTLS = enable
	}
}

// WithAllowedPaths 设置允许的路径前缀列表
func WithAllowedPaths(paths []string) ServerOption {
	return func(c *ServerConfig) {
		c.AllowedPaths = paths
	}
}

// WithTimeout 设置请求超时时间
func WithTimeout(timeout time.Duration) ServerOption {
	return func(c *ServerConfig) {
		if timeout > 0 {
			c.Timeout = timeout
		}
	}
}

// WithDebug 启用调试模式
func WithDebug(debug bool) ServerOption {
	return func(c *ServerConfig) {
		c.Debug = debug
	}
}
