package browser

import "time"

const defaultBrowserID = "default"

type BrowserConfig struct {
	id string

	controlURL   string
	wsAddress    string
	exePath      string
	proxyAddress string

	noSandBox bool
	headless  bool
	leakless  bool

	timeout time.Duration
}

type BrowserOption func(*BrowserConfig)

func newDefaultConfig() *BrowserConfig {
	return &BrowserConfig{
		id:        defaultBrowserID,
		noSandBox: true,
		headless:  true,
		leakless:  false,
		timeout:   30 * time.Second,
	}
}

func parseBrowserOptions(opts ...BrowserOption) *BrowserConfig {
	config := newDefaultConfig()
	for _, opt := range opts {
		opt(config)
	}
	return config
}

func WithID(id string) BrowserOption {
	return func(c *BrowserConfig) {
		c.id = id
	}
}

func WithWsAddress(wsAddress string) BrowserOption {
	return func(c *BrowserConfig) {
		c.wsAddress = wsAddress
	}
}

func WithExePath(exePath string) BrowserOption {
	return func(c *BrowserConfig) {
		c.exePath = exePath
	}
}

func WithProxy(proxyAddress string) BrowserOption {
	return func(c *BrowserConfig) {
		c.proxyAddress = proxyAddress
	}
}

func WithNoSandBox(noSandBox bool) BrowserOption {
	return func(c *BrowserConfig) {
		c.noSandBox = noSandBox
	}
}

func WithHeadless(headless bool) BrowserOption {
	return func(c *BrowserConfig) {
		c.headless = headless
	}
}

func WithLeakless(leakless bool) BrowserOption {
	return func(c *BrowserConfig) {
		c.leakless = leakless
	}
}

func WithTimeout(timeout float64) BrowserOption {
	return func(c *BrowserConfig) {
		c.timeout = time.Duration(timeout * float64(time.Second))
	}
}

func WithControlURL(controlURL string) BrowserOption {
	return func(c *BrowserConfig) {
		c.controlURL = controlURL
	}
}
