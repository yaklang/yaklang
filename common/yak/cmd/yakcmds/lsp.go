package yakcmds

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/lsp"
	"github.com/yaklang/yaklang/common/yakgrpc"
)

var LSPCommand = &cli.Command{
	Name:    "lsp",
	Usage:   "Start Yaklang Language Server Protocol (LSP) Server",
	Aliases: []string{"language-server"},
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "enable debug logging",
		},
		cli.StringFlag{
			Name:  "log-file",
			Usage: "log file path (default: stderr)",
		},
		cli.BoolFlag{
			Name:  "http",
			Usage: "use HTTP transport instead of stdio",
		},
		cli.StringFlag{
			Name:  "host",
			Value: "127.0.0.1",
			Usage: "HTTP server host (only for --http mode)",
		},
		cli.IntFlag{
			Name:  "port",
			Value: 9633,
			Usage: "HTTP server port (only for --http mode)",
		},
	},
	Action: func(c *cli.Context) error {
		// 配置日志输出
		if logFile := c.String("log-file"); logFile != "" {
			f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return err
			}
			defer f.Close()
			log.SetOutput(f)
		} else {
			// LSP 通过 stdio 通信，日志输出到 stderr
			log.SetOutput(os.Stderr)
		}

		if c.Bool("debug") {
			log.SetLevel(log.DebugLevel)
		} else {
			log.SetLevel(log.InfoLevel)
		}

		log.Info("initializing yaklang LSP server...")

		// 创建 gRPC 服务器（不启动网络监听）
		grpcServer, err := yakgrpc.NewServer(
			yakgrpc.WithInitFacadeServer(false),
		)
		if err != nil {
			return err
		}

		// 根据模式启动服务器
		if c.Bool("http") {
			host := c.String("host")
			port := c.Int("port")
			addr := fmt.Sprintf("%s:%d", host, port)
			log.Infof("starting LSP server on HTTP: %s", addr)
			return lsp.StartLSPHTTPServer(grpcServer, addr)
		} else {
			log.Info("starting LSP server on stdio...")
			return lsp.StartLSPServer(grpcServer)
		}
	},
}
