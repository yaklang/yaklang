// Package crawlerx
// @Author bcy2007  2023/7/12 16:20
package crawlerx

import (
	"context"
	"encoding/json"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"regexp"
)

type Config struct {
	browsers   []*BrowserConfig
	baseConfig *BaseConfig
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
	invalidSuffix     []string
	stealth           bool

	targetUrl      string
	ch             chan ReqInfo
	ctx            context.Context
	pageVisit      *tools.StringCountFilter
	resultSent     *tools.StringCountFilter
	uChan          *tools.UChan
	urlTree        *tools.UrlTree
	waitGroup      *utils.SizedWaitGroup
	startWaitGroup *utils.SizedWaitGroup
}

type BrowserConfig struct {
	exePath      string
	wsAddress    string
	proxyAddress *url.URL
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
			invalidSuffix:     make([]string, 0),
			stealth:           false,
		},
	}
}

type BrowserInfo struct {
	ExePath       string `json:"exe_path,omitempty"`
	WsAddress     string `json:"ws_address,omitempty"`
	ProxyAddress  string `json:"proxy_address,omitempty"`
	ProxyUsername string `json:"proxy_username,omitempty"`
	ProxyPassword string `json:"proxy_password,omitempty"`
}

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
		}
		if jsonData.ProxyUsername != "" || jsonData.ProxyPassword != "" {
			proxyUser := url.UserPassword(jsonData.ProxyUsername, jsonData.ProxyPassword)
			proxyUrl.User = proxyUser
		}
		browserConfig.proxyAddress = proxyUrl
	}
	return func(config *Config) {
		config.browsers = append(config.browsers, browserConfig)
	}
}

func WithMaxUrl(maxUrl int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.maxUrlCount = maxUrl
	}
}

func WithMaxDepth(depth int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.maxDepth = depth
	}
}

func WithConcurrent(concurrent int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.concurrent = concurrent
	}
}

func WithBlackList(keywords ...string) ConfigOpt {
	return func(config *Config) {
		for _, keyword := range keywords {
			if keyword == "" {
				continue
			}
			regKeyword, err := regexp.Compile(keyword)
			if err != nil {
				log.Infof("blacklist keyword %s compile error: %s", keyword, err)
				continue
			}
			config.baseConfig.blacklist = append(config.baseConfig.blacklist, regKeyword)
		}
	}
}

func WithWhiteList(keywords ...string) ConfigOpt {
	return func(config *Config) {
		for _, keyword := range keywords {
			if keyword == "" {
				continue
			}
			regKeyword, err := regexp.Compile(keyword)
			if err != nil {
				log.Infof("whitelist keyword %s compile error: %s", keyword, err)
				continue
			}
			config.baseConfig.whitelist = append(config.baseConfig.whitelist, regKeyword)
		}
	}
}

func WithPageTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.pageTimeout = timeout
	}
}

func WithFullTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.fullTimeout = timeout
	}
}

func WithExtraWaitLoadTime(extraWaitLoadTime int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.extraWaitLoadTime = extraWaitLoadTime
	}
}

func WithFormFill(formFills map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range formFills {
			config.baseConfig.formFill[k] = v
		}
	}
}

func WithFileInput(fileInput map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range fileInput {
			config.baseConfig.fileInput[k] = v
		}
	}
}

func WithHeaderInfo(headerInfo string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.headers = append(config.baseConfig.headers, headerRawDataTransfer(headerInfo)...)
	}
}

func WithHeaders(headersInfo map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range headersInfo {
			config.baseConfig.headers = append(config.baseConfig.headers, &headers{k, v})
		}
	}
}

func WithCookieInfo(cookieInfo string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.cookies = append(config.baseConfig.cookies, cookieRawDataTransfer(cookieInfo)...)
	}
}

func WithCookies(cookiesInfo map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range cookiesInfo {
			config.baseConfig.cookies = append(config.baseConfig.cookies, &proto.NetworkCookieParam{Name: k, Value: v})
		}
	}
}

func WithScanRangeLevel(scanRange scanRangeLevel) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.scanRange = scanRange
	}
}

func WithScanRepeatLevel(scanRepeat repeatLevel) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.scanRepeat = scanRepeat
	}
}

func WithIgnoreQueryName(names ...string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.ignoreParams = append(config.baseConfig.ignoreParams, names...)
	}
}

func WithSensitiveWords(words []string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.sensitiveWords = append(config.baseConfig.sensitiveWords, words...)
	}
}

func WithLeakless(leakless string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.leakless = leakless
	}
}

func WithLocalStorage(storage map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range storage {
			config.baseConfig.localStorage[k] = v
		}
	}
}

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

func WithStealth(stealth bool) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.stealth = stealth
	}
}
