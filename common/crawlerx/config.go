// Package crawlerx
// @Author bcy2007  2023/7/12 16:20
package crawlerx

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"regexp"
	"strings"
)

type Config struct {
	browsers   []*BrowserConfig
	baseConfig *BaseConfig
	urlCheck   bool
}

type ConfigOpt func(*Config)

type BaseConfig struct {
	maxUrlCount       int
	maxDepth          int
	concurrent        int
	blacklist         []*regexp.Regexp
	whitelist         []*regexp.Regexp
	pageTimeout       int
	fullTimeout       int
	extraWaitLoadTime int
	formFill          map[string]string
	fileInput         map[string]string
	headers           []*headers
	cookies           []*proto.NetworkCookieParam
	scanRange         scanRangeLevel
	scanRepeat        repeatLevel
	ignoreParams      []string
	sensitiveWords    []string
	leakless          string
	localStorage      map[string]string
	sessionStorage    map[string]string
	invalidSuffix     []string
	stealth           bool
	saveToDB          bool
	runtimeId         string
	evalJs            map[string][]string
	jsResultSave      func(string)
	vue               bool
	response          map[string]string

	targetUrl      string
	ch             chan ReqInfo
	ctx            context.Context
	pageVisit      *tools.StringCountFilter
	resultSent     *tools.StringCountFilter
	uChan          *tools.UChan
	urlTree        *tools.UrlTree
	waitGroup      *utils.SizedWaitGroup
	startWaitGroup *utils.SizedWaitGroup

	sourceType string
	fromPlugin string

	aiInputUrl  string
	aiInputInfo string

	login    bool
	username string
	password string
}

type BrowserConfig struct {
	exePath      string
	wsAddress    string
	proxyAddress *url.URL
}

func NewBrowserConfig(exePath, wsAddress string, proxyAddress *url.URL) *BrowserConfig {
	return &BrowserConfig{
		exePath:      exePath,
		wsAddress:    wsAddress,
		proxyAddress: proxyAddress,
	}
}

type headers struct {
	Key   string
	Value string
}

func NewConfig() *Config {
	return &Config{
		browsers: make([]*BrowserConfig, 0),
		baseConfig: &BaseConfig{
			maxUrlCount:       0,
			maxDepth:          0,
			concurrent:        3,
			blacklist:         make([]*regexp.Regexp, 0),
			whitelist:         make([]*regexp.Regexp, 0),
			pageTimeout:       30,
			fullTimeout:       3000,
			extraWaitLoadTime: 500,
			formFill:          defaultInputMap,
			fileInput:         make(map[string]string),
			headers:           make([]*headers, 0),
			cookies:           make([]*proto.NetworkCookieParam, 0),
			scanRange:         mainDomain,
			scanRepeat:        lowLevel,
			ignoreParams:      make([]string, 0),
			sensitiveWords:    make([]string, 0),
			ch:                make(chan ReqInfo),
			leakless:          "default",
			localStorage:      make(map[string]string),
			sessionStorage:    make(map[string]string),
			invalidSuffix:     make([]string, 0),
			stealth:           false,
			saveToDB:          false,
			evalJs:            make(map[string][]string),
			vue:               false,
			response:          make(map[string]string),
			sourceType:        "crawlerx",
		},
		urlCheck: true,
	}
}

type BrowserInfo struct {
	ExePath       string `json:"exe_path,omitempty"`
	WsAddress     string `json:"ws_address,omitempty"`
	ProxyAddress  string `json:"proxy_address,omitempty"`
	ProxyUsername string `json:"proxy_username,omitempty"`
	ProxyPassword string `json:"proxy_password,omitempty"`
}

// saveToDB 设置爬虫是否将抓取到的请求结果保存到数据库
// 在 yak 中通过 crawlerx.saveToDB 调用
// 参数:
//   - b: 是否保存到数据库
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：抓取结果保存到数据库
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.saveToDB(true))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithSaveToDB(b bool) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.saveToDB = b
	}
}

// runtimeId 设置本次爬虫任务的运行时 ID，便于将结果与特定任务关联
// 在 yak 中通过 crawlerx.runtimeId 或 crawlerx.runtimeID 调用
// 参数:
//   - id: 运行时 ID 字符串
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定运行时 ID
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.runtimeId("task-001"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithRuntimeID(id string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.runtimeId = id
	}
}

