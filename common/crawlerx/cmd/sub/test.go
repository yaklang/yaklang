package sub

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/crawlerx/tag"
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
