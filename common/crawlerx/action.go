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

var invalidUrl = []string{"", "#", "javascript:;", "#/"}

const findHref = `() => {
    let nodes = document.createNodeIterator(document.getRootNode())
    let hrefs = [];
    let node;
    while ((node = nodes.nextNode())) {
        let {href} = node;
        if (href) {
            hrefs.push(href)
        }
    }
    return hrefs
}`

func (starter *BrowserStarter) actionOnPage(page *rod.Page) error {
	originUrl, _ := getCurrentUrl(page)
	log.Debugf(`Crawler on page %s`, originUrl)

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
		err = starter.inputElementsExploit(inputElement)
		if err != nil {
			return utils.Errorf(`Page %v input element %v error: %v`, originUrl, inputElement, err.Error())
		}
	}
	if len(urls) == 0 && len(submitElements) == 0 {
		eventSelectors, err := starter.getEventElements(page)
		if err != nil {
			return utils.Errorf(`Page %s get event elements error: %s`, originUrl, err)
		}
		for _, eventSelector := range eventSelectors {
			err = starter.eventElementsExploit(page, originUrl, eventSelector)
			if err != nil {
				return utils.Errorf(`Page %v click element %v error: %v`, originUrl, eventSelector, err.Error())
			}
		}
	} else {
		for _, url := range urls {
			if starter.banList.Exist(url) {
				continue
			}
			err = starter.urlsExploit(originUrl, url)
			if err != nil {
				return utils.Errorf(`Url %v from %v exploit error: %v`, url, originUrl, err.Error())
			}
		}
		clickSelectors, err := starter.getClickElements(page)
		if err != nil {
			return utils.Errorf(`Page %s get click elements error: %s`, originUrl, err)
		}
		for _, clickSelector := range clickSelectors {
			err = starter.clickElementsExploit(page, originUrl, clickSelector)
			if err != nil {
				return utils.Errorf(`Page %v click selector %v error: %v`, originUrl, clickSelector, err.Error())
			}
		}
	}
	return nil
}

func (starter *BrowserStarter) normalActionOnPage(page *rod.Page) error {
	var err error
	originUrl, _ := getCurrentUrl(page)
	urls, err := starter.getUrls(page)
	if err != nil {
		return utils.Errorf(`Page %s get urls error: %s`, originUrl, err)
	}
	err = starter.doInput(originUrl, page)
	if err != nil {
		return utils.Errorf(`do input error: %v`, err)
	}
	err = starter.extraInputElementsOperator(page)
	if err != nil {
		return utils.Errorf(`do extra input error: %v`, err)
	}
	for _, url := range urls {
		if starter.banList.Exist(url) {
			continue
		}
		err = starter.urlsExploit(originUrl, url)
		if err != nil {
			return utils.Errorf(`Url %v from %v exploit error: %v`, url, originUrl, err.Error())
		}
	}
	clickSelectors, err := starter.getClickElements(page)
	if err != nil {
		return utils.Errorf(`Page %s get click elements error: %s`, originUrl, err)
	}
	for _, clickSelector := range clickSelectors {
		err = starter.clickElementsExploit(page, originUrl, clickSelector)
		if err != nil {
			return utils.Errorf(`Page %v click selector %v error: %v`, originUrl, clickSelector, err.Error())
		}
	}
	return nil
}

func (starter *BrowserStarter) eventActionOnPage(page *rod.Page) error {
	originUrl, _ := getCurrentUrl(page)
	err := starter.doInput(originUrl, page)
	if err != nil {
		return utils.Errorf(`do input error: %v`, err)
	}
	err = starter.extraInputElementsOperator(page)
	if err != nil {
		return utils.Errorf(`do extra input error: %v`, err)
	}
	eventSelectors, err := starter.getEventElements(page)
	if err != nil {
		return utils.Errorf(`Page %s get event elements error: %s`, originUrl, err)
	}
	for _, eventSelector := range eventSelectors {
		err = starter.eventElementsExploit(page, originUrl, eventSelector)
		if err != nil {
			return utils.Errorf(`Page %v click element %v error: %v`, originUrl, eventSelector, err.Error())
		}
	}
	return nil
}

