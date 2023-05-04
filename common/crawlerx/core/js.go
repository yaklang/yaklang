package core

import (
	"net/url"
	"regexp"
	"strings"
)

func (crawler *CrawlerX) JsInfoMatch(baseUrl, jsHtml string) {
	//log.Info(jsHtml)
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
			//log.Info(urlStr)
			//repeatStr := crawler.checkRepeat(urlStr, "get")
			//hashStr := codec.Sha256(repeatStr)
			//if crawler.visited.Exist(hashStr) {
			//	continue
			//}
			//crawler.visited.Insert(hashStr)
			//go func() {
			//	crawler.pageSizedWaitGroup.AddWithContext(crawler.rootContext)
			//	crawler.VisitUrl(urlStr, 0)
			//}()
		}
		crawler.SimpleCheckSend(urlStrList...)
	}
}
