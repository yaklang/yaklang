package simple

import (
	"context"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"runtime"
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
	leakless  bool

	runtimeID  string
	fromPlugin string
	saveToDB   bool
	sourceType string

	timeout int

	responseModification []*ResponseModification
	requestModification  []*RequestModification
}

// simulator.simple.createBrowser / simulator.simpleCreateBrowser 浏览器手动操作模式 进行目标页面操作
//
//	 可以添加零个或多个请求选项，用于对此次请求进行配置
//
//	 返回值为浏览器 可以创建页面
//
//	Example:
//	```
//	proxy = simulator.simple.proxy("http://127.0.0.1:7890") // 代理地址修改
//	exePath = simulator.simple.exePath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome") // 浏览器路径修改
//	timeout = simulator.simple.timeout(20)
//	browser, err = simulator.simple.CreateBrowser(exePath, timeout, proxy)
//	if err != nil {
//		return
//	}
//
//	```
func CreateHeadlessBrowser(opts ...BrowserConfigOpt) (*VBrowser, error) {
	config := &BrowserConfig{
		noSandBox: true,
		headless:  true,
		hijack:    false,
		leakless:  false,

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
		leakless:             config.leakless,
		timeout:              config.timeout,
		runtimeID:            config.runtimeID,
		fromPlugin:           config.fromPlugin,
		saveToDB:             config.saveToDB,
		sourceType:           config.sourceType,
		requestModification:  config.requestModification,
		responseModification: config.responseModification,
	}
	err := browser.browserInit()
	return browser, err
}

func (browser *VBrowser) browserInit() error {
	if browser.wsAddress != "" {
		ctx := context.Background()
		launch, err := launcher.NewManaged(browser.wsAddress)
		if err != nil {
			return utils.Errorf("new managed launcher %s error: %s", browser.wsAddress, err)
		}
		if browser.proxyAddress != "" {
			launch.Proxy(browser.proxyAddress)
		}
		launch = launch.Context(ctx).Set("disable-features", "HttpsUpgrades")
		
		// 在 Windows 上防止 Chrome 创建桌面快捷方式
		if strings.Contains(runtime.GOOS, "windows") {
			launch = launch.Set("no-first-run", "")
			launch = launch.Set("no-default-browser-check", "")
			launch = launch.Set("disable-default-apps", "")
		}
		
		launch = launch.NoSandbox(browser.noSandBox).Headless(browser.headless).Leakless(browser.leakless)
		serviceURL, header := launch.ClientHeader()
		client, err := cdp.StartWithURL(ctx, serviceURL, header)
		if err != nil {
			return utils.Errorf("start cdp client %s error: %s", serviceURL, err)
		}
		browser.browser = browser.browser.Client(client)
	} else {
		launch := launcher.New()
		if browser.exePath != "" {
			launch = launch.Bin(browser.exePath)
		}
		if browser.proxyAddress != "" {
			launch.Proxy(browser.proxyAddress)
		}
		launch = launch.Set("disable-features", "HttpsUpgrades")
		
		// 在 Windows 上防止 Chrome 创建桌面快捷方式
		if strings.Contains(runtime.GOOS, "windows") {
			launch = launch.Set("no-first-run", "")
			launch = launch.Set("no-default-browser-check", "")
			launch = launch.Set("disable-default-apps", "")
		}
		
		launch = launch.NoSandbox(browser.noSandBox).Headless(browser.headless).Leakless(browser.leakless)
		controlUrl, err := launch.Launch()
		if err != nil {
			return utils.Errorf("new launcher launch error: %s", err)
		}
		browser.browser = browser.browser.ControlURL(controlUrl)
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
		return browser.createHijack()
	}
	return nil
}

// Navigate 开启浏览器的一个页面 并跳转到对应url
//
// 其中第二个参数为 页面存在对应元素selector时即认为完成加载
//
//	 Example:
//	 ```
//	proxy = simulator.simple.proxy("http://127.0.0.1:7890") // 代理地址修改
//	exePath = simulator.simple.exePath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome") // 浏览器路径修改
//	timeout = simulator.simple.timeout(20)
//	browser, _ = simulator.simple.CreateBrowser(exePath, timeout, proxy)
//	selector = "#code"
//	// Navigate方法第二个参数不为空时 表示页面处于加载状态直到页面中出现css selector匹配到的元素后完成加载
//	page, err = browser.Navigate("https://example.com", selector)
//	// Navigate方法第二个参数为空时 页面正常通过document.readyState参数获取页面加载状态 完成加载等待
//	page, err = browser.Navigate("https://example.com", "")
//	if err != nil {
//		return
//	}
//
//	```
func (browser *VBrowser) Navigate(urlStr string, waitFor string) (*VPage, error) {
	page, err := browser.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, utils.Errorf("create page error: %v", err)
	}
	p := &VPage{page: page, timeout: browser.timeout}
	err = p.Navigate(urlStr, waitFor)
	if err != nil {
		return nil, utils.Errorf("navigate %v error: %v", urlStr, err)
	}
	return p, nil
}

// Close 关闭浏览器
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
