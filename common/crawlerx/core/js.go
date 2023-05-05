package core

import (
	"net/url"
	"regexp"
	"strings"
)

func (crawler *CrawlerX) JsInfoMatch(baseUrl, jsHtml string) {
	urlIns, _ := url.Parse(baseUrl)
	for _, regStr := range jsUrlRegExps {
		reg, _ := regexp.Compile(regStr)
		urls := reg.FindAllStringSubmatch(jsHtml, -1)
		urlStrList := make([]string, 0)
		for _, url := range urls {
			if len(url) < 2 {
				continue
			}
			urlStr := url[1]
			if !strings.HasPrefix(urlStr, "http") {
				tempIns, err := urlIns.Parse(urlStr)
				if err != nil {
					continue
				}
				urlStr = tempIns.String()
			}
			urlStrList = append(urlStrList, urlStr)
		}
		crawler.SimpleCheckSend(urlStrList...)
	}
}
