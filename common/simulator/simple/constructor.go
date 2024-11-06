package simple

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"strings"
	"time"
)

type VBrowser struct {
	browser *rod.Browser

	exePath       string
	wsAddress     string
	proxyAddress  string
	proxyUsername string
	proxyPassword string

	noSandBox bool
	headless  bool
	hijack    bool

	runtimeID  string
	fromPlugin string
	saveToDB   bool
	sourceType string

	timeout int

	responseModification []*ResponseModification
	requestModification  []*RequestModification
}

func CreateHeadlessBrowser(opts ...BrowserConfigOpt) *VBrowser {
	config := &BrowserConfig{
		noSandBox: true,
		headless:  true,
		hijack:    false,

		timeout: 30,

		responseModification: make([]*ResponseModification, 0),
		requestModification:  make([]*RequestModification, 0),
	}
	for _, opt := range opts {
		opt(config)
	}
	browser := &VBrowser{
		browser:              rod.New(),
		exePath:              config.exePath,
		wsAddress:            config.wsAddress,
		proxyAddress:         config.proxyAddress,
		proxyUsername:        config.proxyUsername,
		proxyPassword:        config.proxyPassword,
		noSandBox:            config.noSandBox,
		headless:             config.headless,
		hijack:               config.hijack,
		timeout:              config.timeout,
		runtimeID:            config.runtimeID,
		fromPlugin:           config.fromPlugin,
		saveToDB:             config.saveToDB,
		sourceType:           config.sourceType,
		requestModification:  config.requestModification,
		responseModification: config.responseModification,
	}
	browser.BrowserInit()
	return browser
}

func (browser *VBrowser) BrowserInit() error {
	if browser.wsAddress != "" {
		launch, err := launcher.NewManaged(browser.wsAddress)
		if err != nil {
			return utils.Errorf("new managed launcher %s error: %s", browser.wsAddress, err)
		}
		if browser.proxyAddress != "" {
			launch.Proxy(browser.proxyAddress)
		}
		launch.NoSandbox(browser.noSandBox).Headless(browser.headless)
		browser.browser.Client(launch.MustClient())
	} else {
		launch := launcher.New()
		if browser.exePath != "" {
			launch = launch.Bin(browser.exePath)
		}
		if browser.proxyAddress != "" {
			launch.Proxy(browser.proxyAddress)
		}
		launch.NoSandbox(browser.noSandBox).Headless(browser.headless)
		controlUrl, err := launch.Launch()
		if err != nil {
			return utils.Errorf("new launcher launch error: %s", err)
		}
		browser.browser.ControlURL(controlUrl)
	}
	err := browser.browser.Connect()
	if err != nil {
		return utils.Errorf("browser connect error: %s", err)
	}
	if browser.proxyUsername != "" {
		go browser.browser.MustHandleAuth(browser.proxyUsername, browser.proxyPassword)()
	}
	_ = browser.browser.IgnoreCertErrors(true)
	if browser.hijack {
		browser.createHijack()
	}
	return nil
}

func (browser *VBrowser) Navigate(urlStr string, waitFor string) *VPage {
	page, err := browser.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		log.Errorf("create page error: %s", err)
		return nil
	}
	p := &VPage{page: page, timeout: browser.timeout}
	err = p.Navigate(urlStr, waitFor)
	if err != nil {
		log.Errorf("navigate error: %s", err)
		return nil
	}
	return p
}

func (browser *VBrowser) Close() error {
	return browser.browser.Close()
}

func (browser *VBrowser) createHijack() error {
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithTimeout(30 * time.Second),
		lowhttp.WithSaveHTTPFlow(browser.saveToDB),
		lowhttp.WithSource(browser.sourceType),
	}
	if browser.proxyAddress != "" {
		opts = append(opts, lowhttp.WithProxy(browser.proxyAddress))
	}
	if browser.runtimeID != "" {
		opts = append(opts, lowhttp.WithRuntimeId(browser.runtimeID))
	}
	if browser.fromPlugin != "" {
		opts = append(opts, lowhttp.WithFromPlugin(browser.fromPlugin))
	}
	router := crawlerx.NewBrowserHijackRequests(browser.browser)
	err := router.Add("*", "", func(hijack *crawlerx.CrawlerHijack) {
		contentType := hijack.Request.Header("Content-Type")
		if strings.Contains(contentType, "multipart/form-data") {
			hijack.ContinueRequest(&proto.FetchContinueRequest{})
			return
		}
		tempOpts := make([]lowhttp.LowhttpOpt, 0)
		tempOpts = append(tempOpts, opts...)
		url := hijack.Request.URL().String()
		if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "wss://") {
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
		return
	})
	if err != nil {
		return utils.Errorf(`create hijack error: %v`, err.Error())
	}
	go func() {
		router.Run()
	}()
	return nil
}
