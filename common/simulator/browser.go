// Package simulator
// @Author bcy2007  2023/8/17 16:17
package simulator

import (
	"context"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/cdp"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"runtime"
	"strings"
)

type BrowserStarter struct {
	browser *rod.Browser
	config  BrowserConfig

	ctx    context.Context
	cancel context.CancelFunc
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
	err := starter.browser.Connect()
	if err != nil {
		return err
	}
	err = starter.browser.IgnoreCertErrors(true)
	if err != nil {
		return err
	}
	return nil
}

func (starter *BrowserStarter) doLaunch(l *launcher.Launcher) *launcher.Launcher {
	if starter.config.proxy != nil {
		l = l.Proxy(starter.config.proxy.String())
	}
	l = l.NoSandbox(true).Headless(true)
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

func (starter *BrowserStarter) Close() error {
	return starter.browser.Close()
}
