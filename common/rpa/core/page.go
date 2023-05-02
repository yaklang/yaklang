package core

import (
	"strings"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func (m *Manager) WaitLoad(page *rod.Page) {
	page.MustWaitLoad()
	//defer page.Close()
	// defer page.MustClose()
	// defer m.PutPage(page)
	//defer m.PutPage(page)
	var lastHtmlHash = ""
	var exitThroshold = 0
	var totalFetcher = 0
	var htmlStr string
	for {
		totalFetcher++
		if totalFetcher > 10 {
			break
		}

		if exitThroshold >= 2 {
			break
		}
		time.Sleep(500 * time.Millisecond)
		htmlStr, err := page.HTML()
		if err != nil {
			log.Errorf("fetch html failed: %s", err)
		}

		if lastHtmlHash == "" && htmlStr == "" {
			exitThroshold++
			continue
		}

		// 有效缓存
		if lastHtmlHash == "" && htmlStr != "" {
			lastHtmlHash = codec.Sha256(htmlStr)
			exitThroshold = 0
			continue
		}

		// 重复三次
		if lastHtmlHash == codec.Sha256(htmlStr) {
			exitThroshold++
			continue
		} else {
			exitThroshold = 0
			lastHtmlHash = codec.Sha256(htmlStr)
		}
	}

	if htmlStr == "" {
		log.Error("page empty")
	}

}

func (m *Manager) page(url string, depth int) error {

	defer m.pageSizedWaitGroup.Done()

	page_block, err := m.GetPage(proto.TargetCreateTarget{
		URL: "about:blank",
	}, depth)
	page := page_block.page.Timeout(time.Duration(m.config.timeout) * time.Second)
	// page := page_block.page
	if err != nil {
		return utils.Errorf("get blank page[%v] failed: %s", url, err)
	}
	if page == nil {
		return utils.Errorf("get blank page[%v] nil: %s", url, err)
	}
	// 页面正常退出
	// defer page.Close()
	defer m.PutPage(page)
	// page.Navigate(url)
	err = page.Navigate(url)
	if err != nil {
		return utils.Errorf("navigate page[%v] failed: %s", url, err)
	}
	page = page.Context(m.rootContext)

	// m.visitedUrl.Insert(url)

	// page.MustWaitLoad()
	// time.Sleep(1 * time.Second)
	wait_count := 0
	for {
		state, err := m.GetReadyState(page)
		if err != nil {
			return utils.Errorf("get ready state error: %s", err)
		}
		if state == "complete" {
			break
		}
		wait_count++
		if wait_count > 20 {
			log.Infof("wait ready state too long error: %s.", state)
			break
		}
		time.Sleep(1 * time.Second)
	}
	var lastHtmlHash = ""
	var exitThroshold = 0
	var totalFetcher = 0
	var htmlStr string
	for {
		totalFetcher++
		if totalFetcher > 10 {
			break
		}

		if exitThroshold >= 2 {
			break
		}
		time.Sleep(500 * time.Millisecond)
		htmlStr, err = page.HTML()
		if err != nil && !strings.Contains(err.Error(), "context canceled") {
			log.Errorf("fetch [%v]'s html failed: %s", url, err)
		}

		if lastHtmlHash == "" && htmlStr == "" {
			exitThroshold++
			continue
		}

		// 有效缓存
		if lastHtmlHash == "" && htmlStr != "" {
			lastHtmlHash = codec.Sha256(htmlStr)
			exitThroshold = 0
			continue
		}

		// 重复三次
		if lastHtmlHash == codec.Sha256(htmlStr) {
			exitThroshold++
			continue
		} else {
			exitThroshold = 0
			lastHtmlHash = codec.Sha256(htmlStr)
		}
	}

	if htmlStr == "" {
		// log.Error("page empty")
		return nil
	}

	//存储响应到chan
	// 到这里认为页面加载差不多了
	// 下面要去做别的事情，比如：
	//   1. 获取页面可点击的链接
	//   2. 获取页面可处理的表格
	// page.MustScreenshot("test.png")

	err = m.extractInput(page)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		log.Errorf("extract blanks and do input failed: %s", err)
	}

	err = m.extractUrls(page_block)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		log.Errorf("extract urls failed: %s", err)
	}

	err = m.extractCommit(page_block)
	if err != nil && !strings.Contains(err.Error(), "context canceled") {
		log.Errorf("extract commit failed: %s", err)
	}

	return nil
}
