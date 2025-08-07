# Local Model Manager

本地模型管理器用于管理和控制本地AI模型服务，特别是基于 llama-server 的嵌入服务。

## 功能特性

- **服务生命周期管理**: 启动、停止、状态监控
- **配置灵活**: 支持选项模式配置服务参数
- **并发安全**: 使用读写锁确保并发操作安全
- **错误处理**: 完善的错误处理和状态跟踪
- **模型支持**: 内置支持多种模型配置

## 快速开始

### 获取管理器

本模块使用单例模式，通过 `GetManager()` 获取管理器实例：

```go
import "github.com/yaklang/yaklang/common/ai/localmodel"

// 获取管理器单例实例
manager := localmodel.GetManager()
```

### 启动嵌入服务

```go
err := manager.StartEmbeddingService(
    "127.0.0.1:11434",
    localmodel.WithEmbeddingModel("Qwen3-Embedding-0.6B-Q4_K_M"),
    localmodel.WithDetached(true),
    localmodel.WithDebug(true),
    localmodel.WithModelPath("/path/to/model.gguf"),
    localmodel.WithContextSize(4096),
    localmodel.WithContBatching(true),
    localmodel.WithBatchSize(1024),
    localmodel.WithThreads(8),
)
if err != nil {
    log.Fatal(err)
}
```

## 本地模型管理

### 检查本地模型

```go
// 检查默认模型是否可用
if manager.IsDefaultModelAvailable() {
    fmt.Println("默认模型可用")
    fmt.Printf("路径: %s\n", manager.GetDefaultEmbeddingModelPath())
}

// 检查特定模型是否存在
if manager.IsLocalModelExists("Qwen3-Embedding-0.6B-Q4_K_M") {
    fmt.Println("Qwen3 模型可用")
}

// 列出所有本地可用的模型
localModels := manager.ListLocalModels()
fmt.Printf("本地可用模型: %v\n", localModels)
```

### 获取模型路径

```go
// 获取默认嵌入模型路径
defaultPath := manager.GetDefaultEmbeddingModelPath()
fmt.Printf("默认模型路径: %s\n", defaultPath)

// 获取特定模型的本地路径
modelPath, err := manager.GetLocalModelPath("Qwen3-Embedding-0.6B-Q4_K_M")
if err != nil {
    log.Printf("获取模型路径失败: %v", err)
} else {
    fmt.Printf("模型路径: %s\n", modelPath)
}
```

### 自动模型路径检测

如果不指定模型路径，管理器会自动使用默认路径：

```go
// 不指定模型路径，将自动使用默认的 Qwen3 模型路径
err := manager.StartEmbeddingService("127.0.0.1:8080")
if err != nil {
    log.Fatal(err)
}
```

## 配置选项

### 可用选项

- `WithHost(host string)`: 设置服务主机地址
- `WithPort(port int32)`: 设置服务端口
- `WithEmbeddingModel(model string)`: 设置嵌入模型名称
- `WithModelPath(path string)`: 设置模型文件路径
- `WithContextSize(size int)`: 设置上下文大小
- `WithContBatching(enabled bool)`: 设置是否启用连续批处理
- `WithBatchSize(size int)`: 设置批处理大小
- `WithThreads(threads int)`: 设置线程数
- `WithDetached(detached bool)`: 设置是否分离模式
- `WithDebug(debug bool)`: 设置调试模式
- `WithStartupTimeout(timeout time.Duration)`: 设置启动超时时间
- `WithArgs(args ...string)`: 设置额外的命令行参数

### 默认配置

```go
config := localmodel.DefaultServiceConfig()
// Host: "127.0.0.1"
// Port: 8080
// ContextSize: 4096
// ContBatching: true
// BatchSize: 1024
// Threads: 8
// Detached: false
// Debug: false
// StartupTimeout: 30 * time.Second
```

## 服务管理

### 查看服务状态

```go
// 获取特定服务状态
status, err := manager.GetServiceStatus("service-name")
if err != nil {
    log.Printf("Service not found: %v", err)
} else {
    fmt.Printf("Service: %s, Status: %s\n", status.Name, status.Status)
}

// 列出所有服务
services := manager.ListServices()
for _, service := range services {
    fmt.Printf("Service: %s, Status: %s, Started: %v\n", 
        service.Name, service.Status, service.StartTime)
}
```

### 停止服务

```go
// 停止特定服务
err := manager.StopService("service-name")
if err != nil {
    log.Printf("Failed to stop service: %v", err)
}

// 停止所有服务
err := manager.StopAllServices()
if err != nil {
    log.Printf("Failed to stop all services: %v", err)
}
```

## 服务状态

服务具有以下状态：

- `StatusStopped`: 已停止
- `StatusStarting`: 启动中
- `StatusRunning`: 运行中
- `StatusStopping`: 停止中
- `StatusError`: 错误状态

