// Package simulator
// @Author bcy2007  2023/8/21 15:48
package simulator

import (
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/crawlerx/preaction"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	UsernameSelector    string
	PasswordSelector    string
	CaptchaSelector     string
	CaptchaImgSelector  string
	LoginButtonSelector string

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

		UsernameSelector:    config.usernameSelector,
		PasswordSelector:    config.passwordSelector,
		CaptchaSelector:     config.captchaSelector,
		CaptchaImgSelector:  config.captchaImgSelector,
		LoginButtonSelector: config.loginButtonSelector,

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
	if len(bruteForce.config.preActions) > 0 {
		err = preaction.PreActs(bruteForce.page, bruteForce.config.preActions)
		if err != nil {
			return
		}
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
		withSaveToDB(bruteForce.config.saveToDB),
		withSourceType(bruteForce.config.sourceType),
		withFromPlugin(bruteForce.config.fromPlugin),
		withRuntimeID(bruteForce.config.runtimeID),
	}
	starter := CreateNewStarter(opts...)
	bruteForce.starter = starter
	err = starter.Start()
	if err != nil {
		return utils.Errorf("browser starter start error: %v", err)
	}
	page, err := starter.CreatePage()
	if err != nil {
		return
	}
	err = page.Navigate(bruteForce.targetUrl)
	if err != nil {
		return
	}
	time.Sleep(3 * time.Second)
	err = page.WaitLoad()
	if bruteForce.config.extraWaitLoadTime != 0 {
		time.Sleep(time.Duration(bruteForce.config.extraWaitLoadTime) * time.Millisecond)
	}
	if err != nil {
		return
	}
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
	captchaModule.SetType(bruteForce.config.captchaType)
	if strings.Contains(bruteForce.config.captchaUrl, "/runtime/text/invoke") || bruteForce.config.captchaType == OtherOcr {
		captchaModule.SetRequest(&NormalCaptchaRequest{})
		captchaModule.SetResponse(&NormalCaptchaResponse{})
	} else if bruteForce.config.captchaType == NewDDDDOcr {
		captchaModule.SetRequest(&NewDDDDCaptcha{})
		captchaModule.SetResponse(&NewDDDDResult{})
	} else {
		captchaModule.SetRequest(&DDDDCaptcha{})
		captchaModule.SetResponse(&DDDDResult{})
	}
	bruteForce.captchaDetect = &captchaModule
	return
}

func (bruteForce *HttpBruteForceCore) elementDetect() (err error) {
	err = bruteForce.inputElementDetect()
	if err != nil {
		return
	}
	err = bruteForce.imgElementDetect()
	if err != nil {
		return
	}
	err = bruteForce.buttonElementDetect()
	return
}

func (bruteForce *HttpBruteForceCore) inputElementDetect() error {
	inputSearchInfo := map[string]map[string][]string{
		"input": {
			"type": {
				"text",
				"password",
				"number",
				"tel",
				"",
			},
		},
	}
	originInputElements, err := customizedGetElement(bruteForce.page, inputSearchInfo)
	if err != nil {
		return err
	}
	if len(originInputElements) == 0 {
		return utils.Error(`cannot find target input elements`)
	}
	removeElements := make(rod.Elements, 0)
	toCheckTags := make([]string, 0)
	//username
	if bruteForce.UsernameSelector != "" {
		usernameElement, err := bruteForce.page.Element(bruteForce.UsernameSelector)
		if err != nil {
			return utils.Errorf("find username element error: %v", err)
		}
		removeElements = append(removeElements, usernameElement)
	} else {
		toCheckTags = append(toCheckTags, "Username")
	}
	//password
	if bruteForce.PasswordSelector != "" {
		passwordElement, err := bruteForce.page.Element(bruteForce.PasswordSelector)
		if err != nil {
			return utils.Errorf("find password element error: %v", err)
		}
		removeElements = append(removeElements, passwordElement)
	} else {
		toCheckTags = append(toCheckTags, "Password")
	}
	//captcha
	if bruteForce.CaptchaSelector != "" {
		captchaElement, err := bruteForce.page.Element(bruteForce.CaptchaSelector)
		if err != nil {
			return utils.Errorf("find captcha element error: %v", err)
		}
		removeElements = append(removeElements, captchaElement)
	} else if bruteForce.config.captchaUrl != "" {
		toCheckTags = append(toCheckTags, "Captcha")
	}
	if len(toCheckTags) == 0 {
		return nil
	}
	inputElements := ElementsMinus(originInputElements, removeElements)
	// calculate
	result, err := CalculateRelevanceMatrix(inputElements, toCheckTags)
	if err != nil {
		return err
	}
	unCheckTags := make([]string, 0)
	removeElements = removeElements[:0]
	values := reflect.ValueOf(bruteForce).Elem()
	for _, tag := range toCheckTags {
		tagResult, ok := result[tag]
		if !ok || result[tag] == nil {
			unCheckTags = append(unCheckTags, tag)
		} else {
			val := values.FieldByName(tag + "Selector")
			tagSelector, err := GetSelector(tagResult)
			if err != nil {
				return utils.Errorf("element get selector error: %v", err)
			}
			t := reflect.ValueOf(tagSelector)
			val.Set(t)
			removeElements = append(removeElements, tagResult)
		}
	}
	// left tags
	if len(unCheckTags) > 0 {
		leftElements := ElementsMinus(inputElements, removeElements)
		unCheckResult, err := CheckTagElementFromParent(leftElements, unCheckTags)
		if err != nil {
			return utils.Errorf("check tag element from parent error: %v", err)
		}
		for _, tag := range unCheckTags {
			r, _ := unCheckResult[tag]
			if r == "" && tag != "Captcha" {
				return utils.Errorf("%v selector not found", tag)
			}
			val := values.FieldByName(tag + "Selector")
			_r := reflect.ValueOf(r)
			val.Set(_r)
		}
	}
	return nil
}

