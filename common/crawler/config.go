package crawler

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/gobwas/glob"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
)

var (
	ExcludedSuffix = []string{
		".js", ".css",
		".jpg", ".jpeg", ".png",
		".mp3", ".mp4",
		".flv", ".aac", ".ogg",
		".svg", "ico", ".gif",
		".doc", "docx", ".pptx",
		".ppt", ".pdf",
	}
	ExcludedMIME = []string{"image/*",
		"audio/*", "video/*", "*octet-stream*",
		"application/ogg", "application/pdf", "application/msword",
		"application/x-ppt", "video/avi", "application/x-ico",
		"*zip",
	}
)

type header struct {
	Key   string
	Value string
}

type cookie struct {
	cookie        *http.Cookie
	allowOverride bool
}

func (c *Config) init() {
	c.BasicAuth = false
	c.concurrent = 20
	c.maxRedirectTimes = 5
	c.connectTimeout = 10 * time.Second
	c.responseTimeout = 10 * time.Second
	c.tlsConfig = &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionSSL30, // nolint[:staticcheck]
		MaxVersion:         tls.VersionTLS13,
	}
	c.userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/83.0.4103.116 Safari/537.36"
	c.maxDepth = 5
	c.maxBodySize = 10 * 1024 * 1024
	c.maxCountOfLinks = 10000
	c.maxCountOfRequest = 1000
	c.disallowSuffix = ExcludedSuffix
	c.disallowMIMEType = ExcludedMIME
	c.startFromParentPath = true
	c.maxRetryTimes = 3
	c.allowMethod = []string{"GET", "POST"}
	c.allowUrlRegexp = make(map[string]*regexp.Regexp)
	c.forbiddenUrlRegexp = make(map[string]*regexp.Regexp)
	c.forbiddenUrlRegexp["(?i)logout"] = regexp.MustCompile(`(?i)logout`)
	c.forbiddenUrlRegexp["(?i)setup"] = regexp.MustCompile(`(?i)setup`)
	c.forbiddenUrlRegexp["(?i)/exit"] = regexp.MustCompile(`(?i)/exit`)
	c.forbiddenUrlRegexp[`\?[^\s]*C=\w*;O=\w*`] = regexp.MustCompile(`\?\S*C=\w*;O=\w*`)
}

type Config struct {
	// 基础认证
	BasicAuth    bool
	AuthUsername string
	AuthPassword string

	// Transport 中的配置
	proxies          []string
	concurrent       int
	maxRedirectTimes int
	connectTimeout   time.Duration // 10s
	responseTimeout  time.Duration // 30s
	tlsConfig        *tls.Config

	// UA
	userAgent string

	// 最大深度，在 preReq 中实现
	maxDepth int // 5

	// maxBodySize 通过 execReq 实现限制
	maxBodySize int // 10 * 1024 * 1024

	// 通过 preReq 限制
	disallowSuffix []string

	// mime 限制
	disallowMIMEType []string

	// 请求最大值限制
	maxCountOfRequest int // 1000

	// maxCountOfLinks 在 handleReqResult 中限制
	maxCountOfLinks int // 2000

	//
	maxRetryTimes int // 3

	startFromParentPath bool     // true
	allowMethod         []string // GET / POST

	//
	allowDomains    []glob.Glob
	forbiddenDomain []glob.Glob

	allowUrlRegexp     map[string]*regexp.Regexp
	forbiddenUrlRegexp map[string]*regexp.Regexp // (?i(logout))

	headers []*header
	cookie  []*cookie

	onRequest func(req *Req)
	onLogin   func(req *Req, client *http.Client)

	extractionRules func(*Req) []interface{}
	// appended links
	extraPathForEveryPath     []string
	extraPathForEveryRootPath []string
}

func (c *Config) CheckShouldBeHandledURL(u *url.URL) bool {
	var pass = false

	// 只要有一个通过就通过
	if len(c.allowDomains) > 0 {
		pass = false
		for _, g := range c.allowDomains {
			if g.Match(u.Hostname()) {
				pass = true
				break
			}
		}
		if !pass {
			return false
		}
	}

	// 只要有一个不通过的黑名单就不通过
	pass = true
	for _, g := range c.forbiddenDomain {
		if g.Match(u.Hostname()) {
			pass = false
			break
		}
	}
	if !pass {
		return false
	}

	// 只要有一个 URL 白名单
	if len(c.allowUrlRegexp) > 0 {
		pass = false
		for _, g := range c.allowUrlRegexp {
			if g.MatchString(u.String()) {
				pass = true
				break
			}
		}
		if !pass {
			return false
		}
	}

	// 只要有一个不通过黑名单就不能通过
	pass = true
	for _, g := range c.forbiddenUrlRegexp {
		if g.MatchString(u.String()) {
			pass = false
			break
		}
	}
	if !pass {
		return false
	}

	if len(c.disallowSuffix) > 0 {
		ext := filepath.Ext(u.Path)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if utils.StringSliceContain(c.disallowSuffix, ext) {
			return false
		}
	}

	return true
}

