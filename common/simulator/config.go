// Package simulator
// @Author bcy2007  2023/8/17 16:18
package simulator

import (
	"encoding/json"
	"net/url"

	"github.com/yaklang/yaklang/common/crawlerx/preaction"
	"github.com/yaklang/yaklang/common/log"
)

type LeaklessMode int

const (
	// simulator.leaklessDefault 默认leakless
	LeaklessDefault LeaklessMode = 0
	// simulator.leaklessOn 开启leakless
	LeaklessOn LeaklessMode = 1
	// simulator.leaklessOff 关闭leakless
	LeaklessOff LeaklessMode = -1
)

type loginDetectMode int

const (
	// simulator.urlChangeMode url变化检测登录
	UrlChangeMode loginDetectMode = 0
	// simulator.htmlChangeMode 页面内容变化检测登录
	HtmlChangeMode loginDetectMode = 1
	// simulator.stringMatchMode 字符串匹配检测登录
	StringMatchMode loginDetectMode = 2
	// simulator.defaultChangeMode 综合变化检测登录
	DefaultChangeMode loginDetectMode = -1
)

type BrowserConfig struct {
	exePath   string
	wsAddress string
	proxy     *url.URL
	leakless  LeaklessMode

	saveToDB   bool
	sourceType string
	fromPlugin string
	runtimeID  string
}

var actionDict = map[string]preaction.ActionType{
	"hover":   preaction.HoverAction,
	"click":   preaction.ClickAction,
	"input":   preaction.InputAction,
	"select":  preaction.SelectAction,
	"setFile": preaction.SetFileAction,
}

type BrowserConfigOpt func(*BrowserConfig)

func CreateNewBrowserConfig() *BrowserConfig {
	config := BrowserConfig{
		leakless:   LeaklessDefault,
		saveToDB:   false,
		sourceType: "scan",
	}
	return &config
}

func withExePath(exePath string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.exePath = exePath
	}
}

func withWsAddress(ws string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.wsAddress = ws
	}
}

func withProxy(proxy *url.URL) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.proxy = proxy
	}
}

func withLeakless(leakless LeaklessMode) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.leakless = leakless
	}
}

func withSaveToDB(saveToDB bool) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.saveToDB = saveToDB
	}
}

func withSourceType(sourceType string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.sourceType = sourceType
	}
}

func withFromPlugin(fromPlugin string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.fromPlugin = fromPlugin
	}
}

func withRuntimeID(runtimeID string) BrowserConfigOpt {
	return func(config *BrowserConfig) {
		config.runtimeID = runtimeID
	}
}

type BruteConfig struct {
	wsAddress     string
	exePath       string
	proxy         string
	proxyUsername string
	proxyPassword string

	usernameList []string
	passwordList []string

	captchaUrl  string
	captchaMode string
	captchaType int

	usernameSelector    string
	passwordSelector    string
	captchaSelector     string
	captchaImgSelector  string
	loginButtonSelector string

	ch                chan Result
	loginDetect       loginDetectMode
	leakless          LeaklessMode
	extraWaitLoadTime int
	similarityDegree  float64
	successMatchers   []string

	saveToDB   bool
	sourceType string
	fromPlugin string
	runtimeID  string

	preActions []*preaction.PreAction
}

type BruteConfigOpt func(*BruteConfig)

var NullBruteConfigOpt = func(*BruteConfig) {}

func NewBruteConfig() *BruteConfig {
	return &BruteConfig{
		usernameList:     make([]string, 0),
		passwordList:     make([]string, 0),
		loginDetect:      DefaultChangeMode,
		leakless:         LeaklessDefault,
		similarityDegree: 0.6,

		saveToDB:   false,
		sourceType: "scan",
	}
}

// wsAddress 是一个请求选项 用于输入浏览器的websocket地址
//
// 参数:
//   - ws: 浏览器的 WebSocket 地址
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.wsAddress("http://127.0.0.1:7317/"))
//
// ```
func WithWsAddress(ws string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.wsAddress = ws
	}
}

// exePath 是一个请求选项 用于输入浏览器路径
//
// 参数:
//   - exePath: 浏览器可执行文件路径
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.exePath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")) // 不存在用户名密码的代理
//
// ```
func WithExePath(exePath string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.exePath = exePath
	}
}

// proxy 是一个请求选项 用于输入代理服务器地址
//
// 参数:
//   - proxy: 代理服务器地址
//   - details: 可选的代理认证信息（用户名、密码）
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.proxy("http://127.0.0.1:8123/")) // 不存在用户名密码的代理
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.proxy("http://127.0.0.1:8123/", "admin", "123321")) // 存在用户名密码的代理
//
// ```
func WithProxy(proxy string, details ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.proxy = proxy
		if len(details) > 1 {
			config.proxyUsername = details[0]
			config.proxyPassword = details[1]
		}
	}
}

