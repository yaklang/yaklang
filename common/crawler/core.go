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

	requestCounter         int64
	linkCounter            int64
	handlingRequestCounter int

	requestedHash *sync.Map
	foundUrls     *sync.Map
	scheduler     *requestScheduler

	ctx    context.Context
	cancel context.CancelFunc

	// login
	loginOnce *sync.Once // := new(sync.Once)
}

type requestScheduler struct {
	ctx context.Context
	q   *chanx.UnlimitedChan[*Req]

	pending     atomic.Int64
	startupDone atomic.Bool
	closed      atomic.Bool
	closeOnce   sync.Once
}

func newRequestScheduler(ctx context.Context, queueSize int) *requestScheduler {
	if ctx == nil {
		ctx = context.Background()
	}
	if queueSize <= 0 {
		queueSize = 10
	}
	return &requestScheduler{
		ctx: ctx,
		q:   chanx.NewUnlimitedChan[*Req](ctx, queueSize),
	}
}

func (s *requestScheduler) Output() <-chan *Req {
	if s == nil || s.q == nil {
		ch := make(chan *Req)
		close(ch)
		return ch
	}
	return s.q.OutputChannel()
}

func (s *requestScheduler) Submit(req *Req) (ok bool) {
	if s == nil || s.q == nil || req == nil || s.contextDone() || s.closed.Load() {
		return false
	}
	s.pending.Add(1)
	defer func() {
		if err := recover(); err != nil {
			s.pending.Add(-1)
			s.maybeClose()
			ok = false
		}
	}()
	if !s.q.SafeFeedWithResult(req) {
		s.pending.Add(-1)
		s.maybeClose()
		return false
	}
	return true
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
	if s == nil || s.q == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.q.Close()
	})
}

func (s *requestScheduler) maybeClose() {
	if s == nil {
		return
	}
	if s.startupDone.Load() && s.pending.Load() == 0 {
		s.Close()
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
	urlsRaw := utils.PrettifyListFromStringSplited(urls, ",")
	urlList := utils.ParseStringToUrlsWith3W(urlsRaw...)
	log.Debugf("actual url list: %v", urlList)

	config := &Config{}
	config.init()

	// 把自己的域名加在里面
	for _, u := range urlList {
		urlIns, err := url.Parse(u)
		if err != nil {
			continue
		}
		WithDomainWhiteList(urlIns.Hostname())(config)
	}

	for _, opt := range opts {
		opt(config)
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
	reqOutput := c.scheduler.Output()

	for {
		select {
		case <-c.ctx.Done():
			c.scheduler.Close()
			workerWG.Wait()
			return
		case r, ok := <-reqOutput:
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
			c.handlingRequestCounter++
			c.preRequestLock.Unlock()

			// 请求最大值限制
			// 判断请求最大值限制
			if c.requestCounter > int64(config.maxCountOfRequest) {
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
				c.handlingRequestCounter--
				c.afterRequestLock.Unlock()
			}(r)
		}
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
		if config.onUrlFound != nil {
			config.onUrlFound(req.Url())
		}
		if ret, err := url.Parse(req.Url()); err != nil {
			if !config.CheckShouldBeHandledURL(ret) {
				return
			}
		}
		c.submit(req)
	}

	var jsContents []*JavaScriptContent

	err := PageInformationWalker(
		lowhttp.GetHTTPPacketContentType([]byte(r.responseHeader)),
		string(r.responseBody),
		WithFetcher_JavaScript(func(content *JavaScriptContent) {
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
					reqHttps, reqBytes, err := NewHTTPRequest(r.IsHttps(), r.requestRaw, r.responseBody, attr.Val)
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

	// External JS contents are needed by both the SSA path (enableJSParser)
	// and the AI extract path (enableAIJSExtract). Fetch once if either toggle
	// is on; the helper is idempotent over content.IsCodeText.
	if config.enableJSParser || config.enableAIJSExtract {
		c.fetchExternalJSCodes(r, jsContents)
	}

	// AI assisted JS / HTML path extraction. Runs independently of jsParser,
	// so users can opt-in to either or both. Each emitted path goes through
	// the same submit() pipeline so deduplication / domain filters apply.
	if config.enableAIJSExtract {
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
				httpsR, reqBytes, err := NewHTTPRequest(r.IsHttps(), r.requestRaw, r.responseBody, p)
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

	if !config.enableJSParser {
		return
	}

	var fullJSCode bytes.Buffer

	for _, i := range jsContents {
		if !i.IsCodeText {
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

			reqHttps, reqBytes, err := NewHTTPRequest(r.IsHttps(), r.requestRaw, r.responseRaw, content.UrlPath)
			if err != nil {
				log.Errorf("build http request(js) failed: %s", content.UrlPath)
				return
			}
			urlIns, _ := lowhttp.ExtractURLFromHTTPRequestRaw(reqBytes, reqHttps)
			if urlIns != nil {
				log.Infof("Start to fetch JS(via URL): %v", urlIns.String())
				// External JS <script src=...> is intentionally skipped by the
				// HtmlTag-based submit pipeline (see handleResponse), so its URL
				// would otherwise never reach onUrlFound. Report it here to keep
				// the discovery channel complete.
				if config.onUrlFound != nil {
					config.onUrlFound(urlIns.String())
				}
			}
			rsp, _, err := config.DoHTTPRequest(reqHttps, c.config.runtimeID, lowhttp.WithRequest(reqBytes))
			if err != nil {
				return
			}

			if !utils.IContains(lowhttp.GetHTTPPacketContentType(rsp.RawPacket), "javascript") {
				return
			}

			_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
			content.Code = string(body)
			content.IsCodeText = true
		}()
	}
	wg.Wait()
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
	c.scheduler = newRequestScheduler(c.ctx, size)
}

func (c *Crawler) submit(r *Req) bool {
	if c == nil || c.scheduler == nil {
		return false
	}
	return c.scheduler.Submit(r)
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
	reqIns.URL = urlIns
	return &Req{
		depth:      preRequest.depth + 1,
		https:      https,
		url:        urlIns.String(),
		request:    reqIns,
		requestRaw: req,
	}, nil
}

func createReqFromUrlEx(preqRequest *Req, method, u string, body io.Reader, c *Crawler) (*Req, error) {
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
	return &Req{
		depth:      depth,
		request:    r,
		requestRaw: reqBytes,
	}, nil
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

	lowRspIns, usedHTTPS, err := c.config.DoHTTPRequest(r.IsHttps(), c.config.runtimeID, lowhttp.WithPacketBytes(r.requestRaw))
	if err != nil {
		r.err = err
		return
	}
	if usedHTTPS != r.IsHttps() {
		if r.request != nil && r.request.URL != nil {
			if usedHTTPS {
				r.request.URL.Scheme = "https"
			} else {
				r.request.URL.Scheme = "http"
			}
		}
		if r.baseURL != nil {
			if usedHTTPS {
				r.baseURL.Scheme = "https"
			} else {
				r.baseURL.Scheme = "http"
			}
		}
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
