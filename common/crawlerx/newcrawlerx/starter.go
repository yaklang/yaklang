// Package newcrawlerx
// @Author bcy2007  2023/3/7 11:51
package newcrawlerx

import (
	"context"
	"crypto/tls"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"net/http"
	"strings"
	"time"
)

type BrowserStarter struct {
	browser       *rod.Browser
	baseUrl       string
	browserConfig *NewBrowserConfig
	baseConfig    *BaseConfig

	ctx    context.Context
	cancel context.CancelFunc

	mainWaitGroup *utils.SizedWaitGroup
	subWaitGroup  *utils.SizedWaitGroup

	pageVisit      *filter.StringCountFilter
	resultSent     *filter.StringCountFilter
	resultSentFunc func(string) bool

	checkFunctions     []func(string) bool
	checkFunctionMap   map[string]func(string) bool
	repeatLevel        limitLevel
	requestAfterRepeat func(*rod.HijackRequest) string
	urlAfterRepeat     func(string) string
	scanLevel          scanRangeLevel
	noParams           []string
	extraFunctions     []func(string) bool
	elementCheck       func(*rod.Element) bool
	scanRangeCheck     func(string) bool

	urlTree *UrlTree

	ch chan ReqInfo

	uChan *UChan

	formFill  map[string]string
	inputFile map[string]string

	concurrent int
	maxUrl     int
	maxDepth   int
	stopSignal bool

	getUrlFunction func(*rod.Page) error
	clickFunction  func(*rod.Page) error
	inputFunction  func(*rod.Page) error
	vueFunction    func(*rod.Page) error
	pageActions    []func(*rod.Page) error

	getUrlsFunction      func(*rod.Page) ([]string, error)
	doUrlsFunction       func(string, string) error
	getClickFunction     func(*rod.Page) ([]string, error)
	doClickFunction      func(*rod.Page, string, string) error
	getInputFunction     func(*rod.Page) (rod.Elements, error)
	doInputFunction      func(*rod.Element) error
	getEventFunction     func(*rod.Page) ([]string, error)
	doEventClickFunction func(*rod.Page, string, string) error
	doActionOnPage       func(*rod.Page) error

	client    http.Client
	transport http.Transport

	extraWaitLoad int
	checkHTML     bool

	// new counter
	c       *Counter
	running bool
}

func NewBrowserStarter(browserConfig *NewBrowserConfig, baseConfig *BaseConfig) *BrowserStarter {
	starter := BrowserStarter{
		baseUrl:       baseConfig.targetUrl,
		browserConfig: browserConfig,
		baseConfig:    baseConfig,

		mainWaitGroup: baseConfig.pageSizedWaitGroup,

		pageVisit:      baseConfig.pageVisit,
		resultSent:     baseConfig.resultSent,
		resultSentFunc: repeatCheckFunctionGenerate(baseConfig.resultSent),

		checkFunctionMap: make(map[string]func(string) bool),
		repeatLevel:      baseConfig.scanRepeat,
		scanLevel:        baseConfig.scanRange,
		noParams:         baseConfig.ignoreParams,
		//noParams:    []string{"cat"},

		urlTree: baseConfig.urlTree,

		ch: baseConfig.ch,

		uChan: baseConfig.uChan,

		formFill:  make(map[string]string),
		inputFile: make(map[string]string),

		concurrent: baseConfig.concurrent,
		maxUrl:     baseConfig.maxUrlCount,
		maxDepth:   baseConfig.maxDepth,
		stopSignal: false,

		pageActions: make([]func(*rod.Page) error, 0),

		extraWaitLoad: baseConfig.extraWaitLoadTime,
		checkHTML:     true,

		c:       NewCounter(baseConfig.concurrent),
		running: false,
	}
	ctx, cancel := context.WithCancel(baseConfig.ctx)
	starter.ctx = ctx
	starter.cancel = cancel
	subGroup := utils.NewSizedWaitGroup(20)
	starter.subWaitGroup = &subGroup
	for k, v := range baseConfig.formFill {
		starter.formFill[k] = v
	}
	starter.inputFile["default"] = "/Users/chenyangbao/1.txt"
	for k, v := range baseConfig.fileInput {
		starter.inputFile[k] = v
	}
	starter.requestAfterRepeat = repeatCheckGenerator(starter.repeatLevel, starter.noParams...)
	starter.urlAfterRepeat = urlRepeatCheckGenerator(starter.repeatLevel, starter.noParams...)
	starter.transport = http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if starter.browserConfig.proxyAddress != nil {
		starter.transport.Proxy = http.ProxyURL(starter.browserConfig.proxyAddress)
	}
	starter.client = http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &starter.transport,
	}
	return &starter
}

