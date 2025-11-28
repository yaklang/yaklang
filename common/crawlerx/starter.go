// Package crawlerx
// @Author bcy2007  2023/7/12 16:19
package crawlerx

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/embed"
	"regexp"
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

	ctx          context.Context
	cancel       context.CancelFunc
	waitGroup    *utils.SizedWaitGroup
	subWaitGroup *utils.SizedWaitGroup

	pageVisit  func(string) bool
	resultSent func(string) bool
	scanRange  func(string) bool
	urlCheck   map[string]func(string) bool
	banList    *tools.StringCountFilter

	requestAfterRepeat func(HijackRequest) string
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
	stealth    bool
	vue        bool

	extraWaitLoadTime int

	// get info
	getUrls          func(*rod.Page) ([]string, error)
	getClickElements func(*rod.Page) ([]string, error)
	getInputElements func(*rod.Page) (rod.Elements, error)
	getEventElements func(*rod.Page) ([]string, error)
	// use info
	urlsExploit          func(string, string) error
	clickElementsExploit func(*rod.Page, string, string) error
	inputElementsExploit func(*rod.Element, interface{}) error
	eventElementsExploit func(*rod.Page, string, string) error

	headers []*headers
	cookies []*proto.NetworkCookieParam

	invalidSuffix []string

	runtimeID    string
	saveToDB     bool
	https        bool
	evalJs       []*JSEval
	jsResultSend func(string)
	sourceType   string
	fromPlugin   string

	aiInputUrl  string
	aiInputInfo string

	// login related
	login         bool
	aiDomain      string
	aiApiKey      string
	loginUsername string
	loginPassword string
}

func NewBrowserStarter(browserConfig *BrowserConfig, baseConfig *BaseConfig) *BrowserStarter {
	starter := BrowserStarter{
		baseUrl:       baseConfig.targetUrl,
		browserConfig: browserConfig,
		baseConfig:    baseConfig,

		waitGroup:    baseConfig.waitGroup,
		subWaitGroup: utils.NewSizedWaitGroup(20),

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
		stealth:    baseConfig.stealth,
		vue:        baseConfig.vue,

		extraWaitLoadTime: baseConfig.extraWaitLoadTime,

		headers: baseConfig.headers,
		cookies: baseConfig.cookies,

		runtimeID: baseConfig.runtimeId,
		saveToDB:  baseConfig.saveToDB,
		https:     false,

		evalJs:     make([]*JSEval, 0),
		sourceType: baseConfig.sourceType,
		fromPlugin: baseConfig.fromPlugin,

		aiInputUrl:  baseConfig.aiInputUrl,
		aiInputInfo: baseConfig.aiInputInfo,

		login:         baseConfig.login,
		loginUsername: baseConfig.username,
		loginPassword: baseConfig.password,
	}
	var ctx context.Context
	var cancel context.CancelFunc
	if starter.baseConfig.fullTimeout != 0 {
		ctx, cancel = context.WithTimeout(baseConfig.ctx, time.Second*time.Duration(starter.baseConfig.fullTimeout))
	} else {
		ctx, cancel = context.WithCancel(baseConfig.ctx)
	}
	starter.ctx = ctx
	starter.cancel = cancel
	starter.pageVisit = repeatCheckFunctionGenerate(baseConfig.pageVisit)
	starter.resultSent = repeatCheckFunctionGenerate(baseConfig.resultSent)
	starter.scanRange = scanRangeFunctionGenerate(starter.baseUrl, baseConfig.scanRange)
	starter.requestAfterRepeat = repeatCheckGenerator(baseConfig.scanRepeat, baseConfig.ignoreParams...)
	starter.urlAfterRepeat = urlRepeatCheckGenerator(baseConfig.scanRepeat, baseConfig.ignoreParams...)
	starter.counter = tools.NewCounter(starter.concurrent)
	if len(starter.baseConfig.invalidSuffix) > 0 {
		starter.invalidSuffix = append(starter.invalidSuffix, starter.baseConfig.invalidSuffix...)
	} else {
		starter.invalidSuffix = defaultInvalidSuffix
	}
	if strings.HasPrefix(starter.baseUrl, "https://") || strings.HasPrefix(starter.baseUrl, "wss://") {
		starter.https = true
	}
	for key, values := range baseConfig.evalJs {
		e := CreateJsEval()
		reg, err := regexp.Compile(key)
		if err != nil {
			log.Errorf(`evaljs target url %v compiler error: %v`, key, err)
			continue
		}
		e.targetUrl = reg
		e.js = append(e.js, values...)
		starter.evalJs = append(starter.evalJs, e)
	}
	starter.jsResultSend = starter.baseConfig.jsResultSave
	return &starter
}

