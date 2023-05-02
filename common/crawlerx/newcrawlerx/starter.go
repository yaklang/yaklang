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
	"net/http"
	"strings"
	"time"
	"github.com/yaklang/yaklang/common/crawlerx/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

	urlTree *UrlTree

	ch chan ReqInfo

	uChan *UChan

	formFill  map[string]string
	inputFile map[string]string

	concurrent int
	maxUrl     int
	stopSignal bool

	getUrlFunction func(*rod.Page) error
	clickFunction  func(*rod.Page) error
	inputFunction  func(*rod.Page) error
	pageActions    []func(*rod.Page) error
	actionMap      map[string]func(*rod.Page) error
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
		stopSignal: false,

		pageActions: make([]func(*rod.Page) error, 0),
		actionMap:   make(map[string]func(*rod.Page) error),
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
	starter.browser = starter.browser.Context(starter.ctx)
	err := starter.browser.Connect()
	if err != nil {
		return utils.Errorf("browser connect error: %s", err)
	}
	starter.browser.IgnoreCertErrors(true)
	if starter.baseConfig.vue {
		starter.vuePageActionGeneration()
	} else {
		starter.generateDefaultPageAction()
	}
	//starter.testActionGenerator()
	starter.newPageDetectEvent()
	return nil
}

func (starter *BrowserStarter) generateDefaultPageAction() {
	starter.checkFunctions = append(starter.checkFunctions,
		scanRangeFunctionGenerate(starter.baseUrl, starter.scanLevel),
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
	//starter.clickFunction = starter.EventClickFunctionGenerator(starter.DefaultDoClick())
	starter.inputFunction = starter.DefaultInputFunctionGenerator(starter.DefaultDoInput())
	starter.pageActions = append(starter.pageActions,
		starter.inputFunction,
		starter.getUrlFunction,
		starter.clickFunction,
	)
}

func (starter *BrowserStarter) vuePageActionGeneration() {
	starter.checkFunctions = append(starter.checkFunctions,
		scanRangeFunctionGenerate(starter.baseUrl, starter.scanLevel),
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
	starter.pageActions = append(starter.pageActions,
		starter.EventClickFunctionGenerator(starter.vueClick(starter.DefaultDoGetUrl())),
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

func (starter *BrowserStarter) newPageDetectEvent() {
	go starter.browser.EachEvent(
		func(e *proto.TargetTargetCreated) {
			starter.subWaitGroup.Add()
			defer starter.subWaitGroup.Done()
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
			page = page.Timeout(time.Second * time.Duration(starter.baseConfig.pageTimeout))
			err = page.WaitLoad()
			if err != nil {
				log.Errorf("targetID %s wait load error: %s", targetID, err)
				return
			}
			err = starter.PageScan(page)
			if err != nil {
				log.Errorf("targetID %s do page scan error: %s", targetID, err)
			}
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
		}
		transport := http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		if starter.browserConfig.proxyAddress != nil {
			transport.Proxy = http.ProxyURL(starter.browserConfig.proxyAddress)
		}
		client.Transport = &transport
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
		var afterRepeatUrl string
		if starter.requestAfterRepeat != nil {
			afterRepeatUrl = starter.requestAfterRepeat(hijack.Request)
			//log.Info("...", afterRepeatUrl)
		} else {
			//log.Info("request after repeat generator function null.")
			afterRepeatUrl = hijack.Request.URL().String()
		}
		if !starter.resultSentFunc(afterRepeatUrl) {
			return
		}

		//
		// do tree
		//

		//if pageUrl != hijack.Request.URL().String() {
		//starter.urlTree.Add(pageUrl, hijack.Request.URL().String())
		//log.Info(pageUrl, " -> ", hijack.Request.URL().String())
		//}

		result := RequestResult{}
		result.request = hijack.Request
		result.response = hijack.Response
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
	starter.StartBrowser()
	headlessBrowser := starter.browser
	for v := range starter.uChan.Out {
		urlStr, ok := v.(string)
		if !ok {
			continue
		}
		starter.mainWaitGroup.Add()
		p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
		starter.createPageHijack(p)
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
	starter.StartBrowser()
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
				err = p.Navigate(urlStr)
				if err != nil {
					log.Errorf("page navigate %s error: %s", urlStr, err)
				}
			default:
				if starter.subWaitGroup.WaitingEventCount > 0 {
					starter.subWaitGroup.Wait()
				} else {
					log.Info(starter.subWaitGroup.WaitingEventCount)
					starter.mainWaitGroup.Done()
					break next
				}
			}
		}
		log.Info("break next.")
	}
	log.Info("done!")
}