func (starter *BrowserStarter) StartBrowser() error {
	starter.browser = rod.New()
	if starter.browserConfig.wsAddress != "" {
		launch, err := launcher.NewManaged(starter.browserConfig.wsAddress)
		if err != nil {
			return utils.Errorf("new launcher %s managed error: %s", starter.browserConfig.wsAddress, err)
		}
		launchCtx := context.Background()
		launch = launch.Context(launchCtx)
		if starter.browserConfig.proxyAddress != nil {
			launch = launch.Proxy(starter.browserConfig.proxyAddress.String())
		}
		launch = launch.NoSandbox(true).Headless(true)
		serviceUrl, header := launch.ClientHeader()
		client, err := cdp.StartWithURL(launchCtx, serviceUrl, header)
		starter.browser = starter.browser.Client(client)
	} else {
		launch := launcher.New()
		if starter.browserConfig.exePath != "" {
			launch = launch.Bin(starter.browserConfig.exePath)
		}
		if starter.browserConfig.proxyAddress != nil {
			launch = launch.Set(flags.ProxyServer, starter.browserConfig.proxyAddress.String())
		}
		launch = launch.NoSandbox(true).Headless(true)
		controlUrl, err := launch.Launch()
		if err != nil {
			return utils.Errorf("new launcher launch error: %s", err)
		}
		starter.browser = starter.browser.ControlURL(controlUrl)
	}
	if starter.baseConfig.timeout != 0 {
		ctx, _ := context.WithTimeout(starter.ctx, time.Second*time.Duration(starter.baseConfig.timeout))
		starter.browser = starter.browser.Context(ctx)
	} else {
		starter.browser = starter.browser.Context(starter.ctx)
	}
	err := starter.browser.Connect()
	if err != nil {
		return utils.Errorf("browser connect error: %s", err)
	}
	starter.browser.IgnoreCertErrors(true)
	//starter.generateDefaultPageAction()
	starter.newDefaultPageActionGenerator()
	starter.newPageDetectEvent()
	return nil
}

func (starter *BrowserStarter) generateDefaultPageAction() {
	starter.scanRangeCheck = scanRangeFunctionGenerate(starter.baseUrl, starter.scanLevel)
	starter.checkFunctions = append(starter.checkFunctions,
		//scanRangeFunctionGenerate(starter.baseUrl, starter.scanLevel),
		starter.scanRangeCheck,
		repeatCheckFunctionGenerate(starter.pageVisit),
	)
	starter.extraFunctions = append(starter.extraFunctions,
		extraUrlCheck(extraUrlKeywords),
	)
	if len(starter.baseConfig.whiteList) != 0 {
		starter.checkFunctionMap["whiteList"] = whiteListCheckGenerator(starter.baseConfig.whiteList)
	}
	if len(starter.baseConfig.blackList) != 0 {
		starter.checkFunctionMap["blackList"] = blackListCheckGenerator(starter.baseConfig.blackList)
	}
	starter.getUrlFunction = starter.DefaultGetUrlFunctionGenerator(starter.DefaultDoGetUrl())
	starter.clickFunction = starter.DefaultClickFunctionGenerator(starter.DefaultDoClick())
	starter.inputFunction = starter.DefaultInputFunctionGenerator(starter.DefaultDoInput())
	starter.vueFunction = starter.EventClickFunctionGenerator(starter.vueClick(starter.DefaultDoGetUrl()))
	starter.pageActions = append(starter.pageActions,
		starter.inputFunction,
		starter.actionOnPage(),
	)
}

