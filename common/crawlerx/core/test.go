package core

import (
	"context"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/filter"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"strings"
)

type SimpleTestCrawler struct {
	base      string
	browser   *rod.Browser
	urls      *filter.StringCountFilter
	waitGroup utils.SizedWaitGroup
	context   context.Context
	config    *Config
}

func (crawler *SimpleTestCrawler) Init() {
	crawler.browser = rod.New()
	crawler.browser.Connect()

	crawler.urls = filter.NewCountFilter()

	crawler.waitGroup = utils.NewSizedWaitGroup(20)

	crawler.context = context.Background()
	crawler.CreateHijack()
}

func (crawler *SimpleTestCrawler) CreateHijack() {
	hijackRouter := crawler.browser.HijackRequests()
	hijackRouter.MustAdd("*", func(hijack *rod.Hijack) {
		client := http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		defer hijack.LoadResponse(&client, true)
		urlRaw := hijack.Request.URL()
		urlStr := urlRaw.String()
		if crawler.urls.Exist(urlStr) {
			return
		} else {
			crawler.urls.Insert(urlStr)
		}
		log.Info(urlStr)
	})
	go func() {
		hijackRouter.Run()
	}()
}

func (crawler *SimpleTestCrawler) NewPageDetectTest(urlStr string) {
	crawler.base = urlStr
	go crawler.browser.EachEvent(
		func(e *proto.TargetTargetCreated) {
			go func() {
				defer crawler.waitGroup.Done()
				//defer page.Close()
				targetID := e.TargetInfo.TargetID
				page, _ := crawler.browser.PageFromTarget(targetID)
				defer page.Close()
				crawler.inputPage(page)
				crawler.clickPage(page)
				crawler.callPage(page)

			}()
		},
	)()
	crawler.waitGroup.AddWithContext(crawler.context)
	page, err := crawler.browser.Page(proto.TargetCreateTarget{URL: urlStr})
	if err != nil {
		log.Info("create page %s error: %s", page, err)
		return
	}
	go page.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(page)
		},
	)()
	crawler.waitGroup.Wait()
	log.Info("done")
}

func (crawler *SimpleTestCrawler) callPage(page *rod.Page) {
	//defer page.Close()
	err := page.WaitLoad()
	if err != nil {
		log.Info(err, " when visit page: %s", page)
		return
	}
	//origin := page.MustInfo().URL
	//log.Infof("check page: %s", origin)
	urlObj, _ := page.Eval(findHref)
	urlArr := urlObj.Value.Arr()
	for _, url := range urlArr {
		urlStr := url.Str()
		if !strings.Contains(urlStr, crawler.base) {
			continue
		}
		if crawler.urls.Exist(urlStr) {
			continue
		} else {
			crawler.urls.Insert(urlStr)
		}
		log.Info(urlStr)
		//log.Infof("find url: %s from %s", urlStr, origin)
		go func() {
			//crawler.urls.Insert(urlStr)
			crawler.waitGroup.AddWithContext(crawler.context)
			p, err := crawler.browser.Page(proto.TargetCreateTarget{URL: urlStr})
			if err != nil {
				log.Info("create sub page %s error: %s", page, err)
				return
			}
			go p.EachEvent(
				func(e *proto.PageJavascriptDialogOpening) {
					_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(p)
				},
			)()
			//log.Infof("call page %s: url: %s", p, urlStr)
		}()
	}
}

func (crawler *SimpleTestCrawler) inputPage(page *rod.Page) {
	err := page.WaitLoad()
	if err != nil {
		log.Info(err, " when visit page: %s", page)
		return
	}
	status, _, err := page.Has("input")
	if err != nil {
		log.Info("input page %s error: %s", page, err)
		return
	}
	if !status {
		return
	}
	elements, err := page.Elements("input")
	if err != nil {
		return
	}
	for _, element := range elements {
		visible, err := element.Visible()
		if err != nil || !visible {
			continue
		}
		elementType := getAttribute(element, "type")
		switch elementType {
		case "text", "password":
			doTextInput(element)
		case "radio", "checkbox":
			element.Click(proto.InputMouseButtonLeft)
		case "submit":
			continue
		default:
			doTextInput(element)
		}
	}
}

func (crawler *SimpleTestCrawler) clickPage(page *rod.Page) {
	originURL := getCurrentUrl(page)
	buttonSelectors := getButtonSelectors(page)
	for _, buttonSelector := range buttonSelectors {
		clickButton(page, buttonSelector)
		currentURL := getCurrentUrl(page)
		if originURL != "" && originURL != currentURL {
			//crawler.callPage(page)
			page.NavigateBack()
			if getCurrentUrl(page) != originURL {
				page.NavigateBack()
			}
		}
	}
}

func getAttribute(element *rod.Element, attribute string) string {
	attributeStr, err := element.Attribute(attribute)
	if err != nil {
		log.Info("element %s get attribute error: %s", element, err)
		return ""
	}
	if attributeStr == nil {
		return ""
	}
	return *attributeStr
}

func doTextInput(element *rod.Element) error {
	keywords := GetAllKeyWords(element)
	for k, v := range DefaultFormFill {
		if strings.Contains(keywords, k) {
			element.Type([]input.Key(v)...)
			return nil
		}
	}
	runeStr := []input.Key("test")
	element.Type(runeStr...)
	return nil
}
