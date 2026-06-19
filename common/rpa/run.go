package rpa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/rpa/core"
)

// Start 启动一个基于浏览器的 RPA 爬虫，对目标 URL 进行自动化爬取（导出名为 rpa.Start）
// 参数:
//   - url: 起始 URL
//   - opt: 可选项，如 rpa.depth、rpa.proxy、rpa.maxUrl、rpa.timeout 等
//
// 返回值:
//   - 请求结果的 channel，可使用 for-range 遍历
//   - 错误信息
//
// Example:
// ```
// // 启动 RPA 爬虫并遍历抓取到的请求（示意性示例，需要本地已安装浏览器）
// ch = rpa.Start("http://example.com", rpa.depth(2), rpa.maxUrl(50))~
//
//	for req := range ch {
//	    println(req.Url())
//	}
//
// ```
func Start(url string, opt ...core.ConfigOpt) (chan core.RequestIf, error) {
	ch := make(chan core.RequestIf)
	opt = append(opt, core.WithOnRequest(func(req *core.Req) {
		ch <- req
	}))
	manager, err := core.NewManager(
		url,
		ch,
		opt...,
	)
	if err != nil {
		// panic(err)
		return nil, err
	}

	go func() {
		defer close(ch)
		err = manager.Run()
		if err != nil {
			// panic(err)
			log.Infof("manager run error:%s", err)
			return
		}
	}()

	return ch, nil
}
