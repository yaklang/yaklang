package examples

import (
	"fmt"
	"regexp"

	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/simulator/core"
	"github.com/yaklang/yaklang/common/simulator/extend"
	"github.com/yaklang/yaklang/common/utils"
)

func BruteForceModuleV2(urlStr string, configOpts ...ConfigOpt) (*BruteForceResult, error) {
	log.Infof("### bruteforce example ###")
	log.Infof("### load config ###")
	config := &Config{
		usernameList: make([]string, 0),
		passwordList: make([]string, 0),
	}
	for _, configOpt := range configOpts {
		configOpt(config)
	}
	log.Infof("### target url: %s ###", urlStr)

	pack := core.PageCreator()
	pack.SetURL(urlStr)
	if config.wsAddress != "" {
		pack.SetWsAddress(config.wsAddress)
	}
	if config.proxy != "" {
		pack.SetProxy(config.proxy, config.proxyUser, config.proxyPass)
	}
	page, _ := pack.Create()

	result := &BruteForceResult{}

	userElement, passElement, captchaElement, capPicElement, loginElement, err := pageInfoCollection(page, *config)
	if err != nil {
		return result, utils.Errorf("page info collection error: %s", err)
	}

	capModule := extend.CreateCaptcha()
	if config.captchaMode != "" {
		if config.captchaUrl == "" {
			capModule.SetIdentifyUrl("http://192.168.0.58:8008/runtime/text/invoke")
		} else {
			capModule.SetIdentifyUrl(config.captchaUrl)
		}
		capModule.SetRequestStruct(&extend.CaptchaRequest{})
		capModule.SetResponseStruct(&extend.CaptchaResult{})
		if config.captchaMode != "" {
			capModule.SetIdentifyMode(config.captchaMode)
		}
	} else {
		capModule.SetIdentifyUrl(config.captchaUrl)
		capModule.SetRequestStruct(&extend.DDDDCaptcha{})
		capModule.SetResponseStruct(&extend.DDDDResult{})
	}

	originHtml := page.HTML()

	go page.OriginPage().EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(page.OriginPage())
		},
	)()

	if len(config.usernameList) == 0 {
		config.usernameList = append(config.usernameList, "admin")
	}
	if len(config.passwordList) == 0 {
		config.passwordList = append(config.passwordList, "admin", "luckyadmin123")
	}

	for _, username := range config.usernameList {
		for _, password := range config.passwordList {
			b64, err := inputClickTry(
				page,
				userElement, passElement, captchaElement, capPicElement, loginElement,
				capModule,
				originHtml, username, password,
			)
			if err == nil {
				cookies := page.OriginPage().MustCookies()
				cookieStr := ""
				for _, cookie := range cookies {
					name := cookie.Name
					value := cookie.Value
					cookieStr += name + "=" + value + "; "
				}
				if cookieStr != "" {
					cookieStr = cookieStr[:len(cookieStr)-2]
				}

				result.SetLoginPngB64(b64)
				result.SetUsername(username)
				result.SetPassword(password)
				result.SetCookie(cookieStr)
				return result, nil
			}
			result.AddLog(err.Error())
			switch err.(type) {
			case *WrongUsernamePasswordError:
				continue
			case error:
				return result, err
			}
		}
	}
	return result, utils.Error("bruteforce failed.")
}

