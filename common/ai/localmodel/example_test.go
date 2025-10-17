package localmodel

import (
	"fmt"
	"time"
)

// ExampleManager_StartEmbeddingService 演示如何使用管理器启动嵌入服务
func ExampleManager_StartEmbeddingService() {
	// 获取管理器单例
	manager := GetManager()

	// 启动嵌入服务
	err := manager.StartEmbeddingService(
		"127.0.0.1:11434",
		WithModel("Qwen3-Embedding-0.6B-Q4_K_M"),
		WithDebug(true),
		WithModelPath("/tmp/Qwen3-Embedding-0.6B-Q4_K_M.gguf"),
		WithContextSize(4096),
		WithContBatching(true),
		WithBatchSize(1024),
		WithThreads(8),
	)
	if err != nil {
		fmt.Printf("Failed to start embedding service: %v\n", err)
		return
	}

	fmt.Println("Embedding service started successfully")

	// 等待一段时间
	time.Sleep(2 * time.Second)

	// 查看服务状态
	services := manager.ListServices()
	for _, service := range services {
		fmt.Printf("Service: %s, Status: %s\n", service.Name, service.Status)
	}

	// 停止所有服务
	err = manager.StopAllServices()
	if err != nil {
		fmt.Printf("Failed to stop services: %v\n", err)
		return
	}

	fmt.Println("All services stopped")
}

// ExampleGetSupportedModels 演示如何获取支持的模型列表
func ExampleGetSupportedModels() {
	models := GetSupportedModels()

	fmt.Printf("Supported models (%d):\n", len(models))
	for _, model := range models {
		fmt.Printf("- %s (%s): %s\n", model.Name, model.Type, model.Description)
		fmt.Printf("  Default Port: %d\n", model.DefaultPort)
		fmt.Printf("  File: %s\n", model.FileName)
		fmt.Println()
	}
}

// ExampleFindModelConfig 演示如何查找模型配置
func ExampleFindModelConfig() {
	modelName := "Qwen3-Embedding-0.6B-Q4_K_M"

	model, err := FindModelConfig(modelName)
	if err != nil {
		fmt.Printf("Model not found: %v\n", err)
		return
	}

	fmt.Printf("Found model: %s\n", model.Name)
	fmt.Printf("Type: %s\n", model.Type)
	fmt.Printf("Description: %s\n", model.Description)
	fmt.Printf("Default Port: %d\n", model.DefaultPort)
}

// Example_withOptions 演示如何使用选项模式
func Example_withOptions() {
	config := DefaultServiceConfig()

	fmt.Printf("Default config:\n")
	fmt.Printf("Host: %s, Port: %d\n", config.Host, config.Port)
	fmt.Printf("Context Size: %d, Cont Batching: %t\n", config.ContextSize, config.ContBatching)
	fmt.Printf("Batch Size: %d, Threads: %d\n", config.BatchSize, config.Threads)

	// 应用选项
	options := []Option{
		WithHost("0.0.0.0"),
		WithPort(9090),
		WithContextSize(8192),
		WithContBatching(false),
		WithBatchSize(2048),
		WithThreads(16),
		WithDebug(true),
	}

	for _, option := range options {
		option(config)
	}

	fmt.Printf("\nAfter applying options:\n")
	fmt.Printf("Host: %s, Port: %d\n", config.Host, config.Port)
	fmt.Printf("Context Size: %d, Cont Batching: %t\n", config.ContextSize, config.ContBatching)
	fmt.Printf("Batch Size: %d, Threads: %d\n", config.BatchSize, config.Threads)
}

// ExampleServiceStatus 演示服务状态
func ExampleServiceStatus() {
	statuses := []ServiceStatus{
		StatusStopped,
		StatusStarting,
		StatusRunning,
		StatusStopping,
		StatusError,
	}

	fmt.Println("Service status values:")
	for _, status := range statuses {
		fmt.Printf("- %d: %s\n", status, status.String())
	}
}
