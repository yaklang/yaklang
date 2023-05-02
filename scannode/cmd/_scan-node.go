package main

import (
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"net"
	"os"
	"yaklang.io/yaklang/common/hybridscan"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/mq"
	"yaklang.io/yaklang/common/spec"
	"yaklang.io/yaklang/common/thirdpartyservices"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/scannode"
	"time"
)

func main() {
	app := cli.NewApp()

	var (
		snode *scannode.ScanNode
	)

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "amqp-url",
			Value: thirdpartyservices.GetAMQPUrl(),
		},
		cli.StringFlag{
			Name: "token",
		},
		cli.StringFlag{
			Name:  "id",
			Value: "dev-scan-node",
		},
	}

	app.Before = func(c *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		s, err := scannode.NewScanAgent(
			spec.CommonRPCExchange, c.String("id"), c.String("token"),
			mq.WithAMQPUrl(c.String("amqp-url")),
		)
		if err != nil {
			return errors.Errorf("init scan agent failed: %v", err)
		}

		snode = s

		go utils.WaitReleaseBySignal(func() {
			log.Info("sleep 3s then exit 1")
			snode.Shutdown()
			time.Sleep(3 * time.Second)
			os.Exit(1)
		})
		snode.Serve()
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:  "scan-port",
			Usage: "Scan Open Port",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "fp",
					Usage: "开启指纹探测功能",
				},
				cli.DurationFlag{
					Name:  "timeout",
					Usage: "设置整体扫描超时时间",
					Value: 30 * time.Second,
				},
				cli.StringFlag{
					Name:  "target,t",
					Usage: "设置扫描目标(支持 123.123.123.123/24 带掩码, 支持 122.123.123.12-233 带范围, 以及多个 IP 逗号分隔)",
				},
				cli.StringFlag{
					Name:  "port,p",
					Usage: "设置扫描端口(支持端口范围), 逗号分隔",
					Value: "80,8000-9000,443,20-60",
				},
				cli.IntFlag{
					Name:  "syn-concurrent",
					Usage: "设置 SYN 扫描并发(每秒多少个 SYN 包)",
					Value: 1000,
				},
			},
			Action: func(c *cli.Context) error {
				go utils.WaitReleaseBySignal(func() {
					os.Exit(1)
				})

				if c.String("target") == "" {
					return errors.New("target cannot be emtpy")
				}

				config, err := hybridscan.NewDefaultConfig()
				if err != nil {
					return errors.Errorf("get scan center config failed: %s", err)
				}
				config.DisableFingerprintMatch = c.Bool("fp")
				config.SynScanConfig.SendPacketsIntervalDuration = time.Second / time.Duration(c.Int("syn-concurrent"))

				log.Infof(
					"start with %v packets/seconds, interval: %v", c.Int("syn-concurrent"), config.SynScanConfig.SendPacketsIntervalDuration,
				)

				rootCtx := utils.TimeoutContext(c.Duration("timeout"))
				scanCenter, err := hybridscan.NewHyperScanCenter(
					rootCtx,
					config,
				)
				if err != nil {
					return errors.Errorf("build hyper scan center failed:%s", err)
				}

				err = scanCenter.Scan(rootCtx, c.String("target"), c.String("port"), false, func(ip net.IP, port int) {
					log.Infof("port open: %15s:%v", ip.String(), port)
				})
				if err != nil {
					return errors.Errorf("scan port failed: %s", err)
				}

				select {
				case <-rootCtx.Done():
					return nil
				}
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
		return
	}
}
