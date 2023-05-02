package main

import (
	"context"
	"github.com/urfave/cli"
	"net"
	"os"
	"time"
	"yaklang/common/fp"
	"yaklang/common/hybridscan"
	"yaklang/common/log"
	"yaklang/common/utils"
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
		}}
	app.Action = func(c *cli.Context) {
		t := c.String("target")
		p := c.String("ports")
		if t == "" || p == "" {
			log.Errorf("empty host[%v] or port[%v]", t, p)
			return
		}

		ctx, _ := context.WithTimeout(context.Background(), c.Duration("timeout"))
		config, err := hybridscan.NewDefaultConfig()
		if err != nil {
			log.Errorf("create default config failed: %s", err)
			return
		}
		center, err := hybridscan.NewHyperScanCenter(ctx, config)
		if err != nil {
			log.Error(err)
			return
		}

		_ = center.RegisterMatcherResultHandler("fpMatch", func(matcherResult *fp.MatchResult, err error) {
			log.Infof("tcp://%v  service: %v", utils.HostPort(matcherResult.Target, matcherResult.Port), matcherResult.GetServiceName())
		})

		err = center.Scan(ctx, t, p, true, false, func(ip net.IP, port int) {
			log.Infof("open port: %v:%v", ip.String(), port)
		})
		if err != nil {
			log.Error(err)
			return
		}

		select {
		case <-ctx.Done():
		}

		return
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
		return
	}
}