// browserInfo 是一个请求选项 用于配制浏览器参数
// 参数:
//   - data: 浏览器配置 JSON 字符串，包含 ws_address、exe_path、proxy_address 等字段
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	browserInfo = {
//	   "ws_address":"",		// 浏览器websocket url
//	   "exe_path":"",		// 浏览器可执行路径
//	   "proxy_address":"",	// 代理地址
//	   "proxy_username":"",	// 代理用户名
//	   "proxy_password":"",	// 代理密码
//	}
//	browserInfoOpt = crawlerx.browserInfo(json.dumps(browserInfo))
//	ch, err = crawlerx.StartCrawler(targetUrl, browserInfoOpt)
//	...
//
// ```
func WithBrowserInfo(data string) ConfigOpt {
	var jsonData BrowserInfo
	err := json.Unmarshal([]byte(data), &jsonData)
	if err != nil {
		log.Errorf("unmarshal data %s error: %s", data, err)
		return func(*Config) {}
	}
	browserConfig := &BrowserConfig{}
	if jsonData.ExePath != "" {
		browserConfig.exePath = jsonData.ExePath
	} else if jsonData.WsAddress != "" {
		browserConfig.wsAddress = jsonData.WsAddress
	}
	if jsonData.ProxyAddress != "" {
		proxyUrl, err := url.Parse(jsonData.ProxyAddress)
		if err != nil {
			log.Errorf("proxy address %s error: %s", jsonData.ProxyAddress, err)
		} else {
			if jsonData.ProxyUsername != "" || jsonData.ProxyPassword != "" {
				proxyUser := url.UserPassword(jsonData.ProxyUsername, jsonData.ProxyPassword)
				proxyUrl.User = proxyUser
			}
		}
		browserConfig.proxyAddress = proxyUrl
	}
	return func(config *Config) {
		config.browsers = append(config.browsers, browserConfig)
	}
}

func WithBrowserData(browserConfig *BrowserConfig) ConfigOpt {
	return func(config *Config) {
		config.browsers = append(config.browsers, browserConfig)
	}
}

// maxUrl 是一个请求选项 用于设置最大爬取url数量
// 参数:
//   - maxUrl: 最大爬取 URL 数量
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.maxUrl(100)) // 设置最大爬取url数量为100
//	...
//
// ```
func WithMaxUrl(maxUrl int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.maxUrlCount = maxUrl
	}
}

// maxDepth 是一个请求选项 用于设置网站最大爬取深度
// 参数:
//   - depth: 网站最大爬取深度
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.maxDepth(3)) // 设置网站最大爬取深度为3
//	...
//
// ```
func WithMaxDepth(depth int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.maxDepth = depth
	}
}

// concurrent 是一个请求选项 用于设置浏览器同时打开的最大页面数量
// 参数:
//   - concurrent: 浏览器同时打开的最大页面数量
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.concurrent(3)) // 设置浏览器同时打开的最大页面数量为3
//	...
//
// ```
func WithConcurrent(concurrent int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.concurrent = concurrent
	}
}

// blacklist 是一个请求选项 用于设置不会被访问的url链接包含的关键词
// 参数:
//   - keywords: 一个或多个黑名单关键词，URL 中包含这些关键词时不会被访问
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.blacklist("logout", "exit", "delete")) // 设置遇到url中包含logout、exit和delete时不会访问
//	...
//
// ```
func WithBlackList(keywords ...string) ConfigOpt {
	return func(config *Config) {
		for _, keyword := range keywords {
			if keyword == "" {
				continue
			}
			regKeyword, err := regexp.Compile(fmt.Sprintf("(?i)%s", keyword))
			if err != nil {
				log.Errorf("blacklist keyword %s compile error: %s", keyword, err)
				continue
			}
			config.baseConfig.blacklist = append(config.baseConfig.blacklist, regKeyword)
		}
	}
}

