package main

import (
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/utils"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/aibalance"
	"github.com/yaklang/yaklang/common/aibalance/aiforwarder"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
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
				log.Errorf("Exiting due to signal [SIGTERM/SIGINT/SIGKILL]")
				os.Exit(1)
				return
			}
		}
	})
}

func main() {
	consts.InitializeYakitDatabase("", "", "")

	app := cli.NewApp()
	app.Name = "aibalance"
	app.Usage = "AI Load Balancer and Management Tool"
	app.Version = "1.0.0"

	// 添加命令
	app.Commands = []cli.Command{
		{
			Name:  "forward",
			Usage: "Create a forwarder for AI models",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "url,u",
					Usage: "URL of the aibalance server",
				},
				cli.StringFlag{
					Name:  "config,c",
					Usage: "Path to configuration file",
					Value: "config.yaml",
				},
			},
			Action: func(c *cli.Context) error {
				url := c.String("url")
				if url == "" {
					return cli.NewExitError("URL Server is required", 1)
				}

				configPath := c.String("config")

				forwarder := aiforwarder.NewAIForwarder(url)
				log.Infof("Loading configuration from: %s", configPath)
				err := forwarder.LoadFromYaml(configPath)
				if err != nil {
					log.Errorf("Failed to load configuration: %v", err)
					return cli.NewExitError("Failed to load configuration", 1)
				}

				serve := func() {
					defer func() {
						if err := recover(); err != nil {
							log.Errorf("%v", utils.ErrorStack(err))
						}
					}()
					err := forwarder.Run()
					if err != nil {
						log.Errorf("start forwarder failed: %v", err)
					}
				}

				for {
					serve()
					log.Error("Exiting due to some signal [SIGTERM/SIGINT] or other reason, retry after 1 second")
					time.Sleep(1 * time.Second)
				}

				return nil
			},
		},
		{
			Name:  "register",
			Usage: "Register a new AI provider to the database",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "wrapper-name, w",
					Usage: "Wrapper model name for external display",
				},
				cli.StringFlag{
					Name:  "model-name, m",
					Usage: "Actual model name used internally",
					Value: "",
				},
				cli.StringFlag{
					Name:  "type-name, t",
					Usage: "Provider type (e.g., openai, chatglm, tongyi)",
					Value: "",
				},
				cli.StringFlag{
					Name:  "domain, d",
					Usage: "API domain or URL",
					Value: "",
				},
				cli.StringFlag{
					Name:  "api-key, k",
					Usage: "API key",
					Value: "",
				},
				cli.BoolFlag{
					Name:  "no-https, n",
					Usage: "Disable HTTPS",
				},
			},
			Action: func(c *cli.Context) error {
				wrapperName := c.String("wrapper-name")
				modelName := c.String("model-name")
				typeName := c.String("type-name")
				domain := c.String("domain")
				apiKey := c.String("api-key")
				noHTTPS := c.Bool("no-https")

				// 验证必填字段
				if wrapperName == "" {
					return cli.NewExitError("wrapper-name is required", 1)
				}
				if typeName == "" {
					return cli.NewExitError("type-name is required", 1)
				}

				// 如果 modelName 为空，使用 wrapperName
				if modelName == "" {
					modelName = wrapperName
				}

				// 注册提供者
				provider, err := aibalance.RegisterAiProvider(
					wrapperName, modelName, typeName, domain, apiKey, noHTTPS,
				)
				if err != nil {
					return cli.NewExitError("Failed to register provider: "+err.Error(), 1)
				}

				log.Infof("Successfully registered AI provider:")
				log.Infof("  ID: %d", provider.ID)
				log.Infof("  Wrapper Name: %s", provider.WrapperName)
				log.Infof("  Model Name: %s", provider.ModelName)
				log.Infof("  Type: %s", provider.TypeName)
				log.Infof("  Domain: %s", provider.DomainOrURL)
				log.Infof("  API Key: %s", strings.Repeat("*", len(provider.APIKey)))
				log.Infof("  Disable HTTPS: %v", provider.NoHTTPS)
				return nil
			},
		},
		{
			Name:  "list",
			Usage: "List all registered AI providers",
			Action: func(c *cli.Context) error {
				providers, err := aibalance.GetAllAiProviders()
				if err != nil {
					return cli.NewExitError("Failed to get AI provider list: "+err.Error(), 1)
				}

				if len(providers) == 0 {
					log.Infof("No registered AI providers found")
					return nil
				}

				log.Infof("Found %d AI providers:", len(providers))
				for i, p := range providers {
					log.Infof("%d. %s (ID: %d)", i+1, p.WrapperName, p.ID)
					log.Infof("   Model Name: %s", p.ModelName)
					log.Infof("   Type: %s", p.TypeName)
					log.Infof("   Domain: %s", p.DomainOrURL)
					log.Infof("   API Key: %s", strings.Repeat("*", len(p.APIKey)))
					log.Infof("   Total Requests: %d (Success: %d, Failure: %d)", p.TotalRequests, p.SuccessCount, p.FailureCount)
					log.Infof("   Health Status: %v", p.IsHealthy)
					log.Infof("   Last Latency: %d ms", p.LastLatency)
					log.Infof("") // Empty line for spacing
				}
				return nil
			},
		},
		{
			Name:  "api-key",
			Usage: "Manage API keys for AI models",
			Subcommands: []cli.Command{
				{
					Name:  "add",
					Usage: "Add a new API key",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "key, k",
							Usage: "API key value",
						},
						cli.StringSliceFlag{
							Name:  "models, m",
							Usage: "Allowed models (comma separated)",
						},
					},
					Action: func(c *cli.Context) error {
						apiKey := c.String("key")
						models := c.StringSlice("models")

						// 验证必填字段
						if apiKey == "" {
							return cli.NewExitError("API key is required", 1)
						}

						if len(models) == 0 {
							return cli.NewExitError("At least one allowed model must be specified", 1)
						}

						// 保存到数据库
						err := aibalance.SaveAiApiKey(apiKey, strings.Join(models, ","))
						if err != nil {
							return cli.NewExitError("Failed to save API key: "+err.Error(), 1)
						}

						log.Infof("Successfully added API key:")
						log.Infof("  Key: %s", strings.Repeat("*", len(apiKey)))
						log.Infof("  Allowed Models: %s", strings.Join(models, ", "))
						return nil
					},
				},
				{
					Name:  "list",
					Usage: "List all API keys",
					Action: func(c *cli.Context) error {
						keys, err := aibalance.GetAllAiApiKeys()
						if err != nil {
							return cli.NewExitError("Failed to get API keys: "+err.Error(), 1)
						}

						if len(keys) == 0 {
							log.Infof("No API keys found")
							return nil
						}

						log.Infof("Found %d API keys:", len(keys))
						for i, k := range keys {
							log.Infof("%d. Key: %s", i+1, strings.Repeat("*", 8)+k.APIKey[len(k.APIKey)-4:])
							log.Infof("   Allowed Models: %s", k.AllowedModels)
							log.Infof("") // Empty line for spacing
						}
						return nil
					},
				},
				{
					Name:  "delete",
					Usage: "Delete an API key",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "key, k",
							Usage: "API key to delete",
						},
					},
					Action: func(c *cli.Context) error {
						apiKey := c.String("key")

						// 验证必填字段
						if apiKey == "" {
							return cli.NewExitError("API key is required", 1)
						}

						// 从数据库删除
						err := aibalance.DeleteAiApiKey(apiKey)
						if err != nil {
							return cli.NewExitError("Failed to delete API key: "+err.Error(), 1)
						}

						log.Infof("Successfully deleted API key: %s", strings.Repeat("*", len(apiKey)))
						return nil
					},
				},
				{
					Name:  "update",
					Usage: "Update allowed models for an API key",
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "key, k",
							Usage: "API key to update",
						},
						cli.StringSliceFlag{
							Name:  "models, m",
							Usage: "New allowed models (comma separated)",
						},
					},
					Action: func(c *cli.Context) error {
						apiKey := c.String("key")
						models := c.StringSlice("models")

						// 验证必填字段
						if apiKey == "" {
							return cli.NewExitError("API key is required", 1)
						}

						if len(models) == 0 {
							return cli.NewExitError("At least one allowed model must be specified", 1)
						}

						// 更新数据库
						err := aibalance.UpdateAiApiKey(apiKey, strings.Join(models, ","))
						if err != nil {
							return cli.NewExitError("Failed to update API key: "+err.Error(), 1)
						}

						log.Infof("Successfully updated API key:")
						log.Infof("  Key: %s", strings.Repeat("*", len(apiKey)))
						log.Infof("  New Allowed Models: %s", strings.Join(models, ", "))
						return nil
					},
				},
			},
		},
	}

	// 添加服务器运行标志
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "config, c",
			Usage: "Path to configuration file",
			Value: "config.yaml",
		},
		cli.StringFlag{
			Name:  "listen, l",
			Usage: "Address to listen on",
			Value: "127.0.0.1:8223",
		},
	}

	app.Before = func(context *cli.Context) error {
		return nil
	}

	// 默认动作（运行服务器）
	app.Action = func(c *cli.Context) error {
		configPath := c.String("config")
		listenAddr := c.String("listen")

		log.Infof("Starting AI load balancer service, config: %s, listen address: %s", configPath, listenAddr)

		// 即使配置文件不存在，也会从数据库加载
		b, err := aibalance.NewBalancer(configPath)
		if err != nil {
			return err
		}

		log.Infof("Service started successfully, listening on: %s", listenAddr)
		return b.RunWithAddr(listenAddr)
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("Command execution failed: [%v] error: %v", strings.Join(os.Args, " "), err)
		return
	}
}
