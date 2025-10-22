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
//	Example:
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
func WithCaptchaMode(mode string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.captchaMode = mode
	}
}

// captchaType 是一个请求选项 用于标识使用验证码的种类 其中1 其他（正常请勿使用）2 老版ddddocr server接口（url以/ocr/b64/json结尾） 3 新版ddddocr server接口（url以/ocr结尾）
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

func WithSaveToDB(saveToDB bool) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.saveToDB = saveToDB
	}
}

func WithSourceType(sourceType string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.sourceType = sourceType
	}
}

func WithFromPlugin(fromPlugin string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.fromPlugin = fromPlugin
	}
}

func WithRuntimeID(runtimeID string) BruteConfigOpt {
	return func(config *BruteConfig) {
		config.runtimeID = runtimeID
	}
}

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
