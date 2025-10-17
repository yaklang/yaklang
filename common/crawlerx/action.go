// Package crawlerx
// @Author bcy2007  2023/7/13 11:10
package crawlerx

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/forge"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"regexp"
	"strings"
	"time"
)

type ElementInfo struct {
	Tag        string             `json:"tag"`
	Attributes []ElementAttribute `json:"types"`
}

type ElementAttribute struct {
	Name string   `json:"attribute"`
	Info []string `json:"info"`
}

var (
	clickElementTemplate = []ElementInfo{
		{
			Tag: "input", Attributes: []ElementAttribute{
				{Name: "type", Info: []string{"submit"}},
			},
		},
		{Tag: "button"},
		{Tag: "[onclick]"},
	}
	submitElementTemplate = []ElementInfo{
		{
			Tag: "input", Attributes: []ElementAttribute{
				{Name: "type", Info: []string{"submit"}},
			},
		}, {
			Tag: "button", Attributes: []ElementAttribute{
				{Name: "type", Info: []string{"submit"}},
			},
		},
	}
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

func (starter *BrowserStarter) eventActionOnPageV2(page *rod.Page) error {
	originUrl, _ := getCurrentUrl(page)
	err := starter.doInput(originUrl, page)
	if err != nil {
		return utils.Errorf(`do input error: %v`, err)
	}
	err = starter.extraInputElementsOperator(page)
	if err != nil {
		return utils.Errorf(`do extra input error: %v`, err)
	}
	selectorQueue, err := getEventElements(page)
	selectorQueue.Range(func(eventSelector string, pos int) bool {
		//err = starter.eventElementsExploit(page, originUrl, eventSelector)
		//if err != nil {
		//	log.Errorf(`Page %v click element %v error: %v`, originUrl, eventSelector, err.Error())
		//	return false
		//}
		var currentSelectors []string
		currentSelectors, err = starter.eventElementsExploitV2(page, originUrl, eventSelector)
		if err != nil {
			log.Errorf(`Page %v click element %v error: %v`, originUrl, eventSelector, err.Error())
			return false
		}
		selectorQueue.Prepend(pos, currentSelectors...)
		return true
	})
	return err
}

func (starter *BrowserStarter) ActionOnPage(page *rod.Page) error {
	if starter.vue {
		log.Debug("determined vue.")
		//return starter.eventActionOnPage(page)
		return starter.eventActionOnPageV2(page)
	}
	status, err := starter.vueCheck(page)
	if err != nil {
		return utils.Errorf(`check vue error: %v`, err)
	}
	if status {
		log.Debug("presume vue")
		//return starter.eventActionOnPage(page)
		return starter.eventActionOnPageV2(page)
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
	submitElements, err := customizedGetElement(page, submitElementTemplate)
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
	baseInfo, err := getBaseInfo(page)
	if err != nil {
		return utils.Errorf(`Page %s get base info error: %s`, originUrl, err)
	}
	for _, inputElement := range inputElements {
		var visible bool
		visible, err = inputElement.Visible()
		if err != nil {
			return utils.Errorf(`get element visible error: %v`, err)
		}
		if !visible {
			continue
		}
		var elementType string
		elementType, err = getAttribute(inputElement, "type")
		if err != nil {
			return utils.Errorf(`get element type error: %v`, err)
		}
		if elementType == "submit" {
			continue
		}
		err = starter.inputElementsExploit(inputElement, baseInfo)
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
		selectors := make([]string, 0)
		clickElements, err := customizedGetElement(page, clickElementTemplate)
		if err != nil {
			return selectors, utils.Errorf(`Page %s get click elements error: %s`, page, err)
		}
		selectors = append(selectors, getElementsSelectors(clickElements)...)
		elementObj, err := EvalOnPage(page, getOnClickAction)
		if err != nil {
			log.Errorf(`page eval check onclick element code error: %v`, err)
			return selectors, nil
		}
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

func getEventElements(page *rod.Page) (*tools.DynamicQueue, error) {
	var queue = tools.NewDynamicQueue()
	elementObjs, err := EvalOnPage(page, getClickEventElement)
	if err != nil {
		return queue, utils.Errorf(`page get click event listener elements error: %v`, err)
	}
	clickableElementArr := elementObjs.Value.Arr()
	if len(clickableElementArr) == 0 {
		log.Debug(`page with no event.`)
		return queue, nil
	}
	for _, element := range clickableElementArr {
		queue.Enqueue(element.String())
	}
	return queue, nil
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

func (starter *BrowserStarter) generateInputElementsExploit() func(*rod.Element, interface{}) error {
	return func(element *rod.Element, _ interface{}) error {
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

func (starter *BrowserStarter) generateAIInputElementsExploit() func(*rod.Element, interface{}) error {
	return func(element *rod.Element, pageData interface{}) error {
		dataStr, ok := pageData.(string)
		if !ok {
			return nil
		}
		text, err := element.HTML()
		if err != nil {
			return err
		}
		if len(text) > 200 {
			reg, _ := regexp.Compile("style=\".+?\"|size=\".+?\"")
			text = reg.ReplaceAllLiteralString(text, "")[:200]
		}
		parent, _ := element.Parent()
		if parent != nil {
			//text += parent.
			class, _ := getAttribute(parent, "class")
			text += " " + class
			grandParent, _ := element.Parent()
			if grandParent != nil {
				grandClass, _ := getAttribute(grandParent, "class")
				text += " " + grandClass
			}
		}
		output, _ := starter.getElementInputByAI(dataStr + " " + text)
		return element.Input(output.TextInput)
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

func (starter *BrowserStarter) eventElementsExploitV2(page *rod.Page, originUrl string, selector string) ([]string, error) {
	var (
		result []string
		err    error
	)
	status := starter.clickElementOnPageBySelector(page, selector)
	if !status {
		return result, nil
	}
	currentUrl, _ := getCurrentUrl(page)
	if currentUrl != "" && currentUrl != originUrl {
		defer page.NavigateBack()
		err = starter.urlsExploit(originUrl, currentUrl)
		if err != nil {
			return result, utils.Errorf(`Url %v from %v exploit error: %v`, currentUrl, originUrl, err.Error())
		}
	}
	// get event selectors
	var newSelectorQueue *tools.DynamicQueue
	newSelectorQueue, err = getEventElements(page)
	if err != nil {
		return result, err
	}
	//clicks, err := EvalOnPage(page, getClickEventElement)
	//if err != nil {
	//	return result, err
	//}
	//fmt.Println("clicks: ", clicks)
	result = newSelectorQueue.ToList()
	return result, nil
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
		selector, err := calculateSelector(selectedOptionElement)
		if err != nil {
			return utils.Errorf("get option element selector error: %v", err)
		}
		err = selectElement.Select([]string{selector}, true, rod.SelectorTypeCSSSector)
		if err != nil {
			return utils.Errorf("%v select element %v error: %v", selectElement, selector, err)
		}
	}
	return nil
}

type AIInput struct {
	HtmlCod   string `json:"html_cod"`
	OtherInfo string `json:"other_info"`
}

type AIInputResult struct {
	Element   string `json:"element"`
	DButt     bool   `json:"dButt"`
	TextInput string `json:"text_input"`
}

func (starter *BrowserStarter) getElementInputByAI(data string) (output AIInputResult, err error) {
	// request
	var input AIInput
	var inputResult AIInputResult
	inputResult.DButt = false
	inputResult.TextInput = "test"
	input.HtmlCod = data
	input.OtherInfo = starter.aiInputInfo
	inputBytes, _ := json.Marshal(input)
	opts := []poc.PocConfigOption{
		poc.WithReplaceHttpPacketHeader("Content-Type", "application/json"),
		poc.WithReplaceHttpPacketBody(inputBytes, false),
		poc.WithTimeout(10),
	}
	if starter.browserConfig.proxyAddress != nil {
		opts = append(opts, poc.WithProxy(starter.browserConfig.proxyAddress.String()))
	}
	result, _, err := poc.DoPOST(starter.aiInputUrl, opts...)
	if err != nil {
		return inputResult, err
	}
	log.Debugf("ai check input: %v, output: %v", starter.aiInputUrl+" "+data+" "+starter.aiInputInfo, string(result.GetBody()))
	err = json.Unmarshal(result.GetBody(), &inputResult)
	return inputResult, err
}

func inputStr(element *rod.Element, dict map[string]string, keywordStr string) error {
	for k, v := range dict {
		if strings.Contains(keywordStr, k) {
			return element.Input(v)
		}
	}
	return element.Input("test")
}

func getBaseInfo(page *rod.Page) (string, error) {
	info, err := page.Info()
	if err != nil {
		return "", err
	}
	return info.Title, nil
}

const (
	selectorPrompt = `你是一个网页结构识别助手，你的任务是对于给出的html内容，识别用户名、密码和点击登陆的按钮对应的selector。
如果有验证码，也需要把验证码输入框和验证码图片的selector返回。
识别结果格式为JSON格式，具体格式如下：
{"username":"#username","password":"#password","captcha":"#captcha","captcha_img":"#img","login":"#button"}
`
	captchaPrompt = `你是一个验证码识别助手，你的任务是识别验证码，并返回识别结果。
如果图片是一个算式 就返回算式结果。
识别结果格式为JSON格式，具体格式如下：
{"result":"123F"}
`
)

type CaptchaResult struct {
	Result string `json:"result"`
}

type LoginSelector struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	Captcha    string `json:"captcha"`
	CaptchaImg string `json:"captcha_img"`
	Login      string `json:"login"`
}

type loginFailError struct {
	err string
}

func (e *loginFailError) Error() string {
	return e.err
}

func NewLoginFailError(info string) error {
	return &loginFailError{
		err: info,
	}
}

func (starter *BrowserStarter) Login(page *rod.Page) error {
	var err error
	for i := 0; i < 3; i++ {
		err = starter.doLogin(page)
		var loginErr *loginFailError
		if !errors.As(err, &loginErr) || err == nil {
			break
		}
	}
	return err
}

func (starter *BrowserStarter) doLogin(page *rod.Page) error {
	html, err := page.HTML()
	if err != nil {
		return err
	}
	if html == "" || html == "<html><head></head><body></body></html>" {
		return NewLoginFailError("null html info")
	}
	//screenshot before login
	originScreenShotBytes, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	if err != nil {
		return err
	}
	originScreenShot := "data:image/png;base64," + base64.StdEncoding.EncodeToString(originScreenShotBytes)
	extractor, err := forge.NewLoginElementExtractor()
	if err != nil {
		return err
	}
	selectors, err := extractor.ExtractLoginElements(page.GetContext(), html)
	if err != nil {
		return err
	}
	if selectors.UsernameSelector == "" {
		return nil
	}
	var captchaResult string
	if selectors.CaptchaInputSelector != "" {
		captchaElements, _ := page.Elements(selectors.CaptchaImageSelector)
		if len(captchaElements) == 0 {
			return NewLoginFailError("captcha not found")
		}
		captchaElement := captchaElements.First()
		var captchaElementBytes []byte
		captchaElementBytes, err = captchaElement.Screenshot(proto.PageCaptureScreenshotFormatPng, 0)
		if err != nil {
			return err
		}
		captchaBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(captchaElementBytes)
		captchaResult = starter.getCaptcha(page.GetContext(), captchaBase64)
	}
	// input
	usernameElements, err := page.Elements(selectors.UsernameSelector)
	if err != nil {
		return err
	}
	if len(usernameElements) != 0 {
		err = elementInput(usernameElements.First(), starter.loginUsername)
		if err != nil {
			return err
		}
	}
	passwordElements, err := page.Elements(selectors.PasswordSelector)
	if err != nil {
		return err
	}
	if len(passwordElements) != 0 {
		err = elementInput(passwordElements.First(), starter.loginPassword)
		if err != nil {
			return err
		}
	}
	if captchaResult != "" {
		var captchaElements rod.Elements
		captchaElements, err = page.Elements(selectors.CaptchaInputSelector)
		if err != nil {
			return err
		}
		err = elementInput(captchaElements.First(), captchaResult)
		if err != nil {
			return err
		}
	}
	time.Sleep(500 * time.Millisecond)
	loginButtons, err := page.Elements(selectors.LoginButtonSelector)
	if err != nil {
		return err
	}
	if len(loginButtons) != 0 {
		err = loginButtons.First().Click(proto.InputMouseButtonLeft, 1)
		if err != nil {
			return err
		}
	}
	err = page.WaitLoad()
	if err != nil {
		return err
	}
	time.Sleep(time.Duration(starter.extraWaitLoadTime+1000) * time.Millisecond)
	// screenshot after login & png compare
	currentScreenshotBytes, err := page.Screenshot(false, &proto.PageCaptureScreenshot{
		Format: proto.PageCaptureScreenshotFormatPng,
	})
	if err != nil {
		return err
	}
	currentScreenshot := "data:image/png;base64," + base64.StdEncoding.EncodeToString(currentScreenshotBytes)
	similarity := tools.GetImgSimilarity(originScreenShot, currentScreenshot)
	if similarity > 0.7 {
		return NewLoginFailError(fmt.Sprintf("page similarity too large: %f", similarity))
	}
	return nil
}

func (starter *BrowserStarter) getCaptcha(ctx context.Context, imgBase64 string) (result string) {
	detector, err := forge.NewCaptchaDetector()
	if err != nil {
		log.Debugf("get captcha detector err: %v", err)
		return
	}
	detectResult, err := detector.DetectCaptcha(ctx, imgBase64)
	if err != nil {
		log.Debugf("get captcha detector err: %v", err)
		return
	}
	if detectResult.CaptchaType == "math" {
		result, err = tools.GetCalculateResult(detectResult.CaptchaText)
		if err != nil {
			log.Debugf("get captcha result err: %v", err)
		}
		return
	}
	result = detectResult.CaptchaText
	return
}

func elementInput(ele *rod.Element, inputStr string) error {
	err := ele.SelectAllText()
	if err != nil {
		return err
	}
	err = ele.Type(input.Backspace)
	if err != nil {
		return err
	}
	return ele.Input(inputStr)
}
