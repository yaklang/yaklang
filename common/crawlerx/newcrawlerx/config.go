// Package newcrawlerx
// @Author bcy2007  2023/3/7 11:32
package newcrawlerx

import (
	"context"
	"encoding/json"
	"github.com/go-rod/rod/lib/proto"
	"net/url"
	"regexp"
	"yaklang/common/crawlerx/filter"
	"yaklang/common/log"
	"yaklang/common/utils"
)

type NewBrowserInfo struct {
	ExePath       string `json:"exe_path,omitempty"`
	WsAddress     string `json:"ws_address,omitempty"`
	ProxyAddress  string `json:"proxy_address,omitempty"`
	ProxyUsername string `json:"proxy_username,omitempty"`
	ProxyPassword string `json:"proxy_password,omitempty"`
}

type NewBrowserConfig struct {
	exePath      string
	wsAddress    string
	proxyAddress *url.URL
}

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Config struct {
	browsers   []*NewBrowserConfig
	baseConfig *BaseConfig
}

type BaseConfig struct {
	//
	hijack bool
	vue    bool
	// top level
	maxUrlCount int
	maxDepth    int

	concurrent int

	blackList []*regexp.Regexp
	whiteList []*regexp.Regexp

	pageTimeout int
	timeout     int

	formFill  map[string]string
	fileInput map[string]string

	headers        []*Header
	cookies        []*proto.NetworkCookieParam
	requestHeaders map[string]string

	scanRange    scanRangeLevel
	scanRepeat   limitLevel
	ignoreParams []string

	ch chan ReqInfo

	// bottom level
	targetUrl string
	ctx       context.Context

	pageVisit  *filter.StringCountFilter
	resultSent *filter.StringCountFilter
	uChan      *UChan
	urlTree    *UrlTree

	pageSizedWaitGroup *utils.SizedWaitGroup
}

type ConfigOpt func(*Config)

func NewConfig() *Config {
	return &Config{
		browsers: make([]*NewBrowserConfig, 0),
		baseConfig: &BaseConfig{
			hijack:         true,
			vue:            false,
			maxUrlCount:    0,
			blackList:      make([]*regexp.Regexp, 0),
			whiteList:      make([]*regexp.Regexp, 0),
			formFill:       defaultInputMap,
			fileInput:      make(map[string]string),
			headers:        make([]*Header, 0),
			cookies:        make([]*proto.NetworkCookieParam, 0),
			requestHeaders: defaultChromeHeaders,

			pageTimeout: 30,
			timeout:     1800,

			scanRepeat:   lowLevel,
			scanRange:    mainDomain,
			ignoreParams: make([]string, 0),

			concurrent: 2,
		},
	}
}

func WithNewBrowser(data string) ConfigOpt {
	var jsonData NewBrowserInfo
	err := json.Unmarshal([]byte(data), &jsonData)
	if err != nil {
		log.Errorf("unmarshal data %s error: %s", data, err)
		return func(*Config) {}
	}
	browserConfig := &NewBrowserConfig{}
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

func WithIfHijack(hijack bool) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.hijack = hijack
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
			config.baseConfig.blackList = append(config.baseConfig.blackList, regKeyword)
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
			config.baseConfig.whiteList = append(config.baseConfig.whiteList, regKeyword)
		}
	}
}

func WithPageTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.pageTimeout = timeout
	}
}

func WithTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.timeout = timeout
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

func WithHeader(kv ...string) ConfigOpt {
	return func(config *Config) {
		for i := 0; i <= len(kv)-1; i += 2 {
			config.baseConfig.headers = append(config.baseConfig.headers, &Header{kv[i], kv[i+1]})
		}
	}
}

func WithHeaders(headers map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range headers {
			config.baseConfig.headers = append(config.baseConfig.headers, &Header{k, v})
		}
	}
}

func WithCookie(domain string, kv ...string) ConfigOpt {
	tempCookie := make([]*proto.NetworkCookieParam, 0)
	for i := 0; i < len(kv)-1; i += 2 {
		tempCookie = append(tempCookie, &proto.NetworkCookieParam{
			Domain: domain,
			Name:   kv[i],
			Value:  kv[i+1],
		})
	}
	return func(config *Config) {
		config.baseConfig.cookies = append(config.baseConfig.cookies, tempCookie...)
	}
}

func WithCookies(domain string, cookies map[string]string) ConfigOpt {
	tempCookie := make([]*proto.NetworkCookieParam, 0)
	for k, v := range cookies {
		tempCookie = append(tempCookie, &proto.NetworkCookieParam{
			Domain: domain,
			Name:   k,
			Value:  v,
		})
	}
	return func(config *Config) {
		config.baseConfig.cookies = append(config.baseConfig.cookies, tempCookie...)
	}
}

func WithScanRangeLevel(scanRange scanRangeLevel) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.scanRange = scanRange
	}
}

func WithScanRepeatLevel(scanRepeat limitLevel) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.scanRepeat = scanRepeat
	}
}

func WithResultChannel(ch chan ReqInfo) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.ch = ch
	}
}

//
//
//

func WithTargetUrl(targetUrl string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.targetUrl = targetUrl
	}
}

func WithContext(ctx context.Context) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.ctx = ctx
	}
}

func WithPageVisitFilter(pageVisitFilter *filter.StringCountFilter) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.pageVisit = pageVisitFilter
	}
}

func WithResultSentFilter(resultSentFilter *filter.StringCountFilter) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.resultSent = resultSentFilter
	}
}

func WithUChan(uChan *UChan) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.uChan = uChan
	}
}

func WithUrlTree(tree *UrlTree) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.urlTree = tree
	}
}

func WithPageSizedWaitGroup(pageSizedWaitGroup *utils.SizedWaitGroup) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.pageSizedWaitGroup = pageSizedWaitGroup
	}
}

func WithIgnoreQueryName(names ...string) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.ignoreParams = append(config.baseConfig.ignoreParams, names...)
	}
}

func WithVueWeb(vue bool) ConfigOpt {
	return func(config *Config) {
		config.baseConfig.vue = vue
	}
}
