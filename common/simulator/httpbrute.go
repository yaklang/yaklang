// Package simulator
// @Author bcy2007  2023/8/21 15:48
package simulator

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type HttpBruteForceCore struct {
	targetUrl string
	config    *BruteConfig
	proxy     *url.URL

	captchaDetect   *CaptchaIdentifier
	resultChannel   chan Result
	html            string
	similarity      float64
	originUrl       string
	loginDetectFunc func() (bool, error)

	starter             *BrowserStarter
	page                *rod.Page
	usernameSelector    string
	passwordSelector    string
	captchaSelector     string
	captchaImgSelector  string
	loginButtonSelector string

	compiler *regexp.Regexp
	observer bool
}

func NewHttpBruteForceCore(targetUrl string, opts ...BruteConfigOpt) (*HttpBruteForceCore, error) {
	config := NewBruteConfig()
	for _, opt := range opts {
		opt(config)
	}
	bruteForceCore := HttpBruteForceCore{
		targetUrl: targetUrl,
		config:    config,

		resultChannel: config.ch,
		similarity:    config.similarityDegree,

		usernameSelector:    config.usernameSelector,
		passwordSelector:    config.passwordSelector,
		captchaSelector:     config.captchaSelector,
		captchaImgSelector:  config.captchaImgSelector,
		loginButtonSelector: config.loginButtonSelector,

		observer: true,
	}
	if config.proxy != "" {
		proxyUrl, err := url.Parse(config.proxy)
		if err != nil {
			return nil, utils.Errorf(`parse proxy url error: %v`, err.Error())
		}
		if config.proxyUsername != "" || config.proxyPassword != "" {
			proxyUser := url.UserPassword(config.proxyUsername, config.proxyPassword)
			proxyUrl.User = proxyUser
		}
		bruteForceCore.proxy = proxyUrl
	}
	compiler, _ := regexp.Compile(`[^0-9a-zA-Z\-]`)
	bruteForceCore.compiler = compiler
	return &bruteForceCore, nil
}

func (bruteForce *HttpBruteForceCore) init() (err error) {
	err = bruteForce.pageCreate()
	if err != nil {
		return
	}
	html, err := bruteForce.page.HTML()
	if err != nil {
		return
	}
	bruteForce.html = html
	go bruteForce.page.EachEvent(func(e *proto.PageJavascriptDialogOpening) {
		_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(bruteForce.page)
	})()
	info, err := bruteForce.page.Info()
	if err != nil {
		return
	}
	bruteForce.originUrl = info.URL
	if bruteForce.config.captchaUrl != "" {
		err = bruteForce.captchaModeInit()
		if err != nil {
			return
		}
	}
	loginDetectMap := map[loginDetectMode]func() (bool, error){
		UrlChangeMode:     bruteForce.loginDetectByUrl,
		HtmlChangeMode:    bruteForce.loginDetectByHTML,
		DefaultChangeMode: bruteForce.loginDetect,
	}
	if fn, ok := loginDetectMap[bruteForce.config.loginDetect]; ok {
		bruteForce.loginDetectFunc = fn
	} else {
		bruteForce.loginDetectFunc = bruteForce.loginDetect
	}
	return
}

func (bruteForce *HttpBruteForceCore) pageCreate() (err error) {
	opts := []BrowserConfigOpt{
		withExePath(bruteForce.config.exePath),
		withWsAddress(bruteForce.config.wsAddress),
		withProxy(bruteForce.proxy),
		withLeakless(bruteForce.config.leakless),
	}
	starter := CreateNewStarter(opts...)
	err = starter.Start()
	if err != nil {
		return
	}
	page, err := starter.CreatePage()
	if err != nil {
		return
	}
	err = page.Navigate(bruteForce.targetUrl)
	if err != nil {
		return
	}
	err = page.WaitLoad()
	if bruteForce.config.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(bruteForce.config.extraWaitLoadTime) * time.Millisecond)
	}
	if err != nil {
		return
	}
	bruteForce.starter = starter
	bruteForce.page = page
	return
}

