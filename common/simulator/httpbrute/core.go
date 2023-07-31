// Package httpbrute
// @Author bcy2007  2023/6/20 15:08
package httpbrute

import (
	"fmt"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/simulator/core"
	"github.com/yaklang/yaklang/common/simulator/extend"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type SimulatorCore interface {
	Start() error
	elementDetect() error
	doBruteForce() error
}

func NewBruteForce(urlStr string, opts ...BruteConfigOpt) (*BruteForceCore, error) {
	bruteConfig := NewBruteConfig()
	for _, opt := range opts {
		opt(bruteConfig)
	}
	bruteForceCore := BruteForceCore{
		targetUrl: urlStr,
		config:    bruteConfig,
		proxy:     nil,

		resultChannel:    bruteConfig.resultChannel,
		similarityDegree: bruteConfig.similarityDegree,
	}
	if bruteConfig.proxy != "" {
		proxyUrl, err := url.Parse(bruteConfig.proxy)
		if err != nil {
			return nil, utils.Errorf(`parse proxy url error: %v`, err.Error())
		}
		if bruteConfig.proxyUsername != "" || bruteConfig.proxyPassword != "" {
			proxyUser := url.UserPassword(bruteConfig.proxyUsername, bruteConfig.proxyPassword)
			proxyUrl.User = proxyUser
		}
		bruteForceCore.proxy = proxyUrl
	}
	return &bruteForceCore, nil
}

type BruteForceCore struct {
	targetUrl string
	config    *BruteConfig
	proxy     *url.URL

	resultChannel       chan Result
	captchaDetectModule *extend.CaptchaIdentifier
	html                string
	similarityDegree    float64
	originUrl           string
	loginDetectFunc     func() (bool, string)

	page              *core.GeneralPage
	usernameElement   *core.GeneralElement
	passwordElement   *core.GeneralElement
	captchaElement    *core.GeneralElement
	captchaImgElement *core.GeneralElement
	buttonElement     *core.GeneralElement

	charCompiler *regexp.Regexp
}

func (bruteForce *BruteForceCore) init() error {
	// captcha detect mode init
	err := bruteForce.captchaModeInit()
	if err != nil {
		return utils.Errorf(`[bruteforce] captcha mode init error: %v`, err.Error())
	}
	// username list & password list init
	// if list is empty, give them the default value
	if len(bruteForce.config.usernameList) == 0 {
		bruteForce.config.usernameList = append(bruteForce.config.usernameList, "admin")
	}
	if len(bruteForce.config.passwordList) == 0 {
		bruteForce.config.passwordList = append(bruteForce.config.passwordList, "admin", "luckyadmin123")
	}
	// page origin html string to detect degree of page info change
	html := bruteForce.page.HTML()
	if html == "" {
		return utils.Error(`bruteforce target page html blank`)
	}
	bruteForce.html = html
	// solve the problem when there is javascript dialog opened
	go bruteForce.page.OriginPage().EachEvent(func(e *proto.PageJavascriptDialogOpening) {
		_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(bruteForce.page.OriginPage())
	})()
	// page origin url which may be not same with target url because of url jump
	originUrl := bruteForce.page.CurrentURL()
	if originUrl == "" {
		return utils.Error(`bruteforce target page current url blank`)
	}
	bruteForce.originUrl = originUrl
	// invalid captcha char detect
	// this must be set because we input char by simulate key press
	tempCompiler, _ := regexp.Compile(`[^0-9a-zA-Z\-]`)
	bruteForce.charCompiler = tempCompiler
	//
	var loginDetectMap = map[loginDetectMode]func() (bool, string){
		UrlChangeMode:     bruteForce.loginDetectByUrl,
		HtmlChangeMode:    bruteForce.loginDetectByHTML,
		DefaultChangeMode: bruteForce.loginDetect,
	}
	if fn, ok := loginDetectMap[bruteForce.config.loginDetect]; !ok {
		bruteForce.loginDetectFunc = fn
	} else {
		bruteForce.loginDetectFunc = bruteForce.loginDetect
	}
	return nil
}

func (bruteForce *BruteForceCore) captchaModeInit() error {
	bruteForce.captchaDetectModule = extend.CreateCaptcha()
	if bruteForce.config.captchaUrl != "" {
		status, msg := ConnectTest(bruteForce.config.captchaUrl, bruteForce.proxy)
		if !status {
			return utils.Errorf(`[bruteforce] captcha url %v connect test error: %v`, bruteForce.config.captchaUrl, msg)
		}
	}
	bruteForce.captchaDetectModule.SetIdentifyUrl(bruteForce.config.captchaUrl)
	if bruteForce.config.captchaMode != "" {
		bruteForce.captchaDetectModule.SetIdentifyMode(bruteForce.config.captchaMode)
	}
	if strings.Contains(bruteForce.config.captchaUrl, "/runtime/text/invoke") {
		bruteForce.captchaDetectModule.SetRequestStruct(&extend.CaptchaRequest{})
		bruteForce.captchaDetectModule.SetResponseStruct(&extend.CaptchaResult{})
	} else {
		bruteForce.captchaDetectModule.SetRequestStruct(&extend.DDDDCaptcha{})
		bruteForce.captchaDetectModule.SetResponseStruct(&extend.DDDDResult{})
	}
	return nil
}

func (bruteForce *BruteForceCore) elementDetect() error {
	elements, err := bruteForce.page.FindElements("input")
	if err != nil {
		return err
	}
	elements = elements.FilteredTypeElement("text", "password", "number", "tel")

	// username element
	if bruteForce.config.usernameSelector != "" {
		bruteForce.usernameElement, err = bruteForce.page.FindElement(bruteForce.config.usernameSelector)
		if err != nil {
			return err
		}
	} else {
		element := ElementsFilter(elements, "username")
		if element == nil {
			return utils.Error("username element not found")
		}
		bruteForce.usernameElement = element
	}
	elements = elements.Slice(bruteForce.usernameElement)

	// password element
	if bruteForce.config.passwordSelector != "" {
		bruteForce.passwordElement, err = bruteForce.page.FindElement(bruteForce.config.passwordSelector)
		if err != nil {
			return err
		}
	} else {
		element := ElementsFilter(elements, "password")
		if element == nil {
			return utils.Error("password element not found")
		}
		bruteForce.passwordElement = element
	}

	// captcha element
	if bruteForce.config.captchaSelector == "" {
		element := ElementsFilter(elements, "captcha")
		if element != nil {
			bruteForce.captchaElement = element
		}
	} else if bruteForce.config.captchaSelector != "none" {
		bruteForce.captchaElement, err = bruteForce.page.FindElement(bruteForce.config.captchaSelector)
		if err != nil {
			return err
		}
	}

	// captcha img element
	if bruteForce.captchaElement != nil {
		if bruteForce.config.captchaImgSelector != "" {
			bruteForce.captchaImgElement, err = bruteForce.page.FindElement(bruteForce.config.captchaImgSelector)
			if err != nil {
				return err
			}
		} else {
			captchaImgElements, err := bruteForce.captchaElement.GeneralGetLatestElements("img", 5)
			if err != nil {
				return err
			}
			if captchaImgElements.Empty() {
				return utils.Errorf("captcha %s exist but captcha element not found", bruteForce.config.captchaSelector)
			} else if captchaImgElements.Single() {
				bruteForce.captchaImgElement = captchaImgElements.First()
			} else {
				bruteForce.captchaImgElement = captchaImgElements.FilteredKeywordElement("captcha")
			}
		}
	}

	// button element
	if bruteForce.config.buttonSelector != "" {
		bruteForce.buttonElement, err = bruteForce.page.FindElement(bruteForce.config.buttonSelector)
		if err != nil {
			return err
		}
	} else {
		buttons, err := bruteForce.page.GeneralFindElements("button")
		if err != nil {
			return err
		}
		if buttons.Single() {
			bruteForce.buttonElement = buttons.First()
		} else if buttons.Multi() {
			bruteForce.buttonElement = buttons.FilteredKeywordElement("login")
		} else {
			return utils.Error("button element not found")
		}
	}
	return nil
}

func (bruteForce *BruteForceCore) doBruteForce() error {
	for _, username := range bruteForce.config.usernameList {
		for _, password := range bruteForce.config.passwordList {
			status, err := bruteForce.inputClickTry(username, password)
			if err != nil {
				return err
			}
			if status {
				return nil
			}
		}
	}
	return nil
}

func (bruteForce *BruteForceCore) inputClickTry(username, password string) (bool, error) {
	bruteForce.page.Refresh()
	if bruteForce.config.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(bruteForce.config.extraWaitLoadTime) * time.Millisecond)
	}
	bruteForce.usernameElement.Input(username)
	bruteForce.passwordElement.Input(password)
	if bruteForce.captchaElement != nil {
		captchaStr, err := bruteForce.captchaDetectModule.Detect(bruteForce.captchaImgElement)
		if err != nil {
			return false, err
		}
		if bruteForce.charCompiler.MatchString(captchaStr) {
			return false, utils.Errorf("invalid captcha char: %s", captchaStr)
		} else {
			log.Infof("detect captcha result: %s", captchaStr)
		}
		bruteForce.captchaElement.Input(captchaStr)
	}
	bruteForce.page.StartListen()
	err := bruteForce.buttonElement.Click()
	if err != nil {
		return false, err
	}
	bruteForce.page.Wait()

	words, err := bruteForce.page.StopListenWithBytes()
	if err != nil {
		log.Errorf("get listen words error: %s", err.Error())
	}
	if len(words) > 500 {
		words = words[:500]
		words = append(words, 46, 46, 46)
	}
	result := BruteResult{
		username: username,
		password: password,
	}
	status, info := bruteForce.loginDetectFunc()
	if status {
		pageB64, _ := bruteForce.page.ScreenShotResult()
		msg := fmt.Sprintf("login success with username: %s & password: %s", username, password)
		result.status = true
		result.bruteInfo = msg + " " + info + " " + string(words)
		result.loginB64 = pageB64
		bruteForce.resultChannel <- &result
		return true, nil
	} else {
		msg := fmt.Sprintf("login failed with username: %s & password: %s", username, password)
		result.status = false
		result.bruteInfo = msg + " " + info + " " + string(words)
		bruteForce.resultChannel <- &result
	}
	return false, nil
}