// whitelist 是一个请求选项 用于设置只会被访问的url链接中包含的关键词
// 参数:
//   - keywords: 一个或多个白名单关键词，只有 URL 中包含这些关键词时才会被访问
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.whitelist("test", "click")) // 设置只会访问url中包含test和click的链接
//	...
//
// ```
func WithWhiteList(keywords ...string) ConfigOpt {
	return func(config *Config) {
		for _, keyword := range keywords {
			if keyword == "" {
				continue
			}
			regKeyword, err := regexp.Compile(fmt.Sprintf("(?i)%s", keyword))
			if err != nil {
				log.Errorf("whitelist keyword %s compile error: %s", keyword, err)
				continue
			}
			config.baseConfig.whitelist = append(config.baseConfig.whitelist, regKeyword)
		}
	}
}

// pageTimeout 是一个请求选项 用于设置单个页面超时时间
// 参数:
//   - timeout: 单个页面超时时间，单位为秒
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.pageTimeout(30)) // 设置单个页面超时时间为30秒
//	...
//
// ```
func WithPageTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.pageTimeout = timeout
	}
}

// fullTimeout 是一个请求选项 用于设置爬虫任务总超时时间
// 参数:
//   - timeout: 爬虫任务总超时时间，单位为秒
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.fullTimeout(1800)) // 设置爬虫任务总超时时间为1800秒
//	...
//
// ```
func WithFullTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.fullTimeout = timeout
	}
}

// extraWaitLoadTime 是一个请求选项 用于设置页面加载的额外页面等待时间
//
// 防止加载vue网站页面时页面状态为加载完成 实际仍在加载中的情况
//
// 参数:
//   - extraWaitLoadTime: 页面加载的额外等待时间，单位为毫秒
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.extraWaitLoadTime(1000)) // 设置页面加载的额外页面等待时间为1000毫秒
//	...
//
// ```
func WithExtraWaitLoadTime(extraWaitLoadTime int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.extraWaitLoadTime = extraWaitLoadTime
	}
}

// formFill 是一个请求选项 用于设置页面输入框填写内容
// 参数:
//   - formFills: 关键词到填写内容的映射，输入框匹配关键词时填写对应内容
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	inputMap = make(map[string]string, 0)
//	inputMap["username"] = "admin"
//	inputMap["password"] = "123321"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.formFill(inputMap)) // 设置遇到输入框元素中存在对应关键词时输入对应内容 默认输入test
//	...
//
// ```
func WithFormFill(formFills map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range formFills {
			config.baseConfig.formFill[k] = v
		}
	}
}

// fileInput 是一个请求选项 用于设置页面遇到input submit时默认上传文件
// 参数:
//   - fileInput: 关键词到文件路径的映射，default 键表示默认上传文件
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	fileMap = make(map[string]string, 0)
//	fileMap["default"] = "/path/to/file/test.txt"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.fileInput(fileMap)) // 设置遇到输入框元素中存在对应关键词时输入对应内容 默认输入test
//	...
//
// ```
func WithFileInput(fileInput map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range fileInput {
			config.baseConfig.fileInput[k] = v
		}
	}
}

// rawHeaders 是一个请求选项 用于设置爬虫发送请求时的headers
// 参数:
//   - headerInfo: 原生 headers 字符串，每行一个 header
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	headers = `Accept: text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7
//	Accept-Encoding: gzip, deflate
//	Accept-Language: zh-CN,zh;q=0.9,en;q=0.8,ja;q=0.7,zh-TW;q=0.6
//	Cache-Control: max-age=0
//	Connection: keep-alive
//	Host: testphp.vulnweb.com
//	Upgrade-Insecure-Requests: 1
//	User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 `
//
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.rawHeaders(headers)) // 原生headers输入
//	...
//
// ```
func WithHeaderInfo(headerInfo string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.headers = append(config.baseConfig.headers, headerRawDataTransfer(headerInfo)...)
	}
}

// headers 是一个请求选项 用于设置爬虫发送请求时的headers
// 参数:
//   - headersInfo: header 名称到值的映射
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	headerMap = make(map[string]string, 0)
//	headerMap["Connection"] = "keep-alive"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.headers(headerMap)) // header以字典形式输入
//	...
//
// ```
func WithHeaders(headersInfo map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range headersInfo {
			config.baseConfig.headers = append(config.baseConfig.headers, &headers{k, v})
		}
	}
}

