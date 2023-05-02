package core

import (
	"github.com/go-rod/rod/lib/proto"
	"time"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
)

func (crawler *CrawlerX) VisitUrl(urlStr string, depth int) error {
	//log.Infof("visit url: %s", urlStr)
	defer crawler.pageSizedWaitGroup.Done()

	page := crawler.GetPage(
		proto.TargetCreateTarget{URL: "about:blank"},
		depth,
	)
	if len(crawler.config.extraHeaders) != 0 {
		page.SetExtraHeaders(crawler.config.extraHeaders...)
	}

	page.SetTimeout(time.Duration(crawler.timeout) * time.Second)
	defer crawler.PutPage(page)

	err := page.Navigate(urlStr)
	if err != nil {
		return utils.Errorf("page %s navigate url %s error: %s", page, urlStr, err)
	}
	err = page.WaitLoad()
	if err != nil {
		return err
	}
	wait := page.MustWaitRequestIdle()
	wait()
	//page.WaitRequestIdle()

	html, _ := page.HTML()
	hashHtml := codec.Sha256(html)
	if crawler.htmlRecord.Exist(hashHtml) {
		return nil
	}
	crawler.htmlRecord.Insert(hashHtml)

	go page.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(page)
		},
	)()

	err = crawler.InputPage(page)
	if err != nil {
		return utils.Errorf("page %s input url error: %s", page, err)
	}
	err = crawler.ExtractUrl(page)
	if err != nil {
		return utils.Errorf("page %s extract url error: %s", page, err)
	}
	err = crawler.ExtractComment(page)
	if err != nil {
		return utils.Errorf("page %s extract comment error: %s", page, err)
	}
	err = crawler.ClickPage(page)
	if err != nil {
		return utils.Errorf("page %s click url error: %s", page, err)
	}
	return nil
}

func (crawler *CrawlerX) VisitPage(page *GeneralPage) error {
	//log.Infof("visit page url: %s", page.GetCurrentUrl())
	html, _ := page.HTML()
	hashHtml := codec.Sha256(html)
	if crawler.htmlRecord.Exist(hashHtml) {
		return nil
	}
	crawler.htmlRecord.Insert(hashHtml)

	err := crawler.InputPage(page)
	err = crawler.ExtractUrl(page)
	err = crawler.ExtractComment(page)
	err = crawler.ClickPage(page)
	if err != nil {
		return utils.Errorf("visit page % extract url error: %s", page, err)
	}
	return nil
}