func (starter *BrowserStarter) startBrowser() error {
	err := starter.baseBrowserStarter()
	if err != nil {
		return err
	}
	if len(starter.baseConfig.localStorage) > 0 {
		err = starter.localStorage()
		if err != nil {
			return utils.Errorf("do local storage error: %v", err)
		}
	}
	err = starter.createBrowserHijack(starter.browser)
	if err != nil {
		return utils.Errorf(`create browser error: %v`, err)
	}
	starter.pageActionGenerator()
	starter.pageDetectEventGenerator()
	return nil
}

func (starter *BrowserStarter) baseBrowserStarter() error {
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
	starter.browser = starter.browser.Context(starter.ctx)
	err := starter.browser.Connect()
	if len(starter.cookies) > 0 {
		err = starter.browser.SetCookies(starter.cookies)
		if err != nil {
			return utils.Errorf(`browser set cookies error: %v`, err)
		}
	}
	if err != nil {
		return utils.Errorf(`browser connect error: %s`, err)
	}
	_ = starter.browser.IgnoreCertErrors(true)
	return nil
}

func (starter *BrowserStarter) doLauncher(l *launcher.Launcher) *launcher.Launcher {
	if starter.browserConfig.proxyAddress != nil {
		l = l.Proxy(starter.browserConfig.proxyAddress.String())
	}
	l = l.NoSandbox(true).Set(flags.Headless, "new").Set("disable-features", "HttpsUpgrades")
	
	// 在 Windows 上防止 Chrome 创建桌面快捷方式
	if strings.Contains(runtime.GOOS, "windows") {
		l = l.Set("no-first-run", "")
		l = l.Set("no-default-browser-check", "")
		l = l.Set("disable-default-apps", "")
	}
	
	if (starter.baseConfig.leakless == "default" && strings.Contains(runtime.GOOS, "windows")) ||
		starter.baseConfig.leakless == "false" {
		l = l.Leakless(false)
	}
	return l
}

func (starter *BrowserStarter) localStorage() error {
	log.Debugf(`do local storage on %s`, starter.baseUrl)
	page, err := starter.browser.Page(proto.TargetCreateTarget{URL: starter.baseUrl})
	defer func() {
		_ = page.Close()
	}()
	if err != nil {
		return utils.Errorf("local storage create base page error: %v", err)
	}
	err = page.WaitLoad()
	if err != nil {
		return utils.Errorf("local storage base page wait load error: %v", err)
	}
	for key, value := range starter.baseConfig.localStorage {
		setStorageJS := fmt.Sprintf(`()=>window.localStorage.setItem("%s", "%s")`, key, value)
		_, err := page.Eval(setStorageJS)
		if err != nil {
			return utils.Errorf("local storage save data error: %v", err)
		}
	}
	return nil
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
	if starter.aiInputUrl != "" {
		starter.inputElementsExploit = starter.generateAIInputElementsExploit()
	} else {
		starter.inputElementsExploit = starter.generateInputElementsExploit()
	}
	starter.clickElementsExploit = starter.generateClickElementsExploit()
	//starter.eventElementsExploit = starter.generateEventElementsExploit()
	starter.eventElementsExploit = starter.newEventElementsExploit()

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
	defer func() {
		log.Debugf(`page with target ID %v closing...`, targetID)
		err = page.Close()
		if err != nil {
			log.Errorf(`page with target ID %v closing error: %v`, targetID, err)
		}
	}()
	go page.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{
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
	urlStr, _ := getCurrentUrl(page)
	log.Debugf(`page opened: %v with targetID %v`, urlStr, targetID)
	_, err = page.EvalOnNewDocument(pageScript)
	if err != nil {
		log.Errorf(`page script run error: %v`, err)
		return
	}
	if starter.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(starter.extraWaitLoadTime) * time.Millisecond)
	}
	// login
	if starter.login {
		// login action
		page = page.Timeout(30 * time.Second)
		err = starter.Login(page)
		if err != nil {
			log.Debugf(`Login error: %v`, err)
		}
		//_ = page.WaitLoad()
		//if starter.extraWaitLoadTime != 0 {
		//	time.Sleep(time.Duration(starter.extraWaitLoadTime) * time.Millisecond)
		//}
		page = page.CancelTimeout()
		starter.login = false
	}
	// session storage
	if len(starter.baseConfig.sessionStorage) > 0 {
		for key, value := range starter.baseConfig.sessionStorage {
			setSessionStorage := fmt.Sprintf(`()=>window.sessionStorage.setItem("%s", "%s")`, key, value)
			_, err := page.Eval(setSessionStorage)
			if err != nil {
				log.Errorf("session storage save data error: %v", err)
				return
			}
		}
	}
	// eval js
	for _, item := range starter.evalJs {
		if item.targetUrl.MatchString(urlStr) {
			for _, js := range item.js {
				resultObj, err := page.Eval(js)
				if err != nil {
					log.Errorf(`page eval custom js error: %v`, err)
					continue
				}
				jsResult := resultObj.Value.String()
				result := JsResultSave{
					TargetUrl: urlStr,
					Js:        js,
					Result:    jsResult,
				}
				resultBytes, err := json.Marshal(result)
				if err != nil {
					log.Errorf(`json marshal result error: %v`, err)
					continue
				}
				if starter.jsResultSend != nil {
					starter.jsResultSend(string(resultBytes))
				} else {
					log.Debugf(`get eval js result: %v`, string(resultBytes))
				}
			}
		}
	}
	//
	if starter.baseConfig.pageTimeout != 0 {
		page = page.Timeout(time.Duration(starter.baseConfig.pageTimeout) * time.Second)
	}
	err = starter.ActionOnPage(page)
	if err != nil {
		log.Errorf(`TargetID %s get page do action error: %s`, targetID, err)
	}
}

