// Package crawlerx
// @Author bcy2007  2023/7/13 11:10
package crawlerx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"time"
)

var invalidUrl = []string{"", "#", "javascript:;"}

func (starter *BrowserStarter) actionOnPage(page *rod.Page) error {
	originUrl, _ := getCurrentUrl(page)
	log.Infof(`Crawler on page %s`, originUrl)

	urls, err := starter.getUrls(page)
	if err != nil {
		return utils.Errorf(`Page %s get urls error: %s`, originUrl, err)
	}
	submitInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"submit",
			},
		},
		"button": {
			"type": {
				"submit",
			},
		},
	}
	submitElements, err := customizedGetElement(page, submitInfo)
	if err != nil {
		return utils.Errorf(`Page %s get submit elements error: %s`, originUrl, err)
	}
	inputElements, err := starter.getInputElements(page)
	if err != nil {
		return utils.Errorf(`Page %s get input elements error: %s`, originUrl, err)
	}
	for _, inputElement := range inputElements {
		starter.inputElementsExploit(inputElement)
	}
	if len(urls) == 0 && len(submitElements) == 0 {
		eventSelectors, err := starter.getEventElements(page)
		if err != nil {
			return utils.Errorf(`Page %s get event elements error: %s`, originUrl, err)
		}
		for _, eventSelector := range eventSelectors {
			starter.eventElementsExploit(page, originUrl, eventSelector)
		}
	} else {
		for _, url := range urls {
			starter.urlsExploit(originUrl, url)
		}
		clickSelectors, err := starter.getClickElements(page)
		if err != nil {
			return utils.Errorf(`Page %s get click elements error: %s`, originUrl, err)
		}
		for _, clickSelector := range clickSelectors {
			starter.clickElementsExploit(page, originUrl, clickSelector)
		}
	}
	return nil
}

func (starter *BrowserStarter) generateGetUrls() func(*rod.Page) ([]string, error) {
	return func(page *rod.Page) ([]string, error) {
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
		if starter.maxDepth != 0 {
			//log.Infof(`max depth: %d`, starter.maxDepth)
			currentNode := starter.urlTree.Find(originUrl)
			if currentNode == nil {
				log.Infof(`Origin url %s current node not found.`, originUrl)
			} else {
				log.Infof(`Current node %s depth: %d`, originUrl, currentNode.Level())
				if currentNode.Level() > starter.maxDepth {
					log.Infof(`Origin url %s reach max depth %d`, originUrl, starter.maxDepth)
					return urls, nil
				}
			}
		}
		urlArr := analysisHtmlInfo(originUrl, html)
		for _, urlStr := range urlArr {
			if StringSuffixList(urlStr, invalidSuffix) {
				continue
			}
			if StringArrayContains(invalidUrl, urlStr) {
				continue
			}
			if !starter.scanRange(urlStr) {
				continue
			}
			urls = append(urls, urlStr)
		}
		return urls, nil
	}
}

func (starter *BrowserStarter) generateGetClickElements() func(*rod.Page) ([]string, error) {
	return func(page *rod.Page) ([]string, error) {
		searchInfo := map[string]map[string][]string{
			"input": {
				"type": {
					"submit",
					"button",
				},
			},
			"button": {},
		}
		clickElements, err := customizedGetElement(page, searchInfo)
		if err != nil {
			return []string{}, utils.Errorf(`Page %s get click elements error: %s`, page, err)
		}
		return getElementsSelectors(clickElements), nil
	}
}

func (starter *BrowserStarter) generateGetInputElements() func(*rod.Page) (rod.Elements, error) {
	return func(page *rod.Page) (rod.Elements, error) {
		status, _, err := page.Has("input")
		if err != nil {
			return nil, utils.Errorf("Page %s detect input element error: %s", page, err)
		}
		if !status {
			return nil, nil
		}
		inputs, err := page.Elements("input")
		if err != nil {
			return nil, utils.Errorf("Page %s get input elements error: %s", page, err)
		}
		return inputs, nil
	}
}