## 支持的模型

### 获取支持的模型列表

```go
models := localmodel.GetSupportedModels()
for _, model := range models {
    fmt.Printf("Model: %s (%s)\n", model.Name, model.Type)
    fmt.Printf("Description: %s\n", model.Description)
    fmt.Printf("Default Port: %d\n", model.DefaultPort)
}
```

### 查找特定模型

```go
model, err := localmodel.FindModelConfig("Qwen3-Embedding-0.6B-Q4_K_M")
if err != nil {
    log.Printf("Model not supported: %v", err)
} else {
    fmt.Printf("Found model: %s\n", model.Name)
}
```

### 检查模型支持

```go
if localmodel.IsModelSupported("Qwen3-Embedding-0.6B-Q4_K_M") {
    fmt.Println("Model is supported")
} else {
    fmt.Println("Model is not supported")
}
```

## 错误处理

管理器提供详细的错误信息：

```go
err := manager.StartEmbeddingService("invalid-address")
if err != nil {
    // 错误信息包含具体的失败原因
    log.Printf("Failed to start service: %v", err)
}
```

常见错误类型：
- 地址格式错误
- 模型文件不存在
- llama-server 未安装
- 端口已被占用
- 服务已在运行

## 完整示例

```go
package main

import (
    "fmt"
    "log"
    "time"
    
    "github.com/yaklang/yaklang/common/ai/localmodel"
)

func main() {
    // 获取管理器单例
    manager := localmodel.GetManager()

    // 启动嵌入服务
    err := manager.StartEmbeddingService(
        "127.0.0.1:8080",
        localmodel.WithEmbeddingModel("Qwen3-Embedding-0.6B-Q4_K_M"),
        localmodel.WithContextSize(4096),
        localmodel.WithContBatching(true),
        localmodel.WithBatchSize(1024),
        localmodel.WithThreads(8),
        localmodel.WithDebug(true),
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Service started, waiting...")
    time.Sleep(5 * time.Second)

    // 查看服务状态
    services := manager.ListServices()
    for _, service := range services {
        fmt.Printf("Service: %s, Status: %s\n", service.Name, service.Status)
    }

    // 停止服务
    err = manager.StopAllServices()
    if err != nil {
        log.Printf("Error stopping services: %v", err)
    }

    fmt.Println("All services stopped")
}
```

## 注意事项

1. **llama-server 依赖**: 确保 llama-server 已正确安装并可在系统路径中找到
2. **模型文件**: 确保模型文件存在于指定路径
3. **端口冲突**: 避免多个服务使用相同端口
4. **资源管理**: 及时停止不需要的服务以释放资源
5. **错误处理**: 始终检查函数返回的错误信息

## 命令行工具

模块提供了一个命令行工具用于测试和管理本地模型服务：

### 构建和运行

```bash
# 构建命令行工具
go build -o localmodel-cli ./common/ai/localmodel/cmd

# 查看帮助
./localmodel-cli -h

# 列出支持的模型
./localmodel-cli -list-models

# 检查本地模型状态
./localmodel-cli -check-model

# 启动默认嵌入服务
./localmodel-cli

# 启动自定义配置的服务
./localmodel-cli -host 0.0.0.0 -port 9090 -debug -parallelism 4

# 使用自定义模型路径
./localmodel-cli -model-path /path/to/model.gguf -debug
```

### 命令行选项

- `-host`: 服务主机地址 (默认: 127.0.0.1)
- `-port`: 服务端口 (默认: 8080)
- `-model`: 模型名称 (默认: Qwen3-Embedding-0.6B-Q4_K_M)
- `-model-path`: 模型文件路径 (可选)
- `-context-size`: 上下文大小 (默认: 4096)
- `-cont-batching`: 启用连续批处理 (默认: true)
- `-batch-size`: 批处理大小 (默认: 1024)
- `-threads`: 线程数 (默认: 8)
- `-detached`: 分离模式
- `-debug`: 调试模式
- `-timeout`: 启动超时时间 (默认: 30秒)
- `-list-models`: 列出支持的模型
- `-check-model`: 检查本地模型是否可用

### 示例输出

```bash
$ ./localmodel-cli -check-model
=== Yaklang Local Model Manager ===

检查本地模型:
1. 默认嵌入模型 (Qwen3-Embedding-0.6B-Q4_K_M):
   路径: /Users/user/yakit-projects/libs/models/Qwen3-Embedding-0.6B-Q4_K_M.gguf
   可用: true

2. llama-server:
   路径: /Users/user/yakit-projects/libs/llama-server
   状态: 可用

3. 所有支持的模型:
   Qwen3-Embedding-0.6B-Q4_K_M: 可用
```