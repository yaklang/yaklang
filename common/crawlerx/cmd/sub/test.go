package sub

import (
	"fmt"
	"github.com/urfave/cli"
	"yaklang.io/yaklang/common/crawlerx"
	"yaklang.io/yaklang/common/crawlerx/tag"
	"yaklang.io/yaklang/common/crawlerx/teststh"
)

var TestModule = cli.Command{
	Name:   "test",
	Usage:  "test sth",
	Before: nil,
	After:  nil,

	OnUsageError: nil,
	Subcommands:  nil,

	Flags: []cli.Flag{},
	Action: func(c *cli.Context) error {
		//teststh.Test()
		//teststh.Test2()
		//teststh.PopUpTest()
		//teststh.GetHrefSelector()
		//teststh.VisitIco()
		teststh.ErrorUrlTest()
		//testFunction := detect.GetURLRepeatCheck(detect.UnLimit)
		//fmt.Println(testFunction("https://go-rod.github.io/#/network?id=proxy", "get"))
		return nil
	},
}

var Monitor = cli.Command{
	Name: "monitor",
	Action: func(c *cli.Context) error {
		//host := core.UploadServer()
		detect := new(tag.TDetect)
		//detect.SetRulePath("/Users/chenyangbao/Project/yak/common/crawlerx/tag/rules/test.yml")
		detect.Init()
		crawler := crawlerx.CreateCrawler("http://192.168.0.3/vulnerabilities/upload/")
		crawler.SetProxy("http://127.0.0.1:8083")
		ch := crawler.GetChannel()
		crawler.Monitor()
		for item := range ch {
			fmt.Println(item.Url(), item.Method(), item.RequestBody())
		}
		return nil
	},
}