func (starter *BrowserStarter) createBrowserHijack(browser *rod.Browser) error {
	browserHijackRouter := NewBrowserHijackRequests(browser)
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithTimeout(30 * time.Second),
		lowhttp.WithSaveHTTPFlow(starter.saveToDB),
		lowhttp.WithSource(starter.sourceType),
	}
	if starter.browserConfig.proxyAddress != nil {
		opts = append(opts, lowhttp.WithProxy(starter.browserConfig.proxyAddress.String()))
	}
	if starter.runtimeID != "" {
		opts = append(opts, lowhttp.WithRuntimeId(starter.runtimeID))
	}
	if starter.fromPlugin != "" {
		opts = append(opts, lowhttp.WithFromPlugin(starter.fromPlugin))
	}
	err := browserHijackRouter.Add("*", "", func(hijack *CrawlerHijack) {
		//if pageUrl == "" {
		//	pageUrl = hijack.Request.URL().String()
		//}
		urlStr := hijack.Request.URL().String()
		starter.subWaitGroup.Add()
		defer starter.subWaitGroup.Done()
		for _, header := range starter.headers {
			hijack.Request.Req().Header.Set(header.Key, header.Value)
		}
		contentType := hijack.Request.Header("Content-Type")
		if strings.Contains(contentType, "octet-stream") {
			hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		if strings.Contains(contentType, "multipart/form-data") {
			hijack.ContinueRequest(&proto.FetchContinueRequest{})
			result := SimpleResult{}
			result.request = hijack.Request
			result.resultType = "file upload result"
			select {
			case <-starter.ctx.Done():
				log.Error("context deadline exceed")
			default:
				starter.ch <- &result
			}
			return
		}
		resourceType := hijack.Request.Type()
		if notLoaderContains(resourceType) {
			hijack.Response.Fail(proto.NetworkErrorReasonBlockedByClient)
			return
		}
		refererInfo := hijack.Request.Req().Header.Get("Referer")
		if refererInfo == "" && urlStr != starter.baseUrl {
			hijack.Request.Req().Header.Add("Referer", starter.baseUrl)
		}
		tempOpts := make([]lowhttp.LowhttpOpt, 0)
		tempOpts = append(tempOpts, opts...)
		if strings.HasPrefix(urlStr, "https://") || strings.HasPrefix(urlStr, "wss://") {
			tempOpts = append(tempOpts, lowhttp.WithHttps(true))
		}
		err := hijack.LoadResponse(tempOpts, true)
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
		if !starter.scanRange(urlStr) {
			return
		}
		var afterRepeatUrl string
		if starter.requestAfterRepeat != nil {
			afterRepeatUrl = starter.requestAfterRepeat(hijack.Request)
		} else {
			afterRepeatUrl = urlStr
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

		//if pageUrl != urlStr {
		//	starter.urlTree.Add(pageUrl, urlStr)
		//}

		if StringArrayContains(jsContentTypes, hijack.Response.Headers().Get("Content-Type")) {
			jsUrls := analysisJsInfo(urlStr, hijack.Response.Body())
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
					from:       urlStr,
				}
				select {
				case <-starter.ctx.Done():
					log.Error("context deadline exceed")
					return
				default:
					starter.ch <- &result
				}
			}
		}

		result := RequestResult{}
		result.request = hijack.Request
		result.response = hijack.Response
		//result.from = pageUrl
		select {
		case <-starter.ctx.Done():
			log.Error("context deadline exceed")
			return
		default:
			starter.ch <- &result
		}
		if starter.maxUrl > 0 && starter.baseConfig.resultSent.Count() >= int64(starter.maxUrl) {
			starter.stopSignal = true
		}
	})
	if err != nil {
		return utils.Errorf(`create hijack error: %v`, err.Error())
	}
	go func() {
		browserHijackRouter.Run()
	}()
	return nil
}

