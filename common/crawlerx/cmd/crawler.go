package main

import (
	"fmt"
	"github.com/urfave/cli"
	"os"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/crawlerx/cmd/sub"
	"github.com/yaklang/yaklang/common/log"
)

func main() {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		sub.TestModule,
		sub.Monitor,
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "url,u",
		},
		cli.BoolFlag{
			Name: "remote",
		},
	}
	app.Action = func(c *cli.Context) error {
		url := c.String("url")
		if url == "" {
			log.Errorf("empty url")
			return nil
		}
		remote := c.Bool("remote")
		crawler := crawlerx.CreateCrawler(url)
		//crawler.SetScanRange(detect.AllDomain)
		//crawler.SetScanRepeatLevel(detect.UnLimit)
		//crawler.SetMaxDepth(3)
		//crawler.SetTags("/Users/chenyangbao/temp.yml")
		//crawler.SetFormFill("用户名", "admin")
		//crawler.SetFormFill("密码", "admin123321")
		//crawler.SetTags("/Users/chenyangbao/Project/yak/common/crawlerx/tag/rules/rule.yml")
		//crawler.SetMaxDepth(3)
		//crawler.SetFullTimeout(30)
		//crawler.SetChromeWS("ws://0.0.0.0:9222/devtools/browser/e3a9e848-940c-493d-a2a8-95d285dd98e5")
		//crawler.SetFormFill("username", "admin")
		//crawler.SetFormFill("password", "password")
		//crawler.SetProxy("http://127.0.0.1:8083")
		if remote {
			crawler.SetUrlFromProxy(true)
			crawler.StartVRemote()
			return nil
		}
		crawler.SetMaxDepth(5)
		ch := crawler.GetChannel()
		//crawler.SetUrlFromProxy(true)
		crawler.StartV2()
		for item := range ch {
			fmt.Println(item.Url())
		}
		return nil
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("simulator cmd running error: %s", err)
		return
	}
}
