package crawler

import (
	"bytes"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
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
}

var configMutex = new(sync.Mutex)

func (c *Config) GetLowhttpConfig() []lowhttp.LowhttpOpt {
	configMutex.Lock()
	defer configMutex.Unlock()

	if len(c._cachedOpts) > 0 {
		return c._cachedOpts
	}

	var opts []lowhttp.LowhttpOpt
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
		// dummy, ignore it
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
