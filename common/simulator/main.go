// Package simulator
// @Author bcy2007  2023/8/22 13:35
package simulator

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// HttpBruteForce 进行目标url的页面进行http爆破 第一个参数为目标url，后面可以添加零个或多个请求选项，用于对此次请求进行配置 返回值包括channel和错误，从channel中获取爆破结果
//
// Example:
// ```
//
//	ch, err = simulator.HttpBruteForce("http://127.0.0.1:8080/", simulator.username("admin"), simulator.password("admin", "luckyadmin123"))
//
//	for item := range ch {
//	    yakit.Info(`[bruteforce] %s:%s login %v with info: %s`, item.Username(), item.Password(), item.Status(), item.Info())
//	}
//
// ```
func HttpBruteForce(targetUrl string, opts ...BruteConfigOpt) (chan Result, error) {
	ch := make(chan Result)
	opts = append(opts, WithResultChannel(ch))
	bruteForce, err := NewHttpBruteForceCore(targetUrl, opts...)
	if err != nil {
		close(ch)
		return nil, utils.Error(err)
	}
	go func() {
		defer close(ch)
		bruteError := bruteForce.Start()
		if bruteError != nil {
			ch <- &BruteResult{
				username:        "",
				password:        "",
				status:          false,
				bruteInfo:       bruteError.Error(),
				b64:             "",
				token:           "",
				loginSuccessUrl: "",
			}
			log.Error(bruteError)
		}
	}()
	return ch, nil
}