func (starter *BrowserStarter) generateGetEventElements() func(*rod.Page) ([]string, error) {
	return func(page *rod.Page) ([]string, error) {
		results := make([]string, 0)
		clickableElementObjs, err := proto.RuntimeEvaluate{
			IncludeCommandLineAPI: true,
			ReturnByValue:         true,
			Expression:            getClickEventElement,
		}.Call(page)
		if err != nil {
			return results, utils.Errorf("Page %s get click event listener elements error: %s", page, err)
		}
		clickableElementArr := clickableElementObjs.Result.Value.Arr()
		for _, element := range clickableElementArr {
			results = append(results, element.String())
		}
		return results, nil
	}
}

func (starter *BrowserStarter) generateUrlsExploit() func(string, string) error {
	return func(originUrl string, targetUrl string) error {
		if starter.stopSignal {
			return nil
		}
		for _, f := range starter.urlCheck {
			afterUrl := starter.urlAfterRepeat(targetUrl)
			if !f(afterUrl) {
				//log.Infof(`%s ban url: %s`, k, targetUrl)
				return nil
			}
		}
		starter.urlTree.Add(originUrl, targetUrl)
		starter.uChan.In <- targetUrl
		return nil
	}
}

func (starter *BrowserStarter) generateClickElementsExploit() func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, clickSelector string) error {
		status := starter.clickElementOnPageBySelector(page, clickSelector)
		if !status {
			return nil
		}
		currentUrl, _ := getCurrentUrl(page)
		// analysis page after click
		if currentUrl != "" && currentUrl != originUrl {
			urls, err := starter.getUrls(page)
			if err != nil {
				log.Errorf(`Page %s get urls error: %s`, originUrl, err)
			} else {
				for _, url := range urls {
					starter.urlsExploit(originUrl, url)
				}
			}
			page.NavigateBack()
			time.Sleep(time.Second)
		}
		return nil
	}
}

func (starter *BrowserStarter) generateInputElementsExploit() func(*rod.Element) error {
	return func(element *rod.Element) error {
		attribute, _ := getAttribute(element, "type")
		switch attribute {
		case "text", "password":
			keywordStr := getAllKeywords(element)
			for k, v := range starter.formFill {
				if strings.Contains(keywordStr, k) {
					return element.Input(v)
				}
			}
			return element.Input("test")
		case "file":
			return starter.defaultUploadFile(element)
		case "radio", "checkbox":
			return element.Click(proto.InputMouseButtonLeft)
		default:
			return utils.Errorf("unknown attribute: %s", attribute)
		}
	}
}

func (starter *BrowserStarter) generateEventElementsExploit() func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, eventSelector string) error {
		err := page.Navigate(originUrl)
		if err != nil {
			return utils.Errorf("page navigate %s error: %s", originUrl, err)
		}
		page.MustWaitLoad()
		time.Sleep(time.Second)
		status := starter.clickElementOnPageBySelector(page, eventSelector)
		if !status {
			return nil
		}
		currentUrl, _ := getCurrentUrl(page)
		if currentUrl != "" && currentUrl != originUrl {
			checkUrl := currentUrl
			if starter.urlAfterRepeat != nil {
				checkUrl = starter.urlAfterRepeat(checkUrl)
			}
			if !starter.resultSent(checkUrl) {
				return nil
			}
			result := SimpleResult{
				url:        currentUrl,
				resultType: "event url",
				method:     "EVENT GET",
				from:       originUrl,
			}
			starter.ch <- &result
			starter.urlsExploit(originUrl, currentUrl)
		}
		return nil
	}
}

func (starter *BrowserStarter) defaultUploadFile(element *rod.Element) error {
	if len(starter.fileUpload) == 0 {
		return utils.Errorf("no upload file set.")
	}
	keywords := getAllKeywords(element)
	for k, v := range starter.fileUpload {
		if strings.Contains(keywords, k) {
			return testUploadFile(element, v)
		}
	}
	defaultPath, ok := starter.fileUpload["default"]
	if !ok {
		return utils.Errorf("no default upload file set.")
	}
	return testUploadFile(element, defaultPath)
}

func testUploadFile(element *rod.Element, filePath string) error {
	return element.SetFiles([]string{filePath})
}
