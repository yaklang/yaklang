// Package newcrawlerx
// @Author bcy2007  2023/5/23 11:20
package newcrawlerx

import (
	"github.com/go-rod/rod"
)

var invalidUrl = []string{"", "#", "javascript:;"}

func getUrl(page *rod.Page) ([]string, error) {
	urls := make([]string, 0)
	html, err := page.HTML()
	if err != nil {
		return urls, err
	}
	htmlInfo, err := page.Info()
	if err != nil {
		return urls, err
	}
	originUrl := htmlInfo.URL
	urlArr := analysisHtmlInfo(originUrl, html)
	//log.Info(urlArr)
	for _, urlStr := range urlArr {
		if StringSuffixList(urlStr, invalidSuffix) {
			continue
		}
		if StringArrayContains(invalidUrl, urlStr) {
			continue
		}
		urls = append(urls, urlStr)
	}
	//log.Info(urls)
	return urls, nil
}

func (starter *BrowserStarter) getUrl(page *rod.Page) ([]string, error) {
	urls := make([]string, 0)
	html, err := page.HTML()
	if err != nil {
		return urls, err
	}
	htmlInfo, err := page.Info()
	if err != nil {
		return urls, err
	}
	originUrl := htmlInfo.URL
	urlArr := analysisHtmlInfo(originUrl, html)
	for _, urlStr := range urlArr {
		if StringSuffixList(urlStr, invalidSuffix) {
			continue
		}
		if StringArrayContains(invalidUrl, urlStr) {
			continue
		}
		if !starter.scanRangeCheck(urlStr) {
			continue
		}
		urls = append(urls, urlStr)
	}
	return urls, nil
}
