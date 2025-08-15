# BaseAICallerConfig 使用说明

`BaseAICallerConfig` 是一个基础的 `AICallerConfigIf` 接口实现，提供了完整的AI调用配置功能，包括事件发射、端点管理、检查点存储、用户交互等核心功能。这个基础配置旨在完全覆盖 `aid.Config` 的核心功能，实现进一步的解耦。

## 主要特性

### 核心功能
- **ID生成器**: 提供唯一的序列ID生成
- **事件发射器**: 支持各种事件类型的发送和处理
- **端点管理**: 完整的端点生命周期管理
- **检查点存储**: 支持AI任务的状态持久化
- **用户交互**: 通过UnlimitedChannel处理用户输入
- **消费量跟踪**: 自动统计输入输出token消费

### 默认配置
- `GetAITransactionAutoRetryCount()`: 默认返回 5
- `RetryPromptBuilder()`: 提供默认的重试提示构建逻辑
- `DoWaitAgree()`: 通过UnlimitedChannel处理外部输入
- `ReleaseInteractiveEvent()`: 完整实现交互事件释放

## 快速开始

### 基本使用

```go
import (
    "context"
    "github.com/yaklang/yaklang/common/ai/aid/aicommon"
    "github.com/yaklang/yaklang/common/consts"
    "github.com/yaklang/yaklang/common/utils"
)

func main() {
    // 创建基本配置
    ctx := context.Background()
    runtimeId := utils.RandStringBytes(8)
    db := consts.GetGormProfileDatabase()
    
    // 初始化配置
    config := aicommon.NewBaseAICallerConfig(ctx, runtimeId, db)
    defer config.Close()
    
    // 基本使用
    id := config.AcquireId()                        // 获取唯一ID
    emitter := config.GetEmitter()                  // 获取事件发射器
    epm := config.GetEndpointManager()              // 获取端点管理器
    retryCount := config.GetAITransactionAutoRetryCount() // 获取重试次数
}
```

### 自定义事件处理

```go
// 创建自定义事件处理器
customHandler := func(e *schema.AiOutputEvent) error {
    log.Printf("接收到事件: type=%s, nodeId=%s", e.Type, e.NodeId)
    // 自定义处理逻辑
    return nil
}

config := aicommon.NewBaseAICallerConfig(ctx, runtimeId, db)
config.SetEmitterHandler(customHandler)
```

### 用户交互处理

```go
// 获取用户交互通道
userChan := config.GetUserInteractionChannel()

// 在另一个goroutine中处理用户输入
go func() {
    for {
        // 模拟用户输入
        userEvent := aicommon.UserInteractionEvent{
            EventID: "some-endpoint-id",
            InvokeParams: aitool.InvokeParams{
                "action": "approve",
                "comment": "用户同意执行",
            },
        }
        userChan.SafeFeed(userEvent)
        time.Sleep(time.Second)
    }
}()

// 创建端点并等待用户同意
endpoint := config.GetEndpointManager().CreateEndpoint()
config.DoWaitAgree(ctx, endpoint) // 会等待用户输入
```

### 自定义检查点存储

```go
// 实现自定义检查点存储
type CustomCheckpointStorage struct {
    // 自定义字段
}

func (c *CustomCheckpointStorage) CreateReviewCheckpoint(runtimeId string, id int64) *schema.AiCheckpoint {
    // 自定义实现
    return &schema.AiCheckpoint{
        CoordinatorUuid: runtimeId,
        Seq:             id,
        Type:            schema.AiCheckpointType_Review,
    }
}

func (c *CustomCheckpointStorage) CreateToolCallCheckpoint(runtimeId string, id int64) *schema.AiCheckpoint {
    // 自定义实现
    return &schema.AiCheckpoint{
        CoordinatorUuid: runtimeId,
        Seq:             id,
        Type:            schema.AiCheckpointType_ToolCall,
    }
}

func (c *CustomCheckpointStorage) SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error {
    // 自定义请求保存逻辑
    return nil
}

func (c *CustomCheckpointStorage) SubmitCheckpointResponse(checkpoint *schema.AiCheckpoint, rsp any) error {
    // 自定义响应保存逻辑
    return nil
}

// 使用自定义存储
config := aicommon.NewBaseAICallerConfig(ctx, runtimeId, db)
customStorage := &CustomCheckpointStorage{}
config.SetCheckpointStorage(customStorage)
```

## API 参考

### 主要方法

所有这些方法都是 `BaseAICallerConfig` 的方法，完全覆盖了 `aid.Config` 的核心功能。

#### 基础配置
- `AcquireId() int64`: 获取唯一序列ID
- `GetDB() *gorm.DB`: 获取数据库连接
- `GetRuntimeId() string`: 获取运行时ID
- `GetContext() context.Context`: 获取上下文
- `IsCtxDone() bool`: 检查上下文是否已取消

#### AI相关
- `GetAITransactionAutoRetryCount() int64`: 获取自动重试次数
- `RetryPromptBuilder(rawPrompt string, retryErr error) string`: 构建重试提示
- `NewAIResponse() *AIResponse`: 创建AI响应对象

#### 事件处理
- `GetEmitter() *Emitter`: 获取事件发射器
- `CallAIResponseConsumptionCallback(current int)`: 记录输出消费
- `CallAIResponseOutputFinishedCallback(output string)`: 处理输出完成回调

#### 端点管理
- `GetEndpointManager() *EndpointManager`: 获取端点管理器
- `DoWaitAgree(ctx context.Context, endpoint *Endpoint)`: 等待用户同意
- `ReleaseInteractiveEvent(eventID string, invokeParams aitool.InvokeParams)`: 释放交互事件

#### 检查点管理
- `CreateReviewCheckpoint(id int64) *schema.AiCheckpoint`: 创建审核检查点
- `CreateToolCallCheckpoint(id int64) *schema.AiCheckpoint`: 创建工具调用检查点
- `SubmitCheckpointRequest(checkpoint *schema.AiCheckpoint, req any) error`: 提交检查点请求
- `SubmitCheckpointResponse(checkpoint *schema.AiCheckpoint, rsp any) error`: 提交检查点响应

### 辅助方法

- `SetCheckpointStorage(storage CheckpointStorage)`: 设置自定义检查点存储
- `SetAITransactionAutoRetryCount(count int64)`: 设置自动重试次数
- `GetInputConsumption() int64`: 获取输入消费量
- `GetOutputConsumption() int64`: 获取输出消费量
- `GetUserInteractionChannel() *chanx.UnlimitedChan[UserInteractionEvent]`: 获取用户交互通道
- `SetEmitterHandler(handler BaseEmitter)`: 设置自定义事件处理器
- `Close()`: 清理资源

## 注意事项

1. **资源管理**: 使用完毕后务必调用 `Close()` 方法清理资源
2. **线程安全**: 所有公共方法都是线程安全的
3. **用户交互**: `DoWaitAgree` 默认超时时间为30秒
4. **检查点存储**: 默认使用数据库存储，可通过 `SetCheckpointStorage` 自定义
5. **事件处理**: 默认事件处理器只记录日志，建议设置自定义处理器

## 示例项目

完整的使用示例请参考 `base_caller_config_example_test.go` 文件，其中包含了：
- 基本功能测试
- 重试提示构建器测试
- 用户交互测试
- 检查点存储测试
- 自定义存储测试
- 消费量跟踪测试