func (starter *BrowserStarter) Start() {
	err := starter.startBrowser()
	if err != nil {
		log.Errorf("browser start error: %s", err)
		starter.baseConfig.startWaitGroup.Done()
		return
	}
	headlessBrowser := starter.browser
	_ = proto.BrowserSetDownloadBehavior{
		Behavior:         proto.BrowserSetDownloadBehaviorBehaviorDeny,
		BrowserContextID: headlessBrowser.BrowserContextID,
	}.Call(headlessBrowser)
	//defer headlessBrowser.MustClose()
	defer func() {
		_ = headlessBrowser.Close()
	}()
	defer starter.cancel()
	stealthJs, err := embed.Asset("data/anti-crawler/stealth.min.js")
	if err != nil {
		log.Errorf("stealth.min.js read error: %v", err.Error())
	} else {
		log.Debug("stealth.min.js load done!")
	}
	starter.baseConfig.startWaitGroup.Done()
running:
	for {
		select {
		case <-starter.ctx.Done():
			if starter.running {
				starter.running = false
				starter.subWaitGroup.Wait()
				starter.waitGroup.Done()
			}
			break running
		case v, ok := <-starter.uChan.Out:
			log.Debugf(`current url chan len: %d`, starter.uChan.Len())
			if !ok {
				log.Debug("break running.")
				break running
			}
			if !starter.running {
				log.Debug(`start running.`)
				starter.waitGroup.Add()
				starter.running = true
			}
			if starter.counter.OverLoad() {
				starter.counter.Wait(starter.concurrent)
			}
			urlStr, _ := v.(string)
			var p *rod.Page
			p, err = headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
			if err != nil {
				log.Errorf("create page error: %v", err)
				continue
			}
			if starter.stealth {
				_, err := p.EvalOnNewDocument(string(stealthJs))
				if err != nil {
					log.Errorf(`eval stealth.min.js on page error: %v`, err.Error())
					starter.stealth = false
				}
			}
			//err = starter.createPageHijack(p)
			//if err != nil {
			//	log.Error(err)
			//	return
			//}
			err = p.Navigate(urlStr)
			if err != nil {
				log.Errorf("page navigate %s error: %s", urlStr, err)
			}
		default:
			if starter.counter.LayDown() && starter.running {
				log.Debug(`lay down. `)
				starter.running = false
				starter.subWaitGroup.Wait()
				starter.waitGroup.Done()
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
	log.Debug("done!")
}

func (starter *BrowserStarter) Test() {
	time.Sleep(500 * time.Millisecond)
	err := starter.startBrowser()
	if err != nil {
		log.Errorf("browser start error: %s", err)
		starter.baseConfig.startWaitGroup.Done()
		return
	}
	headlessBrowser := starter.browser
	//defer headlessBrowser.MustClose()
	defer func() {
		_ = headlessBrowser.Close()
	}()
	defer starter.cancel()
	starter.baseConfig.startWaitGroup.Done()
	starter.waitGroup.Add()
	time.Sleep(500 * time.Millisecond)
	url, ok := <-starter.uChan.Out
	if !ok {
		return
	}
	urlStr, _ := url.(string)
	p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	//err = starter.createPageHijack(p)
	//if err != nil {
	//	log.Error(err)
	//	return
	//}
	err = p.Navigate(urlStr)
	if err != nil {
		log.Errorf("page navigate %s error: %s", urlStr, err)
	}
	time.Sleep(20000 * time.Millisecond)

	starter.waitGroup.Done()
	time.Sleep(500 * time.Millisecond)
}
