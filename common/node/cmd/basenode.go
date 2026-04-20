package main

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"runtime"
	"time"
)

func main() {
	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "api-url",
			Value: "http://127.0.0.1:8080",
		},
		cli.StringFlag{
			Name: "enrollment-token",
		},
		cli.StringFlag{
			Name:  "id",
			Usage: "Legacy node ID fallback; canonical node_id is assigned by platform",
		},
		cli.StringFlag{
			Name:  "name",
			Usage: "Display name reported to Legion",
			Value: fmt.Sprintf("testnode-[%s]", runtime.GOOS+runtime.GOARCH),
		},
		cli.StringFlag{
			Name:  "agent-installation-id",
			Usage: "Override persisted agent installation ID",
		},
		cli.StringFlag{
			Name:  "base-dir",
			Usage: "Node local state base directory",
		},
		cli.DurationFlag{
			Name:  "heartbeat-interval",
			Value: 30 * time.Second,
		},
	}

	app.Action = func(c *cli.Context) error {
		nodeBase, err := node.NewNodeBase(node.BaseConfig{
			NodeType:            spec.NodeType_Scanner,
			NodeID:              c.String("id"),
			DisplayName:         c.String("name"),
			AgentInstallationID: c.String("agent-installation-id"),
			BaseDir:             c.String("base-dir"),
			EnrollmentToken:     c.String("enrollment-token"),
			PlatformAPIBaseURL:  c.String("api-url"),
			HeartbeatInterval:   c.Duration("heartbeat-interval"),
		})
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
