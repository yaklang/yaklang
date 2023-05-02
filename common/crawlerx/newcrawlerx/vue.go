// Package newcrawlerx
// @Author bcy2007  2023/4/12 15:32
package newcrawlerx

import (
	"github.com/go-rod/rod"
	"time"
	"yaklang/common/log"
)

func (starter *BrowserStarter) vueClick(doGetUrl func(string, string) error) func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, selector string) error {
		page.Navigate(originUrl)
		page.WaitLoad()
		time.Sleep(time.Second)
		log.Infof("click %s: ", selector)
		clickElementOnPageBySelector(page, selector)
		currentUrl, _ := getCurrentUrl(page)
		if currentUrl != "" && currentUrl != originUrl {
			doGetUrl(originUrl, currentUrl)
		}
		return nil
	}
}
