package main

import (
	"context"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"strconv"
	"sync"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

func main() {
	app := cli.NewApp()
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "type",
			Usage: "插件类型(空则检查所有)",
		},
		cli.BoolFlag{
			Name:  "skipstatic",
			Usage: "跳过静态代码检查",
		},
		cli.BoolFlag{
			Name:  "skipecho",
			Usage: "跳过echoServer 误报检查",
		},
		cli.BoolFlag{
			Name:  "skiplogic",
			Usage: "跳过发包数量逻辑检查",
		},
		cli.IntFlag{Name: "concurrent", Value: 10, Usage: "并发数(默认10)"},
	}

	app.Action = func(c *cli.Context) error {
		skipStatic := c.Bool("skipstatic")
		skipEcho := c.Bool("skipecho")
		skipLogic := c.Bool("skiplogic")
		typ := c.String("type")
		concurrent := c.Int("concurrent")
		// get post-scan scripts
		db := consts.GetGormProfileDatabase()
		if typ != "" {
			db = db.Where("type = ?", typ)
		}
		db = db.Order("created_at desc")

		fp, err := os.OpenFile("result.csv", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.Errorf("open file failed: %s", err)
			return err
		}

		outputMutex := sync.Mutex{}
		handlerOutput := func(plugin *schema.YakScript, res *ypb.SmokingEvaluatePluginResponse) {
			outputMutex.Lock()
			defer outputMutex.Unlock()
			// write to cvs file, plugin-name | score | result
			line := fmt.Sprintf("%s,%d,", plugin.ScriptName, res.Score)
			for _, r := range res.Results {
				line += fmt.Sprintf("%s,", strconv.Quote(r.Severity+"--"+r.Item))
			}
			line += "\n"
			fp.WriteString(line)
		}

		errorMutex := sync.Mutex{}
		errorOutput := func(plugin *schema.YakScript, err error) {
			errorMutex.Lock()
			defer outputMutex.Unlock()
			line := fmt.Sprintf("%s,0,%s\n", plugin.ScriptName, err.Error())
			fp.WriteString(line)
		}

		// project and profile
		consts.InitializeYakitDatabase("", "")

		// cve
		_, err = consts.InitializeCVEDatabase()
		if err != nil {
			log.Warnf("initialized cve database failed: %v", err)
		}

		// 调用一些数据库初始化的操作
		err = yakit.CallPostInitDatabase()
		if err != nil {
			return err
		}

		// client
		// client, err := NewLocalClient()
		// require.NoError(t, err)
		s, err := yakgrpc.NewServer(yakgrpc.WithInitFacadeServer(true))
		if err != nil {
			log.Errorf("build yakit server failed: %s", err)
			// finalErr = err
			return err
		}
		grpcTrans := grpc.NewServer(
			grpc.MaxRecvMsgSize(100*1024*1024),
			grpc.MaxSendMsgSize(100*1024*1024),
		)
		ypb.RegisterYakServer(grpcTrans, s)

		swg := utils.NewSizedWaitGroup(concurrent)
		// smoking
		for script := range yakit.YieldYakScripts(db, context.Background()) {
			script := script
			swg.Add()
			go func() {
				pluginTestingServer := yakgrpc.NewPluginTestingEchoServer(context.Background())
				res, err := s.EvaluatePluginEx(context.Background(), script.Content, script.Type, pluginTestingServer, skipStatic, skipEcho, skipLogic)
				if err != nil {
					log.Errorf("error plugin:%s err: %v", script.ScriptName, err)
					errorOutput(script, err)
				} else {
					handlerOutput(script, res)
				}
			}()
		}

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
