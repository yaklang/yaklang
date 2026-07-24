package aivizhttp

// Option 是配置 viz server 的函数式选项
type Option func(*VizServerConfig)

// WithHost 设置绑定地址
func WithHost(host string) Option {
	return func(c *VizServerConfig) {
		if host != "" {
			c.Host = host
		}
	}
}

// WithPort 设置绑定端口
func WithPort(port int) Option {
	return func(c *VizServerConfig) {
		if port > 0 {
			c.Port = port
		}
	}
}

// WithRoutePrefix 设置 API 路由前缀
func WithRoutePrefix(prefix string) Option {
	return func(c *VizServerConfig) {
		if prefix != "" {
			c.RoutePrefix = prefix
		}
	}
}

// WithAuthToken 设置 Bearer 认证 token, 空则不启用认证
func WithAuthToken(token string) Option {
	return func(c *VizServerConfig) {
		if token != "" {
			c.AuthToken = token
		}
	}
}

// WithServeFrontend 设置是否提供内置前端页面
func WithServeFrontend(enable bool) Option {
	return func(c *VizServerConfig) {
		c.ServeFrontend = enable
	}
}

// WithDebug 设置调试日志
func WithDebug(enable bool) Option {
	return func(c *VizServerConfig) {
		c.Debug = enable
	}
}
