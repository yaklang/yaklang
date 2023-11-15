// Package crawlerx
// @Author bcy2007  2023/11/14 15:51
package crawlerx

import (
	"github.com/go-rod/rod"
	"github.com/yaklang/yaklang/common/log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

const invalidUrlTestHtml = `<html>
<head><title>TestInvalidUrl</title></head>
<body>
<a href="https://pre-bbs-cn.wanyol.com/%3C%=%20BASE_URL%20%%3Econf/env_config.js?v=%3C%=%20htmlWebpackPlugin.options.version%20%%3E" >click me</a>
</body>
</html>`

func TestHijackInvalidUrl(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(invalidUrlTestHtml))
	}))
	defer server.Close()
	browser := rod.New().MustConnect()
	router := NewBrowserHijackRequests(browser)
	_ = router.Add("*", "", func(hijack *CrawlerHijack) {
		t.Log(hijack.Request.URL().String())
		_ = hijack.LoadResponse(nil, true)
	})
	go func() {
		router.Run()
	}()
	time.Sleep(time.Second)
	page := browser.MustPage(server.URL).MustWaitLoad()
	page.MustElement("body > a").MustClick()
	time.Sleep(3 * time.Second)
}
