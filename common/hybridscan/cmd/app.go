package main

import (
	"context"
	"os"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
)

func main() {
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "target,host,t",
		},
		cli.StringFlag{
			Name:  "ports,p",
			Value: "80,443,8080",
		},
		cli.DurationFlag{
			Name:  "timeout",
			Value: 3 * time.Minute,
		},
	}
	app.Action = func(c *cli.Context) {
		t := c.String("target")
		p := c.String("ports")
		if t == "" || p == "" {
			log.Errorf("empty host[%v] or port[%v]", t, p)
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), c.Duration("timeout"))
		defer cancel()
		resultCh, err := synscanx.Scan(ctx, t, p, synscanx.WithWaiting(c.Duration("timeout").Seconds()))
		if err != nil {
			log.Error(err)
			return
		}
		for result := range resultCh {
			log.Infof("open port: %v", utils.HostPort(result.Host, result.Port))
		}
	}

	if err := app.Run(os.Args); err != nil {
		log.Error(err)
	}
}