// rawCookie 是一个请求选项 用于设置爬虫发送请求时的cookie
// 参数:
//   - domain: cookie 所属域名
//   - cookieInfo: 原生 cookie 字符串
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	cookie = `Apache=5651982500959.057.1731310579958; ULV=1731310579971:11:1:1:5651982500959.057.1731310579958:1727418057693; ALF=1735783078`
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.rawCookie("testphp.vulnweb.com", cookie)) // 原生cookie输入
//	...
//
// ```
func WithCookieInfo(domain, cookieInfo string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.cookies = append(config.baseConfig.cookies, cookieRawDataTransfer(domain, cookieInfo)...)
	}
}

// cookies 是一个请求选项 用于设置爬虫发送请求时的cookie
// 参数:
//   - domain: cookie 所属域名
//   - cookiesInfo: cookie 名称到值的映射
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	cookieMap = make(map[string]string, 0)
//	cookieMap["Apache"] = "5651982500959.057.1731310579958"
//	cookieMap["ULV"] = "1731310579971:11:1:1:5651982500959.057.1731310579958:1727418057693"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.cookies("testphp.vulnweb.com", cookieMap)) // cookie字典形式输入
//	...
//
// ```
func WithCookies(domain string, cookiesInfo map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range cookiesInfo {
			config.baseConfig.cookies = append(config.baseConfig.cookies, &proto.NetworkCookieParam{Name: k, Value: v, Domain: domain})
		}
	}
}

// scanRangeLevel 是一个请求选项 用于设置爬虫扫描范围
// 参数:
//   - scanRange: 扫描范围级别，可选 crawlerx.AllDomainScan、crawlerx.SubMenuScan、crawlerx.UnlimitedDomainScan
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	scanRangeOpt = crawlerx.scanRangeLevel(crawlerx.AllDomainScan)	// 主域名扫描
//	// scanRangeOpt = crawlerx.scanRangeLevel(crawlerx.SubMenuScan)	// 子域名扫描
//	// scanRangeOpt = crawlerx.scanRangeLevel(crawlerx.UnlimitedDomainScan)	// 无限制扫描
//	ch, err = crawlerx.StartCrawler(targetUrl, scanRangeOpt)
//	...
//
// ```
func WithScanRangeLevel(scanRange scanRangeLevel) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.scanRange = scanRange
	}
}

// scanRepeatLevel 是一个请求选项 用于设置爬虫去重强度
// 参数:
//   - scanRepeat: 去重强度级别，可选 crawlerx.UnLimitRepeat、LowRepeatLevel、MediumRepeatLevel、HighRepeatLevel、ExtremeRepeatLevel
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	scanRepeatOpt = crawlerx.scanRepeatLevel(crawlerx.UnLimitRepeat)	// 对page，method，query-name，query-value和post-data敏感
//	// scanRepeatOpt = crawlerx.scanRepeatLevel(crawlerx.LowRepeatLevel)	// 对page，method，query-name和query-value敏感（默认）
//	// scanRepeatOpt = crawlerx.scanRepeatLevel(crawlerx.MediumRepeatLevel)	// 对page，method和query-name敏感
//	// scanRepeatOpt = crawlerx.scanRepeatLevel(crawlerx.HighRepeatLevel)	// 对page和method敏感
//	// scanRepeatOpt = crawlerx.scanRepeatLevel(crawlerx.ExtremeRepeatLevel)	// 对page敏感
//	ch, err = crawlerx.StartCrawler(targetUrl, scanRepeatOpt)
//	...
//
// ```
func WithScanRepeatLevel(scanRepeat repeatLevel) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.scanRepeat = scanRepeat
	}
}

// ignoreQueryName 是一个请求选项 用于设置url中的query名称去重时忽略
// 参数:
//   - names: 一个或多个去重时需要忽略的 query 名称
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.ignoreQueryName("sid", "tid")) // 设置检测url是否重复时无视sid和tid这两个query
//	...
//
// ```
func WithIgnoreQueryName(names ...string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.ignoreParams = append(config.baseConfig.ignoreParams, names...)
	}
}

