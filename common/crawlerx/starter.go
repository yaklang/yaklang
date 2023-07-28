// Package crawlerx
// @Author bcy2007  2023/7/12 16:19
package crawlerx

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/utils"
)

type BrowserStarter struct {
	baseUrl       string
	browser       *rod.Browser
	browserConfig *BrowserConfig
	baseConfig    *BaseConfig

	ctx       context.Context
	cancel    context.CancelFunc
	waitGroup *utils.SizedWaitGroup

	pageVisit  func(string) bool
	resultSent func(string) bool
	scanRange  func(string) bool
	urlCheck   map[string]func(string) bool
	banList    *tools.StringCountFilter

	requestAfterRepeat func(*rod.HijackRequest) string
	urlAfterRepeat     func(string) string
	elementCheck       func(*rod.Element) bool

	urlTree *tools.UrlTree
	uChan   *tools.UChan
	ch      chan ReqInfo

	formFill   map[string]string
	fileUpload map[string]string

	concurrent int
	counter    *tools.Counter
	maxUrl     int
	maxDepth   int

	stopSignal bool
	running    bool

	extraWaitLoadTime int

	transport *http.Transport

	// get info
	getUrls          func(*rod.Page) ([]string, error)
	getClickElements func(*rod.Page) ([]string, error)
	getInputElements func(*rod.Page) (rod.Elements, error)
	getEventElements func(*rod.Page) ([]string, error)
	// use info
	urlsExploit          func(string, string) error
	clickElementsExploit func(*rod.Page, string, string) error
	inputElementsExploit func(*rod.Element) error
	eventElementsExploit func(*rod.Page, string, string) error

	invalidSuffix []string
}

func NewBrowserStarter(browserConfig *BrowserConfig, baseConfig *BaseConfig) *BrowserStarter {
	starter := BrowserStarter{
		baseUrl:       baseConfig.targetUrl,
		browserConfig: browserConfig,
		baseConfig:    baseConfig,

		waitGroup: baseConfig.waitGroup,

		urlCheck: make(map[string]func(string) bool),
		banList:  tools.NewCountFilter(),

		urlTree: baseConfig.urlTree,
		uChan:   baseConfig.uChan,
		ch:      baseConfig.ch,

		formFill:   baseConfig.formFill,
		fileUpload: baseConfig.fileInput,

		concurrent: baseConfig.concurrent,
		maxUrl:     baseConfig.maxUrlCount,
		maxDepth:   baseConfig.maxDepth,

		stopSignal: false,
		running:    false,

		extraWaitLoadTime: baseConfig.extraWaitLoadTime,
	}
	ctx, cancel := context.WithCancel(baseConfig.ctx)
	starter.ctx = ctx
	starter.cancel = cancel
	starter.pageVisit = repeatCheckFunctionGenerate(baseConfig.pageVisit)
	starter.resultSent = repeatCheckFunctionGenerate(baseConfig.resultSent)
	starter.scanRange = scanRangeFunctionGenerate(starter.baseUrl, baseConfig.scanRange)
	starter.requestAfterRepeat = repeatCheckGenerator(baseConfig.scanRepeat, baseConfig.ignoreParams...)
	starter.urlAfterRepeat = urlRepeatCheckGenerator(baseConfig.scanRepeat, baseConfig.ignoreParams...)
	starter.counter = tools.NewCounter(starter.concurrent)
	starter.transport = &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		DialContext:           netx.NewDialContextFunc(30 * time.Second),
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	if starter.browserConfig.proxyAddress != nil {
		starter.transport.Proxy = http.ProxyURL(starter.browserConfig.proxyAddress)
	}
	if len(starter.baseConfig.invalidSuffix) > 0 {
		starter.invalidSuffix = append(starter.invalidSuffix, starter.baseConfig.invalidSuffix...)
	} else {
		starter.invalidSuffix = defaultInvalidSuffix
	}
	return &starter
}

