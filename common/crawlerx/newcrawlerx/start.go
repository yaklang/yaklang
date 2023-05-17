// Package newcrawlerx
// @Author bcy2007  2023/3/23 10:54
package newcrawlerx

import (
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

func StartCrawler(url string, opts ...ConfigOpt) (chan ReqInfo, error) {
	//err := checkTargetUrl(url)
	//if err != nil {
	//	return nil, utils.Errorf("target url %s check error: %s", url, err)
	//}
	ch := make(chan ReqInfo)
	opts = append(opts, WithResultChannel(ch))
	crawler, err := NewCrawler(url, opts...)
	if err != nil {
		return nil, utils.Errorf("create crawler error: %s", err)
	}
	go crawler.Run()
	return ch, nil
}

func checkTargetUrl(targetUrl string) error {
	req := CreateGetRequest(targetUrl)
	req.Request()
	err := req.Do()
	if err != nil {
		return utils.Errorf("http client send request error: %s", err)
	}
	result, err := req.Show()
	if err != nil {
		return utils.Errorf("get response error: %s", err)
	}
	bodyStr := matchBody(result)
	if bodyStr == `<body></body>` {
		return utils.Errorf("target url %s with blank info", targetUrl)
	}
	return nil
}

func matchBody(source string) string {
	removeSpaceReg, _ := regexp.Compile(`\s+`)
	temp := removeSpaceReg.ReplaceAllString(source, "")
	compiler, _ := regexp.Compile("<body>(.*)</body>")
	result := compiler.FindString(temp)
	return result
}
