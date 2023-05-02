// Package teststh
// @Author bcy2007  2023/3/27 11:14
package teststh

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"time"
	"yaklang.io/yaklang/common/crawlerx/newcrawlerx"
	"yaklang.io/yaklang/common/log"
)

var targetURL = `http://123.57.216.171/login.php`

var testUrl = `http://testphp.vulnweb.com/`

//var targetURL = `http://192.168.0.3/login.php`

func createBrowser() *rod.Browser {
	//u := launcher.New().Headless(false).Proxy("http://127.0.0.1:8083").MustLaunch()
	u := launcher.New().Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	browser.MustIgnoreCertErrors(true)
	//hijackRouter := browser.HijackRequests()
	//hijackRouter.MustAdd("*", func(hijack *rod.Hijack) {
	//	log.Info(hijack.Request.URL().String())
	//	hijack.ContinueRequest(&proto.FetchContinueRequest{})
	//})
	//go hijackRouter.Run()
	go browser.EachEvent(
		func(e *proto.TargetTargetCreated) {
			targetID := e.TargetInfo.TargetID
			page, err := browser.PageFromTarget(targetID)
			log.Info(page, err)
		},
	)()
	return browser
}

func PopUpTest() {
	u := launcher.New().Headless(true).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	//browser := rod.New().MustConnect()
	page := browser.MustPage(targetURL)
	log.Info(page.TargetID)
	page.MustElement("#content > form > fieldset > input:nth-child(2)").MustInput("admin")
	page.MustElement("#content > form > fieldset > input:nth-child(5)").MustInput("password")
	page.MustElement("#content > form > fieldset > p > input[type=submit]").MustClick()
	page.MustWaitLoad()
	time.Sleep(time.Second)

	page.Navigate("http://123.57.216.171/vulnerabilities/brute/")
	page.MustWaitLoad()
	go browser.EachEvent(
		func(e *proto.TargetTargetCreated) {
			targetID := e.TargetInfo.TargetID
			page := browser.MustPageFromTargetID(targetID)
			page.MustWaitLoad()
			time.Sleep(time.Second)
			log.Info(targetID, page.MustInfo().URL, page.MustHTML())
			page.Close()
		},
	)()
	page.MustElement("#source_button").MustClick()
	time.Sleep(time.Second)
	page.Close()
	time.Sleep(time.Second)
	pages := browser.MustPages()
	log.Info(pages)
	for _, page := range pages {
		log.Info(page.MustInfo().URL, ": ", page.MustHTML())
	}
}

func GetHrefSelector() {
	browser := createBrowser()
	page := browser.MustPage(testUrl)
	page.MustWaitLoad()
	time.Sleep(time.Second)
	//result, _ := page.Eval(newcrawlerx.FindHref)
	//log.Info(result)
	resultObj, _ := proto.RuntimeEvaluate{
		IncludeCommandLineAPI: true,
		ReturnByValue:         true,
		Expression:            newcrawlerx.GetHrefSelector,
	}.Call(page)
	log.Info(resultObj.Result)
}

func VisitIco() {
	browser := createBrowser()
	browser.MustPage("https://www.zhihu.com/").MustWaitLoad()
}

func ErrorUrlTest() {
	browser := createBrowser()
	page, err := browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	log.Info(err)
	err = page.Navigate("http://111111.com/")
	log.Info(err)
	log.Info(page.MustHTML())
}