// usernameList 是一个请求选项 用于输入爆破的用户名的列表
//
// 参数:
//   - username: 用户名列表
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	userList = ["admin", "root"]
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.usernameList(userList), simulator.password("admin", "luckyadmin123"))
//
// ```
func WithUsername(username []string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameList = append(config.usernameList, username...)
	}
}

// username 是一个请求选项 用于输入爆破的用户名
//
// 参数:
//   - username: 一个或多个用户名
//
// 返回值:
//
//   - 请求选项
//
//     Example:
//
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.username("admin", "root"), simulator.password("admin", "luckyadmin123"))
//
// ```
func WithUsernameList(username ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameList = append(config.usernameList, username...)
	}
}

// passwordList 是一个请求选项 用于输入爆破的密码的列表
//
// 参数:
//   - password: 密码列表
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	userList = ["admin", "root"]
//	passList = ["123", "admin"]
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.usernameList(userList), simulator.passwordList(passList))
//
// ```
func WithPassword(password []string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordList = append(config.passwordList, password...)
	}
}

// password 是一个请求选项 用于输入爆破的密码
//
// 参数:
//   - password: 一个或多个密码
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.username("admin", "root"), simulator.password("admin", "luckyadmin123"))
//
// ```
func WithPasswordList(password ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordList = append(config.passwordList, password...)
	}
}

// captchaUrl 是一个请求选项 用于验证码的url地址
//
// 参数:
//   - url: 验证码识别服务的 URL
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.captchaUrl("http://localhost:8088/"))
//
// ```
func WithCaptchaUrl(url string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaUrl = url
	}
}

// captchaMode 特殊选项 如果你不知道怎么用请勿使用
// 参数:
//   - mode: 验证码模式
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
// opt = simulator.captchaMode("default")
// println(opt)
// ```
func WithCaptchaMode(mode string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaMode = mode
	}
}

// captchaType 是一个请求选项 用于标识使用验证码的种类 其中1 其他（正常请勿使用）2 老版ddddocr server接口（url以/ocr/b64/json结尾） 3 新版ddddocr server接口（url以/ocr结尾）
//
// 参数:
//   - typeEnum: 验证码接口类型
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.captchaType(3))
//
// ```
func WithCaptchaType(typeEnum int) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaType = typeEnum
	}
}

// usernameSelector 是一个请求选项 用于在用户框位置识别错误时输入用户框对应的selector
//
// 参数:
//   - selector: 用户名输入框的 CSS selector
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.usernameSelector("#username"))
//
// ```
func WithUsernameSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.usernameSelector = selector
	}
}

// passwordSelector 是一个请求选项 用于在密码框位置识别错误时输入密码框对应的selector
//
// 参数:
//   - selector: 密码输入框的 CSS selector
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.passwordSelector("#password"))
//
// ```
func WithPasswordSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.passwordSelector = selector
	}
}

// captchaInputSelector 是一个请求选项 用于在验证码输入框位置识别错误时输入验证码输入框对应的selector
//
// 参数:
//   - selector: 验证码输入框的 CSS selector
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.captchaInputSelector("#captcha"))
//
// ```
func WithCaptchaSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaSelector = selector
	}
}

// captchaImgSelector 是一个请求选项 用于在验证码图片位置识别错误时输入验证码图片对应的selector
//
// 参数:
//   - selector: 验证码图片的 CSS selector
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.captchaImgSelector("#img"))
//
// ```
func WithCaptchaImgSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaImgSelector = selector
	}
}

// submitButtonSelector 是一个请求选项 用于在提交登录按钮位置识别错误时输入提交登录按钮对应的selector
//
// 参数:
//   - selector: 登录提交按钮的 CSS selector
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.submitButtonSelector("#login"))
//
// ```
func WithLoginButtonSelector(selector string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.loginButtonSelector = selector
	}
}

func WithResultChannel(ch chan Result) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.ch = ch
	}
}