func (bruteForce *BruteForceCore) loginDetect() (bool, string) {
	status, info := bruteForce.loginDetectByUrl()
	if !status {
		return status, info
	}
	status, info = bruteForce.loginDetectByHTML()
	if !status {
		return status, info
	}
	return status, ""
}

func (bruteForce *BruteForceCore) loginDetectByUrl() (bool, string) {
	currentUrl := bruteForce.page.CurrentURL()
	if currentUrl != bruteForce.originUrl {
		return true, fmt.Sprintf("after login url: %s which is different with origin url: %s", currentUrl, bruteForce.originUrl)
	}
	return false, fmt.Sprintf("after login url: %s which is same with origin url", currentUrl)
}

func (bruteForce *BruteForceCore) loginDetectByHTML() (bool, string) {
	currentHtml := bruteForce.page.HTML()
	degree := extend.GetPageSimilarity(bruteForce.html, currentHtml)
	if degree < bruteForce.similarityDegree {
		return true, fmt.Sprintf("html info same degree %f which is smaller than %f", degree, bruteForce.similarityDegree)
	} else {
		return false, fmt.Sprintf("html info same degree %f which is larger than %f", degree, bruteForce.similarityDegree)
	}
}

func (bruteForce *BruteForceCore) Start() error {
	browserModule := core.PageCreator()
	browserModule.SetURL(bruteForce.targetUrl)
	if bruteForce.config.wsAddress != "" {
		browserModule.SetWsAddress(bruteForce.config.wsAddress)
	}
	if bruteForce.config.exePath != "" {
		browserModule.SetExePath(bruteForce.config.exePath)
	}
	if bruteForce.config.proxy != "" {
		browserModule.SetProxy(bruteForce.config.proxy, bruteForce.config.proxyUsername, bruteForce.config.proxyPassword)
	}
	browserModule.SetLeakless(bruteForce.config.leakless)
	page, err := browserModule.Create()
	if page == nil {
		msg := fmt.Sprintf("bruteforce create page error: %s", err.Error())
		bruteForce.resultChannel <- &BruteResult{bruteInfo: msg}
		return utils.Error(err)
	}
	bruteForce.page = page
	err = bruteForce.init()
	if err != nil {
		msg := fmt.Sprintf("bruteforce init error: %s", err.Error())
		bruteForce.resultChannel <- &BruteResult{bruteInfo: msg}
		return utils.Error(err)
	}
	err = bruteForce.elementDetect()
	if err != nil {
		msg := fmt.Sprintf("bruteforce element detect error: %s", err.Error())
		bruteForce.resultChannel <- &BruteResult{bruteInfo: msg}
		return utils.Error(msg)
	}
	err = bruteForce.doBruteForce()
	if err != nil {
		msg := fmt.Sprintf("do bruteforce error: %s", err.Error())
		bruteForce.resultChannel <- &BruteResult{bruteInfo: msg}
		return utils.Errorf(msg)
	}
	return nil
}
