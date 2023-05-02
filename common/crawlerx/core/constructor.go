package core

import (
	"context"
	"crypto/tls"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/crawlerx/detect"
	"github.com/yaklang/yaklang/common/crawlerx/filter"
	"github.com/yaklang/yaklang/common/crawlerx/tag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type CrawlerX struct {
	targetUrl string
	urlDomain string

	browser  *rod.Browser
	pagePool rod.PagePool

	rootContext context.Context
	cancelFunc  context.CancelFunc

	concurrent         int
	pageSizedWaitGroup utils.SizedWaitGroup

	sent       *filter.StringCountFilter
	visited    *filter.StringCountFilter
	htmlRecord *filter.StringCountFilter

	urlCount int
	maxDepth int
	timeout  int

	sendInfoChannel chan ReqInfo
	onRequest       func(ReqInfo)

	config *Config

	proxy         string
	proxyUsername string
	proxyPassword string

	cookies []*proto.NetworkCookieParam
	headers []*Header

	rangeLevel      int
	checkRangeValid func(string) bool
	repeatLevel     int
	checkRepeat     func(string, string) string
	checkDanger     func(string) bool

	blackList []*regexp.Regexp
	whiteList []*regexp.Regexp

	formFill map[string]string

	tagDetect *tag.TDetect

	chromeWS string
}

func NewCrawler(targetUrl string, configOpts ...ConfigOpt) (*CrawlerX, error) {
	config := &Config{
		timeout:            30,
		concurrent:         20,
		maxDepth:           1,
		scanRange:          detect.AllDomain,
		scanRepeat:         detect.UnLimit,
		fullCrawlerTimeout: 360,
		formFill:           make(map[string]string, 0),
		urlFromProxy:       false,
	}

	for _, configOpt := range configOpts {
		configOpt(config)
	}

	if !strings.HasPrefix(targetUrl, "http") {
		targetUrl = "http://" + targetUrl
	}
	var cText context.Context
	var cancel func()
	if config.fullCrawlerTimeout == 0 {
		cText, cancel = context.WithCancel(context.Background())
	} else {
		cText, cancel = context.WithTimeout(context.Background(), time.Duration(config.fullCrawlerTimeout)*time.Second)
	}
	//context, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	crawlerX := &CrawlerX{
		targetUrl: targetUrl,

		browser:  rod.New(),
		pagePool: rod.NewPagePool(50),

		rootContext: cText,
		cancelFunc:  cancel,

		concurrent: config.concurrent,

		sent:       filter.NewCountFilter(),
		visited:    filter.NewCountFilter(),
		htmlRecord: filter.NewCountFilter(),

		urlCount: config.maxUrlCount,
		maxDepth: config.maxDepth,
		timeout:  config.timeout,

		headers: config.headers,
		cookies: config.cookies,

		rangeLevel:  config.scanRange,
		repeatLevel: config.scanRepeat,

		blackList: config.blackList,
		whiteList: config.whiteList,

		formFill: DefaultFormFill,

		config: config,

		chromeWS: config.chromeWS,
	}
	for k, v := range config.formFill {
		crawlerX.formFill[k] = v
	}
	if config.channel != nil {
		crawlerX.sendInfoChannel = config.channel
	}
	if config.onRequest != nil {
		crawlerX.onRequest = config.onRequest
	}
	if config.checkDanger != nil {
		crawlerX.checkDanger = config.checkDanger
	}
	if config.tags != "" {
		crawlerX.tagDetect = new(tag.TDetect)
		crawlerX.tagDetect.SetRulePath(config.tags)
		crawlerX.tagDetect.Init()
	}
	if config.proxy != "" {
		crawlerX.proxy = config.proxy
		if config.proxyUsername != "" {
			crawlerX.proxyUsername = config.proxyUsername
			crawlerX.proxyPassword = config.proxyPassword
		}
	}

	err := crawlerX.init()
	if err != nil {
		return nil, utils.Errorf("crawler module initial error: %s", err)
	}
	return crawlerX, nil
}

func (crawler *CrawlerX) init() error {
	// browser
	if crawler.proxy != "" {
		//l := launcher.MustNewManaged("ws://192.168.0.115:7317")
		//l.Proxy(crawler.proxy).Headless(true).NoSandbox(true)
		//crawler.browser = rod.New().Client(l.MustClient())
		log.Info("proxy!")
		launch := launcher.New().Set(flags.ProxyServer, crawler.proxy)
		controlUrl, err := launch.Launch()
		if err != nil {
			return utils.Errorf("proxy %s launch error: %s", crawler.proxy, err)
		}
		crawler.browser = crawler.browser.ControlURL(controlUrl)
	} else if crawler.chromeWS != "" {
		log.Info("ws start!")
		crawler.browser = crawler.browser.ControlURL(crawler.chromeWS)
	}
	crawler.browser = crawler.browser.Context(crawler.rootContext)
	err := crawler.browser.Connect()
	if err != nil {
		return utils.Errorf("browser connect error: %s", err)
	}
	if crawler.proxyUsername != "" {
		go crawler.browser.MustHandleAuth(crawler.proxyUsername, crawler.proxyPassword)()
	}
	crawler.browser.IgnoreCertErrors(true)
	if len(crawler.cookies) > 0 {
		crawler.setCookies()
	}
	// others
	crawler.setMainDomain()
	crawler.checkRangeValid = detect.GetValidRangeFunc(crawler.targetUrl, crawler.rangeLevel)
	crawler.checkRepeat = detect.GetURLRepeatCheck(crawler.repeatLevel)
	if !crawler.config.urlFromProxy {
		crawler.createHijack()
	} else {
		close(crawler.config.channel)
		crawler.sendInfoChannel = nil
		if crawler.proxy == "" {
			return utils.Errorf("no proxy to receive netflow data.")
		}
	}
	crawler.pageSizedWaitGroup = utils.NewSizedWaitGroup(crawler.concurrent)
	return nil
}