// sensitiveWords 是一个请求选项 用于设置页面按钮点击时的敏感词
// 参数:
//   - words: 敏感词列表，按钮所在元素包含这些关键词时不会被点击
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	sensitiveWords = "logout,delete"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.sensitiveWords(sensitiveWords.Split(","))) // 当按钮所在元素中存在logout和delete关键词时不会点击
//	...
//
// ```
func WithSensitiveWords(words []string) ConfigOpt {
	return func(config *Config) {
		for _, word := range words {
			config.baseConfig.sensitiveWords = append(config.baseConfig.sensitiveWords, strings.ToLower(word))
		}
	}
}

// leakless 设置 leakless 模式，控制浏览器进程在异常退出时的清理行为
// 在 yak 中通过 crawlerx.leakless 调用，取值可为 "default"、"true"、"false"
// 参数:
//   - leakless: leakless 模式开关字符串
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 leakless 模式
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.leakless("default"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithLeakless(leakless string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.leakless = leakless
	}
}

// localStorage 设置爬虫启动时注入浏览器的 localStorage 键值对
// 在 yak 中通过 crawlerx.localStorage 调用
// 参数:
//   - storage: 需要注入的 localStorage 键值映射
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：注入 localStorage
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.localStorage({"token": "abc"}))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithLocalStorage(storage map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range storage {
			config.baseConfig.localStorage[k] = v
		}
	}
}

// sessionStorage 设置爬虫启动时注入浏览器的 sessionStorage 键值对
// 在 yak 中通过 crawlerx.sessionStorage 调用
// 参数:
//   - storage: 需要注入的 sessionStorage 键值映射
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：注入 sessionStorage
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.sessionStorage({"sid": "xyz"}))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithSessionStorage(storage map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range storage {
			config.baseConfig.sessionStorage[k] = v
		}
	}
}

// invalidSuffix 设置爬虫需要忽略的 URL 后缀(如静态资源)，命中后缀的链接不会被访问
// 在 yak 中通过 crawlerx.invalidSuffix 调用
// 参数:
//   - suffix: 需要忽略的后缀列表，如 [".png", ".css"]
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：忽略图片与样式表
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.invalidSuffix([".png", ".css"]))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithInvalidSuffix(suffix []string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.invalidSuffix = append(config.baseConfig.invalidSuffix, suffix...)
	}
}

// transport in code

func WithTargetUrl(targetUrl string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.targetUrl = targetUrl
	}
}

func WithResultChannel(ch chan ReqInfo) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.ch = ch
	}
}

func WithContext(ctx context.Context) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.ctx = ctx
	}
}

func WithPageVisitFilter(pageVisitFilter *tools.StringCountFilter) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.pageVisit = pageVisitFilter
	}
}

func WithResultSentFilter(resultSentFilter *tools.StringCountFilter) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.resultSent = resultSentFilter
	}
}

func WithUChan(uChan *tools.UChan) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.uChan = uChan
	}
}

func WithUrlTree(tree *tools.UrlTree) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.urlTree = tree
	}
}

func WithPageSizedWaitGroup(pageSizedWaitGroup *utils.SizedWaitGroup) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.waitGroup = pageSizedWaitGroup
	}
}

func WithStartWaitGroup(waitGroup *utils.SizedWaitGroup) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.startWaitGroup = waitGroup
	}
}

// stealth 设置爬虫是否启用 stealth(隐身)模式，规避部分浏览器自动化检测
// 在 yak 中通过 crawlerx.stealth 调用
// 参数:
//   - stealth: 是否启用隐身模式
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：启用隐身模式
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.stealth(true))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithStealth(stealth bool) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.stealth = stealth
	}
}

// evalJs 设置在指定页面注入并执行的 JavaScript 代码
// 在 yak 中通过 crawlerx.evalJs 调用
// 参数:
//   - target: 目标页面 URL
//   - evalJs: 要在该页面执行的 JavaScript 代码
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：在指定页面执行 JS
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.evalJs("http://testphp.vulnweb.com/", "console.log(1)"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithEvalJs(target string, evalJs string) ConfigOpt {
	return func(config *Config) {
		if item, ok := config.baseConfig.evalJs[target]; ok {
			config.baseConfig.evalJs[target] = append(item, evalJs)
		} else {
			config.baseConfig.evalJs[target] = []string{evalJs}
		}
	}
}

// jsResultSend 设置接收注入 JS 执行结果的回调函数
// 在 yak 中通过 crawlerx.jsResultSend 调用
// 参数:
//   - storage: 接收 JS 执行结果字符串的回调函数
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：接收 JS 执行结果
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.jsResultSend(func(s) { println(s) }))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithJsResultSave(storage func(s string)) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.jsResultSave = storage
	}
}

