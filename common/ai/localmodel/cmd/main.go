package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/ai/localmodel"
)

var (
	host         = flag.String("host", "127.0.0.1", "服务主机地址")
	port         = flag.Int("port", 8080, "服务端口")
	model        = flag.String("model", "", "模型名称 (默认使用 Qwen3-Embedding-0.6B-Q4_K_M)")
	modelPath    = flag.String("model-path", "", "模型文件路径 (如果不指定，将使用默认路径)")
	contextSize  = flag.Int("context-size", 4096, "上下文大小")
	contBatching = flag.Bool("cont-batching", true, "启用连续批处理")
	batchSize    = flag.Int("batch-size", 1024, "批处理大小")
	threads      = flag.Int("threads", 8, "线程数")
	detached     = flag.Bool("detached", false, "分离模式")
	debug        = flag.Bool("debug", false, "调试模式")
	timeout      = flag.Int("timeout", 30, "启动超时时间 (秒)")
	listModels   = flag.Bool("list-models", false, "列出支持的模型")
	checkModel   = flag.Bool("check-model", false, "检查本地模型是否可用")
)

func main() {
	flag.Parse()

	fmt.Println("=== Yaklang Local Model Manager ===")

	// 获取管理器单例
	manager := localmodel.GetManager()

	// 如果只是列出模型
	if *listModels {
		listSupportedModels(manager)
		return
	}

	// 如果只是检查模型
	if *checkModel {
		checkLocalModels(manager)
		return
	}

	// 启动嵌入服务
	startEmbeddingService(manager)
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

func startEmbeddingService(manager *localmodel.Manager) {
	// 构建地址
	address := fmt.Sprintf("%s:%d", *host, *port)

	fmt.Printf("\n启动嵌入服务: %s\n", address)

	// 打印配置信息
	fmt.Println("\n配置信息:")
	fmt.Printf("  地址: %s\n", address)
	if *model != "" {
		fmt.Printf("  模型: %s\n", *model)
	} else {
		fmt.Printf("  模型: Qwen3-Embedding-0.6B-Q4_K_M (默认)\n")
	}
	if *modelPath != "" {
		fmt.Printf("  模型路径: %s\n", *modelPath)
	} else {
		defaultPath := localmodel.GetDefaultEmbeddingModelPath()
		fmt.Printf("  模型路径: %s (默认)\n", defaultPath)
	}
	fmt.Printf("  上下文大小: %d\n", *contextSize)
	fmt.Printf("  连续批处理: %t\n", *contBatching)
	fmt.Printf("  批处理大小: %d\n", *batchSize)
	fmt.Printf("  线程数: %d\n", *threads)
	fmt.Printf("  分离模式: %t\n", *detached)
	fmt.Printf("  调试模式: %t\n", *debug)
	fmt.Printf("  启动超时: %d秒\n", *timeout)

	// 检查模型是否可用
	modelName := *model
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

	if *model != "" {
		options = append(options, localmodel.WithModel(*model))
	}

	if *modelPath != "" {
		options = append(options, localmodel.WithModelPath(*modelPath))
	}

	options = append(options,
		localmodel.WithContextSize(*contextSize),
		localmodel.WithContBatching(*contBatching),
		localmodel.WithBatchSize(*batchSize),
		localmodel.WithThreads(*threads),
		localmodel.WithDebug(*debug),
		localmodel.WithStartupTimeout(time.Duration(*timeout)*time.Second),
	)

	// 启动服务
	fmt.Println("\n启动服务...")
	err := manager.StartEmbeddingService(address, options...)
	if err != nil {
		log.Fatalf("启动服务失败: %v", err)
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

	// 等待信号
	<-sigChan

	fmt.Println("\n正在停止服务...")
	err = manager.StopAllServices()
	if err != nil {
		log.Printf("停止服务时出错: %v", err)
	} else {
		fmt.Println("所有服务已停止")
	}
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Yaklang Local Model Manager\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n")
		fmt.Fprintf(os.Stderr, "  %s [选项]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "选项:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n示例:\n")
		fmt.Fprintf(os.Stderr, "  # 列出支持的模型\n")
		fmt.Fprintf(os.Stderr, "  %s -list-models\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 检查本地模型\n")
		fmt.Fprintf(os.Stderr, "  %s -check-model\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 启动默认嵌入服务\n")
		fmt.Fprintf(os.Stderr, "  %s\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 启动自定义配置的服务\n")
		fmt.Fprintf(os.Stderr, "  %s -host 0.0.0.0 -port 9090 -debug -threads 4 -batch-size 2048\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  # 使用自定义模型路径\n")
		fmt.Fprintf(os.Stderr, "  %s -model-path /path/to/model.gguf -debug\n\n", os.Args[0])
	}
}
