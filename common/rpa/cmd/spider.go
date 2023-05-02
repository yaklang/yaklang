package main

import (
	"fmt"
	"os"
	"github.com/yaklang/yaklang/common/rpa"
	"github.com/yaklang/yaklang/common/rpa/cmd/semi"
	"github.com/yaklang/yaklang/common/rpa/core"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/urfave/cli"
)

func main() {
	app := cli.NewApp()
	app.Commands = []cli.Command{
		semi.BruteForce,
		semi.Test,
	}
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "host, H",
			Value: "127.0.0.1",
			Usage: "spider target host `URL`",
		},
		cli.IntFlag{
			Name:  "depth, D",
			Value: 3,
			Usage: "spider depth",
		},
		cli.StringFlag{
			Name:  "proxy",
			Usage: "spider proxy",
		},
		cli.StringFlag{
			Name:  "proxy-username",
			Usage: "spider proxy username",
		},
		cli.StringFlag{
			Name:  "proxy-password",
			Usage: "spider proxy password",
		},
		cli.BoolFlag{
			Name:  "stricturl",
			Usage: "strict url scan mode. do not click sensitive url",
		},
		cli.StringFlag{
			Name:  "headers",
			Usage: "scan headers by headers info file path (recommend) or json string (not recommend)",
		},
		cli.IntFlag{
			Name:  "maxurl",
			Value: 0,
			Usage: "max url scan number. 0 means no limit",
		},
		cli.IntFlag{
			Name:  "timeout",
			Usage: "page timeout default 20s",
		},
	}
	app.Action = func(c *cli.Context) error {
		host := c.String("host")
		depth := c.Int("depth")
		proxy := c.String("proxy")
		username := c.String("username")
		password := c.String("password")
		stricturl := c.Bool("stricturl")
		headers := c.String("headers")
		maxurl := c.Int("maxurl")
		timeout := c.Int("timeout")
		opts := make([]core.ConfigOpt, 0)
		opts = append(opts,
			core.WithSpiderDepth(depth),
			core.WithStrictUrlDetect(stricturl),
			core.WithUrlCount(maxurl),
			core.WithTimeout(timeout),
		)
		if headers != "" {
			opts = append(opts, core.WithHeader(headers))
		}
		if proxy != "" {
			if username == "" {
				opts = append(opts, core.WithBrowserProxy(proxy))
			} else {
				opts = append(opts, core.WithBrowserProxy(proxy, username, password))
			}
		}

		rsts, err := rpa.Start(host, opts...)
		if err != nil {
			return utils.Errorf("spider run error:%s", err)
		}
		// hasPrint filter repeat urls
		// hasPrint := filter.NewFilter()
		for rst := range rsts {
			url := rst.Url()
			fmt.Println(url)
		}
		return nil
	}
	err := app.Run(os.Args)
	if err != nil {
		println(err.Error())
		return
	}
}