type roundRobinSwitcher struct {
	proxyURLs []*url.URL
	index     uint32
}

func (r *roundRobinSwitcher) GetProxy(pr *http.Request) (*url.URL, error) {
	u := r.proxyURLs[r.index%uint32(len(r.proxyURLs))]
	atomic.AddUint32(&r.index, 1)
	return u, nil
}

// RoundRobinProxySwitcher creates a proxy switcher function which rotates
// ProxyURLs on every request.
// The proxy type is determined by the URL scheme. "http", "https"
// and "socks5" are supported. If the scheme is empty,
// "http" is assumed.
func RoundRobinProxySwitcher(ProxyURLs ...string) (func(r *http.Request) (*url.URL, error), error) {
	urls := make([]*url.URL, len(ProxyURLs))
	for i, u := range ProxyURLs {
		parsedU, err := url.Parse(u)
		if err != nil {
			return nil, err
		}
		urls[i] = parsedU
	}
	return (&roundRobinSwitcher{urls, 0}).GetProxy, nil
}

func (c *Config) CreateHTTPClient() *http.Client {
	// 设置 Transport
	httpTr := &http.Transport{
		// 关闭 http keep-alive
		DisableKeepAlives: true,
		// 设置最大平行连接数
		MaxConnsPerHost: c.concurrent,
		// 设置超时
		DialContext: (&net.Dialer{
			Timeout: c.connectTimeout,
		}).DialContext,
		// 设置 HTTP 响应获取超时
		ResponseHeaderTimeout: c.responseTimeout,
		// 使用自定义的 tlsConfig 进行 TCP 连接
		TLSClientConfig: c.tlsConfig,

		// 不要用 gzip
		DisableCompression: true,
	}

	// 设置代理
	if len(c.proxies) > 0 {
		proxySwitcher, err := RoundRobinProxySwitcher(c.proxies...)
		if err != nil {
			log.Errorf("create proxy switcher failed: %s", err)
		}
		if proxySwitcher != nil {
			httpTr.Proxy = proxySwitcher
		}
	}

	return &http.Client{
		Transport: httpTr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirectVector := ""
			for _, r := range via {
				redirectVector += fmt.Sprintf("%s => ", r.URL.String())
			}
			redirectVector += req.URL.String()
			if len(via) > c.maxRedirectTimes {
				return utils.Errorf("max redirect times reach: %v", redirectVector)
			}
			log.Warnf("redirect: %v", redirectVector)
			return nil
		},
	}
}

type configOpt func(c *Config)

func WithDisallowSuffix(d []string) configOpt {
	return func(c *Config) {
		c.disallowSuffix = d
	}
}

func WithDisallowMIMEType(d []string) configOpt {
	return func(c *Config) {
		c.disallowMIMEType = d
	}
}

func WithBasicAuth(user, pass string) configOpt {
	return func(c *Config) {
		c.BasicAuth = true
		c.AuthUsername = user
		c.AuthPassword = pass
	}
}

func WithProxy(proxies ...string) configOpt {
	return func(c *Config) {
		c.proxies = proxies
	}
}

func WithConcurrent(concurrent int) configOpt {
	return func(c *Config) {
		c.concurrent = concurrent
	}
}

func WithMaxRedirectTimes(maxRedirectTimes int) configOpt {
	return func(c *Config) {
		c.maxRedirectTimes = maxRedirectTimes
	}
}

func WithDomainWhiteList(domain string) configOpt {
	var pattern string
	if !strings.HasPrefix(domain, "*") {
		pattern = "*" + domain
	}
	p, err := glob.Compile(pattern)
	if err != nil {
		log.Errorf("limit domain[%v] failed: %v", domain, err)
		return func(c *Config) {
			for _, i := range HostToWildcardGlobs(domain) {
				c.allowDomains = append(c.allowDomains, i)
			}
		}
	}

	return func(c *Config) {
		for _, i := range HostToWildcardGlobs(domain) {
			c.allowDomains = append(c.allowDomains, i)
		}
		c.allowDomains = append(c.allowDomains, p)
	}
}