func (bruteForce *HttpBruteForceCore) captchaModeInit() (err error) {
	err = connectTest(bruteForce.config.captchaUrl, bruteForce.proxy)
	if err != nil {
		return utils.Errorf(`captcha url %v connect error: %v`, bruteForce.config.captchaUrl, err)
	}
	captchaModule := CaptchaIdentifier{}
	captchaModule.SetUrl(bruteForce.config.captchaUrl)
	captchaModule.SetMode(bruteForce.config.captchaMode)
	if strings.Contains(bruteForce.config.captchaUrl, "/runtime/text/invoke") {
		captchaModule.SetRequest(&NormalCaptchaRequest{})
		captchaModule.SetResponse(&NormalCaptchaResponse{})
	} else {
		captchaModule.SetRequest(&DDDDCaptcha{})
		captchaModule.SetResponse(&DDDDResult{})
	}
	bruteForce.captchaDetect = &captchaModule
	return
}

func (bruteForce *HttpBruteForceCore) elementDetect() (err error) {
	inputSearchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"text",
				"password",
				"number",
				"tel",
			},
		},
	}
	if bruteForce.config.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(bruteForce.config.extraWaitLoadTime) * time.Millisecond)
	}
	inputElements, err := customizedGetElement(bruteForce.page, inputSearchInfo)
	if err != nil {
		return
	}
	if len(inputElements) == 0 {
		return utils.Error(`cannot find target input elements`)
	}
	log.Infof(`find input elements: %d`, len(inputElements))
	tags := []string{"username", "password", "captcha"}
	result, err := CalculateRelevanceMatrix(inputElements, tags)
	if err != nil {
		return
	}
	if bruteForce.usernameSelector == "" {
		if selector, ok := result["username"]; !ok || selector == "" {
			return utils.Error(`username selector not found`)
		} else {
			bruteForce.usernameSelector = selector
		}
	}
	if bruteForce.passwordSelector == "" {
		if selector, ok := result["password"]; !ok || selector == "" {
			return utils.Error(`password selector not found`)
		} else {
			bruteForce.passwordSelector = selector
		}
	}
	if bruteForce.captchaSelector == "" {
		if selector, ok := result["captcha"]; ok && selector != "" {
			bruteForce.captchaSelector = selector
		}
	}
	if bruteForce.captchaSelector != "" {
		var selector string
		var ok bool
		elements, err := FindLatestElement(bruteForce.page, bruteForce.captchaSelector, "img", 5)
		if err != nil {
			return utils.Error(err)
		}
		if len(elements) == 0 {
			return utils.Error(`cannot find target img elements`)
		} else if len(elements) == 1 {
			selector, err = GetSelector(elements.First())
			if err != nil {
				return utils.Error(err)
			}
		} else {
			log.Infof(`find img elements: %d`, len(elements))
			imgResult, err := CalculateRelevanceMatrix(elements, []string{"captcha"})
			if err != nil {
				return utils.Error(err)
			}
			if selector, ok = imgResult["captcha"]; !ok || selector == "" {
				return utils.Error(`captcha img element not found`)
			}
		}
		if bruteForce.captchaImgSelector == "" {
			bruteForce.captchaImgSelector = selector
		}
	}
	// captcha img
	buttonSearchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"submit",
				"button",
			},
		},
		"button": {},
	}
	buttonElements, err := customizedGetElement(bruteForce.page, buttonSearchInfo)
	if err != nil {
		return
	}
	if len(buttonElements) == 0 {
		return utils.Error(`cannot find target button elements`)
	}
	log.Infof(`find button elements: %d`, len(buttonElements))
	buttonTags := []string{"login"}
	buttonResult, err := CalculateRelevanceMatrix(buttonElements, buttonTags)
	if err != nil {
		return
	}
	if bruteForce.loginButtonSelector == "" {
		if selector, ok := buttonResult["login"]; !ok || selector == "" {
			return utils.Error(`login button selector not found`)
		} else {
			bruteForce.loginButtonSelector = selector
		}
	}
	return
}

func (bruteForce *HttpBruteForceCore) bruteForce() (err error) {
	for _, username := range bruteForce.config.usernameList {
		for _, password := range bruteForce.config.passwordList {
			status, err := bruteForce.login(username, password)
			if err != nil {
				return utils.Error(err)
			}
			if status {
				return nil
			}
		}
	}
	return nil
}

