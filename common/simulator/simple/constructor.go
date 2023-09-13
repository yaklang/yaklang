package simple

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"net/url"
)

type VBrowser struct {
	browser *rod.Browser

	wsAddress     string
	proxyAddress  string
	proxyUsername string
	proxyPassword string

	noSandBox bool
	headless  bool

	responseModification []*ResponseModification
	requestModification  []*RequestModification
}

func CreateHeadlessBrowser(opts ...BrowserConfigOpt) *VBrowser {
	config := &BrowserConfig{
		noSandBox: true,
		headless:  true,

		responseModification: make([]*ResponseModification, 0),
		requestModification:  make([]*RequestModification, 0),
	}
	for _, opt := range opts {
		opt(config)
	}
	browser := &VBrowser{
		browser:              rod.New(),
		wsAddress:            config.wsAddress,
		proxyAddress:         config.proxyAddress,
		proxyUsername:        config.proxyUsername,
		proxyPassword:        config.proxyPassword,
		noSandBox:            config.noSandBox,
		headless:             config.headless,
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
	browser.browser.IgnoreCertErrors(true)
	browser.createHijack()
	return nil
}

func (browser *VBrowser) Navigate(urlStr string) *VPage {
	page, err := browser.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		log.Errorf("create page error: %s", err)
		return nil
	}
	p := &VPage{page: page}
	p.Navigate(urlStr)
	return p
}

func (browser *VBrowser) createHijack() error {
	hijackRouter := browser.browser.HijackRequests()
	hijackRouter.MustAdd("*", func(hijack *rod.Hijack) {
		for _, modify := range browser.requestModification {
			reg := modify.GetReg()
			if reg.MatchString(hijack.Request.URL().String()) {
				err := modify.Modify(hijack.Request)
				if err != nil {
					log.Error(err)
				}
			}
		}
		client := http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		transport := netx.NewDefaultHTTPTransport()
		if browser.proxyAddress != "" {
			proxyUrl, err := url.Parse(browser.proxyAddress)
			if err != nil {
				return
			}
			transport.Proxy = http.ProxyURL(proxyUrl)
		}
		client.Transport = transport

		err := hijack.LoadResponse(&client, true)
		if err != nil {
			log.Errorf("hijack load response error: %s", err)
			return
		}
		for _, modify := range browser.responseModification {
			reg := modify.GetReg()
			if reg.MatchString(hijack.Request.URL().String()) {
				err := modify.Modify(hijack.Response)
				if err != nil {
					log.Info(err)
				}
			}
		}
	})
	go func() {
		hijackRouter.Run()
	}()
	return nil
}
