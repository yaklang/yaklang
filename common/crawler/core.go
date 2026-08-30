package crawler

import (
	"bytes"
	"context"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/html"

	"github.com/gobwas/glob"
)

const (
	twoMB = 2 * 1024 * 1024
)

var URLPattern, _ = regexp.Compile(`(((?:[a-zA-Z]{1,10}://|//)[^"'/]{1,}\.[a-zA-Z]{2,}[^"']{0,})|((?:/|\.\./|\./)[^"'><,;|*()(%%$^/\\\[\]][^"'><,;|()]{1,})|([a-zA-Z0-9_\-/]{1,}/[a-zA-Z0-9_\-/]{1,}\.(?:[a-zA-Z]{1,4}|action)(?:[\?|/][^"|']{0,}|))|([a-zA-Z0-9_\-]{1,}\.(?:\.{1,10})(?:\?[^"|']{0,}|)))`)

type Crawler struct {
	originUrls []string
	config     *Config

	preRequestLock   *sync.Mutex
	afterRequestLock *sync.Mutex

	//
	finished *utils.AtomicBool
	starting *utils.AtomicBool

	requestCounter int64
	linkCounter    int64
	discoveryMu    sync.Mutex

	requestedHash *sync.Map
	foundUrls     *sync.Map
	reportedUrls  *sync.Map
	scheduler     *requestScheduler

	ctx    context.Context
	cancel context.CancelFunc

	// login
	loginOnce *sync.Once // := new(sync.Once)
}

type requestScheduler struct {
	ctx context.Context

	mu            sync.Mutex
	high          []*Req
	normal        []*Req
	queueCapacity int
	highStreak    int
	wake          chan struct{}

	pending     atomic.Int64
	startupDone atomic.Bool
	closed      atomic.Bool
}

const requestSchedulerHighBurst = 8

func newRequestScheduler(ctx context.Context, queueSize int) *requestScheduler {
	if ctx == nil {
		ctx = context.Background()
	}
	if queueSize < 0 {
		queueSize = 0
	}
	return &requestScheduler{
		ctx:           ctx,
		queueCapacity: queueSize,
		wake:          make(chan struct{}, 1),
	}
}

func (s *requestScheduler) Submit(req *Req) (ok bool) {
	if s == nil || req == nil || s.contextDone() || s.closed.Load() {
		return false
	}

	s.mu.Lock()
	if s.closed.Load() || s.contextDone() {
		s.mu.Unlock()
		return false
	}
	queue := &s.normal
	if req.priority {
		queue = &s.high
	}
	// A zero capacity is intentionally unlimited. It is used only when the
	// caller explicitly disables maxUrls; positive limits retain bounded
	// queues.
	if s.queueCapacity > 0 && len(*queue) >= s.queueCapacity {
		s.mu.Unlock()
		return false
	}
	*queue = append(*queue, req)
	s.pending.Add(1)
	s.mu.Unlock()
	s.signal()
	return true
}

// Next returns the next accepted request. Asset requests are preferred, while
// a bounded burst keeps ordinary pages from being permanently starved.
func (s *requestScheduler) Next() (*Req, bool) {
	if s == nil {
		return nil, false
	}
	for {
		s.mu.Lock()
		if s.closed.Load() {
			s.mu.Unlock()
			return nil, false
		}
		if len(s.high) > 0 && (s.highStreak < requestSchedulerHighBurst || len(s.normal) == 0) {
			req := s.high[0]
			s.high[0] = nil
			s.high = s.high[1:]
			s.highStreak++
			s.mu.Unlock()
			return req, true
		}
		if len(s.normal) > 0 {
			req := s.normal[0]
			s.normal[0] = nil
			s.normal = s.normal[1:]
			s.highStreak = 0
			s.mu.Unlock()
			return req, true
		}
		s.mu.Unlock()

		select {
		case <-s.ctx.Done():
			return nil, false
		case <-s.wake:
		}
	}
}

func (s *requestScheduler) Done() {
	if s == nil {
		return
	}
	left := s.pending.Add(-1)
	if left < 0 {
		log.Errorf("crawler request scheduler pending counter is negative")
		s.pending.Store(0)
		left = 0
	}
	if left == 0 {
		s.maybeClose()
	}
}

func (s *requestScheduler) StartupDone() {
	if s == nil {
		return
	}
	s.startupDone.Store(true)
	s.maybeClose()
}

func (s *requestScheduler) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed.Store(true)
	s.mu.Unlock()
	s.signal()
}

func (s *requestScheduler) maybeClose() {
	if s == nil {
		return
	}
	s.mu.Lock()
	shouldClose := s.startupDone.Load() && s.pending.Load() == 0 && len(s.high) == 0 && len(s.normal) == 0
	if shouldClose {
		s.closed.Store(true)
	}
	s.mu.Unlock()
	if shouldClose {
		s.signal()
	}
}

func (s *requestScheduler) signal() {
	if s == nil || s.wake == nil {
		return
	}
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *requestScheduler) contextDone() bool {
	if s == nil || s.ctx == nil {
		return false
	}
	select {
	case <-s.ctx.Done():
		return true
	default:
		return false
	}
}

// Hash 返回当前请求的哈希值，其值由请求的URL与请求方法组成
// Example:
// ```
// req.Hash()
// ```
func (r *Req) Hash() string {
	return utils.CalcSha1(r.request.URL.String(), r.request.Method)
}

// IsLoginForm 判断当前请求是否是一个登录表单
// Example:
// ```
// req.IsLoginForm()
// ```
func (r *Req) IsLoginForm() bool {
	return r.maybeLoginForm
}

// IsUploadForm 判断当前请求是否是一个上传表单
// Example:
// ```
// req.IsUploadForm()
// ```
func (r *Req) IsUploadForm() bool {
	return r.maybeUploadForm
}

// IsForm 判断当前请求是否是一个表单
// Example:
// ```
// req.IsForm()
// ```
func (r *Req) IsForm() bool {
	return r.isForm
}