func (starter *BrowserStarter) startBrowser() error {
	starter.browser = rod.New()
	if starter.browserConfig.wsAddress == "" {
		launch := launcher.New()
		if starter.browserConfig.exePath != "" {
			launch = launch.Bin(starter.browserConfig.exePath)
		}
		launch = starter.doLauncher(launch)
		controlUrl, err := launch.Launch()
		if err != nil {
			return utils.Errorf(`Launcher launch error: %s`, err)
		}
		starter.browser = starter.browser.ControlURL(controlUrl)
	} else {
		launch, err := launcher.NewManaged(starter.browserConfig.wsAddress)
		if err != nil {
			return utils.Errorf(`New launcher %s managed error: %s`, starter.browserConfig.wsAddress, err)
		}
		launcherCtx := context.Background()
		launch = launch.Context(launcherCtx)
		launch = starter.doLauncher(launch)
		serviceUrl, header := launch.ClientHeader()
		client, err := cdp.StartWithURL(launcherCtx, serviceUrl, header)
		if err != nil {
			return utils.Errorf(`Cdp start with url %s error: %s`, serviceUrl, err)
		}
		starter.browser = starter.browser.Client(client)
	}
	if starter.baseConfig.fullTimeout != 0 {
		ctx, _ := context.WithTimeout(starter.ctx, time.Second*time.Duration(starter.baseConfig.fullTimeout))
		starter.browser = starter.browser.Context(ctx)
	} else {
		starter.browser = starter.browser.Context(starter.ctx)
	}
	err := starter.browser.Connect()
	if err != nil {
		return utils.Errorf(`browser connect error: %s`, err)
	}
	starter.browser.IgnoreCertErrors(true)
	starter.pageActionGenerator()
	starter.pageDetectEventGenerator()
	return nil
}

func (starter *BrowserStarter) doLauncher(l *launcher.Launcher) *launcher.Launcher {
	if starter.browserConfig.proxyAddress != nil {
		l = l.Proxy(starter.browserConfig.proxyAddress.String())
	}
	l = l.NoSandbox(true).Headless(true)
	if (starter.baseConfig.leakless == "default" && strings.Contains(runtime.GOOS, "windows")) ||
		starter.baseConfig.leakless == "false" {
		l = l.Leakless(false)
	}
	return l
}

func (starter *BrowserStarter) pageActionGenerator() {
	starter.urlCheck["crawler_range"] = starter.scanRange
	starter.urlCheck["repeat_url"] = starter.pageVisit
	if len(starter.baseConfig.whitelist) > 0 {
		starter.urlCheck["whitelist"] = whiteListCheckGenerator(starter.baseConfig.whitelist)
	}
	if len(starter.baseConfig.blacklist) > 0 {
		starter.urlCheck["blacklist"] = blackListCheckGenerator(starter.baseConfig.blacklist)
	}

	starter.getUrls = starter.generateGetUrls()
	starter.getInputElements = starter.generateGetInputElements()
	starter.getClickElements = starter.generateGetClickElements()
	starter.getEventElements = starter.generateGetEventElements()

	starter.urlsExploit = starter.generateUrlsExploit()
	starter.inputElementsExploit = starter.generateInputElementsExploit()
	starter.clickElementsExploit = starter.generateClickElementsExploit()
	starter.eventElementsExploit = starter.generateEventElementsExploit()

	starter.elementCheck = starter.elementCheckGenerate()
}

func (starter *BrowserStarter) pageDetectEventGenerator() {
	go starter.browser.EachEvent(
		func(e *proto.TargetTargetCreated) {
			targetID := e.TargetInfo.TargetID
			go starter.scanCreatedTarget(targetID)
		},
	)()
}

