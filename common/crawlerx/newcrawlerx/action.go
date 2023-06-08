// Package newcrawlerx
// @Author bcy2007  2023/5/23 11:02
package newcrawlerx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"time"
)

func (starter *BrowserStarter) actionOnPage() func(*rod.Page) error {
	return func(page *rod.Page) error {
		urls, err := getUrl(page)
		if err != nil {
			return err
		}
		searchInfo := map[string]map[string][]string{
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
		elements, err := customizedGetElement(page, searchInfo)
		if err != nil {
			return err
		}
		if len(urls) == 0 && len(elements) == 0 {
			info, _ := page.Info()
			var url string
			if info != nil {
				url = info.URL
			}
			log.Infof("no a href urls & form input found in page %s.", url)
			log.Info("try to find click event in page.")
			starter.vueFunction(page)
		} else {
			starter.getUrlFunction(page)
			starter.clickFunction(page)
		}
		return nil
	}
}

func (starter *BrowserStarter) ActionOnPage() func(*rod.Page) error {
	return func(page *rod.Page) error {
		currentUrl, _ := getCurrentUrl(page)
		urls, err := starter.getUrlsFunction(page)
		if err != nil {
			return utils.Errorf("get page %s urls error: %s", page.TargetID, err)
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
		tempElement, err := customizedGetElement(page, submitInfo)
		if err != nil {
			return err
		}
		inputElements, err := starter.getInputFunction(page)
		if err != nil {
			return utils.Errorf("get page %s input element error: %s", page.TargetID, err)
		}
		for _, inputElement := range inputElements {
			starter.doInputFunction(inputElement)
		}
		if len(urls) == 0 && len(tempElement) == 0 {
			//log.Info("vue status.")
			eventSelectors, err := starter.getEventFunction(page)
			if err != nil {
				return utils.Errorf("get page event error: %s", err)
			}
			for _, eventSelector := range eventSelectors {
				starter.doEventClickFunction(page, currentUrl, eventSelector)
			}
		} else {
			//log.Info("normal status.")
			//log.Info(urls)
			for _, urlStr := range urls {
				starter.doUrlsFunction(currentUrl, urlStr)
			}
			selectors, err := starter.getClickFunction(page)
			if err != nil {
				return utils.Errorf("get page click element error: %s", err)
			}
			for _, selector := range selectors {
				starter.doClickFunction(page, currentUrl, selector)
			}
		}

		return nil
	}
}

func (starter *BrowserStarter) ActionOnJS() func(*rod.Page) error {
	return func(page *rod.Page) error {
		currentUrl, err := getCurrentUrl(page)
		if err != nil {
			return utils.Errorf("page %s get js url error: %s", page.TargetID, err)
		}
		html, err := page.HTML()
		if err != nil {
			return utils.Errorf("page %s get html error: %s", page.TargetID, err)
		}
		jsUrls := analysisJsInfo(currentUrl, html)
		for _, jsUrl := range jsUrls {
			result := SimpleResult{
				url:        jsUrl,
				resultType: "js url",
			}
			starter.ch <- &result
		}
		return nil
	}
}

func (starter *BrowserStarter) getUrlsFunctionGenerator() func(*rod.Page) ([]string, error) {
	return func(page *rod.Page) ([]string, error) {
		urls, err := getUrl(page)
		if err != nil {
			return urls, utils.Errorf("page %s get url error: %s", page.TargetID, err)
		}
		return urls, nil
	}
}

func (starter *BrowserStarter) doUrlsFunctionGenerator() func(string, string) error {
	return starter.DefaultDoGetUrl()
}

func (starter *BrowserStarter) getClickFunctionGenerator() func(*rod.Page) ([]string, error) {
	return func(page *rod.Page) ([]string, error) {
		return GetDefaultClickElementSelectors_(page)
	}
}

func (starter *BrowserStarter) doClickFunctionGenerator() func(*rod.Page, string, string) error {
	return starter.DefaultDoClick()
}

func (starter *BrowserStarter) getInputFunctionGenerator() func(*rod.Page) (rod.Elements, error) {
	return func(page *rod.Page) (rod.Elements, error) {
		status, _, err := page.Has("input")
		if err != nil {
			return nil, utils.Errorf("page %s detect input element error: %s", page, err)
		}
		if !status {
			return nil, nil
		}
		inputs, err := page.Elements("input")
		if err != nil {
			return nil, utils.Errorf("page %s get input elements error: %s", page, err)
		}
		return inputs, nil
	}
}

func (starter *BrowserStarter) doInputFunctionGenerator() func(*rod.Element) error {
	return starter.DefaultDoInput()
}

func (starter *BrowserStarter) getEventFunctionGenerator() func(*rod.Page) ([]string, error) {
	return func(page *rod.Page) ([]string, error) {
		results := make([]string, 0)
		clickableElementObjs, err := proto.RuntimeEvaluate{
			IncludeCommandLineAPI: true,
			ReturnByValue:         true,
			Expression:            testJs,
		}.Call(page)
		if err != nil {
			return results, utils.Errorf("page %s get click event listener error: %s", page, err)
		}
		clickableElementArr := clickableElementObjs.Result.Value.Arr()
		for _, element := range clickableElementArr {
			results = append(results, element.String())
		}
		return results, nil
	}
}

func (starter *BrowserStarter) doEventClickFunctionGenerator() func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, selector string) error {
		err := page.Navigate(originUrl)
		if err != nil {
			return utils.Errorf("page navigate %s error: %s", originUrl, err)
		}
		page.MustWaitLoad()
		time.Sleep(time.Second)
		clickElementOnPageBySelector(page, selector)
		currentUrl, _ := getCurrentUrl(page)
		if currentUrl != "" && currentUrl != originUrl {
			starter.doUrlsFunction(originUrl, currentUrl)
		}
		return nil
	}
}
