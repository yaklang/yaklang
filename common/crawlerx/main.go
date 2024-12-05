// Package crawlerx
// @Author bcy2007  2023/7/14 10:44
package crawlerx

import (
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	_ "github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"context"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type CrawlerCore struct {
	targetUrl string

	manager *BrowserManager
	config  *Config

	uChan          *tools.UChan
	ch             chan ReqInfo
	waitGroup      *utils.SizedWaitGroup
	startWaitGroup *utils.SizedWaitGroup

	cancel func()
}

func NewCrawlerCore(targetUrl string, opts ...ConfigOpt) (*CrawlerCore, error) {
	config := NewConfig()
	pageVisit, resultSent := tools.NewCountFilter(), tools.NewCountFilter()
	uChan, err := tools.NewUChan(128)
	if err != nil {
		return nil, utils.Errorf(`Data channel create error: %s`, err)
	}
	waitGroup := utils.NewSizedWaitGroup(20)
	startWaitGroup := utils.NewSizedWaitGroup(20)
	ctx, cancel := context.WithCancel(context.Background())
	baseOpts := make([]ConfigOpt, 0)
	baseOpts = append(baseOpts,
		WithTargetUrl(targetUrl),
		WithContext(ctx),
		WithPageVisitFilter(pageVisit),
		WithResultSentFilter(resultSent),
		WithUChan(uChan),
		WithPageSizedWaitGroup(waitGroup),
		WithStartWaitGroup(startWaitGroup),
	)
	for _, opt := range baseOpts {
		opt(config)
	}
	for _, opt := range opts {
		opt(config)
	}
	browsers := config.browsers
	var proxy *url.URL
	if len(browsers) > 0 {
		proxy = browsers[0].proxyAddress
	} else {
		proxy = nil
	}
	var checkedUrl string
	if config.urlCheck {
		checkedUrl, err = TargetUrlCheck(targetUrl, proxy)
		if err != nil {
			cancel()
			return nil, utils.Errorf(`target url %s check failed: %s`, targetUrl, err)
		}
	} else {
		checkedUrl = targetUrl
	}
	if config.baseConfig.aiInputUrl != "" {
		err = connectTest(config.baseConfig.aiInputUrl, proxy)
		if err != nil {
			cancel()
			return nil, utils.Errorf(`ai input url %s check failed: %s`, config.baseConfig.aiInputUrl, err)
		}
	}
	WithTargetUrl(checkedUrl)(config)
	WithUrlTree(tools.CreateTree(checkedUrl))(config)
	core := CrawlerCore{
		targetUrl:      checkedUrl,
		config:         config,
		uChan:          uChan,
		ch:             config.baseConfig.ch,
		waitGroup:      waitGroup,
		startWaitGroup: startWaitGroup,
		cancel:         cancel,
	}
	err = core.init()
	if err != nil {
		return nil, utils.Errorf(`crawlerx core init error: %v`, err)
	}
	return &core, nil
}

func (core *CrawlerCore) init() error {
	manager := NewBrowserManager(core.config)
	manager.CreateBrowserStarters()
	core.manager = manager
	return nil
}

func (core *CrawlerCore) Start() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh)
	core.manager.Start()
	core.uChan.In <- core.targetUrl
	go func() {
		for {
			s := <-sigCh
			if s == syscall.SIGTERM || s == syscall.SIGINT {
				os.Exit(0)
			}
		}
	}()
	log.Debug(`[crawlerx core]starting wait...`)
	core.startWaitGroup.Wait()
	log.Debug(`[crawlerx core]started!`)
	time.Sleep(500 * time.Millisecond)
	core.waitGroup.Wait()
	core.cancel()
	close(core.uChan.In)
	close(core.ch)
	log.Debug(`Close uChan & channel.`)
	time.Sleep(2 * time.Second)
	log.Debug(`core done.`)
}

func (core *CrawlerCore) Test() {
	core.manager.Test()
	core.uChan.In <- core.targetUrl
	core.startWaitGroup.Wait()
	time.Sleep(500 * time.Millisecond)
	core.waitGroup.Wait()
	close(core.uChan.In)
	close(core.ch)
	time.Sleep(2 * time.Second)
}

// StartCrawler 开启一个无头浏览器模拟点击爬虫任务 第一个参数为目标url，后面可以添加零个或多个请求选项，用于对此次请求进行配置 返回值包括channel和错误，从channel中获取爬虫结果
//
// Examples:
// ```
//
//	targetUrl = "http://testphp.vulnweb.com/"
//	ch, err = crawlerx.StartCrawler(targetUrl, crawlerx.pageTimeout(30), crawlerx.concurrent(3))
//	for item = range ch {
//		yakit.Info(item.Method() + " " + item.Url())
//	}
//
// ```
func StartCrawler(url string, opts ...ConfigOpt) (chan ReqInfo, error) {
	ch := make(chan ReqInfo)
	opts = append(opts, WithResultChannel(ch))
	crawlerX, err := NewCrawlerCore(url, opts...)
	if err != nil {
		return nil, utils.Errorf(`Create crawler core error: %s`, err)
	}
	go crawlerX.Start()
	return ch, nil
}

func StartCrawlerTest(url string, opts ...ConfigOpt) (chan ReqInfo, error) {
	ch := make(chan ReqInfo)
	opts = append(opts, WithResultChannel(ch))
	crawlerX, err := NewCrawlerCore(url, opts...)
	if err != nil {
		return nil, utils.Errorf(`Create crawler core error: %s`, err)
	}
	go crawlerX.Test()
	return ch, nil
}

func TargetUrlCheck(targetUrl string, proxy *url.URL) (string, error) {
	var tempTargetUrl string
	if !strings.Contains(targetUrl, "://") {
		tempTargetUrl = "http://" + targetUrl
	} else {
		tempTargetUrl = targetUrl
	}
	r := CreateRequest()
	r.url = tempTargetUrl
	r.method = "GET"
	r.defaultHeaders = defaultChromeHeaders
	if proxy != nil {
		r.proxy = proxy
	}
	r.init()
	err := r.Request()
	if err != nil {
		return "", err
	}
	err = r.Do()
	if err != nil {
		return "", err
	}
	finalUrl := r.GetUrl()
	tempTargetObj, err := url.Parse(tempTargetUrl)
	if err != nil {
		return "", err
	}
	finalUrlObj, err := url.Parse(finalUrl)
	if err != nil {
		return "", err
	}
	if tempTargetObj.Scheme == finalUrlObj.Scheme && tempTargetObj.Host == finalUrlObj.Host {
		return tempTargetUrl, nil
	}
	return finalUrl, nil
}

func connectTest(targetUrl string, proxy *url.URL) error {
	opts := []poc.PocConfigOption{
		poc.WithTimeout(10),
	}
	if proxy != nil {
		opts = append(opts, poc.WithProxy(proxy.String()))
	}
	_, _, err := poc.DoGET(targetUrl, opts...)
	return err
}