func (starter *BrowserStarter) testActionGenerator() {
	starter.checkFunctions = append(starter.checkFunctions,
		scanRangeFunctionGenerate(starter.baseUrl, starter.scanLevel),
		repeatCheckFunctionGenerate(starter.pageVisit),
	)
	if len(starter.baseConfig.whiteList) != 0 {
		starter.checkFunctionMap["whiteList"] = whiteListCheckGenerator(starter.baseConfig.whiteList)
	}
	if len(starter.baseConfig.blackList) != 0 {
		starter.checkFunctionMap["blackList"] = blackListCheckGenerator(starter.baseConfig.blackList)
	}
	starter.getUrlFunction = starter.DefaultGetUrlFunctionGenerator(starter.DefaultDoGetUrl())
	starter.pageActions = append(starter.pageActions, starter.getUrlFunction)
}

func (starter *BrowserStarter) newDefaultPageActionGenerator() {
	starter.scanRangeCheck = scanRangeFunctionGenerate(starter.baseUrl, starter.scanLevel)
	starter.checkFunctions = append(starter.checkFunctions,
		//scanRangeFunctionGenerate(starter.baseUrl, starter.scanLevel),
		starter.scanRangeCheck,
		repeatCheckFunctionGenerate(starter.pageVisit),
	)
	starter.extraFunctions = append(starter.extraFunctions,
		extraUrlCheck(extraUrlKeywords),
	)
	if len(starter.baseConfig.whiteList) != 0 {
		starter.checkFunctionMap["whiteList"] = whiteListCheckGenerator(starter.baseConfig.whiteList)
	}
	if len(starter.baseConfig.blackList) != 0 {
		starter.checkFunctionMap["blackList"] = blackListCheckGenerator(starter.baseConfig.blackList)
	}
	starter.getUrlsFunction = starter.getUrlsFunctionGenerator()
	starter.doUrlsFunction = starter.doUrlsFunctionGenerator()
	starter.getClickFunction = starter.getClickFunctionGenerator()
	starter.doClickFunction = starter.doClickFunctionGenerator()
	starter.getInputFunction = starter.getInputFunctionGenerator()
	starter.doInputFunction = starter.doInputFunctionGenerator()
	starter.getEventFunction = starter.getEventFunctionGenerator()
	starter.doEventClickFunction = starter.doEventClickFunctionGenerator()
	starter.doActionOnPage = starter.ActionOnPage()
	starter.elementCheck = starter.elementCheckGenerate()
}

func (starter *BrowserStarter) newPageDetectEvent() {
	go starter.browser.EachEvent(
		func(e *proto.TargetTargetCreated) {
			//starter.subWaitGroup.Add()
			//log.Infof(`current concurrent %d`, starter.subWaitGroup.WaitingEventCount)
			//defer starter.subWaitGroup.Done()
			go func() {
				log.Infof(`current concurrent %d`, starter.c.Number())
				status := starter.c.Add()
				log.Info("counter add status: ", status)
				defer starter.c.Minus()
				targetID := e.TargetInfo.TargetID
				page, err := starter.browser.PageFromTarget(targetID)
				if err != nil {
					log.Errorf("targetID %s page get error: %s", targetID, err)
				}
				defer page.Close()
				go page.EachEvent(func(e *proto.PageJavascriptDialogOpening) {
					proto.PageHandleJavaScriptDialog{
						Accept:     false,
						PromptText: "",
					}.Call(page)
				})()
				// wait for page navigate
				time.Sleep(500 * time.Millisecond)
				err = page.WaitLoad()
				if err != nil {
					log.Errorf("targetID %s wait load error: %s", targetID, err)
					return
				}
				if starter.extraWaitLoad != 0 {
					time.Sleep(time.Duration(starter.extraWaitLoad) * time.Millisecond)
				}
				if starter.checkHTML {
					pageInfo, err := page.HTML()
					if err == nil {
						bodyInfo := matchBody(pageInfo)
						if bodyInfo == `<body></body>` {
							log.Errorf("blank info in page: %s", page)
						}
						starter.checkHTML = false
					}
				}
				page = page.Timeout(time.Second * time.Duration(starter.baseConfig.pageTimeout))
				//err = starter.PageScan(page)
				err = starter.doActionOnPage(page)
				if err != nil {
					log.Errorf("targetID %s do page scan error: %s", targetID, err)
				}
			}()
		},
	)()
}