func (starter *BrowserStarter) ActionOnPage(page *rod.Page) error {
	if starter.vue {
		log.Debug("determined vue.")
		return starter.eventActionOnPage(page)
	}
	status, err := starter.vueCheck(page)
	if err != nil {
		return utils.Errorf(`check vue error: %v`, err)
	}
	if status {
		log.Debug("presume vue")
		return starter.eventActionOnPage(page)
	} else {
		return starter.normalActionOnPage(page)
	}
}

func (starter *BrowserStarter) vueCheck(page *rod.Page) (bool, error) {
	urlObj, err := page.Eval(findHref)
	if err != nil {
		return false, utils.Errorf(`page find href error: %v`, err)
	}
	urlArr := urlObj.Value.Arr()
	for _, url := range urlArr {
		if StringArrayContains(invalidUrl, url.String()) {
			continue
		}
		if StringSuffixList(url.String(), starter.invalidSuffix) {
			continue
		} else {
			return false, nil
		}
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
		return false, utils.Errorf(`get submit elements error: %s`, err)
	}
	if len(submitElements) == 0 {
		return true, nil
	}
	return false, nil
}

func (starter *BrowserStarter) doInput(originUrl string, page *rod.Page) error {
	inputElements, err := starter.getInputElements(page)
	if err != nil {
		return utils.Errorf(`Page %s get input elements error: %s`, originUrl, err)
	}
	for _, inputElement := range inputElements {
		visible, err := inputElement.Visible()
		if err != nil {
			return utils.Errorf(`get element visible error: %v`, err)
		}
		if !visible {
			continue
		}
		err = starter.inputElementsExploit(inputElement)
		if err != nil {
			return utils.Errorf(`Page %v input element %v error: %v`, originUrl, inputElement, err.Error())
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
		if strings.HasSuffix(originUrl, "#") {
			originUrl = originUrl[:len(originUrl)-1]
		}
		if starter.maxDepth != 0 {
			currentNode := starter.urlTree.Find(originUrl)
			if currentNode == nil {
				log.Debugf(`Origin url %s current node not found.`, originUrl)
			} else {
				if currentNode.Level() > starter.maxDepth {
					return urls, nil
				}
			}
		}
		urlArr := analysisHtmlInfo(originUrl, html)
		for _, urlStr := range urlArr {
			if StringSuffixList(urlStr, starter.invalidSuffix) {
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
		selectors := make([]string, 0)
		clickElements, err := customizedGetElement(page, searchInfo)
		if err != nil {
			return selectors, utils.Errorf(`Page %s get click elements error: %s`, page, err)
		}
		selectors = append(selectors, getElementsSelectors(clickElements)...)
		elementObj, err := EvalOnPage(page, getOnClickAction)
		if err != nil {
			log.Errorf(`page eval check onclick element code error: %v`, err)
		} else {
			elementArr := elementObj.Value.Arr()
			for _, elementGson := range elementArr {
				elementStr := elementGson.String()
				if elementStr == "" {
					continue
				}
				if !StringArrayContains(selectors, elementStr) {
					selectors = append(selectors, elementStr)
				}
			}
		}
		return selectors, nil
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
		elementObjs, err := EvalOnPage(page, getClickEventElement)
		if err != nil {
			return results, utils.Errorf(`page get click event listener elements error: %v`, err)
		}
		clickableElementArr := elementObjs.Value.Arr()
		if len(clickableElementArr) == 0 {
			log.Debug(`page with no event.`)
			return results, nil
		}
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
		for k, f := range starter.urlCheck {
			afterUrl := starter.urlAfterRepeat(targetUrl)
			if !f(afterUrl) {
				log.Debugf(`%s ban url: %s`, k, targetUrl)
				if !starter.banList.Exist(targetUrl) {
					starter.banList.Insert(targetUrl)
				}
				return nil
			}
		}
		starter.urlTree.Add(originUrl, targetUrl)
		select {
		case <-starter.ctx.Done():
			return utils.Error("context deadline exceed")
		default:
			starter.uChan.In <- targetUrl
		}
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
			if !starter.urlTree.Has(currentUrl) {
				starter.urlTree.Add(originUrl, currentUrl)
			}
			urls, err := starter.getUrls(page)
			if err != nil {
				log.Errorf(`Page %s get urls error: %s`, originUrl, err)
			} else {
				for _, url := range urls {
					if starter.banList.Exist(url) {
						continue
					}
					err = starter.urlsExploit(currentUrl, url)
					if err != nil {
						return utils.Errorf(`Url %v from %v exploit error: %v`, url, currentUrl, err.Error())
					}

				}
			}
			_ = page.NavigateBack()
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
			return inputStr(element, starter.formFill, getAllKeywords(element))
		case "file":
			return starter.defaultUploadFile(element)
		case "radio", "checkbox":
			return element.Click(proto.InputMouseButtonLeft, 1)
		default:
			return nil
		}
	}
}

func (starter *BrowserStarter) generateEventElementsExploit() func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, eventSelector string) error {
		err := page.Navigate(originUrl)
		if err != nil {
			return utils.Errorf("page navigate %s error: %s", originUrl, err)
		}
		err = page.WaitLoad()
		if err != nil {
			return utils.Errorf(`page wait load error: %v`, err)
		}
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
			select {
			case <-starter.ctx.Done():
				return utils.Error("context deadline exceed")
			default:
				starter.ch <- &result
			}
			if starter.banList.Exist(currentUrl) {
				return nil
			}
			err = starter.urlsExploit(originUrl, currentUrl)
			if err != nil {
				return utils.Errorf(`Url %v from %v exploit error: %v`, currentUrl, originUrl, err.Error())
			}
		}
		return nil
	}
}

