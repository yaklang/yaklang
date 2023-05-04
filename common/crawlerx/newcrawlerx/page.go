// Package newcrawlerx
// @Author bcy2007  2023/3/7 15:47
package newcrawlerx

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"strings"
	"time"
)

func (starter *BrowserStarter) PageScan(page *rod.Page) error {
	for _, doAction := range starter.pageActions {
		err := doAction(page)
		if err != nil {
			return utils.Errorf("do action error: %s", err)
		}
	}
	return nil
}

//
// get url
//

func (starter *BrowserStarter) DefaultGetUrlFunctionGenerator(doGetUrl func(string, string) error) func(*rod.Page) error {
	return func(page *rod.Page) error {
		originUrl, _ := getCurrentUrl(page)
		urlObj, err := page.Eval(findOnlyHref)
		if err != nil {
			return utils.Errorf("page %s get url by eval js error: %s", page, err)
		}
		urlArr := urlObj.Value.Arr()
		for _, urlRaw := range urlArr {
			urlStr := urlRaw.Str()
			if urlStr == "" {
				continue
			}
			doGetUrl(originUrl, urlStr)
		}
		return nil
	}
}

func (starter *BrowserStarter) DefaultDoGetUrl() func(string, string) error {
	return func(originUrl string, targetUrl string) error {
		if starter.stopSignal {
			return nil
		}
		//for _, f := range starter.extraFunctions {
		//	if f(targetUrl) {
		//		log.Infof("extra rules: %s", targetUrl)
		//		starter.uChan.In <- targetUrl
		//		return nil
		//	}
		//}
		for k, f := range starter.checkFunctionMap {
			if !f(targetUrl) {
				log.Errorf("%s banned url: %s", k, targetUrl)
				return nil
			}
		}
		afterUrl := starter.urlAfterRepeat(targetUrl)
		for _, f := range starter.checkFunctions {
			if !f(afterUrl) {
				return nil
			}
		}
		//starter.urlTree.Add(originUrl, targetUrl)
		//log.Info(originUrl, " -> ", targetUrl)
		starter.uChan.In <- targetUrl
		//log.Info(starter.uChan.Len(), " ", starter.uChan.BufLen())
		return nil
	}
}

//
// click
//

func (starter *BrowserStarter) DefaultClickFunctionGenerator(doClick func(*rod.Page, string, string) error) func(*rod.Page) error {
	return func(page *rod.Page) error {
		currentUrl, err := getCurrentUrl(page)
		if err != nil {
			return utils.Errorf("get current page %s url error: %s", page, err)
		}
		selectors := GetDefaultClickElementSelectors(page)
		for _, selector := range selectors {
			//log.Info("click selectors: ", selector)
			doClick(page, currentUrl, selector)
		}
		return nil
	}
}

func (starter *BrowserStarter) EventClickFunctionGenerator(doClick func(*rod.Page, string, string) error) func(*rod.Page) error {
	return func(page *rod.Page) error {
		currentUrl, err := getCurrentUrl(page)
		if err != nil {
			return utils.Errorf("get current page %s url error: %s", page, err)
		}
		clickableElementObjs, err := proto.RuntimeEvaluate{
			IncludeCommandLineAPI: true,
			ReturnByValue:         true,
			Expression:            testJs,
		}.Call(page)
		if err != nil {
			return utils.Errorf("page %s get click event listener error: %s", page, err)
		}
		clickableElementArr := clickableElementObjs.Result.Value.Arr()
		for _, clickableElementSelectorRaw := range clickableElementArr {
			clickableElementSelector := clickableElementSelectorRaw.Str()
			doClick(page, currentUrl, clickableElementSelector)
		}
		return nil
	}
}

func (starter *BrowserStarter) DefaultDoClick() func(*rod.Page, string, string) error {
	return func(page *rod.Page, originUrl string, selector string) error {
		clickElementOnPageBySelector(page, selector)
		currentUrl, _ := getCurrentUrl(page)
		if starter.getUrlFunction == nil {
			return utils.Errorf("get url function null")
		}
		starter.getUrlFunction(page)
		if currentUrl != "" && currentUrl != originUrl {
			page.NavigateBack()
			//page.MustWaitLoad()
			time.Sleep(time.Second)
		}
		return nil
	}
}

