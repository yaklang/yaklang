package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/aibalance"
	"gopkg.in/yaml.v3"
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
		cli.StringFlag{
			Name:  "config, c",
			Usage: "配置文件路径",
			Value: "config.yaml",
		},
		cli.StringFlag{
			Name:  "listen, l",
			Usage: "监听地址",
			Value: ":8080",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		configPath := c.String("config")
		listenAddr := c.String("listen")

		// 读取配置文件
		data, err := os.ReadFile(configPath)
		if err != nil {
			return errors.Errorf("读取配置文件失败: %v", err)
		}

		fmt.Printf("配置文件内容:\n%s\n", string(data))

		var yamlConfig aibalance.YamlConfig
		if err := yaml.Unmarshal(data, &yamlConfig); err != nil {
			return errors.Errorf("解析配置文件失败: %v", err)
		}

		fmt.Printf("解析后的配置:\nKeys: %+v\nModels: %+v\n", yamlConfig.Keys, yamlConfig.Models)

		// 转换为内部配置
		config := yamlConfig.ToConfig()

		// 启动服务器
		listener, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return errors.Errorf("启动服务器失败: %v", err)
		}
		defer listener.Close()

		fmt.Printf("服务器启动成功，监听地址: %s\n", listenAddr)

		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Printf("接受连接失败: %v\n", err)
				continue
			}
			go config.Serve(conn)
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("command: [%v] failed: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