type Req struct {
	// 当前请求所属深度
	depth int

	url         string
	https       bool
	request     *http.Request
	requestRaw  []byte
	response    *http.Response
	responseRaw []byte

	// 如果请求失败了，原因是
	err error

	// 如果有的话，寻找 html/js 信息
	responseBody   []byte
	responseHeader string

	// 请求计数，请求过几次成功了
	requestedCounter int

	// 是不是从表单解析出来的？
	isForm bool

	// 这个请求是不是可能和登录相关？
	maybeLoginForm     bool
	maybeLoginUsername string
	maybeLoginPassword string
	maybeUploadForm    bool

	baseURL *url.URL

	// 私有，判断是否是 "同域"
	// 这个 "域" 简单暴力，仅检测 host 部分是不是类似？*origin-domain* glob 语法
	_selfDomainGlobs []glob.Glob

	// default
	disallowedMITMType bool

	// priority marks bounded, high-value assets such as JavaScript chunks,
	// source maps and runtime manifests. It is intentionally crawler-private.
	priority bool
}

func HostToWildcardGlobs(host string) []glob.Glob {
	var globsIns []glob.Glob
	g, err := glob.Compile(host)
	if err != nil {
		log.Errorf("compile self error: %s", err)
		return nil
	}
	globsIns = append(globsIns, g)

	if utils.IsIPv4(host) {
		list := strings.Split(host, ".")
		list[len(list)-1] = "*"
		g, err := glob.Compile(strings.Join(list, "."))
		if err != nil {
			log.Errorf("compile glob[%s] failed: %s", g, err)
			return globsIns
		}
		globsIns = append(globsIns, g)
	} else {
		list := strings.Split(host, ".")
		var globs []string
		globs = append(globs, host, host+"*", host+".*", "*"+host, "*."+host)
		if len(list) > 0 {
			if strings.Contains(list[0], "www") {
				list2 := list[:]
				list2[0] = "*"
				globs = append(globs, strings.Join(list2, "."))
			}
		}
		for _, g := range globs {
			ins, err := glob.Compile(g)
			if err != nil {
				log.Errorf("compile glob[%s] failed: %s", g, err)
				continue
			}
			globsIns = append(globsIns, ins)
		}
	}
	return globsIns
}

// SameWildcardOrigin 判断当前请求与传入的请求是否是同域的
// Example:
// ```
// req1.SameWildcardOrigin(req2)
// ```
func (r *Req) SameWildcardOrigin(s *Req) bool {
	if s.baseURL == nil {
		return false
	}
	targetHost, _, _ := utils.ParseStringToHostPort(s.baseURL.String())
	if r.baseURL == nil || targetHost == "" {
		return false
	}
	if r._selfDomainGlobs != nil {
		host, _, _ := utils.ParseStringToHostPort(r.baseURL.String())
		if host == "" {
			return false
		}
		r._selfDomainGlobs = HostToWildcardGlobs(host)
	}

	for _, i := range r._selfDomainGlobs {
		if i.Match(targetHost) {
			return true
		}
	}
	return false
}

// AbsoluteURL 根据当前请求的URL，将传入的相对路径转换为绝对路径
// Example:
// ```
// req.AbsoluteURL("/a/b/c")
// ```
func (r *Req) AbsoluteURL(u string) string {
	if u == "" {
		return ""
	}

	if strings.HasPrefix(u, "#") {
		return ""
	}
	var base *url.URL
	if r.baseURL != nil {
		base = r.baseURL
	} else {
		base = r.request.URL
	}
	absURL, err := base.Parse(u)
	if err != nil {
		return ""
	}
	absURL.Fragment = ""
	if absURL.Scheme == "//" {
		absURL.Scheme = r.request.URL.Scheme
	}
	return absURL.String()
}

// Start 启动爬虫爬取某个URL，它还可以接收零个到多个选项函数，用于影响爬取行为
// 返回一个Req结构体引用管道与错误
// 参数:
//   - url: 起始爬取的 URL
//   - opt: 零个或多个爬虫配置选项函数
//
// 返回值:
//   - 一个可迭代的 Req 结构体引用管道，用于读取爬取到的请求
//   - error: 启动失败时返回错误
//
// Example:
// ```
// ch, err := crawler.Start("https://www.baidu.com", crawler.concurrent(10))
// for req in ch {
// println(req.Response()~)
// }
// ```
func StartCrawler(url string, opt ...ConfigOpt) (chan *Req, error) {
	var resultChan *chanx.UnlimitedChan[*Req]
	opt = append(opt, WithOnRequest(func(req *Req) {
		resultChan.SafeFeed(req)
	}))

	crawler, err := NewCrawler(url, opt...)
	if err != nil {
		return nil, utils.Errorf("create crawler failed: %s", err)
	}
	ch := make(chan *Req, 64)
	resultChan = chanx.NewUnlimitedChanEx[*Req](crawler.ctx, make(chan *Req, 64), ch, 64)
	go func() {
		defer resultChan.Close()

		err := crawler.Run()
		if err != nil {
			log.Error(err)
		}
	}()
	return ch, nil
}

