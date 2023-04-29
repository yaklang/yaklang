// Package newcrawlerx
// @Author bcy2007  2023/3/7 11:34
package newcrawlerx

import (
	"context"
	"github.com/apex/log"
	"os"
	"os/signal"
	"yaklang/common/crawlerx/filter"
	"yaklang/common/utils"
	"syscall"
	"time"
)

type CrawlerCore struct {
	targetUrl string

	manager *BrowserManager
	config  *Config

	uChan     *UChan
	ch        chan ReqInfo
	waitGroup *utils.SizedWaitGroup
}

func NewCrawler(targetUrl string, opts ...ConfigOpt) *CrawlerCore {
	config := NewConfig()
	ctx := context.Background()
	pageVisit, resultSent := filter.NewCountFilter(), filter.NewCountFilter()
	uChan, _ := NewUChan(128)
	urlTree := CreateTree(targetUrl)
	pageSizedWaitGroup := utils.NewSizedWaitGroup(20)
	opts = append(opts,
		WithTargetUrl(targetUrl),
		WithContext(ctx),
		WithPageVisitFilter(pageVisit),
		WithResultSentFilter(resultSent),
		WithUChan(uChan),
		WithUrlTree(urlTree),
		WithPageSizedWaitGroup(&pageSizedWaitGroup),
	)
	for _, opt := range opts {
		opt(config)
	}
	core := &CrawlerCore{
		targetUrl: targetUrl,
		config:    config,
		uChan:     uChan,
		ch:        config.baseConfig.ch,
		waitGroup: &pageSizedWaitGroup,
	}
	core.init()
	return core
}

func (core *CrawlerCore) init() {
	manager := NewBrowserManager(core.config)
	manager.CreateBrowserStarters()
	core.manager = manager
}

func (core *CrawlerCore) Run() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh)
	core.manager.Run()
	core.uChan.In <- core.targetUrl
	go func() {
		for {
			s := <-sigCh
			if s == syscall.SIGTERM || s == syscall.SIGINT {
				os.Exit(0)
			}
		}
	}()
	time.Sleep(time.Second)
	core.waitGroup.Wait()
	close(core.uChan.In)
	close(core.ch)
	log.Info("close UChan & channel.")
	time.Sleep(2 * time.Second)
	log.Info("core done.")
}