func (starter *BrowserStarter) createPageHijack(page *rod.Page) {
	pageHijackRouter := page.HijackRequests()
	var pageUrl string
	pageHijackRouter.MustAdd("*", func(hijack *rod.Hijack) {
		if pageUrl == "" {
			pageUrl = hijack.Request.URL().String()
		}
		contentType := hijack.Request.Header("Content-Type")
		if strings.Contains(contentType, "multipart/form-data") {
			hijack.ContinueRequest(&proto.FetchContinueRequest{})
			result := SimpleResult{}
			result.request = hijack.Request
			result.resultType = "file upload result"
			starter.ch <- &result
			return
		}
		resourceType := hijack.Request.Type()
		if notLoaderContains(resourceType) {
			hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		refererInfo := hijack.Request.Req().Header.Get("Referer")
		if refererInfo == "" && hijack.Request.URL().String() != starter.baseUrl {
			hijack.Request.Req().Header.Add("Referer", starter.baseUrl)
		}
		client := http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &starter.transport,
		}
		//err := hijack.LoadResponse(&starter.client, true)
		err := hijack.LoadResponse(&client, true)
		if err != nil {
			if !strings.Contains(err.Error(), "context canceled") {
				log.Errorf("load response error: %s", err)
			}
			hijack.Response.SetHeader()
			hijack.Response.SetBody("")
			return
		}
		if starter.stopSignal {
			return
		}
		if !starter.scanRangeCheck(hijack.Request.URL().String()) {
			return
		}
		var afterRepeatUrl string
		if starter.requestAfterRepeat != nil {
			afterRepeatUrl = starter.requestAfterRepeat(hijack.Request)
			//log.Info("...", afterRepeatUrl)
		} else {
			//log.Info("request after repeat generator function null.")
			afterRepeatUrl = hijack.Request.URL().String()
			if starter.urlAfterRepeat != nil {
				afterRepeatUrl = starter.urlAfterRepeat(afterRepeatUrl)
			}
		}
		if !starter.resultSentFunc(afterRepeatUrl) {
			return
		}

		//
		// do tree
		//

		if pageUrl != hijack.Request.URL().String() {
			starter.urlTree.Add(pageUrl, hijack.Request.URL().String())
			//log.Info(pageUrl, " -> ", hijack.Request.URL().String())
		}

		//log.Info(hijack.Response.Headers().Get("Content-Type"))
		if StringArrayContains(jsContentTypes, hijack.Response.Headers().Get("Content-Type")) {
			//log.Info("analysis js file: ", hijack.Request.URL().String())
			jsUrls := analysisJsInfo(hijack.Request.URL().String(), hijack.Response.Body())
			for _, jsUrl := range jsUrls {
				var jsAfterRepeatUrl string
				if starter.urlAfterRepeat != nil {
					jsAfterRepeatUrl = starter.urlAfterRepeat(jsUrl)
				} else {
					jsAfterRepeatUrl = jsUrl
				}
				if !starter.resultSentFunc(jsAfterRepeatUrl) {
					continue
				}
				result := SimpleResult{
					url:        jsUrl,
					resultType: "js url",
					method:     "JS GET",
					from:       hijack.Request.URL().String(),
				}
				starter.ch <- &result
			}
		}

		result := RequestResult{}
		result.request = hijack.Request
		result.response = hijack.Response
		result.from = pageUrl
		starter.ch <- &result
		if starter.maxUrl != 0 && starter.resultSent.Count() >= int64(starter.maxUrl) {
			starter.stopSignal = true
		}
	})
	go func() {
		pageHijackRouter.Run()
	}()
}