func (bruteForce *HttpBruteForceCore) imgElementDetect() error {
	if bruteForce.CaptchaSelector == "" {
		log.Debugf("null captcha selector. img detect canceled")
		return nil
	}
	if bruteForce.CaptchaImgSelector != "" {
		return nil
	}

	elements, err := FindLatestElement(bruteForce.page, bruteForce.CaptchaSelector, "img", 5)
	if err != nil {
		return utils.Error(err)
	}
	if len(elements) == 0 {
		return utils.Error(`cannot find target img elements`)
	} else if len(elements) == 1 {
		selector, err := GetSelector(elements.First())
		if err != nil {
			return utils.Error(err)
		}
		bruteForce.CaptchaImgSelector = selector
	} else {
		log.Debugf(`find img elements: %d`, len(elements))
		imgResult, err := CalculateRelevanceMatrix(elements, []string{"captcha"})
		if err != nil {
			return utils.Error(err)
		}
		imgElement, ok := imgResult["captcha"]
		if !ok || imgElement == nil {
			return utils.Error(`captcha img element not found`)
		}
		selector, err := GetSelector(imgElement)
		if err != nil {
			return utils.Error(err)
		}
		bruteForce.CaptchaImgSelector = selector
	}
	return nil
}

func (bruteForce *HttpBruteForceCore) buttonElementDetect() error {
	if bruteForce.LoginButtonSelector != "" {
		return nil
	}
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
		return utils.Errorf("get summit/button element error: %v", err)
	}
	if len(buttonElements) == 0 {
		return utils.Error(`cannot find target summit/button elements`)
	}
	log.Debugf(`find summit/button elements: %d`, len(buttonElements))
	buttonTags := []string{"Login"}
	buttonResult, err := CalculateRelevanceMatrix(buttonElements, buttonTags)
	if err != nil {
		return err
	}
	if buttonElement, ok := buttonResult["Login"]; !ok || buttonElement == nil {
		loginResult, err := CheckTagElementFromParent(buttonElements, buttonTags)
		if err != nil {
			return utils.Errorf("check login element from parents error: %v", err)
		}
		var loginSelector string
		var ok bool
		if loginSelector, ok = loginResult["Login"]; !ok || loginSelector == "" {
			return utils.Error(`login button selector not found`)
		}
		bruteForce.LoginButtonSelector = loginSelector
	} else {
		loginSelector, err := GetSelector(buttonElement)
		if err != nil {
			return utils.Error(err)
		}
		bruteForce.LoginButtonSelector = loginSelector
	}

	return nil
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
	//err := bruteForce.page.Navigate(bruteForce.originUrl)
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
	if len(bruteForce.config.preActions) > 0 {
		err = preaction.PreActs(bruteForce.page, bruteForce.config.preActions)
		if err != nil {
			return false, err
		}
	}
	err = ElementInput(bruteForce.page, bruteForce.UsernameSelector, username)
	if err != nil {
		return false, utils.Error(err)
	}
	err = ElementInput(bruteForce.page, bruteForce.PasswordSelector, password)
	if err != nil {
		return false, utils.Error(err)
	}
	if bruteForce.CaptchaSelector != "" {
		if bruteForce.captchaDetect == nil {
			return false, utils.Error("captcha detect mode not load")
		}
		captchaWords, err := bruteForce.captchaDetect.Detect(bruteForce.page, bruteForce.CaptchaImgSelector)
		if err != nil {
			return false, utils.Error(err)
		}
		log.Debugf("captcha words: %v", captchaWords)
		err = ElementInput(bruteForce.page, bruteForce.CaptchaSelector, captchaWords)
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
	err = ElementClick(bruteForce.page, bruteForce.LoginButtonSelector)
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
		time.Sleep(time.Second)
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
	log.Debugf(`%v %v`, info.URL, bruteForce.originUrl)
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
	log.Debugf(`%f %f`, degree, bruteForce.similarity)
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
		if bruteForce.starter == nil {
			return
		}
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
	log.Debugf("username: %v\npassword: %v\ncaptcha: %v\ncaptcha img: %v\nlogin button: %v\n",
		bruteForce.UsernameSelector, bruteForce.PasswordSelector, bruteForce.CaptchaSelector, bruteForce.CaptchaImgSelector, bruteForce.LoginButtonSelector)
	err = bruteForce.bruteForce()
	if err != nil {
		return utils.Errorf(`brute force error: %v`, err.Error())
	}
	return nil
}
