// Package simulator
// @Author bcy2007  2023/8/17 16:17
package simulator

import (
	"context"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"runtime"
	"strings"
	"time"
)

type BrowserStarter struct {
	browser *rod.Browser
	config  BrowserConfig

	ctx    context.Context
	cancel context.CancelFunc
	ready  bool
}

func CreateNewStarter(opts ...BrowserConfigOpt) *BrowserStarter {
	config := CreateNewBrowserConfig()
	for _, opt := range opts {
		opt(config)
	}
	starter := BrowserStarter{
		browser: rod.New(),
		config:  *config,
	}
	starter.init()
	return &starter
}

func (starter *BrowserStarter) init() {
	ctx, cancel := context.WithCancel(context.Background())
	starter.ctx = ctx
	starter.cancel = cancel
}

func (starter *BrowserStarter) Start() error {
	if starter.config.wsAddress == "" {
		launch := launcher.New()
		if starter.config.exePath != "" {
			launch = launch.Bin(starter.config.exePath)
		}
		launch = starter.doLaunch(launch)
		controlUrl, err := launch.Launch()
		if err != nil {
			return err
		}
		starter.browser = starter.browser.ControlURL(controlUrl)
	} else {
		launch, err := launcher.NewManaged(starter.config.wsAddress)
		if err != nil {
			return err
		}
		ctx := context.Background()
		launch = launch.Context(ctx)
		launch = starter.doLaunch(launch)
		serviceUrl, header := launch.ClientHeader()
		client, err := cdp.StartWithURL(ctx, serviceUrl, header)
		if err != nil {
			return err
		}
		starter.browser = starter.browser.Client(client)
	}
	starter.browser = starter.browser.Context(starter.ctx)
	if err := starter.browser.Connect(); err != nil {
		return err
	}
	starter.ready = true
	if err := starter.browser.IgnoreCertErrors(true); err != nil {
		return err
	}
	if err := starter.createBrowserHijack(); err != nil {
		return err
	}
	return nil
}

func (starter *BrowserStarter) doLaunch(l *launcher.Launcher) *launcher.Launcher {
	if starter.config.proxy != nil {
		l = l.Proxy(starter.config.proxy.String())
	}
	l = l.NoSandbox(true).Set(flags.Headless, "new").Set("disable-features", "HttpsUpgrades")
	
	// 在 Windows 上防止 Chrome 创建桌面快捷方式
	if strings.Contains(runtime.GOOS, "windows") {
		l = l.Set("no-first-run", "")
		l = l.Set("no-default-browser-check", "")
		l = l.Set("disable-default-apps", "")
	}
	
	if starter.config.leakless == LeaklessOff {
		l = l.Leakless(false)
	} else if starter.config.leakless == LeaklessDefault && strings.Contains(runtime.GOOS, "windows") {
		l = l.Leakless(false)
	} else if starter.config.leakless == LeaklessOn {
		l = l.Leakless(true)
	}
	return l
}

func (starter *BrowserStarter) CreatePage() (*rod.Page, error) {
	page, err := starter.browser.Page(
		proto.TargetCreateTarget{URL: "about:blank"},
	)
	if err != nil {
		return nil, err
	}
	return page, nil
}

func (starter *BrowserStarter) createBrowserHijack() error {
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithTimeout(30 * time.Second),
		lowhttp.WithSaveHTTPFlow(starter.config.saveToDB),
		lowhttp.WithSource(starter.config.sourceType),
	}
	if starter.config.proxy != nil {
		opts = append(opts, lowhttp.WithProxy(starter.config.proxy.String()))
	}
	if starter.config.runtimeID != "" {
		opts = append(opts, lowhttp.WithRuntimeId(starter.config.runtimeID))
	}
	if starter.config.fromPlugin != "" {
		opts = append(opts, lowhttp.WithFromPlugin(starter.config.fromPlugin))
	}
	router := crawlerx.NewBrowserHijackRequests(starter.browser)
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

func (starter *BrowserStarter) Close() error {
	if !starter.ready || starter.browser == nil {
		return nil
	}
	return starter.browser.Close()
}