// loginDetectMode 是一个请求选项 用于选择识别登录跳转的模式，
//
// 其中simulator.htmlChangeMode 表示检测html变化程度 超过一定数字则认为发生登录跳转
// simulator.urlChangeMode 表示检测url变化 如果url发生变化则认为登录成功
// simulator.defaultChangeMode 表示同时使用以上两种策略
// simulator.stringMatchMode 表示使用页面内容或变动中的字符串匹配结果判断登录
// 第二个参数表示检测html变化程度的比例，超过该比例则认为发生变化 默认为0.6
//
// 参数:
//   - mode: 登录检测模式，如 simulator.htmlChangeMode、simulator.urlChangeMode
//   - degree: 可选的 html 变化比例阈值（默认 0.6）
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.loginDetectMode(simulator.htmlChangeMode, 0.6))
//
// ```
func WithLoginDetectMode(mode loginDetectMode, degree ...float64) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.loginDetect = mode
		if len(degree) > 0 {
			config.similarityDegree = degree[0]
		}
	}
}

// leaklessStatus 是一个请求选项 用于选择是否自动关闭浏览器进程
//
// simulator.leaklessOn为开启 simulator.leaklessOff为关闭 simulator.leaklessDefault为默认
// 浏览器自动进程关闭进行在windows下会报病毒 默认在windows下会关闭
// 当关闭时 如果强制关闭爬虫进程时chrome进程会存在于后台 浏览器进程后台过多时请手动进行关闭
//
// 参数:
//   - leakless: leakless 模式，如 simulator.leaklessOn、simulator.leaklessOff、simulator.leaklessDefault
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.leaklessStatus(simulator.leaklessDefault))
//
// ```
func WithLeakless(leakless LeaklessMode) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.leakless = leakless
	}
}

// extraWaitLoadTime 是一个请求选项 用于选择页面加载的额外页面等待时间 单位毫秒
//
// 参数:
//   - time: 额外等待时间（毫秒）
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.extraWaitLoadTime(1000))
//
// ```
func WithExtraWaitLoadTime(time int) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.extraWaitLoadTime = time
	}
}

// successMatchers 是一个请求选项 用于在页面变化中匹配指定字符串来判断登录成功
//
// 参数:
//   - matchers: 一个或多个用于判断登录成功的字符串
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.successMatchers("login success"))
//
// ```
func WithSuccessMatchers(matchers ...string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.successMatchers = append(config.successMatchers, matchers...)
	}
}

// saveToDB 是一个请求选项 用于设置是否将爆破结果保存到数据库（导出名为 simulator.saveToDB）
// 参数:
//   - saveToDB: 是否保存到数据库
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
// opt = simulator.saveToDB(true)
// println(opt)
// ```
func WithSaveToDB(saveToDB bool) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.saveToDB = saveToDB
	}
}

// sourceType 是一个请求选项 用于标识结果来源类型（导出名为 simulator.sourceType）
// 参数:
//   - sourceType: 来源类型，如 "scan"
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
// opt = simulator.sourceType("scan")
// println(opt)
// ```
func WithSourceType(sourceType string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.sourceType = sourceType
	}
}

// fromPlugin 是一个请求选项 用于标识结果来源插件名（导出名为 simulator.fromPlugin）
// 参数:
//   - fromPlugin: 来源插件名
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
// opt = simulator.fromPlugin("my-plugin")
// println(opt)
// ```
func WithFromPlugin(fromPlugin string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.fromPlugin = fromPlugin
	}
}

// runtimeID 是一个请求选项 用于绑定运行时 ID（导出名为 simulator.runtimeID）
// 参数:
//   - runtimeID: 运行时 ID
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
// opt = simulator.runtimeID("runtime-uuid")
// println(opt)
// ```
func WithRuntimeID(runtimeID string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.runtimeID = runtimeID
	}
}

// preAction 是一个请求选项 用于在登录前执行预置动作（以 JSON 字符串描述，导出名为 simulator.preAction）
// 参数:
//   - actionsJs: 预置动作的 JSON 字符串
//
// 返回值:
//   - 请求选项
//
// Example:
// ```
// // 预置动作 JSON 用于在爆破前执行点击、输入等操作（示意性示例）
// opt = simulator.preAction(`[]`)
// println(opt)
// ```
func WithPreActions(actionsJs string) BruteConfigOpt {
	var actions []preaction.PreActionJson
	err := json.Unmarshal([]byte(actionsJs), &actions)
	if err != nil {
		log.Errorf("unmarshal preaction json string error: %v", err)
		return NullBruteConfigOpt
	}
	return func(config *BruteConfig) {
		tempActions := make([]*preaction.PreAction, 0)
		for _, action := range actions {
			actionType, ok := actionDict[action.Action]
			if !ok {
				log.Errorf("invalid action string: %v", action.Action)
				continue
			}
			tempActions = append(tempActions, &preaction.PreAction{
				Action:   actionType,
				Selector: action.Selector,
				Params:   action.Params,
			})
		}
		config.preActions = tempActions
	}
}
