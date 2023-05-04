package main

import (
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mq"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/thirdpartyservices"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"time"
)

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "amqp-url",
			Value: thirdpartyservices.GetAMQPUrl(),
		},
	}

	app.Action = func(c *cli.Context) error {
		nodeBase, err := node.NewNodeBase(
			spec.CommonRPCExchange, "testnode", "", "",
			mq.WithAMQPUrl(c.String("amqp-url")),
		)
		if err != nil {
			return err
		}

		go utils.WaitReleaseBySignal(func() {
			log.Info("sleep 3s then exit 1")
			nodeBase.Shutdown()
			time.Sleep(3 * time.Second)
			os.Exit(1)
		})
		nodeBase.Serve()
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Error(err)
		return
	}
}
