package aispec

import (
	"io"
	"os"
	"time"
)

type AIConfig struct {
	// gateway network config
	BaseURL string
	Domain  string
	NoHttps bool

	// basic model
	Model    string
	Timeout  float64
	Deadline time.Time

	APIKey        string
	Proxy         string
	StreamHandler func(io.Reader)
	Type          string
}

func NewDefaultAIConfig(opts ...AIConfigOption) *AIConfig {
	c := &AIConfig{
		Timeout: 30,
	}
	for _, p := range opts {
		p(c)
	}
	return c
}

type AIConfigOption func(*AIConfig)

func WithBaseURL(baseURL string) AIConfigOption {
	return func(c *AIConfig) {
		c.BaseURL = baseURL
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
