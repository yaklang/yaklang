package core

import (
	"context"
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"time"
)

type GeneralPage struct {
	*rod.Page
	currentDepth int
	cleanup      func()
}

func (generalPage *GeneralPage) String() string {
	pageStr := generalPage.Page.String()
	return fmt.Sprintf("%s depth:%d>", pageStr[:len(pageStr)-1], generalPage.currentDepth)
}

func (generalPage *GeneralPage) SetContext(ctx context.Context) {
	generalPage.Page = generalPage.Context(ctx)
}

func (generalPage *GeneralPage) SetTimeout(d time.Duration) {
	generalPage.Page = generalPage.Timeout(d)
}

func (generalPage *GeneralPage) GetCurrentUrl() string {
	obj, err := generalPage.Eval(`()=>document.URL`)
	if err != nil {
		return ""
	}
	return obj.Value.Str()
}

func getCurrentUrl(page *rod.Page) string {
	obj, err := page.Eval(`()=>document.URL`)
	if err != nil {
		return ""
	}
	return obj.Value.Str()
}

func (generalPage *GeneralPage) CurrentDepth() int {
	return generalPage.currentDepth
}

func (generalPage *GeneralPage) StartListen() {
	generalPage.Eval(createObserver)
}

func (generalPage *GeneralPage) StopListen() string {
	result, err := generalPage.Eval(getObserverResult)
	if err != nil {
		return ""
	}
	return result.Value.Str()
}

func (generalPage *GeneralPage) GoDeeper() {
	generalPage.currentDepth++
}

func (generalPage *GeneralPage) GoBack() {
	//wait := generalPage.MustWaitRequestIdle()
	generalPage.NavigateBack()
	//generalPage.MustWaitLoad()
	//wait()
	generalPage.currentDepth--
}

func (generalPage *GeneralPage) History() int {
	value, err := generalPage.Eval(`()=>history.length`)
	if err != nil {
		return -1
	}
	return value.Value.Int()
}

func (generalPage *GeneralPage) SetExtraHeaders(headers ...string) {
	cleanup := generalPage.Page.MustSetExtraHeaders(headers...)
	generalPage.cleanup = cleanup
}

func (crawler *CrawlerX) PutPage(page *GeneralPage) {
	if page.cleanup != nil {
		page.cleanup()
	}
	crawler.putPage(page.Page)
}

func (crawler *CrawlerX) putPage(page *rod.Page) {
	page = page.CancelTimeout()
	crawler.pagePool.Put(page)
}

func (crawler *CrawlerX) GetPage(opts proto.TargetCreateTarget, depth int) *GeneralPage {
	create := func() *rod.Page {
		page, err := crawler.browser.Page(opts)
		if err != nil {
			return nil
		}
		return page
	}
	return &GeneralPage{
		Page:         crawler.pagePool.Get(create),
		currentDepth: depth,
	}
}
