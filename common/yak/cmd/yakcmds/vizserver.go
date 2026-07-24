package yakcmds

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/yaklang/yaklang/common/log"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yakgrpc/aivizhttp"
)

// VizServerCommands 可视化监控服务相关命令
// 关键词: viz-server, agent monitor, dashboard
var VizServerCommands = []*cli.Command{
	vizServerCommand,
}

// viz-server 启动 AI Agent 可视化监控 HTTP 服务
// 与 gRPC server (yak grpc) 同进程运行, 通过 aireact 进程内注册表订阅实时事件,
// 同时从 SQLite profile DB 读取历史事件进行回放分析.
var vizServerCommand = &cli.Command{
	Name:  "viz-server",
	Usage: "Start the AI Agent visualization & monitoring dashboard server",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "host",
			Usage: "HTTP server bind host (default 0.0.0.0)",
		},
		cli.IntFlag{
			Name:  "port",
			Usage: "HTTP server bind port (default 9100)",
		},
		cli.StringFlag{
			Name:  "prefix",
			Usage: "route prefix (default /api/viz)",
		},
		cli.StringFlag{
			Name:  "auth-token",
			Usage: "bearer auth token (empty means no auth)",
		},
		cli.BoolTFlag{
			Name:  "fe",
			Usage: "serve the built-in dashboard web UI at the root path (default: on; use --fe=false to disable)",
		},
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug logging",
		},
	},
	Action: func(c *cli.Context) error {
		opts := []aivizhttp.Option{
			aivizhttp.WithHost(c.String("host")),
			aivizhttp.WithPort(c.Int("port")),
			aivizhttp.WithRoutePrefix(c.String("prefix")),
			aivizhttp.WithAuthToken(c.String("auth-token")),
			aivizhttp.WithServeFrontend(c.Bool("fe")),
			aivizhttp.WithDebug(c.Bool("debug")),
		}

		server, err := aivizhttp.NewVizHTTPServer(opts...)
		if err != nil {
			return fmt.Errorf("create viz http failed: %w", err)
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			log.Infof("received signal %v, shutting down...", sig)
			server.Shutdown()
		}()

		return server.Start()
	},
}
