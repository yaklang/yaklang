package core

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/url"
	"strings"
)

func (crawler *CrawlerX) ExtractUrl(page *GeneralPage) error {
	urlObj, err := page.Eval(findHref)
	if err != nil {
		return utils.Errorf("page %s find href & src error: %s", page, err)
	}
	urlArr := urlObj.Value.Arr()
	for _, urlRaw := range urlArr {
		urlStr := urlRaw.Str()
		if urlStr == "" {
			continue
		}
		if !crawler.checkRangeValid(urlStr) {
			continue
		}
		if !crawler.CheckValidSuffix(urlStr) {
			continue
		}
		if !crawler.CheckValidHost(urlStr) {
			continue
		}
		if crawler.checkDanger != nil && crawler.checkDanger(urlStr) {
			fmt.Printf("%s checked!\n", urlStr)
			continue
		}
		subUrls := crawler.SubmitCutUrl(urlStr)
		crawler.SimpleCheckSend(subUrls...)
		if page.CurrentDepth() >= crawler.maxDepth {
			repeatStr := crawler.checkRepeat(urlStr, "get")
			hashStr := codec.Sha256(repeatStr)
			if crawler.sent.Exist(hashStr) {
				continue
			}
			crawler.sent.Insert(hashStr)
			req := &SimpleRequest{}
			req.url = urlStr
			if crawler.onRequest != nil {
				crawler.onRequest(req)
			} else if crawler.sendInfoChannel != nil {
				crawler.sendInfoChannel <- req
			} else {
				log.Infof("get url: %s without request", req.Url())
			}

			if crawler.urlCount != 0 && crawler.sent.Count() >= int64(crawler.urlCount) {
				crawler.cancelFunc()
				return nil
			}
			continue
		} else {
			repeatStr := crawler.checkRepeat(urlStr, "get")
			hashStr := codec.Sha256(repeatStr)
			if crawler.visited.Exist(hashStr) {
				continue
			}
			crawler.visited.Insert(hashStr)
			go func() {
				crawler.pageSizedWaitGroup.AddWithContext(crawler.rootContext)
				e := crawler.VisitUrl(urlStr, page.CurrentDepth()+1)
				if e != nil {
					log.Infof("visit url %s error: %s", urlStr, e)
				}
			}()
		}
	}
	return nil
}

func (crawler *CrawlerX) ExtractComment(page *GeneralPage) error {
	urlObj, err := page.Eval(CommentMatch)
	if err != nil {
		return utils.Errorf("page %s find comment href & src error: %s", page, err)
	}
	resultList := make([]string, 0)
	urlArr := urlObj.Value.Arr()
	currentUrl := page.GetCurrentUrl()
	urlIns, _ := url.Parse(currentUrl)
	for _, urlRaw := range urlArr {
		urlStr := urlRaw.Str()
		if !strings.HasPrefix(urlStr, "http") {
			tempIns, err := urlIns.Parse(urlStr)
			if err != nil {
				continue
			}
			urlStr = tempIns.String()
		}
		resultList = append(resultList, urlStr)
	}
	crawler.SimpleCheckSend(resultList...)
	return nil
}
