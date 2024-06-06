package aispec

import (
	"github.com/yaklang/yaklang/common/consts"
	"io"
	"os"
	"time"
)

type AIConfig struct {
	// gateway network config
	BaseURL string
	Domain  string `app:"name:第三方加速域名,id:4"`
	NoHttps bool   `app:"name:IsHttps,desc:是否使用https请求api,id:3"`

	// basic model
	Model    string  `app:"name:模型名称,id:2"`
	Timeout  float64 // `app:"name:请求超时时长"`
	Deadline time.Time

	APIKey        string `app:"name:ApiKey,desc:APIKey / Token,required:true,id:1"`
	Proxy         string `app:"name:代理地址,id:5"`
	StreamHandler func(io.Reader)
	Type          string

	FunctionCallRetryTimes int
}

func NewDefaultAIConfig(opts ...AIConfigOption) *AIConfig {
	c := &AIConfig{
		Timeout:                120,
		FunctionCallRetryTimes: 5,
	}
	for _, p := range opts {
		p(c)
	}
	cfg := consts.GetThirdPartyApplicationConfig(c.Type)
	if cfg.APIKey != "" {
		c.APIKey = cfg.APIKey
	}
	if cfg.Domain != "" {
		c.Domain = cfg.Domain
	}
	if cfg.GetExtraParam("model") != "" {
		c.Model = cfg.GetExtraParam("model")
	}
	if cfg.GetExtraParam("domain") != "" {
		c.Domain = cfg.GetExtraParam("domain")
	}
	if cfg.GetExtraParam("proxy") != "" {
		c.Proxy = cfg.GetExtraParam("proxy")
	}
	return c
}

type AIConfigOption func(*AIConfig)

func WithBaseURL(baseURL string) AIConfigOption {
	return func(c *AIConfig) {
		c.BaseURL = baseURL
	}
}

func WithStreamAndConfigHandler(h func(reader io.Reader, cfg *AIConfig)) AIConfigOption {
	return func(c *AIConfig) {
		c.StreamHandler = func(reader io.Reader) {
			h(reader, c)
		}
	}
}

func WithStreamHandler(h func(io.Reader)) AIConfigOption {
	return func(c *AIConfig) {
		c.StreamHandler = h
	}
}

func WithDebugStream(h ...bool) AIConfigOption {
	return func(c *AIConfig) {
		if len(h) <= 0 {
			c.StreamHandler = func(r io.Reader) {
				io.Copy(os.Stdout, r)
			}
			return
		}
		if h[0] {
			c.StreamHandler = func(r io.Reader) {
				io.Copy(os.Stdout, r)
			}
		}
	}
}

func WithDomain(domain string) AIConfigOption {
	return func(c *AIConfig) {
		c.Domain = domain
	}
}

func WithModel(model string) AIConfigOption {
	return func(c *AIConfig) {
		c.Model = model
	}
}

func WithType(t string) AIConfigOption {
	return func(config *AIConfig) {
		config.Type = t
	}
}

func WithTimeout(timeout float64) AIConfigOption {
	return func(c *AIConfig) {
		c.Timeout = timeout
	}
}

func WithProxy(p string) AIConfigOption {
	return func(c *AIConfig) {
		c.Proxy = p
	}
}

func WithAPIKey(k string) AIConfigOption {
	return func(c *AIConfig) {
		c.APIKey = k
	}
}

func WithNoHttps(b bool) AIConfigOption {
	return func(c *AIConfig) {
		c.NoHttps = b
	}
}

func WithFunctionCallRetryTimes(times int) AIConfigOption {
	return func(c *AIConfig) {
		c.FunctionCallRetryTimes = times
	}
}
