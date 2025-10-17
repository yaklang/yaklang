package yakcmds

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/ai/localmodel"
	"github.com/yaklang/yaklang/common/log"
)

var LocalModelCommands = []*cli.Command{
	{
		Name:    "localmodel",
		Aliases: []string{"lm"},
		Usage:   "Local Model Manager for Yaklang AI Services",
		Flags: []cli.Flag{
			cli.StringFlag{
				Name:  "host",
				Value: "127.0.0.1",
				Usage: "服务主机地址",
			},
			cli.IntFlag{
				Name:  "port",
				Value: 8080,
				Usage: "服务端口",
			},
			cli.StringFlag{
				Name:  "model",
				Usage: "模型名称 (默认使用 Qwen3-Embedding-0.6B-Q4_K_M)",
			},
			cli.StringFlag{
				Name:  "model-path",
				Usage: "模型文件路径 (如果不指定，将使用默认路径)",
			},
			cli.IntFlag{
				Name:  "context-size",
				Value: 4096,
				Usage: "上下文大小",
			},
			cli.BoolFlag{
				Name:  "cont-batching",
				Usage: "启用连续批处理 (默认启用)",
			},
			cli.IntFlag{
				Name:  "batch-size",
				Value: 1024,
				Usage: "批处理大小",
			},
			cli.IntFlag{
				Name:  "threads",
				Value: 8,
				Usage: "线程数",
			},
			cli.BoolFlag{
				Name:  "detached",
				Usage: "分离模式",
			},
			cli.BoolFlag{
				Name:  "debug",
				Usage: "调试模式",
			},
			cli.IntFlag{
				Name:  "timeout",
				Value: 30,
				Usage: "启动超时时间 (秒)",
			},
			cli.BoolFlag{
				Name:  "list-models",
				Usage: "列出支持的模型",
			},
			cli.BoolFlag{
				Name:  "check-model",
				Usage: "检查本地模型是否可用",
			},
			cli.StringFlag{
				Name:  "llama-server-path",
				Usage: "llama-server 路径",
			},
			cli.StringFlag{
				Name:  "service-type",
				Usage: "服务类型",
			},
		},
		Action: func(c *cli.Context) error {
			fmt.Println("=== Yaklang Local Model Manager ===")

			// 获取管理器单例
			manager := localmodel.GetManager()
			// 如果只是列出模型
			if c.Bool("list-models") {
				listSupportedModels(manager)
				return nil
			}

			// 如果只是检查模型
			if c.Bool("check-model") {
				checkLocalModels(manager)
				return nil
			}

			// 启动嵌入服务
			return startEmbeddingService(c, manager)
		},
	},
}

func listSupportedModels(manager *localmodel.Manager) {
	fmt.Println("\n支持的模型:")
	models := localmodel.GetSupportedModels()
	for i, model := range models {
		fmt.Printf("%d. %s (%s)\n", i+1, model.Name, model.Type)
		fmt.Printf("   描述: %s\n", model.Description)
		fmt.Printf("   默认端口: %d\n", model.DefaultPort)
		fmt.Printf("   文件名: %s\n", model.FileName)

		// 检查本地是否存在
		exists := manager.IsLocalModelExists(model.Name)
		fmt.Printf("   本地可用: %t\n", exists)

		if exists {
			modelPath, _ := manager.GetLocalModelPath(model.Name)
			fmt.Printf("   路径: %s\n", modelPath)
		}
		fmt.Println()
	}

	fmt.Println("本地可用的模型:")
	localModels := manager.ListLocalModels()
	if len(localModels) == 0 {
		fmt.Println("  无本地可用模型")
	} else {
		for _, modelName := range localModels {
			fmt.Printf("  - %s\n", modelName)
		}
	}
}

func checkLocalModels(manager *localmodel.Manager) {
	fmt.Println("\n检查本地模型:")

	// 检查默认模型
	fmt.Println("1. 默认嵌入模型 (Qwen3-Embedding-0.6B-Q4_K_M):")
	defaultPath := localmodel.GetDefaultEmbeddingModelPath()
	fmt.Printf("   路径: %s\n", defaultPath)

	available := localmodel.IsDefaultModelAvailable()
	fmt.Printf("   可用: %t\n", available)

	// 检查 llama-server
	fmt.Println("\n2. llama-server:")
	llamaPath, err := localmodel.GetLlamaServerPath()
	if err != nil {
		fmt.Printf("   状态: 未安装 (%v)\n", err)
	} else {
		fmt.Printf("   路径: %s\n", llamaPath)
		fmt.Printf("   状态: 可用\n")
	}

	// 检查所有支持的模型
	fmt.Println("\n3. 所有支持的模型:")
	models := localmodel.GetSupportedModels()
	for _, model := range models {
		exists := manager.IsLocalModelExists(model.Name)
		status := "不可用"
		if exists {
			status = "可用"
		}
		fmt.Printf("   %s: %s\n", model.Name, status)
	}
}