// vue 设置是否针对 Vue 等单页应用(SPA)启用专门的爬取策略
// 在 yak 中通过 crawlerx.vue 调用
// 参数:
//   - vue: 是否启用 SPA 爬取策略
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：针对 Vue 单页应用爬取
// ch = crawlerx.StartCrawler("http://spa.example.com/", crawlerx.vue(true))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithVue(vue bool) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.vue = vue
	}
}

// response 为指定 URL 预设响应内容，命中该 URL 时直接使用预设响应而不发起真实请求
// 在 yak 中通过 crawlerx.response 调用
// 参数:
//   - targetUrl: 目标 URL
//   - response: 预设的响应内容
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：为指定 URL 预设响应
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.response("http://testphp.vulnweb.com/", "HTTP/1.1 200 OK\r\n\r\nhello"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithResponse(targetUrl string, response string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.response[targetUrl] = response
	}
}

// sourceType 设置爬虫的来源类型标记，用于区分结果数据的产生来源
// 在 yak 中通过 crawlerx.sourceType 调用
// 参数:
//   - sourceType: 来源类型字符串
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置来源类型
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.sourceType("scan"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithSourceType(sourceType string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.sourceType = sourceType
	}
}

// fromPlugin 设置标记爬虫任务来源的插件名称
// 在 yak 中通过 crawlerx.fromPlugin 调用
// 参数:
//   - fromPlugin: 来源插件名称
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：标记来源插件
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.fromPlugin("my-plugin"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithFromPlugin(fromPlugin string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.fromPlugin = fromPlugin
	}
}

// urlCheck 是一个请求选项 用于设置是否在爬虫前进行url存活检测
// 参数:
//   - check: 是否在爬虫前进行 URL 存活检测
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.urlCheck(true))
//	...
//
// ```
func WithUrlCheck(check bool) ConfigOpt {
	return func(config *Config) {
		config.urlCheck = check
	}
}

// aiInputUrl 设置由 AI 辅助进行表单输入的目标页面 URL
// 在 yak 中通过 crawlerx.aiInputUrl 调用
// 参数:
//   - url: 需要 AI 辅助输入的页面 URL
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定 AI 辅助输入页面
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.aiInputUrl("http://testphp.vulnweb.com/login.php"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithAIInputUrl(url string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.aiInputUrl = url
	}
}

// aiInputInfo 设置提供给 AI 进行表单填充的背景信息(如业务上下文)
// 在 yak 中通过 crawlerx.aiInputInfo 调用
// 参数:
//   - info: 提供给 AI 的背景信息文本
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：提供 AI 输入背景信息
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.aiInputInfo("use test account"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithAIInputInf(info string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.aiInputInfo = info
	}
}

// loginUsername 设置自动登录时使用的用户名(设置后会启用登录流程)
// 在 yak 中通过 crawlerx.loginUsername 调用
// 参数:
//   - username: 登录用户名
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置自动登录用户名
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.loginUsername("admin"), crawlerx.loginPassword("admin"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithLoginUsername(username string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.login = true
		config.baseConfig.username = username
	}
}

// loginPassword 设置自动登录时使用的密码(设置后会启用登录流程)
// 在 yak 中通过 crawlerx.loginPassword 调用
// 参数:
//   - password: 登录密码
//
// 返回值:
//   - 一个 crawlerx.StartCrawler 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置自动登录密码
// ch = crawlerx.StartCrawler("http://testphp.vulnweb.com/", crawlerx.loginUsername("admin"), crawlerx.loginPassword("admin"))~
//
//	for req = range ch {
//	    println(req.UrlStr())
//	}
//
// ```
func WithLoginPassword(password string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.login = true
		config.baseConfig.password = password
	}
}
