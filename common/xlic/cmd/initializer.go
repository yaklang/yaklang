package main

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/xlic"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	oss "github.com/aliyun/aliyun-oss-go-sdk/oss"
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
				return
			}
		}
	})
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{
		{
			Name: "gen-request",
			Action: func(c *cli.Context) error {
				req, err := xlic.Machine.GenerateRequest()
				if err != nil {
					return err
				}
				fmt.Println(req)
				return nil
			},
		},
	}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "ak",
		},
		cli.StringFlag{
			Name: "sk",
		},
		cli.StringFlag{
			Name:  "endpoint",
			Value: `oss-accelerate.aliyuncs.com`,
		},
	}

	app.Action = func(c *cli.Context) error {
		log.Info("start to create oss util to %v", c.String("endpoint"))
		client, err := oss.New(c.String("endpoint"), c.String("ak"), c.String("sk"))
		if err != nil {
			return err
		}

		bucket, err := client.Bucket("yaklang")
		if err != nil {
			return err
		}

		priKey := `common/xlic/yaklang.client.license.pri.secret`
		pubKey := `common/xlic/yaklang.client.license.pub.secret`
		os.RemoveAll(priKey)
		os.RemoveAll(pubKey)

		log.Infof("start to download file: %v", priKey)
		err = bucket.DownloadFile(
			"yaklang.client.license.pri.secret", priKey,
			100*1024*1024)
		if err != nil {
			return err
		}
		defer os.RemoveAll(priKey)

		log.Infof("start to download file: %v", pubKey)
		err = bucket.DownloadFile(
			"yaklang.client.license.pub.secret", pubKey,
			100*1024*1024)
		if err != nil {
			return err
		}
		defer os.RemoveAll(pubKey)

		priGzip := `common/xlic/pri.gzip`
		pubGzip := `common/xlic/pub.gzip`

		os.RemoveAll(priGzip)
		os.RemoveAll(pubGzip)

		log.Infof("start to gzip file: %v", priKey)
		priRaw, err := ioutil.ReadFile(priKey)
		if err != nil {
			return err
		}
		priRaw, err = utils.GzipCompress(priRaw)
		if err != nil {
			return err
		}
		err = os.WriteFile(priGzip, priRaw, 0644)
		if err != nil {
			return err
		}

		log.Infof("start to gzip file: %v", pubKey)
		pubRaw, err := ioutil.ReadFile(pubKey)
		if err != nil {
			return err
		}
		pubRaw, err = utils.GzipCompress(pubRaw)
		if err != nil {
			return err
		}
		err = os.WriteFile(pubGzip, pubRaw, 0644)
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
