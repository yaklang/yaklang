// Package newcrawlerx
// @Author bcy2007  2023/4/12 15:32
package newcrawlerx

import (
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

func (starter *BrowserStarter) vueClick(doGetUrl func(string, string) error) func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, selector string) error {
		//err := page.Navigate(originUrl)
		//if err != nil {
		//	return utils.Errorf("page navigate %s error: %s", originUrl, err)
		//}
		//page.MustWaitLoad()
		time.Sleep(time.Second)
		starter.clickElementOnPageBySelector(page, selector)
		currentUrl, err := getCurrentUrl(page)
		if err != nil {
			return utils.Error(err)
		}
		if currentUrl != "" && currentUrl != originUrl {
			doGetUrl(originUrl, currentUrl)
			err := page.Navigate(originUrl)
			if err != nil {
				return utils.Errorf("page navigate %s error: %s", originUrl, err)
			}
		}
		return nil
	}
}

func (starter *BrowserStarter) textGetUrl(doGetUrl func(string, string) error) func(*rod.Page) error {
	return func(page *rod.Page) error {
		originUrl, _ := getCurrentUrl(page)
		htmlText, err := page.HTML()
		if err != nil {
			log.Errorf("page %s get html info error: %s", page, err)
		}
		urls := analysisHtmlInfo(originUrl, htmlText)
		for _, url := range urls {
			doGetUrl(originUrl, url)
		}
		return nil
	}
}

func (starter *BrowserStarter) jsGetUrl() func(*rod.Page) error {
	return func(page *rod.Page) error {
		return nil
	}
}
