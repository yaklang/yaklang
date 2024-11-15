package simple

type BrowserConfig struct {
	wsAddress     string
	exePath       string
	proxyAddress  string
	proxyUsername string
	proxyPassword string

	noSandBox bool
	headless  bool
	hijack    bool
	leakless  bool

	runtimeID  string
	fromPlugin string
	sourceType string
	saveToDB   bool

	timeout int

	responseModification []*ResponseModification
	requestModification  []*RequestModification
}

type BrowserConfigOpt func(*BrowserConfig)

// simulator.simple.wsAddress 是一个请求选项 用于输入浏览器的websocket地址
//
// Example:
// ```
//
//	proxy = simulator.simple.proxy("http://127.0.0.1:7890")
//	browser = simulator.simple.createBrowser(proxy)
//
// ```
func WithWsAddress(wsAddress string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.wsAddress = wsAddress
	}
}

// simulator.simple.exePath 是一个请求选项 用于输入浏览器的websocket地址
//
// Example:
// ```
//
//	exePath = simulator.simple.exePath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
//	browser = simulator.simple.createBrowser(exePath)
//
// ```
func WithExePath(exePath string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.exePath = exePath
	}
}

// simulator.simple.proxy 是一个请求选项 用于输入代理服务器地址
//
// Example:
// ```
//
//	proxy = simulator.simple.proxy("http://127.0.0.1:7890")
//	browser = simulator.simple.createBrowser(proxy)
//
// ```
func WithProxy(proxyAddress string, proxyUserInfo ...string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.proxyAddress = proxyAddress
		if len(proxyUserInfo) > 1 {
			config.proxyUsername = proxyUserInfo[0]
			config.proxyPassword = proxyUserInfo[1]
		}
	}
}

// simulator.simple.noSandBox 是一个请求选项 用于开启/关闭sandbox
//
// Example:
// ```
//
//	sandBox = simulator.simple.noSandBox(true)
//	browser = simulator.simple.createBrowser(sandBox)
//
// ```
func WithNoSandBox(noSandBox bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.noSandBox = noSandBox
	}
}

// simulator.simple.headless 是一个请求选项 用于开启关闭headless模式
//
// Example:
// ```
//
//	headless = simulator.simple.headless(true)
//	browser = simulator.simple.createBrowser(headless)
//
// ```
func WithHeadless(headless bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.headless = headless
	}
}

// simulator.simple.hijack 是一个请求选项 用于开启流量劫持模式
//
// Example:
// ```
//
//	hijack = simulator.simple.hijack(true)
//	browser = simulator.simple.createBrowser(hijack)
//
// ```
func WithHijack(hijack bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.hijack = hijack
	}
}

func WithRuntimeID(runtimeID string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.runtimeID = runtimeID
	}
}

func WithFromPlugin(fromPlugin string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.fromPlugin = fromPlugin
	}
}

func WithSaveToDB(saveToDB bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.saveToDB = saveToDB
	}
}

func WithSourceType(sourceType string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.sourceType = sourceType
	}
}

func WithResponseModification(modifyUrl string, modifyTarget ModifyTarget, modifyResult interface{}) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.responseModification = append(config.responseModification, &ResponseModification{
			baseModify{modifyUrl: modifyUrl, modifyTarget: modifyTarget, modifyResult: modifyResult},
		})
	}
}

func WithRequestModification(modifyUrl string, modifyTarget ModifyTarget, modifyResult interface{}) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.requestModification = append(config.requestModification, &RequestModification{
			baseModify{modifyUrl: modifyUrl, modifyTarget: modifyTarget, modifyResult: modifyResult},
		})
	}
}

// simulator.simple.timeout 是一个请求选项 用于设置页面最大加载时间 单位秒
//
// Example:
// ```
//
//	timeout = simulator.simple.timeout(30)
//	browser = simulator.simple.createBrowser(timeout)
//
// ```
func WithTimeout(timeout int) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.timeout = timeout
	}
}

// simulator.simple.leakless 是一个请求选项 用于设置在程序运行结束时强行杀死浏览器
// 注意在windows上可能会报毒，windows建议关闭
//
// Example:
// ```
//
//	leakless = simulator.simple.leakless(true)
//	browser = simulator.simple.createBrowser(leakless)
//
// ```
func WithLeakless(leakless bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.leakless = leakless
	}
}
