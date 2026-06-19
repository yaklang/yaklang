// Package crawlerx
// @Author bcy2007  2023/11/1 10:16
package crawlerx

import (
	"context"
	"encoding/base64"
	"github.com/go-rod/rod/lib/proto"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"strings"
	"time"
)

func (starter *BrowserStarter) targetResponseReplace() error {
	hijackRouter := NewBrowserHijackRequests(starter.browser)
	err := hijackRouter.Add("*", "", func(hijack *CrawlerHijack) {
		requestUrl := hijack.Request.URL().String()
		for targetUrl, res := range starter.baseConfig.response {
			if targetUrl == requestUrl {
				header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(res))
				headerList := strings.Split(header, "\n")
				headerObj := make([]string, 0)
				for _, h := range headerList {
					h = strings.TrimSpace(h)
					items := strings.Split(h, ": ")
					if len(items) > 1 {
						headerObj = append(headerObj, items[0], items[1])
					}
				}
				hijack.Response.SetBody(body)
				hijack.Response.SetHeader(headerObj...)
				return
			}
		}
		hijack.ContinueRequest(&proto.FetchContinueRequest{})
	})
	if err != nil {
		return utils.Errorf("create hijack router error: %v", err)
	}
	go func() {
		hijackRouter.Run()
	}()
	return nil
}

// PageScreenShot 使用浏览器访问目标页面并截图，返回截图结果(通常为 base64 编码字符串)
// 在 yak 中通过 crawlerx.PageScreenShot 调用，依赖本地 Chrome 浏览器环境
// 参数:
//   - targetUrl: 需要截图的目标页面 URL
//   - opts: 可选配置项，如 crawlerx.browserInfo、crawlerx.stealth 等
//
// 返回值:
//   - 截图结果字符串
//   - 错误信息，失败时非 nil
//
// Example:
// ```
// // 该示例为示意性用法：对目标页面截图
// code = crawlerx.PageScreenShot("http://testphp.vulnweb.com/")~
// println(len(code))
// ```
func NewPageScreenShot(targetUrl string, opts ...ConfigOpt) (code string, err error) {
	config := NewConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	opts = append(opts,
		WithTargetUrl(targetUrl),
		WithContext(ctx),
	)
	for _, opt := range opts {
		opt(config)
	}
	var browserConfig *BrowserConfig
	if len(config.browsers) > 0 {
		browserConfig = config.browsers[0]
	} else {
		browserConfig = &BrowserConfig{}
	}
	starter := NewBrowserStarter(browserConfig, config.baseConfig)
	err = starter.baseBrowserStarter()
	if err != nil {
		return
	}
	//if response != "" {
	err = starter.targetResponseReplace()
	if err != nil {
		return
	}
	//}
	page, err := starter.browser.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return
	}
	err = page.Navigate(targetUrl)
	if err != nil {
		return
	}
	err = page.WaitLoad()
	if err != nil {
		return
	}
	pngBytes, err := page.Screenshot(
		true,
		&proto.PageCaptureScreenshot{
			Format: proto.PageCaptureScreenshotFormatPng,
		},
	)
	if err != nil {
		return
	}
	pngBase64 := base64.StdEncoding.EncodeToString(pngBytes)
	code = "data:image/png;base64," + pngBase64
	return
}