func NewCrawler(urls string, opts ...ConfigOpt) (*Crawler, error) {
	config := &Config{}
	config.init()
	for _, opt := range opts {
		opt(config)
	}

	urlsRaw := utils.PrettifyListFromStringSplited(urls, ",")
	var urlList []string
	if config.exactOrigins {
		urlList = utils.ParseStringToUrls(urlsRaw...)
	} else {
		urlList = utils.ParseStringToUrlsWith3W(urlsRaw...)
	}
	for i, rawURL := range urlList {
		urlList[i] = stripURLFragment(rawURL)
	}
	log.Debugf("actual url list: %v", urlList)

	// 把自己的域名加在里面
	for _, u := range urlList {
		urlIns, err := url.Parse(u)
		if err != nil {
			continue
		}
		if config.exactOrigins {
			WithDomainWhiteListExactPattern(urlIns.Hostname())(config)
		} else {
			WithDomainWhiteList(urlIns.Hostname())(config)
		}
	}

	if config.concurrent <= 0 {
		config.concurrent = 20
	}
	if config.ctx == nil {
		config.ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(config.ctx)
	config.ctx = ctx
	config._cachedOpts = nil

	c := &Crawler{
		originUrls:       urlList,
		config:           config,
		preRequestLock:   new(sync.Mutex),
		afterRequestLock: new(sync.Mutex),

		finished:      utils.NewBool(false),
		starting:      utils.NewBool(false),
		requestedHash: new(sync.Map),
		foundUrls:     new(sync.Map),
		reportedUrls:  new(sync.Map),
		ctx:           ctx,
		cancel:        cancel,
		loginOnce:     new(sync.Once),
	}

	return c, nil
}

func (c *Crawler) Run() error {
	if c.finished.IsSet() || c.starting.IsSet() {
		return utils.Errorf("cannot call Run multi-times...")
	}
	c.initScheduler()

	defer func() {
		if c.scheduler != nil {
			c.scheduler.Close()
		}
		if c.cancel != nil {
			c.cancel()
		}
		c.finished.Set()
	}()

	c.starting.Set()
	defer c.starting.UnSet()

	go func() {
		defer func() {
			utils.Debug(func() {
				log.Debugf("finished dispatching all tasks...")
			})
			c.scheduler.StartupDone()
		}()

		log.Debug("start to submit tasks...")
		if c.config.startFromParentPath {
			// 从父路径开始
			var moreUrl []string
			for _, u := range c.originUrls {
				urlIns, err := url.Parse(u)
				if err != nil {
					continue
				}
				raw := strings.Split(urlIns.Path, "/")
				for i := 0; i < len(raw); i++ {
					rawPath := strings.Join(raw[:len(raw)-i], "/")
					if !strings.HasPrefix(rawPath, "/") {
						rawPath = "/" + rawPath
					}
					urlIns.Path = rawPath
					urlIns.RawQuery = ""
					moreUrl = append(moreUrl, urlIns.String())

					if !strings.HasSuffix(urlIns.Path, "/") {
						urlIns.Path += "/"
						urlIns.RawQuery = ""
						moreUrl = append(moreUrl, urlIns.String())
					}
				}
			}
		}
		for _, u := range c.originUrls {
			if c.contextDone() {
				return
			}
			newReq, err := c.createReqFromUrl(nil, u)
			if err != nil {
				log.Error(err)
				continue
			}
			log.Debugf("submit request from url: %s", u)
			c.submit(newReq)
		}
	}()

	log.Debug("start to handling requests")
	c.run()
	return nil
}

func (c *Crawler) run() {
	config := c.config
	concurrent := config.concurrent
	if concurrent <= 0 {
		concurrent = 1
	}
	workerLimiter := make(chan struct{}, concurrent)
	var workerWG sync.WaitGroup
	for {
		if c.contextDone() {
			c.scheduler.Close()
			workerWG.Wait()
			return
		}
		r, ok := c.scheduler.Next()
		if !ok {
			workerWG.Wait()
			return
		}
		if c.contextDone() {
			c.scheduler.Done()
			continue
		}

		log.Debugf("start to handling request: %v", r.request.URL.String())

		// 预处理失败
		c.preRequestLock.Lock()
		if c.contextDone() || !c.preReq(r) {
			c.preRequestLock.Unlock()
			c.scheduler.Done()
			continue
		}

		c.requestCounter++
		overRequestLimit := c.requestCounter > int64(config.maxCountOfRequest)
		c.preRequestLock.Unlock()

		// Keep the limit decision under preRequestLock: direct JavaScript
		// asset workers reserve from the same counter.
		if overRequestLimit {
			c.scheduler.Done()
			continue
		}

		// 已经被请求过了
		_, ok = c.requestedHash.Load(r.Hash())
		if ok {
			c.scheduler.Done()
			continue
		}

		// 检查是不是符合访问标准
		if r.request.URL.Host == "" {
			r.request, _ = utils.ReadHTTPRequestFromBytes(r.requestRaw)
		}
		if !config.CheckShouldBeHandledURL(r.request.URL) {
			c.requestedHash.Store(r.Hash(), nil)
			c.scheduler.Done()
			continue
		}

		select {
		case workerLimiter <- struct{}{}:
		case <-c.ctx.Done():
			c.scheduler.Done()
			continue
		}
		workerWG.Add(1)
		go func(r *Req) {
			defer func() {
				<-workerLimiter
				c.scheduler.Done()
				workerWG.Done()
			}()
			log.Debugf("request to %v", r.request.URL.String())
			c.requestedHash.Store(r.Hash(), nil)
			c.execReq(r)
			if c.contextDone() {
				return
			}

			// 发送结束了
			c.afterRequestLock.Lock()
			c.handleReqResult(r)
			c.afterRequestLock.Unlock()
		}(r)
	}
}

// RequestsFromFlow 尝试从一次请求与响应中爬取出所有可能的请求，返回所有可能请求的原始报文与错误
// 参数:
//   - isHttps: 该流量是否为 HTTPS
//   - reqBytes: 请求原始报文
//   - rspBytes: 响应原始报文
//
// 返回值:
//   - [][]byte: 爬取到的所有可能请求的原始报文列表
//   - error: 处理失败时返回错误
//
// Example:
// ```
// reqs, err = crawler.RequestsFromFlow(false, reqBytes, rspBytes)
// ```
func HandleRequestResult(isHttps bool, reqBytes, rspBytes []byte) ([][]byte, error) {
	var err error
	header, body := lowhttp.SplitHTTPPacketFast(rspBytes)
	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, isHttps)
	if err != nil {
		return nil, utils.Errorf("cannot extract url from request: %s", err)
	}
	rootReq := &Req{
		depth:          1,
		https:          isHttps,
		url:            urlIns.String(),
		requestRaw:     reqBytes,
		responseRaw:    rspBytes,
		responseBody:   body,
		responseHeader: header,
	}
	rootReq.request, err = lowhttp.ParseBytesToHttpRequest(reqBytes)
	if err != nil {
		return nil, utils.Errorf("parse bytes to http request failed: %s", err)
	}
	rootReq.response, err = lowhttp.ParseBytesToHTTPResponse(rspBytes)
	if err != nil {
		return nil, utils.Errorf("parse bytes to http.Response failed: %s", err)
	}

	rootReq.baseURL, err = lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, isHttps)
	if err != nil {
		return nil, utils.Errorf("recover url from request failed: %s", err)
	}
	//if utils.IContains(rootReq.request.Header.Get("Content-Type"), "javascript") {
	//	log.Debugf("start to extract javascript info.. from body size: %v", len(string(body)))
	//	rootReq.jsDocumentResult, err = javascript.BasicJavaScriptASTWalker(string(body))
	//	if err != nil {
	//		return nil, utils.Errorf("javascript ast analysis failed: %s", err)
	//	}
	//} else {
	//	rootReq.htmlDocument, err = goquery.NewDocumentFromReader(bytes.NewBuffer(body))
	//	if err != nil {
	//		return nil, utils.Errorf("create html document reader failed: %s", err)
	//	}
	//}

	var subReqs []*Req
	urlFilter := filter.NewCuckooFilter()
	handleReqResultEx(rootReq, func(nReq *Req) bool {
		subReqs = append(subReqs, nReq)
		return true
	}, func(s string) bool {
		if urlFilter.Exist(s) {
			return true
		}
		urlFilter.Insert(s)

		req, err := createReqFromUrlEx(rootReq, "GET", s, http.NoBody, nil)
		if err != nil {
			log.Errorf("create Req from url %v failed: %s", s, err)
			return true
		}
		subReqs = append(subReqs, req)
		return true
	}, nil)
	urlFilter.Close()

	var result [][]byte
	funk.ForEach(subReqs, func(i *Req) {
		if i.requestRaw != nil {
			result = append(result, i.requestRaw)
		}
	})
	return result, nil
}

