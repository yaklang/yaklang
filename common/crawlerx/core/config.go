package core

import (
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/detect"
	"github.com/yaklang/yaklang/common/log"
	"regexp"
)

type Header struct {
	Key   string
	Value string
}

type Config struct {
	chromeWS string

	proxy         string
	proxyUsername string
	proxyPassword string

	maxUrlCount int

	whiteList []*regexp.Regexp
	blackList []*regexp.Regexp

	timeout            int
	fullCrawlerTimeout int
	maxDepth           int

	formFill map[string]string

	headers      []*Header
	extraHeaders []string

	concurrent int

	cookies []*proto.NetworkCookieParam

	scanRange  int
	scanRepeat int

	channel   chan ReqInfo
	onRequest func(ReqInfo)

	checkDanger func(string) bool

	tags string

	urlFromProxy bool
}

type ConfigOpt func(*Config)

func WithProxy(proxyAddr string, proxyInfo ...string) ConfigOpt {
	if len(proxyInfo) == 0 {
		return func(config *Config) {
			config.proxy = proxyAddr
		}
	} else if len(proxyInfo) >= 2 {
		return func(config *Config) {
			config.proxy = proxyAddr
			config.proxyUsername = proxyInfo[0]
			config.proxyPassword = proxyInfo[1]
		}
	}
	return func(config *Config) {}
}

func WithMaxUrl(maxUrlCount int) ConfigOpt {
	return func(config *Config) {
		config.maxUrlCount = maxUrlCount
	}
}

func WithWhiteList(regStr string) ConfigOpt {
	if regStr == "" {
		return func(config *Config) {

		}
	}
	reg, err := regexp.Compile(regStr)
	if err != nil {
		log.Errorf("reg string %s compile white list error: %s", regStr, err)
		return func(config *Config) {}
	}
	return func(config *Config) {
		config.whiteList = append(config.whiteList, reg)
	}
}

func WithBlackList(regStr string) ConfigOpt {
	if regStr == "" {
		return func(config *Config) {

		}
	}
	reg, err := regexp.Compile(regStr)
	if err != nil {
		log.Errorf("reg string %s compile black list error: %s", regStr, err)
		return func(config *Config) {}
	}
	return func(config *Config) {
		config.blackList = append(config.blackList, reg)
	}
}

func WithTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.timeout = timeout
	}
}

func WithMaxDepth(depth int) ConfigOpt {
	return func(config *Config) {
		config.maxDepth = depth
	}
}

func WithFormFill(k, v string) ConfigOpt {
	return func(config *Config) {
		config.formFill[k] = v
	}
}

func WithHeader(k, v string) ConfigOpt {
	return func(config *Config) {
		config.headers = append(config.headers, &Header{
			Key:   k,
			Value: v,
		})
	}
}

func WithHeaders(kv map[string]string) ConfigOpt {
	return func(config *Config) {
		for k, v := range kv {
			h := Header{
				Key:   k,
				Value: v,
			}
			config.headers = append(config.headers, &h)
		}
	}
}

func WithConcurrent(concurrent int) ConfigOpt {
	return func(config *Config) {
		config.concurrent = concurrent
	}
}

func WithCookie(domain, k, v string) ConfigOpt {
	return func(config *Config) {
		config.cookies = append(config.cookies, &proto.NetworkCookieParam{
			Domain: domain,
			Name:   k,
			Value:  v,
		})
	}
}

func WithCookies(domain string, kv map[string]string) ConfigOpt {
	tempCookies := make([]*proto.NetworkCookieParam, 0)
	for k, v := range kv {
		tempCookies = append(tempCookies, &proto.NetworkCookieParam{
			Domain: domain,
			Name:   k,
			Value:  v,
		})
	}
	return func(config *Config) {
		config.cookies = append(config.cookies, tempCookies...)
	}
}

func WithScanRange(scanRange int) ConfigOpt {
	if scanRange > 3 || scanRange < 1 {
		log.Errorf("scan range error: %d, to default AllDomain", scanRange)
		return func(config *Config) {}
	}
	return func(config *Config) {
		config.scanRange = scanRange
	}
}

func WithScanRepeat(scanRepeat int) ConfigOpt {
	if scanRepeat > 3 || scanRepeat < 0 {
		log.Errorf("scan repeat error: %d, to default LowLevel", scanRepeat)
		return func(config *Config) {}
	}
	return func(config *Config) {
		config.scanRepeat = scanRepeat
	}
}

func WithChannel(ch chan ReqInfo) ConfigOpt {
	return func(config *Config) {
		config.channel = ch
	}
}

func WithOnRequest(f func(req ReqInfo)) ConfigOpt {
	return func(config *Config) {
		config.onRequest = f
	}
}

func WithCheckDanger() ConfigOpt {
	return func(config *Config) {
		config.checkDanger = detect.NormalCheckDangerUrl(SensitiveWords)
	}
}

func WithTags(tagsPath string) ConfigOpt {
	return func(config *Config) {
		config.tags = tagsPath
	}
}

func WithFullCrawlerTimeout(timeout int) ConfigOpt {
	return func(config *Config) {
		config.fullCrawlerTimeout = timeout
	}
}

func WithChromeWS(wsAddress string) ConfigOpt {
	return func(config *Config) {
		config.chromeWS = wsAddress
	}
}

func WithGetUrlRemote(confirm bool) ConfigOpt {
	return func(config *Config) {
		config.urlFromProxy = confirm
	}
}

func WithExtraHeaders(headers ...string) ConfigOpt {
	return func(config *Config) {
		for _, header := range headers {
			config.extraHeaders = append(config.extraHeaders, header)
		}
	}
}
