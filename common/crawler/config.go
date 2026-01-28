package crawler

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var (
	ExcludedSuffix = []string{
		".css",
		".jpg", ".jpeg", ".png",
		".mp3", ".mp4",
		".flv", ".aac", ".ogg",
		".svg", "ico", ".gif",
		".doc", "docx", ".pptx",
		".ppt", ".pdf",
	}
	ExcludedMIME = []string{
		"image/*",
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
	c.enableJSParser = false
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
	// tlsConfig        *tls.Config

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
	onLogin   func(req *Req)

	extractionRules func(*Req) []interface{}
	// appended links
	extraPathForEveryPath     []string
	extraPathForEveryRootPath []string

	// cache, do not
	_cachedOpts []lowhttp.LowhttpOpt

	// runtime id
	runtimeID string
	// js parser
	enableJSParser bool
}

var configMutex = new(sync.Mutex)

func (c *Config) GetLowhttpConfig() []lowhttp.LowhttpOpt {
	configMutex.Lock()
	defer configMutex.Unlock()

	if len(c._cachedOpts) > 0 {
		return c._cachedOpts
	}

	var opts []lowhttp.LowhttpOpt
	opts = append(opts, lowhttp.WithSource("crawler")) // 设置爬虫流量来源
	opts = append(opts, lowhttp.WithProxy(c.proxies...))
	if c.AuthUsername != "" || c.AuthPassword != "" {
		opts = append(opts, lowhttp.WithUsername(c.AuthUsername), lowhttp.WithPassword(c.AuthPassword))
	}
	if c.maxRedirectTimes > 0 {
		opts = append(opts, lowhttp.WithRedirectTimes(c.maxRedirectTimes))
	}
	if c.connectTimeout > 0 {
		opts = append(opts, lowhttp.WithTimeout(c.connectTimeout))
	}
	if c.maxRetryTimes > 0 {
		opts = append(opts, lowhttp.WithRetryTimes(c.maxRetryTimes))
	}
	c._cachedOpts = opts
	return opts
}

func (c *Config) CheckShouldBeHandledURL(u *url.URL) bool {
	pass := false

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

type ConfigOpt func(c *Config)

// disallowSuffix 是一个选项函数，用于指定爬虫时的后缀黑名单
// Example:
// ```
// crawler.Start("https://example.com", crawler.disallowSuffix(".css", ".jpg", ".png")) // 爬虫时不会爬取css、jpg、png文件
// ```
func WithDisallowSuffix(d []string) ConfigOpt {
	return func(c *Config) {
		c.disallowSuffix = d
	}
}

func WithDisallowMIMEType(d []string) ConfigOpt {
	return func(c *Config) {
		c.disallowMIMEType = d
	}
}

// basicAuth 是一个选项函数，用于指定爬虫时的自动该填写的基础认证用户名和密码
// Example:
// ```
// crawler.Start("https://example.com", crawler.basicAuth("admin", "admin"))
// ```
func WithBasicAuth(user, pass string) ConfigOpt {
	return func(c *Config) {
		c.BasicAuth = true
		c.AuthUsername = user
		c.AuthPassword = pass
	}
}

// proxy 是一个选项函数，用于指定爬虫时的代理
// Example:
// ```
// crawler.Start("https://example.com", crawler.proxy("http://127.0.0.1:8080"))
// ```
func WithProxy(proxies ...string) ConfigOpt {
	return func(c *Config) {
		c.proxies = proxies
	}
}

// concurrent 是一个选项函数，用于指定爬虫时的并发数，默认为20
// Example:
// ```
// crawler.Start("https://example.com", crawler.concurrent(10))
// ```
func WithConcurrent(concurrent int) ConfigOpt {
	return func(c *Config) {
		c.concurrent = concurrent
	}
}

// maxRedirect 是一个选项函数，用于指定爬虫时的最大重定向次数，默认为5
// Example:
// ```
// crawler.Start("https://example.com", crawler.maxRedirect(10))
// ```
func WithMaxRedirectTimes(maxRedirectTimes int) ConfigOpt {
	return func(c *Config) {
		c.maxRedirectTimes = maxRedirectTimes
	}
}

// domainInclude 是一个选项函数，用于指定爬虫时的域名白名单
// domain允许使用glob语法，例如*.example.com
// Example:
// ```
// crawler.Start("https://example.com", crawler.domainInclude("*.example.com"))
// ```
func WithDomainWhiteList(domain string) ConfigOpt {
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

// domainExclude 是一个选项函数，用于指定爬虫时的域名黑名单
// domain允许使用glob语法，例如*.example.com
// Example:
// ```
// crawler.Start("https://example.com", crawler.domainExclude("*.baidu.com"))
// ```
func WithDomainBlackList(domain string) ConfigOpt {
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

func WithExtraSuffixForEveryPath(path ...string) ConfigOpt {
	return func(c *Config) {
		c.extraPathForEveryPath = path
	}
}

func WithExtraSuffixForEveryRootPath(path ...string) ConfigOpt {
	return func(c *Config) {
		c.extraPathForEveryPath = path
	}
}

// urlRegexpInclude 是一个选项函数，用于指定爬虫时的URL正则白名单
// Example:
// ```
// crawler.Start("https://example.com", crawler.urlRegexpInclude(`\.html`))
// ```
func WithUrlRegexpWhiteList(re string) ConfigOpt {
	p, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("limit url regexp[%v] whitelist failed: %v", re, err)
		return func(c *Config) {}
	}
	return func(c *Config) {
		c.allowUrlRegexp[re] = p
	}
}

// urlRegexpExclude 是一个选项函数，用于指定爬虫时的URL正则黑名单
// Example:
// ```
// crawler.Start("https://example.com", crawler.urlRegexpExclude(`\.jpg`))
// ```
func WithUrlRegexpBlackList(re string) ConfigOpt {
	p, err := regexp.Compile(re)
	if err != nil {
		log.Errorf("limit url regexp[%v] blacklist failed: %v", re, err)
		return func(c *Config) {}
	}
	return func(c *Config) {
		c.forbiddenUrlRegexp[re] = p
	}
}

// connectTimeout 是一个选项函数，用于指定爬虫时的连接超时时间，默认为10s
// Example:
// ```
// crawler.Start("https://example.com", crawler.connectTimeout(5))
// ```
func WithConnectTimeout(f float64) ConfigOpt {
	return func(c *Config) {
		c.connectTimeout = time.Duration(int64(f * float64(time.Second)))
	}
}

// responseTimeout 是一个选项函数，用于指定爬虫时的响应超时时间，默认为10s
// ! 未实现
// Example:
// ```
// crawler.Start("https://example.com", crawler.responseTimeout(5))
// ```
func WithResponseTimeout(f float64) ConfigOpt {
	return func(c *Config) {
		// dummy, ignore it
	}
}

// userAgent 是一个选项函数，用于指定爬虫时的User-Agent
// Example:
// ```
// crawler.Start("https://example.com", crawler.userAgent("yaklang-crawler"))
// ```
func WithUserAgent(ua string) ConfigOpt {
	return func(c *Config) {
		c.userAgent = ua
	}
}

// maxDepth 是一个选项函数，用于指定爬虫时的最大深度，默认为5
// Example:
// ```
// crawler.Start("https://example.com", crawler.maxDepth(10))
// ```
func WithMaxDepth(depth int) ConfigOpt {
	return func(c *Config) {
		c.maxDepth = depth
	}
}

// bodySize 是一个选项函数，用于指定爬虫时的最大响应体大小，默认为10MB
// Example:
// ```
// crawler.Start("https://example.com", crawler.bodySize(1024 * 1024))
// ```
func WithBodySize(size int) ConfigOpt {
	return func(c *Config) {
		c.maxBodySize = size
	}
}

// maxRequest 是一个选项函数，用于指定爬虫时的最大请求数，默认为1000
// Example:
// ```
// crawler.Start("https://example.com", crawler.maxRequest(10000))
// ```
func WithMaxRequestCount(limit int) ConfigOpt {
	return func(c *Config) {
		c.maxCountOfRequest = limit
	}
}

// maxUrls 是一个选项函数，用于指定爬虫时的最大链接数，默认为10000
// Example:
// ```
// crawler.Start("https://example.com", crawler.maxUrls(20000))
// ```
func WithMaxUrlCount(limit int) ConfigOpt {
	return func(c *Config) {
		c.maxCountOfLinks = limit
	}
}

// maxRetry 是一个选项函数，用于指定爬虫时的最大重试次数，默认为3
// Example:
// ```
// crawler.Start("https://example.com", crawler.maxRetry(10))
// ```
func WithMaxRetry(limit int) ConfigOpt {
	return func(c *Config) {
		c.maxRetryTimes = limit
	}
}

// forbiddenFromParent 是一个选项函数，用于指定爬虫时的是否禁止从根路径发起请求，默认为false
// 对于一个起始URL，如果其并不是从根路径开始且没有禁止从根路径发起请求，那么爬虫会从其根路径开始爬取
// Example:
// ```
// crawler.Start("https://example.com/a/b/c", crawler.forbiddenFromParent(false)) // 这会从 https://example.com/ 开始爬取
// ```
func WithForbiddenFromParent(b bool) ConfigOpt {
	return func(c *Config) {
		c.startFromParentPath = !b
	}
}

// header 是一个选项函数，用于指定爬虫时的请求头
// Example:
// ```
// crawler.Start("https://example.com", crawler.header("User-Agent", "yaklang-crawler"))
// ```
func WithHeader(k, v string) ConfigOpt {
	return func(c *Config) {
		c.headers = append(c.headers, &header{
			Key:   k,
			Value: v,
		})
	}
}

// urlExtractor 是一个选项函数，它接收一个函数作为参数，用于为爬虫添加额外的链接提取规则
// Example:
// ```
// crawler.Start("https://example.com", crawler.urlExtractor(func(req) {
// 尝试编写自己的规则，从响应体(req.Response()或req.ResponseRaw())中提取额外的链接
// })
// ```
func WithUrlExtractor(f func(*Req) []interface{}) ConfigOpt {
	return func(c *Config) {
		c.extractionRules = f
	}
}

// cookie 是一个选项函数，用于指定爬虫时的cookie
// Example:
// ```
// crawler.Start("https://example.com", crawler.cookie("key", "value"))
// ```
func WithFixedCookie(k, v string) ConfigOpt {
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

func WithOnRequest(f func(req *Req)) ConfigOpt {
	return func(c *Config) {
		c.onRequest = f
	}
}

// autoLogin 是一个选项函数，用于指定爬虫时的自动填写可能存在的登录表单
// Example:
// ```
// crawler.Start("https://example.com", crawler.autoLogin("admin", "admin"))
// ```
func WithAutoLogin(username, password string, flags ...string) ConfigOpt {
	return func(c *Config) {
		c.onLogin = func(req *Req) {
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
			req.request.GetBody = func() (io.ReadCloser, error) {
				req.request.ContentLength = int64(len(valueStr))
				return ioutil.NopCloser(bytes.NewBufferString(valueStr)), nil
			}
			req.request.Body, _ = req.request.GetBody()

			opts := c.GetLowhttpConfig()
			req.requestRaw, _ = utils.DumpHTTPRequest(req.request, true)
			opts = append(opts, lowhttp.WithPacketBytes(req.requestRaw), lowhttp.WithHttps(req.IsHttps()))

			rspIns, err := lowhttp.HTTP(opts...)
			if err != nil {
				req.err = err
				return
			}
			for _, cookieItem := range lowhttp.ExtractCookieJarFromHTTPResponse(rspIns.RawPacket) {
				c.cookie = append(c.cookie, &cookie{cookie: cookieItem, allowOverride: false})
			}
			req.responseRaw = rspIns.RawPacket
			req.response, _ = utils.ReadHTTPResponseFromBytes(rspIns.RawPacket, req.request)
		}
	}
}

func WithRuntimeID(id string) ConfigOpt {
	return func(c *Config) {
		c.runtimeID = id
	}
}

// jsParser 是一个选项函数，用于指定爬虫时是否进行对于JS的代码解析。
// 填写该选项默认开启，也可以传入false强制关闭。
// Example:
// ```
// crawler.Start("https://example.com", crawler.jsParser()) // 开启
// crawler.Start("https://example.com", crawler.jsParser(true)) // 开启
// crawler.Start("https://example.com", crawler.jsParser(false)) // 关闭
// ```
func WithJSParser(enable ...bool) ConfigOpt {
	return func(c *Config) {
		if len(enable) > 0 {
			c.enableJSParser = enable[0]
		} else {
			c.enableJSParser = true
		}
	}
}
