// Package crawlerx
// @Author bcy2007  2023/7/17 14:25
package crawlerx

import (
	"encoding/json"
	"testing"
)

func TestUrlCheck(t *testing.T) {
	urlA := `baidu.com`
	urlAResult, err := TargetUrlCheck(urlA, nil)
	t.Log(urlAResult, err)

	urlB := `111111111111111.com`
	urlBResult, err := TargetUrlCheck(urlB, nil)
	t.Log(urlBResult, err)

	urlC := `http://www.baidu.com`
	urlCResult, err := TargetUrlCheck(urlC, nil)
	t.Log(urlCResult, err)
}

func TestStartCrawler(t *testing.T) {
	urlStr := `http://testphp.vulnweb.com/`
	opts := make([]ConfigOpt, 0)
	browserInfo := BrowserInfo{
		ExePath:       "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		WsAddress:     "",
		ProxyAddress:  "",
		ProxyUsername: "",
		ProxyPassword: "",
	}
	browserBytes, _ := json.Marshal(&browserInfo)
	opts = append(opts,
		WithBrowserInfo(string(browserBytes)),
		WithFormFill(map[string]string{"username": "admin", "password": "password"}),
		WithFileInput(map[string]string{"default": "/Users/chenyangbao/1.txt"}),
		WithBlackList("logout"),
		WithMaxDepth(2),
		WithLeakless("true"),
		WithExtraWaitLoadTime(500),
		WithLocalStorage(map[string]string{"test": "abc"}),
		WithConcurrent(3),
	)
	ch, err := StartCrawler(urlStr, opts...)
	if err != nil {
		t.Error(err)
		return
	}
	for item := range ch {
		t.Logf(`%s %s from %s`, item.Method(), item.Url(), item.From())
	}
	t.Log(`done!`)
	return
}
