package main

import (
	"context"
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

var (
	sigExitOnce = new(sync.Once)
)

func init() {
	go sigExitOnce.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
		defer signal.Stop(c)

		for {
			select {
			case <-c:
				fmt.Printf("exit by signal [SIGTERM/SIGINT/SIGKILL]")
				os.Exit(1)
				return
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name: "transparent-mode",
		},
		cli.BoolFlag{
			Name: "hijack",
		},
		cli.StringFlag{
			Name:  "host",
			Value: "127.0.0.1",
		},
		cli.IntFlag{
			Name:  "port",
			Value: 8083,
		},
		cli.StringFlag{
			Name: "downstream-proxy,proxy",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		server, err := crep.NewMITMServer(
			crep.MITM_SetDownstreamProxy(strings.Split(c.String("proxy"), ",")...),
			crep.MITM_SetHTTPResponseMirror(func(isHttps bool, reqUrl string, _ *http.Request, _ *http.Response, remoteAddr string) {

			}),
			crep.MITM_SetTransparentMirror(func(isHttps bool, req []byte, rsp []byte) {
				log.Info("recv https connection")
				log.Infof("request: \n%v\n\n%v", string(req), string(rsp))
			}),
		)
		if err != nil {
			return utils.Errorf("create mitm server failed: %s", err)
		}

		if c.Bool("hijack") {
			err = server.Configure(crep.MITM_SetTransparentHijackMode(true))
			if err != nil {
				return utils.Errorf("configure failed: %s", err)
			}
		}

		var addr = utils.HostPort(c.String("host"), c.String("port"))
		if c.Bool("transparent-mode") {
			log.Infof("start transparent mitm server: %s", addr)
			err = server.ServeTransparentTLS(
				context.Background(),
				addr,
			)
			if err != nil {
				return err
			}
		}

		err = server.Serve(
			context.Background(),
			addr,
		)
		if err != nil {
			return err
		}
		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
