// Package newcrawlerx
// @Author bcy2007  2023/4/12 15:32
package newcrawlerx

import (
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

func (starter *BrowserStarter) vueClick(doGetUrl func(string, string) error) func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, selector string) error {
		err := page.Navigate(originUrl)
		if err != nil {
			return utils.Errorf("page navigate %s error: %s", originUrl, err)
		}
		page.MustWaitLoad()
		time.Sleep(time.Second)
		clickElementOnPageBySelector(page, selector)
		currentUrl, _ := getCurrentUrl(page)
		if currentUrl != "" && currentUrl != originUrl {
			doGetUrl(originUrl, currentUrl)
		}
		return nil
	}
}
