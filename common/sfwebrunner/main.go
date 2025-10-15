package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sfweb"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var (
	sigExitOnce          = new(sync.Once)
	runCtx, runCtxCancel = context.WithCancel(context.Background())
)

func init() {
	go sigExitOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		defer func() {
			runCtxCancel()
			signal.Stop(c)
		}()

		for {
			select {
			case <-c:
				fmt.Println("exit by signal [SIGTERM/SIGINT/SIGKILL]")
				runCtxCancel()
				os.Exit(1)
				return
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	// aes rsa - http://116.214.131.28/wui/index.html#/?logintype=1&_key=g2jsh9

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port,p",
			Value: 13338,
		},
		cli.BoolFlag{
			Name: "nohttps",
		},
		cli.StringFlag{
			Name:  "host,t",
			Value: `127.0.0.1`,
		},
		cli.StringFlag{
			Name:  "key",
			Usage: "chatglm api key",
		},
		cli.BoolFlag{
			Name: "debug",
		},
		cli.StringFlag{
			Name:  "crtPath",
			Usage: "server.crt path",
		},
		cli.StringFlag{
			Name:  "keyPath",
			Usage: "server.key path",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		yakit.InitialDatabase()
		sfbuildin.SyncEmbedRule(func(process float64, ruleName string) {
			log.Infof("sync embed rule: %s, process: %f", ruleName, process)
		})
		debug := c.Bool("debug")
		port := c.Int("port")
		opts := []sfweb.ServerOpt{
			sfweb.WithHost(c.String("host")),
			sfweb.WithPort(port),
			sfweb.WithDebug(debug),
			sfweb.WithHttps(!c.Bool("nohttps")),
			sfweb.WithChatGLMAPIKey(c.String("key")),
			sfweb.WithServerCrtPath(c.String("crtPath")),
			sfweb.WithServerKeyPath(c.String("keyPath")),
		}
		servers, err := sfweb.NewSyntaxFlowWebServer(runCtx, opts...)
		if debug {
			sfweb.SfWebLogger.SetLevel("debug")
		}
		if err != nil {
			log.Errorf("start syntaxflow web server failed: %v", err)
			return err
		}
		ifs, _ := net.Interfaces()
		for _, i := range ifs {
			addrs, _ := i.Addrs()
			for _, addr := range addrs {
				ip := addr.String()
				ip, _, _ = strings.Cut(ip, "/")
				if !utils.IsIPv4(ip) {
					continue
				}
				log.Infof("checking on: %v:%v", ip, port)
			}
		}
		log.Infof("SyntaxFlow Web Server running in: %s", servers)
		select {
		case <-runCtx.Done():
			return nil
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