func pageInfoCollection(page *core.GeneralPage, config Config) (
	*core.GeneralElement,
	*core.GeneralElement,
	*core.GeneralElement,
	*core.GeneralElement,
	*core.GeneralElement,
	error) {
	log.Info("### Page Load End. Start Scan ###")
	elements, _ := page.FindElements("input")
	elements = elements.FilteredTypeElement("text", "password", "number", "tel")
	if elements.Empty() {
		return nil, nil, nil, nil, nil, utils.Errorf("none input found from page.")
	}

	// username
	var userElement *core.GeneralElement
	if config.usernameSelector != "" {
		_userElement, err := page.FindElement(config.usernameSelector)
		if err != nil {
			return nil, nil, nil, nil, nil, utils.Errorf("username selector find error: %s", err)
		}
		userElement = _userElement
	} else {
		userElements := elements.FilteredKeywordElements("username")
		if userElements.Single() {
			userElement = userElements.First()
		} else if userElements.Multi() {
			userElement = userElements.FilteredKeywordElement("username")
		} else {
			return nil, nil, nil, nil, nil, utils.Errorf("username element not found.")
		}
	}
	elements = elements.Slice(userElement)

	// password
	var passElement *core.GeneralElement
	if config.passwordSelector != "" {
		_passElement, err := page.FindElement(config.passwordSelector)
		if err != nil {
			return nil, nil, nil, nil, nil, utils.Errorf("password selector find error: %s", err)
		}
		passElement = _passElement
	} else {
		passElements := elements.FilteredKeywordElements("password")
		if passElements.Single() {
			passElement = passElements.First()
		} else if passElements.Multi() {
			passElement = passElements.FilteredKeywordElement("password")
		} else {
			return nil, nil, nil, nil, nil, utils.Errorf("password element not found.")
		}
	}
	elements = elements.Slice(passElement)

	// captcha
	var captchaElement *core.GeneralElement
	if config.captchaSelector != "" {
		_captchaElement, err := page.FindElement(config.captchaSelector)
		if err != nil {
			return nil, nil, nil, nil, nil, utils.Errorf("captcha selector find error: %s", err)
		}
		captchaElement = _captchaElement
	} else {
		captchaElements := elements.FilteredKeywordElements("captcha")
		if captchaElements.Single() {
			captchaElement = captchaElements.First()
		} else if captchaElements.Multi() {
			captchaElement = captchaElements.FilteredKeywordElement("captcha")
		}
	}

	// captcha img
	var captchaImgElement *core.GeneralElement
	if captchaElement != nil {
		if config.captchaImgSelector != "" {
			_captchaImgElement, err := page.FindElement(config.captchaImgSelector)
			if err != nil {
				return nil, nil, nil, nil, nil, utils.Errorf("captcha image selector find error: %s", err)
			}
			captchaImgElement = _captchaImgElement
		} else {
			captchaImgElements, _ := captchaElement.GeneralGetLatestElements("img", 5)
			if captchaImgElements.Length() == 0 {
				return nil, nil, nil, nil, nil, utils.Errorf("captcha %s exist but captcha img not exist", captchaElement)
			} else if captchaImgElements.Length() == 1 {
				captchaImgElement = captchaImgElements.First()
			} else {
				captchaImgElement = captchaImgElements.FilteredKeywordElement("captcha")
			}
		}
	}

	// submit button
	var button *core.GeneralElement
	if config.submitButtonSelector != "" {
		_button, err := page.FindElement(config.submitButtonSelector)
		if err != nil {
			return nil, nil, nil, nil, nil, utils.Errorf("button selector find error: %s", err)
		}
		button = _button
	} else {
		buttons, _ := page.GeneralFindElements("button")
		buttons = buttons.Slice(userElement)
		buttons = buttons.Slice(passElement)
		if buttons.Single() {
			button = buttons.First()
		} else if buttons.Multi() {
			button = buttons.FilteredKeywordElement("login")
		} else {
			return nil, nil, nil, nil, nil, utils.Errorf("button element not found.")
		}
	}
	log.Info("detect element: \nusername element: ", userElement,
		"\npassword element: ", passElement,
		"\ncaptcha element: ", captchaElement,
		"\ncaptcha image element: ", captchaImgElement,
		"\nlogin button element: ", button)
	return userElement, passElement, captchaElement, captchaImgElement, button, nil
}

func inputClickTry(
	page *core.GeneralPage,
	userElement, passElement, captchaElement, capPicElement, loginElement *core.GeneralElement,
	capModule *extend.CaptchaIdentifier,
	originHtml, username, password string) (string, error) {
	page.Refresh()
	log.Info("current account: ", username, ":", password)
	userElement.Input(username)
	passElement.Input(password)
	charCompiler, _ := regexp.Compile(`[^0-9a-zA-Z\-]`)
	if captchaElement != nil {
		capStr, err := capModule.Detect(capPicElement)
		if err != nil {
			log.Info(err)
			return "", err
		} else if charCompiler.MatchString(capStr) {
			return "", utils.Errorf("invalid captcha char: %s", capStr)
		} else {
			log.Infof("detect captcha result: %s", capStr)
		}
		captchaElement.Input(capStr)
	}
	err := page.StartListen()
	if err != nil {
		log.Infof("start listen err: %s", err)
	}
	err = loginElement.Click()
	if err != nil {
		log.Infof("login button click error: %s", err)
		return "", utils.Errorf("login button click error: %s", err)
	}
	page.Wait()
	//break
	words, err := page.StopListenWithBytes()
	if err != nil {
		log.Infof("get listen word error: %s", err)
	}
	if len(words) > 500 {
		words = words[:500]
		words = append(words, 46, 46, 46)
	}
	currentHtml := page.HTML()
	degree := extend.GetPageSimilarity(originHtml, currentHtml)
	log.Info("current page similarity degree: ", degree)
	log.Infof("page element change info: \n%s\n", string(words))
	if degree < 0.6 {
		log.Infof("login success! with username: %s & password: %s", username, password)
		pageB64, _ := page.ScreenShotResult()
		return pageB64, nil
	} else if degree > 0.8 {
		errInfo := fmt.Sprintf("login failed with %s:%s", username, password)
		if string(words) != "" {
			errInfo += "; page change info: " + string(words)
		}
		return "", NewWrongUsernamePasswordErrorf(errInfo)
	} else {
		if page.CurrentURL() != page.Url() {
			log.Infof("login success! with username: %s & passwprd: %s", username, password)
			pageB64, _ := page.ScreenShotResult()
			return pageB64, nil
		}
	}
	errInfo := fmt.Sprintf("login failed with %s:%s", username, password)
	if string(words) != "" {
		errInfo += "; page change info: " + string(words)
	}
	return "", NewWrongUsernamePasswordErrorf(errInfo)
}