func (starter *BrowserStarter) Run() {
	err := starter.StartBrowser()
	if err != nil {
		log.Errorf("browser start error: %s", err)
		return
	}
	headlessBrowser := starter.browser
	for v := range starter.uChan.Out {
		urlStr, ok := v.(string)
		if !ok {
			continue
		}
		starter.mainWaitGroup.Add()
		p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
		if starter.baseConfig.hijack {
			starter.createPageHijack(p)
		}
		//log.Infof("open url: %s...", urlStr)
		err := p.Navigate(urlStr)
		if err != nil {
			log.Errorf("page navigate %s error: %s", urlStr, err)
		}
		starter.subWaitGroup.Wait()
	next:
		for {
			select {
			case v := <-starter.uChan.Out:
				if starter.stopSignal {
					continue
				}
				urlStr, _ := v.(string)
				p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
				if p == nil {
					log.Errorf("url %s create page nil.", urlStr)
					continue
				}
				starter.createPageHijack(p)
				//log.Infof("open url: %s...", urlStr)
				err = p.Navigate(urlStr)
				if err != nil {
					log.Errorf("page navigate %s error: %s", urlStr, err)
				}
				starter.subWaitGroup.Wait()
			default:
				starter.mainWaitGroup.Done()
				break next
			}
		}
	}
	log.Info("done!")
}

func (starter *BrowserStarter) MultiRun() {
	err := starter.StartBrowser()
	if err != nil {
		log.Errorf("browser start error: %s", err)
		return
	}
	headlessBrowser := starter.browser
	for v := range starter.uChan.Out {
		urlStr, ok := v.(string)
		if !ok {
			continue
		}
		starter.mainWaitGroup.Add()
		p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
		if starter.baseConfig.hijack {
			starter.createPageHijack(p)
		}
		//log.Infof("open url: %s...", urlStr)
		err := p.Navigate(urlStr)
		if err != nil {
			log.Errorf("page navigate %s error: %s", urlStr, err)
		}
	next:
		for {
			time.Sleep(500 * time.Millisecond)
			select {
			case v := <-starter.uChan.Out:
				if starter.stopSignal {
					continue
				}
				if starter.subWaitGroup.WaitingEventCount >= starter.concurrent {
					starter.subWaitGroup.Wait()
				}
				urlStr, _ := v.(string)
				p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
				if starter.baseConfig.hijack {
					starter.createPageHijack(p)
				}
				//log.Infof("open url: %s... in page %s", urlStr, p.TargetID)
				err = p.Navigate(urlStr)
				if err != nil {
					log.Errorf("page navigate %s error: %s", urlStr, err)
				}
			default:
				if starter.subWaitGroup.WaitingEventCount > 0 {
					starter.subWaitGroup.Wait()
				} else {
					starter.mainWaitGroup.Done()
					break next
				}
			}
		}
	}
	log.Info("done!")
}

func (starter *BrowserStarter) NewEngine() {
	err := starter.StartBrowser()
	if err != nil {
		log.Errorf("browser start error: %s", err)
		return
	}
	headlessBrowser := starter.browser
running:
	for {
		select {
		case v, ok := <-starter.uChan.Out:
			log.Infof(`current url chan len: %d`, starter.uChan.Len())
			if !ok {
				log.Info("break running.")
				break running
			}
			if !starter.running {
				log.Info(`start running.`)
				starter.mainWaitGroup.Add()
				starter.running = true
			}
			if starter.c.OverLoad() {
				log.Infof(`overload, waiting for concurrent: %d`, starter.c.Number())
				starter.c.Wait(starter.concurrent)
				log.Infof(`overload done: %d`, starter.c.Number())
			}
			urlStr, _ := v.(string)
			p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
			if starter.baseConfig.hijack {
				starter.createPageHijack(p)
			}
			err = p.Navigate(urlStr)
			if err != nil {
				log.Errorf("page navigate %s error: %s", urlStr, err)
			}
		default:
			if starter.c.LayDown() && starter.running {
				log.Info(`lay down. `)
				starter.running = false
				starter.mainWaitGroup.Done()
			} else {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
	log.Info("done!")
}