func WithDomainBlackList(domain string) configOpt {
	var pattern string
	if !strings.HasPrefix(domain, "*") {
		pattern = "*" + domain
	}
	p, err := glob.Compile(pattern)
	if err != nil {
		log.Errorf("limit domain[%v] failed: %v", domain, err)
		return func(c *Config) {}
	}
	return func(c *Config) {
		c.forbiddenDomain = append(c.forbiddenDomain, p)
	}
}

func WithExtraSuffixForEveryPath(path ...string) configOpt {
	return func(c *Config) {
		c.extraPathForEveryPath = path
	}
}

func WithExtraSuffixForEveryRootPath(path ...string) configOpt {
	return func(c *Config) {
		c.extraPathForEveryPath = path
	}
}

func WithUrlRegexpWhiteList(re string) configOpt {
	p, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("limit url regexp[%v] whitelist failed: %v", re, err)
		return func(c *Config) {}
	}
	return func(c *Config) {
		c.allowUrlRegexp[re] = p
	}
}

func WithUrlRegexpBlackList(re string) configOpt {
	p, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("limit url regexp[%v] blacklist failed: %v", re, err)
		return func(c *Config) {}
	}
	return func(c *Config) {
		c.forbiddenUrlRegexp[re] = p
	}
}

func WithConnectTimeout(f float64) configOpt {
	return func(c *Config) {
		c.connectTimeout = time.Duration(int64(f * float64(time.Second)))
	}
}

func WithResponseTimeout(f float64) configOpt {
	return func(c *Config) {
		c.responseTimeout = time.Duration(int64(f * float64(time.Second)))
	}
}

func WithUserAgent(ua string) configOpt {
	return func(c *Config) {
		c.userAgent = ua
	}
}

func WithMaxDepth(depth int) configOpt {
	return func(c *Config) {
		c.maxDepth = depth
	}
}

func WithBodySize(size int) configOpt {
	return func(c *Config) {
		c.maxBodySize = size
	}
}

func WithMaxRequestCount(limit int) configOpt {
	return func(c *Config) {
		c.maxCountOfRequest = limit
	}
}

func WithMaxUrlCount(limit int) configOpt {
	return func(c *Config) {
		c.maxCountOfLinks = limit
	}
}

func WithMaxRetry(limit int) configOpt {
	return func(c *Config) {
		c.maxRetryTimes = limit
	}
}

func WithForbiddenFromParent(b bool) configOpt {
	return func(c *Config) {
		c.startFromParentPath = !b
	}
}

func WithHeader(k, v string) configOpt {
	return func(c *Config) {
		c.headers = append(c.headers, &header{
			Key:   k,
			Value: v,
		})
	}
}
func WithUrlExtractor(f func(*Req) []interface{}) configOpt {
	return func(c *Config) {
		c.extractionRules = f
	}
}
func WithFixedCookie(k, v string) configOpt {
	return func(c *Config) {
		c.cookie = append(c.cookie, &cookie{
			cookie: &http.Cookie{
				Name:  k,
				Value: v,
			},
			allowOverride: false,
		})
	}
}

func WithOnRequest(f func(req *Req)) configOpt {
	return func(c *Config) {
		c.onRequest = f
	}
}

func WithAutoLogin(username, password string, flags ...string) configOpt {
	return func(c *Config) {
		c.onLogin = func(req *Req, client *http.Client) {
			if !utils.IContains(req.Request().Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
				return
			}

			_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(req.requestRaw)
			if body == nil {
				log.Errorf("auto login failed... body empty")
				return
			}

			values, err := url.ParseQuery(string(body))
			if err != nil {
				log.Errorf("parse body to query kvs failed: %s", err)
				return
			}

			values.Set(req.maybeLoginUsername, username)
			values.Set(req.maybeLoginPassword, password)

			valueStr := strings.TrimSpace(values.Encode())
			if c.maxRetryTimes <= 0 {
				c.maxRetryTimes = 1
			}
			for range make([]int, c.maxRetryTimes) {
				req.request.GetBody = func() (io.ReadCloser, error) {
					req.request.ContentLength = int64(len(valueStr))
					return ioutil.NopCloser(bytes.NewBufferString(valueStr)), nil
				}
				req.request.Body, _ = req.request.GetBody()
				rsp, err := client.Do(req.request)
				if err != nil {
					log.Errorf("execute login form request failed: %s", err)
					continue
				}
				for _, originCookie := range rsp.Cookies() {
					c.cookie = append(c.cookie, &cookie{
						cookie:        originCookie,
						allowOverride: false,
					})
				}
				req.response = rsp
				break
			}
		}
	}
}