func (bruteForce *HttpBruteForceCore) login(username, password string) (bool, error) {
	err := bruteForce.page.Reload()
	if err != nil {
		return false, utils.Error(err)
	}
	if bruteForce.config.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(bruteForce.config.extraWaitLoadTime) * time.Millisecond)
	}
	err = ElementInput(bruteForce.page, bruteForce.usernameSelector, username)
	if err != nil {
		return false, utils.Error(err)
	}
	err = ElementInput(bruteForce.page, bruteForce.passwordSelector, password)
	if err != nil {
		return false, utils.Error(err)
	}
	if bruteForce.captchaSelector != "" {
		captchaWords, err := bruteForce.captchaDetect.Detect(bruteForce.page, bruteForce.captchaImgSelector)
		if err != nil {
			return false, utils.Error(err)
		}
		err = ElementInput(bruteForce.page, bruteForce.captchaSelector, captchaWords)
		if err != nil {
			return false, utils.Error(err)
		}
	}
	if bruteForce.observer {
		_, err = bruteForce.page.Eval(observer)
		if err != nil {
			log.Errorf(`create observer error: %v`, err)
			bruteForce.observer = false
		}
	}
	err = ElementClick(bruteForce.page, bruteForce.loginButtonSelector)
	if err != nil {
		return false, utils.Error(err)
	}
	err = bruteForce.page.WaitLoad()
	if err != nil {
		return false, utils.Error(err)
	}
	if bruteForce.config.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(bruteForce.config.extraWaitLoadTime) * time.Millisecond)
	}
	var objStr string
	if bruteForce.observer {
		obj, err := bruteForce.page.Eval(getObverserResult)
		if err != nil {
			log.Errorf(`get observer result error: %v`, err)
			bruteForce.observer = false
		} else {
			objStr = obj.Value.String()
			if len(objStr) > 500 {
				objStr = objStr[:500]
			}
		}
	}
	result := BruteResult{
		username:  username,
		password:  password,
		bruteInfo: objStr,
	}
	status, err := bruteForce.loginDetectFunc()
	if err != nil {
		return false, utils.Error(err)
	}
	if status {
		b64, _ := ScreenShot(bruteForce.page)
		result.b64 = b64
		result.status = true
		info, err := bruteForce.page.Info()
		if err == nil {
			result.loginSuccessUrl = info.URL
		}
		bruteForce.resultChannel <- &result
		return true, nil
	}
	result.status = false
	bruteForce.resultChannel <- &result
	return false, nil
}

func (bruteForce *HttpBruteForceCore) loginDetectByUrl() (bool, error) {
	info, err := bruteForce.page.Info()
	if err != nil {
		return false, utils.Error(err)
	}
	log.Infof(`%v %v`, info.URL, bruteForce.originUrl)
	if info.URL != bruteForce.originUrl {
		return true, nil
	}
	return false, nil
}

func (bruteForce *HttpBruteForceCore) loginDetectByHTML() (bool, error) {
	currentHtml, err := bruteForce.page.HTML()
	if err != nil {
		return false, utils.Error(err)
	}
	degree := GetPageSimilarity(currentHtml, bruteForce.html)
	log.Infof(`%f %f`, degree, bruteForce.similarity)
	if degree < bruteForce.similarity {
		return true, nil
	}
	return false, nil
}

func (bruteForce *HttpBruteForceCore) loginDetect() (bool, error) {
	status, err := bruteForce.loginDetectByUrl()
	if err != nil {
		return status, utils.Errorf(`login url detect error: %v`, err.Error())
	}
	if !status {
		return status, nil
	}
	status, err = bruteForce.loginDetectByHTML()
	if err != nil {
		return status, utils.Errorf(`login html info detect error: %v`, err.Error())
	}
	return status, nil
}

func (bruteForce *HttpBruteForceCore) Start() error {
	err := bruteForce.init()
	defer func() {
		err := bruteForce.starter.Close()
		if err != nil {
			log.Errorf(`browser close error: %v`, err.Error())
		}
	}()
	if err != nil {
		return utils.Errorf(`bruteforce init error: %v`, err.Error())
	}
	err = bruteForce.elementDetect()
	if err != nil {
		return utils.Errorf(`element detect error: %v`, err.Error())
	}
	log.Infof("username: %v\npassword: %v\ncaptcha: %v\ncaptcha img: %v\nlogin button: %v\n",
		bruteForce.usernameSelector, bruteForce.passwordSelector, bruteForce.captchaSelector, bruteForce.captchaImgSelector, bruteForce.loginButtonSelector)
	err = bruteForce.bruteForce()
	if err != nil {
		return utils.Errorf(`brute force error: %v`, err.Error())
	}
	return nil
}
