package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/urfave/cli"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
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
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.IntFlag{
			Name:  "port,p",
			Value: 8084,
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		addr := fmt.Sprintf("127.0.0.1:%v", c.Int("port"))

		defaultClient := utils.NewDefaultHTTPClient()

		r := mux.NewRouter()
		// ?url=http://www.baidu.com
		r.HandleFunc("/ssrf", func(writer http.ResponseWriter, request *http.Request) {
			defer func() {
				writer.WriteHeader(200)
			}()
			urlIns, err := lowhttp.ExtractURLFromHTTPRequest(request, false)
			if err != nil {
				writer.Write([]byte("empty"))
				return
			}
			targetUrl := urlIns.Query().Get("url")
			log.Infof("start to trigger ssrf: %v", targetUrl)
			rsp, err := defaultClient.Get(targetUrl)
			if err != nil {
				writer.Write([]byte(err.Error()))
				return
			}
			raw, err := utils.HttpDumpWithBody(rsp, true)
			if err != nil {
				writer.Write([]byte(err.Error()))
				return
			}
			writer.Write(raw)
			return
		})
		server := &http.Server{
			Handler:      r,
			Addr:         addr,
			WriteTimeout: 10 * time.Second,
			ReadTimeout:  10 * time.Second,
		}
		err := server.ListenAndServe()
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