func (r *Req) newHTTPRequest(responseRaw []byte, target string) (bool, []byte, error) {
	// lowhttp's historical URL join treats the complete request path as a
	// directory. Resolve here with net/url semantics so a response at
	// /deep/page maps "runtime.js" to /deep/runtime.js, while still preserving
	// NewHTTPRequest's packet/cookie construction behavior.
	if absolute := r.AbsoluteURL(target); absolute != "" {
		target = absolute
	}
	return NewHTTPRequest(r.IsHttps(), r.requestRaw, responseRaw, target)
}

func (c *Crawler) handleReqResult(r *Req) {
	if c.contextDone() {
		return
	}
	if r.err != nil {
		log.Errorf("request error: %s", r.err.Error())
		return
	}

	config := c.config
	if r.disallowedMITMType {
		return
	}

	submit := func(reqHttps bool, reqBytes []byte) {
		if c.contextDone() {
			return
		}
		req, err := c.createReqFromBytes(r, reqHttps, reqBytes)
		if err != nil {
			log.Errorf("create request from bytes error: %s", err.Error())
			return
		}
		ret, err := url.Parse(req.Url())
		if err != nil || ret == nil || ret.Scheme == "" || ret.Host == "" {
			return
		}
		if !c.reportDiscoveredURL(ret.String()) {
			return
		}
		if !config.CheckShouldBeHandledURL(ret) {
			return
		}
		c.submit(req)
	}

	for _, candidate := range responseHeaderAssetCandidates(r.response) {
		reqHTTPS, reqBytes, err := r.newHTTPRequest(r.responseRaw, candidate)
		if err != nil {
			log.Debugf("response header asset: build request failed for %q: %v", candidate, err)
			continue
		}
		submit(reqHTTPS, reqBytes)
	}

	var jsContents []*JavaScriptContent

	err := PageInformationWalker(
		lowhttp.GetHTTPPacketContentType([]byte(r.responseHeader)),
		string(r.responseBody),
		WithFetcher_JavaScript(func(content *JavaScriptContent) {
			// Adaptive AI analysis needs compiled/minified and oversized assets.
			// Preserve every asset here; legacy SSA-only behavior keeps its old
			// filters below.
			if config.enableAIJSExtract && config.aiJSExtractConfig != nil && config.aiJSExtractConfig.AdaptiveTrigger {
				jsContents = append(jsContents, content)
				return
			}
			// skip min.js
			if strings.HasSuffix(content.UrlPath, ".min.js") {
				return
			}
			if isPopularJSLibrary(content.UrlPath) {
				return
			}
			// skip max than 2MB js
			if len(content.Code) > twoMB {
				return
			}

			jsContents = append(jsContents, content)
		}),
		WithFetcher_HtmlTag(func(s string, node *html.Node) {
			if s == "script" {
				return
			}

			for _, attr := range node.Attr {
				switch strings.ToLower(attr.Key) {
				case "href", "src", "action":
					if attr.Val == "" {
						continue
					}
					reqHttps, reqBytes, err := r.newHTTPRequest(r.responseBody, attr.Val)
					if err != nil {
						log.Errorf("new request error: %s", err.Error())
						continue
					}
					submit(reqHttps, reqBytes)
				}
			}
		}),
	)
	if err != nil {
		log.Errorf("page information walker error: %s", err.Error())
	}

	adaptiveAI := config.enableAIJSExtract && config.aiJSExtractConfig != nil && config.aiJSExtractConfig.AdaptiveTrigger
	if adaptiveAI {
		// In adaptive mode, external scripts are first-class crawler requests.
		// Their responses therefore pass through onRequest, recursive discovery,
		// the shared request budget and the AI pipeline exactly once.
		for _, content := range jsContents {
			if content == nil || content.IsCodeText || strings.TrimSpace(content.UrlPath) == "" {
				continue
			}
			reqHTTPS, reqBytes, err := r.newHTTPRequest(r.responseRaw, content.UrlPath)
			if err != nil {
				log.Debugf("adaptive JavaScript asset: build request failed for %q: %v", content.UrlPath, err)
				continue
			}
			submit(reqHTTPS, reqBytes)
		}
	}

	// Legacy SSA and non-adaptive AI retain their historical direct downloader.
	// Adaptive responses are instead analyzed when their scheduled request is
	// handled, including when jsParser is enabled alongside adaptive AI.
	if (config.enableJSParser || config.enableAIJSExtract) && !adaptiveAI {
		c.fetchExternalJSCodes(r, jsContents)
	}

	// AI assisted JS / HTML path extraction. Runs independently of jsParser,
	// so users can opt-in to either or both. Each emitted path goes through
	// the same submit() pipeline so deduplication / domain filters apply.
	if config.enableAIJSExtract {
		// Adaptive mode keeps source boundaries and analyzes each response/asset
		// independently under one crawler-wide AI budget. Legacy callers retain
		// the previous concatenated behavior below.
		if config.aiJSExtractConfig.AdaptiveTrigger {
			extractCfg := *config.aiJSExtractConfig
			extractCfg.IsHTTPS = r.IsHttps()
			extractCfg.RequestRaw = r.requestRaw

			contentType := lowhttp.GetHTTPPacketContentType([]byte(r.responseHeader))
			assets := []AIJSAsset{{
				SourceURL:   r.Url(),
				ContentType: contentType,
				Body:        string(r.responseBody),
			}}
			for _, jsContent := range jsContents {
				// Inline code is already present in the owning HTML response. A direct
				// JavaScript response is likewise represented by r.responseBody.
				if !jsContent.IsCodeText || jsContent.Code == "" || jsContent.UrlPath == "" {
					continue
				}
				assets = append(assets, AIJSAsset{
					SourceURL:   r.AbsoluteURL(jsContent.UrlPath),
					ContentType: "application/javascript",
					Body:        jsContent.Code,
				})
			}

			err := RunAIJSExtractAssets(c.ctx, assets, &extractCfg, func(p string) {
				if c.contextDone() {
					return
				}
				httpsR, reqBytes, err := r.newHTTPRequest(r.responseRaw, p)
				if err != nil {
					log.Debugf("ai js extract: build http request failed for %q: %v", p, err)
					return
				}
				// The shared submit closure reports a valid candidate before it
				// enforces scope. Do not pre-filter here: an out-of-scope AI finding
				// must remain observable without ever entering the request queue.
				submit(httpsR, reqBytes)
			})
			if err != nil {
				log.Warnf("ai js extract: adaptive pipeline error: %v", err)
			}
		} else {
			var combined bytes.Buffer
			if len(r.responseBody) > 0 {
				combined.Write(r.responseBody)
				// Block-end markers must NOT start with "//" or look like a path,
				// otherwise both the regex pre-filter and the AI step will mis-read
				// them as protocol-relative URLs (regression: leaked as
				// "http://---html-end---/" downstream).
				combined.WriteString("\n/* yak-html-end */\n")
			}
			for _, j := range jsContents {
				if j.IsCodeText && j.Code != "" {
					combined.WriteString(j.Code)
					combined.WriteString("\n/* yak-js-end */\n")
				}
			}
			if combined.Len() > 0 {
				// Build a per-request shallow copy so that RequestRaw / IsHTTPS do
				// not leak across concurrent crawler requests sharing the shared
				// config.aiJSExtractConfig template.
				extractCfg := *config.aiJSExtractConfig
				extractCfg.IsHTTPS = r.IsHttps()
				extractCfg.RequestRaw = r.requestRaw

				extractCtx, extractCancel := context.WithTimeout(c.ctx, 5*time.Minute)
				err := RunAIJSExtract(extractCtx, combined.String(), &extractCfg, func(p string) {
					if c.contextDone() {
						return
					}
					httpsR, reqBytes, err := r.newHTTPRequest(r.responseBody, p)
					if err != nil {
						log.Debugf("ai js extract: build http request failed for %q: %v", p, err)
						return
					}
					submit(httpsR, reqBytes)
				})
				extractCancel()
				if err != nil {
					log.Warnf("ai js extract: pipeline error: %v", err)
				}
			}
		}
	}

	if !config.enableJSParser {
		return
	}

	var fullJSCode bytes.Buffer

	for _, i := range jsContents {
		if !i.IsCodeText {
			continue
		}
		// Keep the historical SSA input policy even when adaptive AI retained a
		// broader asset set above.
		if strings.HasSuffix(i.UrlPath, ".min.js") || isPopularJSLibrary(i.UrlPath) || len(i.Code) > twoMB {
			continue
		}
		fullJSCode.WriteString(i.Code)
		fullJSCode.WriteByte(';')
		fullJSCode.WriteByte('\n')
	}
	jsCtx, jsCancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer jsCancel()
	_ = utils.CallWithCtx(jsCtx, func() {
		HandleJSGetNewRequest(r.https, r.requestRaw, fullJSCode.String(), func(b bool, i []byte) {
			if c.contextDone() {
				return
			}
			submit(b, i)
		})
	})
}

