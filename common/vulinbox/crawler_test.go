package vulinbox

import (
	"context"
	"github.com/yaklang/yaklang/common/crawler"
	"github.com/yaklang/yaklang/common/log"
	"strings"
	"sync"
	"testing"
)

func reqOnce(target string, onReq func(req *crawler.Req)) {
	crawler, err := crawler.NewCrawler(target, crawler.WithOnRequest(onReq))
	if err != nil {
		panic(err)
	}
	err = crawler.Run()
	if err != nil {
		log.Error(err)
	}
}

// TestCrawler 测试对 vulbox 爬取多次得到的 urls 是否相同
func TestCrawler(t *testing.T) {
	vulboxAdderss, err := NewVulinServer(context.Background())
	if err != nil {
		panic(err)
	}
	crawlerUrls := make([]map[string]struct{}, 0)
	for range make([]int, 10) {
		us := make(map[string]struct{})
		mux := sync.Mutex{}
		reqOnce(vulboxAdderss, func(req *crawler.Req) {
			mux.Lock()
			defer mux.Unlock()
			if strings.Contains(req.Url(), "tn=baidu") {
				return
			}
			us[req.Url()] = struct{}{}
		})
		crawlerUrls = append(crawlerUrls, us)
	}
	var baseResult map[string]struct{}
	if len(crawlerUrls) > 0 {
		baseResult = crawlerUrls[0]
		crawlerUrls = crawlerUrls[1:]
	}
	for _, url := range crawlerUrls {
		if len(url) != len(baseResult) {
			panic("crawler unstable(total)")
		}
		for k, _ := range baseResult {
			if _, ok := url[k]; !ok {
				panic("crawler unstable(in BaseResult)")
			}
		}
	}
}
