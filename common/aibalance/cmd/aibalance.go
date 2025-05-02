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
				fmt.Printf("Exiting due to signal [SIGTERM/SIGINT/SIGKILL]")
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
			Usage: "Path to configuration file",
			Value: "config.yaml",
		},
		cli.StringFlag{
			Name:  "listen, l",
			Usage: "Address to listen on",
			Value: ":8080",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) error {
		configPath := c.String("config")
		listenAddr := c.String("listen")

		// Read configuration file
		data, err := os.ReadFile(configPath)
		if err != nil {
			return errors.Errorf("Failed to read configuration file: %v", err)
		}

		fmt.Printf("Configuration file content:\n%s\n", string(data))

		var yamlConfig aibalance.YamlConfig
		if err := yaml.Unmarshal(data, &yamlConfig); err != nil {
			return errors.Errorf("Failed to parse configuration file: %v", err)
		}

		fmt.Printf("Parsed configuration:\nKeys: %+v\nModels: %+v\n", yamlConfig.Keys, yamlConfig.Models)

		// Convert to internal configuration
		config := yamlConfig.ToServerConfig()

		// Start server
		listener, err := net.Listen("tcp", listenAddr)
		if err != nil {
			return errors.Errorf("Failed to start server: %v", err)
		}
		defer listener.Close()

		fmt.Printf("Server started successfully, listening on: %s\n", listenAddr)

		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Printf("Failed to accept connection: %v\n", err)
				continue
			}
			go config.Serve(conn)
		}
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("Command execution failed: [%v] error: %v\n", strings.Join(os.Args, " "), err)
		return
	}
}