// fetchExternalJSCodes pulls remote JS bodies referenced by jsContents (where
// IsCodeText is false) and stamps them back as code text. The function is
// idempotent: items that already carry inline code are skipped, so calling it
// from multiple gates costs nothing extra.
func (c *Crawler) fetchExternalJSCodes(r *Req, jsContents []*JavaScriptContent) {
	config := c.config
	jsConcurrent := config.concurrent / 2
	if jsConcurrent <= 0 {
		jsConcurrent = 3
	}
	workerLimiter := make(chan struct{}, jsConcurrent)
	var wg sync.WaitGroup
FETCH_LOOP:
	for _, content := range jsContents {
		if c.contextDone() {
			break
		}
		if content.IsCodeText {
			continue
		}
		select {
		case workerLimiter <- struct{}{}:
		case <-c.ctx.Done():
			break FETCH_LOOP
		}
		wg.Add(1)
		content := content
		go func() {
			defer func() {
				<-workerLimiter
				wg.Done()
			}()
			if c.contextDone() {
				return
			}

			reqHttps, reqBytes, err := r.newHTTPRequest(r.responseRaw, content.UrlPath)
			if err != nil {
				log.Errorf("build http request(js) failed: %s", content.UrlPath)
				return
			}
			urlIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, reqHttps)
			if urlIns == nil {
				return
			}
			// External JS <script src=...> is intentionally skipped by the
			// HtmlTag-based submit pipeline (see handleResponse), so its URL
			// would otherwise never reach onUrlFound. Discovery and fetching
			// are deliberately separate: report the asset, then enforce scope,
			// deduplication, and the crawler-wide request budget before I/O.
			if !c.reportDiscoveredURL(urlIns.String()) {
				return
			}
			if !config.CheckShouldBeHandledURL(urlIns) {
				log.Debugf("skip out-of-scope JavaScript asset: %v", urlIns)
				return
			}
			if !c.reserveDirectJSAssetRequest(urlIns) {
				log.Debugf("skip duplicate or over-budget JavaScript asset: %v", urlIns)
				return
			}
			log.Infof("Start to fetch JS(via URL): %v", urlIns.String())
			rsp, _, err := config.DoHTTPRequest(reqHttps, c.config.runtimeID, lowhttp.WithRequest(reqBytes))
			if err != nil {
				return
			}

			responseContentType := lowhttp.GetHTTPPacketContentType(rsp.RawPacket)
			assetPath := strings.ToLower(content.UrlPath)
			looksLikeJavaScriptPath := strings.HasSuffix(assetPath, ".js") ||
				strings.HasSuffix(assetPath, ".mjs") ||
				strings.HasSuffix(assetPath, ".cjs") ||
				strings.Contains(assetPath, ".js?") ||
				strings.Contains(assetPath, ".mjs?") ||
				strings.Contains(assetPath, ".cjs?")
			// The path-suffix fallback is intentionally adaptive-only. Historical
			// SSA and legacy AI callers required a JavaScript MIME type; widening
			// that policy for every jsParser caller could make an HTML error page
			// named *.js enter the legacy parser.
			adaptivePathFallback := config.enableAIJSExtract &&
				config.aiJSExtractConfig != nil &&
				config.aiJSExtractConfig.AdaptiveTrigger &&
				looksLikeJavaScriptPath
			if !utils.IContains(responseContentType, "javascript") && !adaptivePathFallback {
				return
			}

			_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
			content.Code = string(body)
			content.IsCodeText = true
		}()
	}
	wg.Wait()
}

