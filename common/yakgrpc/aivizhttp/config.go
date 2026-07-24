package aivizhttp

// VizServerConfig 可视化监控服务的配置
// 关键词: viz config, agent monitor, dashboard
type VizServerConfig struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	RoutePrefix string `yaml:"route_prefix"`
	AuthToken   string `yaml:"auth_token"`

	ServeFrontend bool `yaml:"serve_frontend"`
	Debug         bool `yaml:"debug"`
}

// DefaultTitle 默认页面标题
const DefaultTitle = "Yaklang Agent Viz"

// NewDefaultConfig 返回带默认值的配置
func NewDefaultConfig() *VizServerConfig {
	return &VizServerConfig{
		Host:          "127.0.0.1",
		Port:          9100,
		RoutePrefix:   "/api/viz",
		ServeFrontend: true,
		Debug:         false,
	}
}

// fillDefaults 为零值字段回填默认值
func (c *VizServerConfig) fillDefaults() {
	if c.Host == "" {
		c.Host = "127.0.0.1"
	}
	if c.Port == 0 {
		c.Port = 9100
	}
	if c.RoutePrefix == "" {
		c.RoutePrefix = "/api/viz"
	}
}