func (starter *BrowserStarter) newEventElementsExploit() func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, eventSelector string) error {
		status := starter.clickElementOnPageBySelector(page, eventSelector)
		if !status {
			return nil
		}
		currentUrl, _ := getCurrentUrl(page)
		if currentUrl != "" && currentUrl != originUrl {
			defer func() {
				err := page.Navigate(originUrl)
				if err != nil {
					log.Errorf("page navigate %s error: %s", originUrl, err)
					return
				}
				//page.MustWaitLoad()
				err = page.WaitLoad()
				if err != nil {
					log.Errorf(`page wait load error: %v`, err)
					return
				}
				if starter.extraWaitLoadTime != 0 {
					time.Sleep(time.Duration(starter.extraWaitLoadTime) * time.Millisecond)
				}
			}()
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
			select {
			case <-starter.ctx.Done():
				return utils.Error("context deadline exceed")
			default:
				starter.ch <- &result
			}
			if starter.banList.Exist(currentUrl) {
				return nil
			}
			err := starter.urlsExploit(originUrl, currentUrl)
			if err != nil {
				return utils.Errorf(`Url %v from %v exploit error: %v`, currentUrl, originUrl, err.Error())
			}
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

func (starter *BrowserStarter) extraInputElementsOperator(page *rod.Page) error {
	// textarea
	textElements, err := page.Elements("textarea")
	if err != nil {
		return utils.Errorf("page get textarea elements error: %v", err)
	}
	for _, textElement := range textElements {
		visible, err := textElement.Visible()
		if err != nil {
			return err
		}
		if !visible {
			continue
		}
		keywordStr := getAllKeywords(textElement)
		err = inputStr(textElement, starter.formFill, keywordStr)
		if err != nil {
			return utils.Errorf("input element %v error: %v", textElement, err)
		}
	}
	// select
	selectElements, err := page.Elements("select")
	if err != nil {
		return utils.Errorf("page get select elements error: %v", err)
	}
	for _, selectElement := range selectElements {
		visible, err := selectElement.Visible()
		if err != nil {
			return err
		}
		if !visible {
			continue
		}
		options, err := selectElement.Elements("option")
		if err != nil {
			return utils.Errorf("page get option elements error: %v", err)
		}
		optionsLength := len(options)
		if optionsLength == 0 {
			log.Debugf("select element %v get no options", selectElement)
			continue
		}
		selectedOptionElement := options[optionsLength-1]
		optionValue, _ := getAttribute(selectedOptionElement, "value")
		err = selectElement.Select([]string{optionValue}, true, rod.SelectorTypeText)
		if err != nil {
			return utils.Errorf("%v select element %v error: %v", selectElement, optionValue, err)
		}
	}
	return nil
}

func inputStr(element *rod.Element, dict map[string]string, keywordStr string) error {
	for k, v := range dict {
		if strings.Contains(keywordStr, k) {
			return element.Input(v)
		}
	}
	return element.Input("test")
}
