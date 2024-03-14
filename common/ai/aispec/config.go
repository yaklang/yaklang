package aispec

type AIConfig struct {
	// gateway network config
	BaseURL string
	Domain  string
	NoHttps bool

	// basic model
	Model   string
	Timeout float64

	APIKey string
	Proxy  string
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

func WithTimeout(timeout float64) AIConfigOption {
	return func(c *AIConfig) {
		c.Timeout = timeout
	}
}
