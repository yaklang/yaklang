package test

import (
	"fmt"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/simulator/core"
	"github.com/yaklang/yaklang/common/simulator/examples"
	"github.com/yaklang/yaklang/common/simulator/extend"
	"time"
)

func EvalTest() {
	//page := rod.New().MustConnect().MustPage("http://192.168.0.80/member.php?c=login")
	browser := rod.New()
	browser.Connect()
	//go browser.MustHandleAuth("admin", "admin")()
	w := browser.HandleAuth("admin", "admin")
	go w()
	waitttt := func(username, password string) func() {
		paused := &proto.FetchRequestPaused{}
		auth := &proto.FetchAuthRequired{}

		waitPaused := browser.WaitEvent(paused)
		waitAuth := browser.WaitEvent(auth)

		log.Info("!!")
		return func() {
			log.Info(1)
			waitPaused()
			log.Info(1.5)
			proto.FetchContinueRequest{
				RequestID: paused.RequestID,
			}.Call(browser)
			log.Info(2)
			waitAuth()
			log.Info(2.5)
			proto.FetchContinueWithAuth{
				RequestID: auth.RequestID,
				AuthChallengeResponse: &proto.FetchAuthChallengeResponse{
					Response: proto.FetchAuthChallengeResponseResponseProvideCredentials,
					Username: username,
					Password: password,
				},
			}.Call(browser)
			log.Info(3)
		}
	}
	go func() { waitttt("admin", "admin") }()
	//go waitttt("admin", "admin")
	//go browser.EachEvent(
	//	func(e *proto.TargetTargetCreated) {
	//		targetInfo := e.TargetInfo
	//		fmt.Println(targetInfo.URL, targetInfo.TargetID)
	//	},
	//)()
	//time.Sleep(time.Second)
	log.Info("aaa")
	page := browser.MustPage("http://192.168.1.1/")

	log.Info("bbb")
	page.MustWaitLoad()
	//browser.WaitEvent(proto.TargetTargetCreated{})

	go page.EachEvent(
		func(e *proto.PageJavascriptDialogOpening) {
			fmt.Println("qq")
			_ = proto.PageHandleJavaScriptDialog{Accept: false, PromptText: ""}.Call(page)
		},
		func(e *proto.PageWindowOpen) {
			fmt.Println("new windows open.")
			fmt.Println(e.WindowName)
			fmt.Println("---")
			pages, _ := browser.Pages()
			for _, page := range pages {
				fmt.Println(page.MustInfo().URL, page.TargetID)
			}
		},
	)()
	log.Info(page.HTML())
	time.Sleep(2 * time.Second)
}

func GetElementString() {
	url := "http://192.168.0.68/#/login"
	log.Infof("### bruteforce example ###")
	log.Infof("### target url: %s", url)
	pack := core.PageCreator()
	pack.SetURL(url)
	page := pack.Create()
	elements, _ := page.FindElements("input")
	fmt.Println(elements)
}

func GetElementParent() {
	url := "http://192.168.0.68/#/login"
	pack := core.PageCreator()
	pack.SetURL(url)
	page := pack.Create()
	selector := "#code"
	element, _ := page.FindElement(selector)
	if element == nil {
		fmt.Println("nil")
		return
	}
	//time.Sleep(time.Second)
	capmodule := extend.CaptchaIdentifier{}
	capmodule.SetIdentifyUrl("http://101.35.184.3:19199/runtime/text/invoke")
	capmodule.SetRequestStruct(&extend.CaptchaRequest{})
	capmodule.SetResponseStruct(&extend.CaptchaResult{})
	capStr, err := capmodule.Detect(element)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(capStr)
}

func test() core.GeneralElements {
	return core.GeneralElements{}
}

func testError() error {
	//return examples.NewWrongUsernamePasswordError("test error")
	//return utils.Error("aaaaa")
	return nil
}

func Length() {
	errA := testError()
	//errB := utils.Error("origin error")
	switch errA.(type) {
	case *examples.WrongUsernamePasswordError:
		log.Info("abcdefg")
	case error:
		log.Info("origin")
	case nil:
		log.Info("nilllll")
	}
}
