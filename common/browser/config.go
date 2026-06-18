package browser

import (
	"runtime"
	"time"
)

const defaultBrowserID = "default"

// defaultLeakless 决定是否默认开启 go-rod 的 leakless 守护进程。
// leakless 会拉起一个监视进程，在父进程(yak)退出时强制 kill 浏览器，
// 从而避免 yak 退出后 Chromium 变成孤儿进程无法回收。
// Windows 上 leakless 的辅助二进制经常被杀软误报，因此与 crawlerx 保持一致，仅在非 Windows 默认开启。
func defaultLeakless() bool {
	return runtime.GOOS != "windows"
}

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
		leakless:  defaultLeakless(),
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