// reserveDirectJSAssetRequest brings the historical direct <script src>
// downloader under the same crawler-run request cap as scheduled requests.
// LoadOrStore is also an in-flight reservation, so duplicate tags cannot race
// into multiple downloads.
func (c *Crawler) reserveDirectJSAssetRequest(assetURL *url.URL) bool {
	if c == nil || c.config == nil || assetURL == nil || c.requestedHash == nil {
		return false
	}
	hash := utils.CalcSha1(assetURL.String(), http.MethodGet)
	if _, loaded := c.requestedHash.LoadOrStore(hash, struct{}{}); loaded {
		return false
	}

	c.preRequestLock.Lock()
	defer c.preRequestLock.Unlock()
	limit := int64(c.config.maxCountOfRequest)
	if limit <= 0 || c.requestCounter >= limit {
		// The crawler-wide budget cannot grow later in the run. Keep the
		// reservation so repeated tags do not repeatedly retry a permanently
		// over-budget asset.
		return false
	}
	c.requestCounter++
	return true
}

func handleReqResultEx(r *Req, reqHandler func(*Req) bool, urlHandler func(string) bool, extractionRulesHandler func(*Req) []interface{}) {
	foundPathOrUrls := new(sync.Map)
	foundFormRequests := new(sync.Map)

	handleFinalExtraUrls := func(u string) {
		urlIns, err := url.Parse(u)
		if err != nil {
			return
		}
		pathRaw := urlIns.Path
		for {
			dirName := path.Dir(pathRaw)
			if dirName == "" || dirName == "/" || pathRaw == dirName {
				return
			}
			urlIns.RawQuery = ""
			pathRaw = dirName
			urlIns.Path = dirName
			foundPathOrUrls.Store(urlIns.String(), nil)
		}
	}
	_ = handleFinalExtraUrls
	if extractionRulesHandler != nil {

		urls := extractionRulesHandler(r)
		for _, iurl := range urls {
			url := utils.InterfaceToString(iurl)
			foundPathOrUrls.Store(url, nil)
		}
	} else {
		//if r.htmlDocument != nil {
		//	// meta redirect or ...
		//	r.htmlDocument.Find("meta").Each(func(_ int, selection *goquery.Selection) {
		//		t, _ := selection.Attr("content")
		//		for _, results := range metaUrlExtractor.FindAllStringSubmatch(t, -1) {
		//			if len(results) > 1 {
		//				rawUrl := strings.TrimRight(results[1], `"';`)
		//				var raw = r.AbsoluteURL(rawUrl)
		//				foundPathOrUrls.Store(raw, nil)
		//				handleFinalExtraUrls(raw)
		//			}
		//		}
		//	})
		//	r.htmlDocument.Find("[href]").Each(func(_ int, selection *goquery.Selection) {
		//		raw, _ := selection.Attr("href")
		//		raw = r.AbsoluteURL(raw)
		//		if raw != "" {
		//			foundPathOrUrls.Store(raw, nil)
		//			handleFinalExtraUrls(raw)
		//
		//		}
		//	})
		//	r.htmlDocument.Find("[src]").Each(func(i int, selection *goquery.Selection) {
		//		raw, _ := selection.Attr("src")
		//		raw = r.AbsoluteURL(raw)
		//		if raw != "" {
		//			foundPathOrUrls.Store(raw, nil)
		//			handleFinalExtraUrls(raw)
		//		}
		//	})
		//	r.htmlDocument.Find("form").Each(func(i int, selection *goquery.Selection) {
		//		var maybeUser, maybePass string
		//		method, reqUrl, contentType, body, err := HandleElementForm(
		//			selection, r.request.URL, func(user, pass string, extra map[string][]string) {
		//				maybeUser = user
		//				maybePass = pass
		//			},
		//		)
		//		if err != nil {
		//			log.Debugf("parse form error: %s", err)
		//			return
		//		}
		//
		//		fReq, err := createReqFromUrlEx(r, method, reqUrl, bytes.NewBufferString(body.String()), nil)
		//		if err != nil {
		//			log.Errorf("create Req from url (ex) failed: %s", err)
		//			return
		//		}
		//		fReq.isForm = true
		//		lowerBody := strings.ToLower(utils.InterfaceToString(body)) + strings.ToLower(reqUrl)
		//		fReq.maybeLoginForm = utils.MatchAnyOfSubString(
		//			lowerBody,
		//			"user", "name", "mail", "id", "xingming", "phone", "unique",
		//		) && utils.MatchAnyOfSubString(
		//			lowerBody,
		//			"pass", "word", "mima", "code", "secret", "key", "passwd", "pw", "pwd", "pd",
		//		)
		//		fReq.maybeUploadForm = utils.MatchAllOfRegexp(contentType, `application\/form-data`)
		//		fReq.request.Header.Set("Content-Type", contentType)
		//		fReq.depth = r.depth
		//		fReq.maybeLoginUsername = maybeUser
		//		fReq.maybeLoginPassword = maybePass
		//		foundFormRequests.Store(uuid.New().String(), fReq)
		//	})
		//}
		//
		//if r.jsDocumentResult != nil {
		//	for _, stringLiteral := range r.jsDocumentResult.StringLiteral {
		//		for _, url := range URLPattern.FindAllString(stringLiteral, -1) {
		//			url = r.AbsoluteURL(url)
		//			if url != "" {
		//				foundPathOrUrls.Store(url, nil)
		//				handleFinalExtraUrls(url)
		//			}
		//		}
		//	}
		//}
	}

	foundFormRequests.Range(func(key, value interface{}) bool {
		req, ok := value.(*Req)
		if !ok {
			return true
		}
		return reqHandler(req)
	})

	foundPathOrUrls.Range(func(key, value interface{}) bool {
		targetUrl := key.(string)
		return urlHandler(targetUrl)
	})
}

