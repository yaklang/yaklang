// Package httpbrute
// @Author bcy2007  2023/6/21 14:04
package httpbrute

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/yaklang/yaklang/common/simulator/config"
	"testing"
	"time"
)

func TestHttpBruteForce(t *testing.T) {
	urlStr := "http://192.168.0.68/#/login"
	captchaUrl := "http://192.168.3.20:8008/runtime/text/invoke"
	opts := make([]BruteConfigOpt, 0)
	opts = append(opts,
		WithCaptchaUrl(captchaUrl),
		WithCaptchaMode("common_arithmetic"),
		WithUsername("admin"),
		WithPassword("admin", "admin123321"),
		WithExePath("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
		WithExtraWaitLoadTime(500),
		WithLeakless(config.LeaklessDefault),
	)
	ch, _ := HttpBruteForce(urlStr, opts...)
	for item := range ch {
		t.Logf(`[bruteforce] %s:%s login %v with info: %s`, item.Username(), item.Password(), item.Status(), item.Info())
		if item.Status() == true {
			t.Log(item.Base64())
		}
	}
}

func TestImgSrcGet(t *testing.T) {
	url := `http://192.168.3.20/#/login`
	browser := rod.New()
	launch := launcher.New()
	launch.Headless(false)
	controlUrl, _ := launch.Launch()
	browser = browser.ControlURL(controlUrl)
	browser.MustConnect()
	page := browser.MustPage(url).MustWaitLoad()
	element := page.MustElement(`#code`)
	t.Log(element)
	time.Sleep(3 * time.Second)
	t.Log(*element.MustAttribute("src"))
	result := page.MustEval(`()=>{return document.querySelector("#code").getAttribute("src")}`).String()
	t.Log(result)
	//time.Sleep(100 * time.Second)
}