func (crawler *CrawlerX) createHijack() error {
	hijackRouter := crawler.browser.HijackRequests()
	hijackRouter.MustAdd("*", func(hijack *rod.Hijack) {
		urlRaw := hijack.Request.URL()
		urlStr := urlRaw.String()
		for _, header := range crawler.headers {
			if strings.ToLower(header.Key) == "host" {
				hijack.Request.Req().Host = header.Value
			} else {
				hijack.Request.Req().Header.Add(header.Key, header.Value)
			}
		}

		client := http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		transport := http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		if crawler.proxy != "" {
			proxyUrl, err := url.Parse(crawler.proxy)
			if err != nil {
				return
			}
			//transport := http.Transport{
			//	Proxy: http.ProxyURL(proxyUrl),
			//}
			//client.Transport = &transport
			transport.Proxy = http.ProxyURL(proxyUrl)
		}
		client.Transport = &transport

		err := hijack.LoadResponse(&client, true)
		if err != nil {
			if !strings.Contains(err.Error(), "context canceled") {
				//log.Errorf("load response error: %s", err)
			}
			hijack.Response.SetHeader()
			hijack.Response.SetBody("")
			return
		}

		go func() {
			crawler.pageSizedWaitGroup.AddWithContext(crawler.rootContext)
			defer crawler.pageSizedWaitGroup.Done()
			req := hijack.Request.Req()
			reqHeaders := hijack.Request.Headers()
			reqBody := hijack.Request.Body()
			reqMethod := hijack.Request.Method()
			resHeaders := hijack.Response.Headers()
			resBody := hijack.Response.Body()

			if crawler.checkRangeValid != nil {
				if !crawler.checkRangeValid(urlStr) {
					//hijack.Response.SetHeader()
					//hijack.Response.SetBody("")
					return
				}
			}
			var checkUrl string
			if crawler.checkRepeat != nil {
				checkUrl = crawler.checkRepeat(urlStr, reqMethod)
			} else {
				checkUrl = urlStr
			}
			hashUrl := codec.Sha256(checkUrl)
			if crawler.sent.Exist(hashUrl) {
				return
			}
			crawler.sent.Insert(hashUrl)

			r := &RequestInfo{}
			r.url = urlStr
			r.requestHeaders = &reqHeaders
			r.requestBody = reqBody
			r.requestMethod = reqMethod
			r.responseHeaders = &resHeaders
			r.responseBody = resBody
			r.req = req

			if strings.HasSuffix(urlStr, ".js") {
				crawler.JsInfoMatch(urlStr, resBody)
			}

			if crawler.tagDetect != nil {
				tagStr := crawler.tagDetect.GetTag(r)
				r.SetTag(tagStr)
			}

			if crawler.onRequest != nil {
				crawler.onRequest(r)
			} else if crawler.sendInfoChannel != nil {
				crawler.sendInfoChannel <- r
			} else {
				log.Infof("request: %s with", r.Url())
			}

			if crawler.urlCount != 0 && crawler.sent.Count() >= int64(crawler.urlCount) {
				crawler.cancelFunc()
			}

			subUrls := crawler.SubmitCutUrl(urlStr)
			crawler.SimpleCheckSend(subUrls...)
		}()
	})

	go func() {
		hijackRouter.Run()
	}()

	return nil
}

func (crawler *CrawlerX) setCookies() error {
	return crawler.browser.SetCookies(crawler.cookies)
}

func (crawler *CrawlerX) setMainDomain() {
	r, _ := regexp.Compile("http(s??)://.+?/")
	mainDomains := r.FindAllString(crawler.targetUrl, -1)
	if len(mainDomains) > 0 {
		crawler.urlDomain = mainDomains[0]
	}
}

func (crawler *CrawlerX) SetChannel(ch chan ReqInfo) {
	crawler.sendInfoChannel = ch
}

func (crawler *CrawlerX) GetChannel() chan ReqInfo {
	return crawler.sendInfoChannel
}

func (crawler *CrawlerX) PageSizedGroup() *utils.SizedWaitGroup {
	return &crawler.pageSizedWaitGroup
}

func (crawler *CrawlerX) RootContext() context.Context {
	return crawler.rootContext
}