func (c *Crawler) preReq(r *Req) bool {
	config := c.config

	// 检查最大深度
	if r.depth > config.maxDepth {
		return false
	}

	// 添加头
	for _, h := range config.headers {
		r.request.Header.Set(h.Key, h.Value)
	}

	// 添加基础认证
	if c.config.BasicAuth {
		r.request.SetBasicAuth(config.AuthUsername, config.AuthPassword)
	}

	// 添加UA
	r.request.Header.Set("User-Agent", config.userAgent)

	// 设置 Cookie
	for _, cookie := range config.cookie {
		if !cookie.allowOverride {
			r.request.AddCookie(cookie.cookie)
		}
	}

	// 验证后缀
	ext := filepath.Ext(r.request.URL.Path)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if utils.StringSliceContain(config.disallowSuffix, ext) {
		return false
	}

	r.requestRaw, _ = utils.HttpDumpWithBody(r.request, true)
	return true
}

func (c *Crawler) contextDone() bool {
	if c == nil || c.ctx == nil {
		return false
	}
	select {
	case <-c.ctx.Done():
		return true
	default:
		return false
	}
}

func (c *Crawler) initScheduler() {
	if c == nil {
		return
	}

	size := c.config.concurrent * 3
	if c.config.maxCountOfRequest > size {
		size = c.config.maxCountOfRequest
	}
	if c.config.maxCountOfLinks <= 0 {
		// maxUrls explicitly documents non-positive values as unlimited. Do not
		// reintroduce an implicit queue cap after a discovery has already been
		// reported and reserved.
		size = 0
	} else {
		// Every admitted discovery may be submitted while a worker is still
		// processing the owning page. Keep enough room for the complete bounded
		// discovery set plus seeds, otherwise a transient full queue permanently
		// loses a URL that reportDiscoveredURL has already reserved.
		discoveryCapacity := c.config.maxCountOfLinks
		maxInt := int(^uint(0) >> 1)
		if len(c.originUrls) <= maxInt-discoveryCapacity {
			discoveryCapacity += len(c.originUrls)
		} else {
			discoveryCapacity = maxInt
		}
		if discoveryCapacity > size {
			size = discoveryCapacity
		}
	}
	c.scheduler = newRequestScheduler(c.ctx, size)
}

func (c *Crawler) submit(r *Req) bool {
	if c == nil || c.scheduler == nil {
		return false
	}
	if r == nil || r.request == nil || r.request.URL == nil {
		return false
	}
	if isHighPriorityAssetURL(r.request.URL) {
		r.priority = true
	}
	if _, loaded := c.foundUrls.LoadOrStore(r.Hash(), nil); loaded {
		return false
	}
	if !c.scheduler.Submit(r) {
		c.foundUrls.Delete(r.Hash())
		return false
	}
	return true
}

// reportDiscoveredURL is the crawler-run admission gate for newly discovered
// URLs. Seed URLs are free, while every other unique candidate consumes one
// maxUrls slot before callbacks, scope checks, or scheduling. Keeping the
// decision before the scope check is intentional: an out-of-scope candidate is
// still part of the discovered surface and must not provide an unbounded side
// channel around maxUrls.
func (c *Crawler) reportDiscoveredURL(rawURL string) bool {
	if c == nil || c.config == nil {
		return false
	}
	rawURL = stripURLFragment(strings.TrimSpace(rawURL))
	if rawURL == "" {
		return false
	}

	c.discoveryMu.Lock()
	accepted := func() bool {
		defer c.discoveryMu.Unlock()

		// originUrls is immutable after construction. Check it directly rather
		// than relying only on foundUrls, because seed submission happens in a
		// goroutine and discovery can otherwise race ahead of a later seed.
		for _, seedURL := range c.originUrls {
			if rawURL == seedURL {
				return false
			}
		}
		// Seed URLs and already scheduled requests are not newly discovered. This
		// also prevents a fragment-only link from reporting the current document.
		if c.foundUrls != nil {
			if _, scheduled := c.foundUrls.Load(utils.CalcSha1(rawURL, http.MethodGet)); scheduled {
				return false
			}
		}
		if c.reportedUrls == nil {
			c.reportedUrls = new(sync.Map)
		}
		if _, loaded := c.reportedUrls.Load(rawURL); loaded {
			return false
		}
		if c.config.maxCountOfLinks > 0 && c.linkCounter >= int64(c.config.maxCountOfLinks) {
			return false
		}
		if _, loaded := c.reportedUrls.LoadOrStore(rawURL, struct{}{}); loaded {
			return false
		}
		c.linkCounter++
		return true
	}()
	if !accepted {
		return false
	}

	// User callbacks stay outside the admission lock so a slow observer cannot
	// block unrelated crawler workers. The URL is already durably reserved.
	if c.config.onUrlFound != nil {
		c.config.onUrlFound(rawURL)
	}
	return true
}

func (c *Crawler) createReqFromUrl(preRequest *Req, u string) (*Req, error) {
	return createReqFromUrlEx(preRequest, "GET", u, http.NoBody, c)
}

func (c *Crawler) createReqFromBytes(preRequest *Req, https bool, req []byte) (*Req, error) {
	reqIns, err := utils.ReadHTTPRequestFromBytes(req)
	if err != nil {
		return nil, err
	}
	urlIns, err := lowhttp.ExtractURLFromHTTPRequestRaw(req, https)
	if err != nil {
		return nil, err
	}
	urlIns.Fragment = ""
	urlIns.RawFragment = ""
	reqIns.URL = urlIns
	reqIns.RequestURI = ""
	req, err = utils.HttpDumpWithBody(reqIns, true)
	if err != nil {
		return nil, err
	}
	baseURL := *urlIns
	return &Req{
		depth:      preRequest.depth + 1,
		https:      https,
		url:        urlIns.String(),
		request:    reqIns,
		requestRaw: req,
		baseURL:    &baseURL,
	}, nil
}

func createReqFromUrlEx(preqRequest *Req, method, u string, body io.Reader, c *Crawler) (*Req, error) {
	u = stripURLFragment(u)
	r, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, utils.Errorf("create request from url[%v] failed: %s", u, err)
	}

	// 设置 Request Cookie
	// 继承 Cookie
	if preqRequest != nil && preqRequest.request != nil {
		for _, cookie := range preqRequest.request.Cookies() {
			r.AddCookie(cookie)
		}
	}

	// 设置上一个请求产生的 Set-Cookie
	if preqRequest != nil && preqRequest.response != nil {
		for _, cookie := range preqRequest.response.Cookies() {
			r.AddCookie(cookie)
		}
	}

	if c != nil {
		for _, ck := range c.config.cookie {
			r.AddCookie(ck.cookie)
		}
	}

	reqBytes, _ := utils.HttpDumpWithBody(r, true)
	depth := 0
	if preqRequest != nil {
		depth = preqRequest.depth + 1
	}
	baseURL := *r.URL
	return &Req{
		depth:      depth,
		https:      strings.EqualFold(r.URL.Scheme, "https"),
		url:        r.URL.String(),
		request:    r,
		requestRaw: reqBytes,
		baseURL:    &baseURL,
	}, nil
}

func isHighPriorityAssetURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	lowerPath := strings.ToLower(u.Path)
	for _, suffix := range []string{".js", ".mjs", ".cjs", ".map", ".wasm"} {
		if strings.HasSuffix(lowerPath, suffix) {
			return true
		}
	}
	base := strings.ToLower(path.Base(lowerPath))
	return strings.Contains(lowerPath, "/.well-known/") ||
		strings.Contains(base, "chunk") ||
		strings.Contains(base, "runtime-config") ||
		strings.Contains(base, "asset-manifest") ||
		strings.Contains(base, "routes") ||
		strings.Contains(base, "service-worker") ||
		strings.Contains(base, "openapi") ||
		strings.Contains(base, "swagger")
}

const maxResponseHeaderAssetCandidates = 64

func responseHeaderAssetCandidates(response *http.Response) []string {
	if response == nil || response.Header == nil {
		return nil
	}
	seen := make(map[string]struct{})
	result := make([]string, 0, 8)
	appendCandidate := func(candidate string) {
		if len(result) >= maxResponseHeaderAssetCandidates {
			return
		}
		candidate = strings.Trim(strings.TrimSpace(candidate), `"'`)
		if candidate == "" || len(candidate) > 16*1024 || strings.ContainsAny(candidate, "\x00\r\n") {
			return
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed == nil || parsed.Fragment != "" && parsed.Path == "" && parsed.Host == "" {
			return
		}
		if parsed.IsAbs() && !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}

	for _, name := range []string{"SourceMap", "X-SourceMap"} {
		for _, value := range response.Header.Values(name) {
			appendCandidate(value)
		}
	}
	for _, value := range response.Header.Values("Link") {
		for _, part := range splitHTTPLinkHeader(value) {
			start := strings.IndexByte(part, '<')
			if start < 0 {
				continue
			}
			endOffset := strings.IndexByte(part[start+1:], '>')
			if endOffset < 0 {
				continue
			}
			appendCandidate(part[start+1 : start+1+endOffset])
		}
	}
	return result
}

func splitHTTPLinkHeader(value string) []string {
	var result []string
	start := 0
	inAngle := false
	quote := byte(0)
	for index := 0; index < len(value); index++ {
		current := value[index]
		if quote != 0 {
			if current == '\\' {
				index++
				continue
			}
			if current == quote {
				quote = 0
			}
			continue
		}
		switch current {
		case '\'', '"':
			quote = current
		case '<':
			inAngle = true
		case '>':
			inAngle = false
		case ',':
			if !inAngle {
				result = append(result, value[start:index])
				start = index + 1
			}
		}
	}
	result = append(result, value[start:])
	return result
}

func (c *Crawler) adoptFinalRequestProvenance(r *Req, response *lowhttp.LowhttpResponse) error {
	if r == nil || response == nil || len(response.RawRequest) == 0 {
		return utils.Errorf("crawler response is missing final request provenance")
	}

	finalRequest, err := utils.ReadHTTPRequestFromBytes(response.RawRequest)
	if err != nil {
		return utils.Errorf("parse crawler final request failed: %v", err)
	}

	var finalURL *url.URL
	if rawURL := strings.TrimSpace(response.Url); rawURL != "" {
		parsed, parseErr := url.Parse(rawURL)
		if parseErr == nil && parsed.Host != "" {
			finalURL = parsed
		}
	}
	if finalURL == nil {
		finalURL, err = lowhttp.ExtractURLFromHTTPRequestRaw(response.RawRequest, response.Https)
		if err != nil || finalURL == nil {
			return utils.Errorf("extract crawler final request URL failed: %v", err)
		}
	}
	if response.Https {
		finalURL.Scheme = "https"
	} else {
		finalURL.Scheme = "http"
	}
	finalURL.Fragment = ""
	finalURL.RawFragment = ""

	finalRequest.URL = finalURL
	finalRequest.RequestURI = ""
	finalBaseURL := *finalURL
	r.requestRaw = append(r.requestRaw[:0], response.RawRequest...)
	r.request = finalRequest
	r.url = finalURL.String()
	r.https = response.Https
	r.baseURL = &finalBaseURL

	// Keep both the originally submitted identity and the final effective
	// identity. A queued request for a redirect/fallback target can then be
	// rejected before another network call, while the original seed remains
	// deduplicated as well.
	finalHash := r.Hash()
	if c != nil && c.requestedHash != nil {
		c.requestedHash.Store(finalHash, nil)
	}
	if c != nil && c.foundUrls != nil {
		c.foundUrls.Store(finalHash, nil)
	}
	return nil
}

func (c *Crawler) execReq(r *Req) {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
		}
	}()
	if r.request == nil {
		return
	}
	if c.contextDone() {
		r.err = c.ctx.Err()
		return
	}

	if c.config.onLogin != nil && r.IsLoginForm() && r.IsForm() {
		c.loginOnce.Do(func() {
			if c.contextDone() {
				return
			}
			c.config.onLogin(r)
		})
	}

	lowRspIns, _, err := c.config.DoHTTPRequest(r.IsHttps(), c.config.runtimeID, lowhttp.WithPacketBytes(r.requestRaw))
	if err != nil {
		r.err = err
		return
	}
	if err := c.adoptFinalRequestProvenance(r, lowRspIns); err != nil {
		r.err = err
		return
	}
	rsp, err := utils.ReadHTTPResponseFromBytes(lowRspIns.RawPacket, r.request)
	if err != nil {
		r.err = err
		return
	}
	r.response = rsp
	r.responseRaw = lowRspIns.RawPacket
	r.responseHeader, r.responseBody = lowhttp.SplitHTTPPacketFast(lowRspIns.RawPacket)
	// 获取 MIME 类型
	mimeType, _, _ := mime.ParseMediaType(rsp.Header.Get("Content-Type"))
	if mimeType != "" {
		log.Debugf("checking url: %s for response mime type: %s", r.Url(), mimeType)
		if utils.MatchAnyOfGlob(mimeType, c.config.disallowMIMEType...) {
			r.disallowedMITMType = true
		}
	}
	if c.config.onRequest != nil {
		c.config.onRequest(r)
	}
}
