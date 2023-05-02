// Package newcrawlerx
// @Author bcy2007  2023/3/7 16:47
package newcrawlerx

import (
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
)

func (starter *BrowserStarter) SinglePageCheck(urlStr string) error {
	starter.mainWaitGroup.Add()
	starter.StartBrowser()
	headlessBrowser := starter.browser
	p, _ := headlessBrowser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	starter.createPageHijack(p)
	p.Navigate(urlStr)
	starter.subWaitGroup.Wait()
next:
	for {
		select {
		case v := <-starter.uChan.Out:
			log.Info(v)
		default:
			starter.mainWaitGroup.Done()
			break next

		}
	}
	return nil
}

func (manager *BrowserManager) TestRun(urlStr string) {
	for _, starter := range manager.browsers {
		go starter.SinglePageCheck(urlStr)
	}
}