//
// input
//

func (starter *BrowserStarter) DefaultInputFunctionGenerator(doInput func(*rod.Element) error) func(*rod.Page) error {
	return func(page *rod.Page) error {
		status, _, err := page.Has("input")
		if err != nil {
			return utils.Errorf("page %s detect input element error: %s", page, err)
		}
		if !status {
			return nil
		}
		inputs, err := page.Elements("input")
		if err != nil {
			return utils.Errorf("page %s get input elements error: %s", page, err)
		}
		for _, input := range inputs {
			if visible, _ := isVisible(input); !visible {
				continue
			}
			doInput(input)
		}
		return nil
	}
}

func (starter *BrowserStarter) DefaultDoInput() func(*rod.Element) error {
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
			//log.Info("pretend do upload.")
			starter.defaultUploadFile(element)
			return nil
		case "radio", "checkbox":
			return element.Click(proto.InputMouseButtonLeft)
		default:
			return utils.Errorf("unknown attribute: %s", attribute)
		}
	}
}

func (starter *BrowserStarter) defaultUploadFile(element *rod.Element) error {
	if len(starter.inputFile) == 0 {
		return utils.Errorf("no upload file set.")
	}
	keywords := getAllKeywords(element)
	for k, v := range starter.inputFile {
		if strings.Contains(keywords, k) {
			testUploadFile(element, v)
			return nil
		}
	}
	defaultPath, ok := starter.inputFile["default"]
	if !ok {
		return utils.Errorf("no default upload file set.")
	}
	testUploadFile(element, defaultPath)
	return nil
}

func (starter *BrowserStarter) HttpPostFile(element *rod.Element) error {
	formElement, err := element.Parent()
	if err != nil {
		return utils.Errorf("get element parent error: %s", err)
	}
	// get post url
	var postUrl string
	baseUrlObj, err := formElement.Eval(`()=>document.URL`)
	if err != nil {
		return utils.Errorf("cannot get page url: %s", err)
	}
	baseUrl := baseUrlObj.Value.String()
	action, _ := getAttribute(formElement, "action")
	if action == "" {
		return utils.Errorf("cannot get file post url")
	} else if action == "#" {
		postUrl = baseUrl
	} else {
		baseUrlParse, _ := url.Parse(baseUrl)
		postUrlParse, _ := baseUrlParse.Parse(action)
		postUrl = postUrlParse.String()
	}
	// get post params
	inputElements, err := formElement.Elements("input")
	formValues := make(map[string]string)
	fileValues := make(map[string]string)
	for _, inputElement := range inputElements {
		name, _ := getAttribute(inputElement, "name")
		if name == "" {
			continue
		}
		value, _ := getAttribute(inputElement, "value")
		if value != "" {
			formValues[name] = value
			continue
		}
		elementType, _ := getAttribute(inputElement, "type")
		if elementType == "file" {
			fileValues[name] = starter.GetUploadFile(element)
		} else if elementType == "reset" || elementType == "submit" {
			continue
		} else if StringArrayContains(inputStringElementTypes, elementType) {
			formValues[name] = starter.GetFormFill(element)
		}
	}
	// do post
	// tbc
	r := CreateFileRequest(postUrl, "POST", formValues, fileValues)
	r.Request()
	r.Do()
	return nil
}

func (starter *BrowserStarter) GetFormFill(element *rod.Element) string {
	keywords := getAllKeywords(element)
	for k, v := range starter.formFill {
		if strings.Contains(keywords, k) {
			return v
		}
	}
	return "test"
}

func (starter *BrowserStarter) GetUploadFile(element *rod.Element) string {
	keywords := getAllKeywords(element)
	for k, v := range starter.inputFile {
		if strings.Contains(keywords, k) {
			return v
		}
	}
	v, ok := starter.inputFile["default"]
	if !ok {
		return ""
	}
	return v
}
