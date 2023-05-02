package core

import (
	"context"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"strings"
	"time"
	"yaklang/common/crawlerx/detect"
	"yaklang/common/crawlerx/filter"
	"yaklang/common/crawlerx/tag"
	"yaklang/common/utils"
)

func NewCrawlerV2(targetUrl string, configOpts ...ConfigOpt) (*CrawlerX, error) {
	config := &Config{
		timeout:            60,
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
	err := crawlerX.initV2()
	if err != nil {
		return nil, utils.Errorf("crawler module initial error: %s", err)
	}
	return crawlerX, nil
}

func (crawler *CrawlerX) initV2() error {
	if crawler.chromeWS != "" {
		launch, err := launcher.NewManaged(crawler.chromeWS)
		if err != nil {
			return utils.Errorf("new launcher %s managed error: %s", crawler.chromeWS, err)
		}
		if crawler.proxy != "" {
			launch = launch.Proxy(crawler.proxy)
		}
		launch = launch.NoSandbox(true).Headless(true)
		crawler.browser = crawler.browser.Client(launch.MustClient())
	} else if crawler.proxy != "" {
		launch := launcher.New()
		launch = launch.Set(flags.ProxyServer, crawler.proxy)
		controlUrl, err := launch.Launch()
		if err != nil {
			return utils.Errorf("new launcher launch error: %s", err)
		}
		crawler.browser = crawler.browser.ControlURL(controlUrl)
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
	if crawler.proxy == "" || (crawler.proxy != "" && !crawler.config.urlFromProxy) {
		if crawler.config.channel != nil {
			crawler.sendInfoChannel = crawler.config.channel
		} else if crawler.config.onRequest != nil {
			crawler.onRequest = crawler.config.onRequest
		} else {
			return utils.Error("No Send Info Channel to Get Crawler Result.")
		}
		crawler.createHijack()
	} else {
		if crawler.config.channel != nil {
			close(crawler.config.channel)
		}
	}
	crawler.pageSizedWaitGroup = utils.NewSizedWaitGroup(crawler.concurrent)
	return nil
}

func NewScreenShotCrawler(configOpts ...ConfigOpt) (*CrawlerX, error) {
	config := &Config{
		formFill:     make(map[string]string, 0),
		urlFromProxy: false,
	}

	for _, configOpt := range configOpts {
		configOpt(config)
	}

	crawlerX := &CrawlerX{

		browser:  rod.New(),
		pagePool: rod.NewPagePool(50),

		headers: config.headers,
		cookies: config.cookies,

		rootContext: context.Background(),

		blackList: config.blackList,
		whiteList: config.whiteList,

		formFill: DefaultFormFill,

		config: config,

		chromeWS: config.chromeWS,
	}
	if config.proxy != "" {
		crawlerX.proxy = config.proxy
		if config.proxyUsername != "" {
			crawlerX.proxyUsername = config.proxyUsername
			crawlerX.proxyPassword = config.proxyPassword
		}
	}
	err := crawlerX.screenShotInit()
	if err != nil {
		return nil, utils.Errorf("crawler module initial error: %s", err)
	}
	return crawlerX, nil
}

func (crawler *CrawlerX) screenShotInit() error {
	if crawler.chromeWS != "" {
		launch, err := launcher.NewManaged(crawler.chromeWS)
		if err != nil {
			return utils.Errorf("new launcher %s managed error: %s", crawler.chromeWS, err)
		}
		if crawler.proxy != "" {
			launch = launch.Proxy(crawler.proxy)
		}
		launch = launch.NoSandbox(true).Headless(true)
		crawler.browser = crawler.browser.Client(launch.MustClient())
	} else if crawler.proxy != "" {
		launch := launcher.New()
		launch = launch.Set(flags.ProxyServer, crawler.proxy)
		controlUrl, err := launch.Launch()
		if err != nil {
			return utils.Errorf("new launcher launch error: %s", err)
		}
		crawler.browser = crawler.browser.ControlURL(controlUrl)
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
	return nil
}