func startEmbeddingService(c *cli.Context, manager *localmodel.Manager) error {
	// 构建地址
	address := fmt.Sprintf("%s:%d", c.String("host"), c.Int("port"))

	fmt.Printf("\n启动嵌入服务: %s\n", address)

	// 打印配置信息
	fmt.Println("\n配置信息:")
	fmt.Printf("  地址: %s\n", address)
	if c.String("model") != "" {
		fmt.Printf("  模型: %s\n", c.String("model"))
	} else {
		fmt.Printf("  模型: Qwen3-Embedding-0.6B-Q4_K_M (默认)\n")
	}
	if c.String("model-path") != "" {
		fmt.Printf("  模型路径: %s\n", c.String("model-path"))
	} else {
		defaultPath := localmodel.GetDefaultEmbeddingModelPath()
		fmt.Printf("  模型路径: %s (默认)\n", defaultPath)
	}
	fmt.Printf("  上下文大小: %d\n", c.Int("context-size"))

	// 处理 cont-batching 的默认值 (默认为 true)
	contBatching := true
	if c.IsSet("cont-batching") {
		contBatching = c.Bool("cont-batching")
	}
	fmt.Printf("  连续批处理: %t\n", contBatching)

	fmt.Printf("  批处理大小: %d\n", c.Int("batch-size"))
	fmt.Printf("  线程数: %d\n", c.Int("threads"))
	fmt.Printf("  分离模式: %t\n", c.Bool("detached"))
	fmt.Printf("  调试模式: %t\n", c.Bool("debug"))
	fmt.Printf("  启动超时: %d秒\n", c.Int("timeout"))

	// 检查模型是否可用
	modelName := c.String("model")
	if modelName == "" {
		modelName = "Qwen3-Embedding-0.6B-Q4_K_M"
	}

	if !manager.IsLocalModelExists(modelName) {
		fmt.Printf("\n警告: 模型 %s 在本地不存在\n", modelName)
		modelPath, _ := manager.GetLocalModelPath(modelName)
		fmt.Printf("期望路径: %s\n", modelPath)
		fmt.Println("请先下载模型文件或检查路径是否正确")
	}

	// 构建选项
	var options []localmodel.Option

	if c.String("model") != "" {
		options = append(options, localmodel.WithModel(c.String("model")))
	}

	if llamaServerPath := c.String("llama-server-path"); llamaServerPath != "" {
		options = append(options, localmodel.WithLlamaServerPath(llamaServerPath))
	}

	if c.String("model-path") != "" {
		options = append(options, localmodel.WithModelPath(c.String("model-path")))
	}

	options = append(options,
		localmodel.WithContextSize(c.Int("context-size")),
		localmodel.WithContBatching(contBatching),
		localmodel.WithBatchSize(c.Int("batch-size")),
		localmodel.WithThreads(c.Int("threads")),
		localmodel.WithDebug(c.Bool("debug")),
		localmodel.WithStartupTimeout(time.Duration(c.Int("timeout"))*time.Second),
		localmodel.WithModelType(c.String("service-type")),
	)

	// 启动服务
	fmt.Println("\n启动服务...")
	err := manager.StartService(address, options...)
	if err != nil {
		return fmt.Errorf("启动服务失败: %v", err)
	}

	fmt.Println("服务启动成功!")

	// 等待一段时间让服务完全启动
	time.Sleep(3 * time.Second)

	// 显示服务状态
	services := manager.ListServices()
	if len(services) > 0 {
		fmt.Println("\n当前运行的服务:")
		for _, service := range services {
			fmt.Printf("  服务名: %s\n", service.Name)
			fmt.Printf("  状态: %s\n", service.Status)
			fmt.Printf("  启动时间: %s\n", service.StartTime.Format("2006-01-02 15:04:05"))
			if service.LastError != "" {
				fmt.Printf("  最后错误: %s\n", service.LastError)
			}
		}
	}
	// 设置信号处理
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("\n服务正在运行中... 按 Ctrl+C 停止")
	fmt.Printf("嵌入服务地址: http://%s\n", address)

	// 启动监控协程
	serviceName := fmt.Sprintf("%s-%s-%d", c.String("service-type"), c.String("host"), c.Int("port"))
	if serviceName == "--" {
		serviceName = fmt.Sprintf("embedding-%s-%d", c.String("host"), c.Int("port"))
	}

	monitorDone := make(chan bool, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// 检查服务状态
				serviceInfo, err := manager.GetServiceStatus(serviceName)
				if err != nil {
					log.Errorf("获取服务状态失败: %v", err)
					fmt.Printf("\n服务状态检查失败，正在退出: %v\n", err)
					monitorDone <- true
					return
				}

				// 检查服务是否在运行中
				if serviceInfo.Status != localmodel.StatusRunning {
					log.Errorf("服务状态异常: %s", serviceInfo.Status.String())
					fmt.Printf("\n服务状态不是运行中 (%s)，正在退出\n", serviceInfo.Status.String())
					monitorDone <- true
					return
				}
			case <-monitorDone:
				return
			}
		}
	}()

	// 等待信号或监控协程退出
	select {
	case <-sigChan:
		fmt.Println("\n接收到停止信号...")
		monitorDone <- true
	case <-monitorDone:
		fmt.Println("\n监控协程检测到异常，程序即将退出...")
	}

	fmt.Println("\n正在停止服务...")
	err = manager.StopService(serviceName)
	if err != nil {
		log.Errorf("停止服务时出错: %v", err)
	} else {
		fmt.Println("所有服务已停止")
	}

	return nil
}
