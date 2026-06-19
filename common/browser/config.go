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

// WithID 指定浏览器实例 ID（导出名为 browser.id）
// 参数:
//   - id: 浏览器实例 ID
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.id("main")
// println(opt)
// ```
func WithID(id string) BrowserOption {
	return func(c *BrowserConfig) {
		c.id = id
	}
}

// WithWsAddress 指定通过 WebSocket 地址连接已有浏览器（导出名为 browser.wsAddress）
// 参数:
//   - wsAddress: 浏览器调试 WebSocket 地址
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.wsAddress("ws://127.0.0.1:9222/devtools/browser/xxx")
// println(opt)
// ```
func WithWsAddress(wsAddress string) BrowserOption {
	return func(c *BrowserConfig) {
		c.wsAddress = wsAddress
	}
}

// WithExePath 指定浏览器可执行文件路径（导出名为 browser.exePath）
// 参数:
//   - exePath: 浏览器可执行文件路径
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.exePath("/usr/bin/google-chrome")
// println(opt)
// ```
func WithExePath(exePath string) BrowserOption {
	return func(c *BrowserConfig) {
		c.exePath = exePath
	}
}

// WithProxy 指定浏览器使用的代理地址（导出名为 browser.proxy）
// 参数:
//   - proxyAddress: 代理地址
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.proxy("http://127.0.0.1:8080")
// println(opt)
// ```
func WithProxy(proxyAddress string) BrowserOption {
	return func(c *BrowserConfig) {
		c.proxyAddress = proxyAddress
	}
}

// WithNoSandBox 设置浏览器是否以 no-sandbox 模式启动（导出名为 browser.noSandBox）
// 参数:
//   - noSandBox: 是否禁用沙箱
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.noSandBox(true)
// println(opt)
// ```
func WithNoSandBox(noSandBox bool) BrowserOption {
	return func(c *BrowserConfig) {
		c.noSandBox = noSandBox
	}
}

// WithHeadless 设置浏览器是否以无头模式启动（导出名为 browser.headless）
// 参数:
//   - headless: 是否无头模式
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.headless(true)
// println(opt)
// ```
func WithHeadless(headless bool) BrowserOption {
	return func(c *BrowserConfig) {
		c.headless = headless
	}
}

// WithLeakless 设置是否启用 leakless 守护进程以确保浏览器进程被清理（导出名为 browser.leakless）
// 参数:
//   - leakless: 是否启用 leakless
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.leakless(true)
// println(opt)
// ```
func WithLeakless(leakless bool) BrowserOption {
	return func(c *BrowserConfig) {
		c.leakless = leakless
	}
}

// WithTimeout 设置浏览器操作的超时时间（导出名为 browser.timeout）
// 参数:
//   - timeout: 超时时间（秒）
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.timeout(30)
// println(opt)
// ```
func WithTimeout(timeout float64) BrowserOption {
	return func(c *BrowserConfig) {
		c.timeout = time.Duration(timeout * float64(time.Second))
	}
}

// WithControlURL 指定通过 control URL 连接已有浏览器（导出名为 browser.controlURL）
// 参数:
//   - controlURL: 浏览器的控制地址
//
// 返回值:
//   - 浏览器可选项
//
// Example:
// ```
// opt = browser.controlURL("http://127.0.0.1:9222")
// println(opt)
// ```
func WithControlURL(controlURL string) BrowserOption {
	return func(c *BrowserConfig) {
		c.controlURL = controlURL
	}
}