func (starter *BrowserStarter) scanCreatedTarget(targetID proto.TargetTargetID) {
	starter.counter.Add()
	defer starter.counter.Minus()
	page, err := starter.browser.PageFromTarget(targetID)
	if err != nil {
		log.Errorf(`TargetID %s get page error: %s`, targetID, err)
		return
	}
	defer page.Close()
	go page.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			proto.PageHandleJavaScriptDialog{
				Accept:     false,
				PromptText: "",
			}.Call(page)
		},
	)()
	time.Sleep(500 * time.Millisecond)
	err = page.WaitLoad()
	if err != nil {
		log.Errorf(`TargetID %s get page wait load error: %s`, targetID, err)
		return
	}
	if starter.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(starter.extraWaitLoadTime) * time.Millisecond)
	}
	if starter.baseConfig.pageTimeout != 0 {
		page = page.Timeout(time.Duration(starter.baseConfig.pageTimeout) * time.Second)
	}
	err = starter.actionOnPage(page)
	if err != nil {
		log.Errorf(`TargetID %s get page do action error: %s`, targetID, err)
	}
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
			Transport: starter.transport,
		}
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
		if !starter.scanRange(hijack.Request.URL().String()) {
			return
		}
		var afterRepeatUrl string
		if starter.requestAfterRepeat != nil {
			afterRepeatUrl = starter.requestAfterRepeat(hijack.Request)
		} else {
			afterRepeatUrl = hijack.Request.URL().String()
			if starter.urlAfterRepeat != nil {
				afterRepeatUrl = starter.urlAfterRepeat(afterRepeatUrl)
			}
		}
		if !starter.resultSent(afterRepeatUrl) {
			return
		}

		//
		// do tree
		//

		if pageUrl != hijack.Request.URL().String() {
			starter.urlTree.Add(pageUrl, hijack.Request.URL().String())
		}

		if StringArrayContains(jsContentTypes, hijack.Response.Headers().Get("Content-Type")) {
			jsUrls := analysisJsInfo(hijack.Request.URL().String(), hijack.Response.Body())
			for _, jsUrl := range jsUrls {
				var jsAfterRepeatUrl string
				if starter.urlAfterRepeat != nil {
					jsAfterRepeatUrl = starter.urlAfterRepeat(jsUrl)
				} else {
					jsAfterRepeatUrl = jsUrl
				}
				if !starter.resultSent(jsAfterRepeatUrl) {
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
		if starter.maxUrl != 0 && starter.baseConfig.resultSent.Count() >= int64(starter.maxUrl) {
			starter.stopSignal = true
		}
	})
	go func() {
		pageHijackRouter.Run()
	}()
}

func (starter *BrowserStarter) Start() {
	err := starter.startBrowser()
	if err != nil {
		log.Errorf("browser start error: %s", err)
		return
	}
	headlessBrowser := starter.browser
	defer headlessBrowser.MustClose()
	starter.baseConfig.startWaitGroup.Done()
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
				starter.waitGroup.Add()
				starter.running = true
			}
			if starter.counter.OverLoad() {
				log.Infof(`overload, waiting for concurrent: %d`, starter.counter.Number())
				starter.counter.Wait(starter.concurrent)
				log.Infof(`overload done: %d`, starter.counter.Number())
			}
			urlStr, _ := v.(string)
			p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
			starter.createPageHijack(p)
			err = p.Navigate(urlStr)
			if urlStr == starter.baseUrl && len(starter.baseConfig.localStorage) > 0 {
				log.Infof(`do local storage on %s`, urlStr)
				for key, value := range starter.baseConfig.localStorage {
					setStorageJS := fmt.Sprintf(`(key, value) => { window.localStorage.setItem(%s, %s) }`, key, value)
					_, err := p.EvalOnNewDocument(setStorageJS)
					if err != nil {
						log.Errorf(`local storage save error: %s`, err)
					}
				}
			}
			if err != nil {
				log.Errorf("page navigate %s error: %s", urlStr, err)
			}
		default:
			if starter.counter.LayDown() && starter.running {
				log.Info(`lay down. `)
				starter.running = false
				starter.waitGroup.Done()
			} else {
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
	log.Info("done!")
}
