# StdinManager 使用说明

## 问题描述

在多goroutine环境中，当不同的goroutine同时尝试读取`os.Stdin`时会发生争抢问题，特别是在使用promptui等交互式库时。这会导致：

1. 数据丢失或重复读取
2. goroutine阻塞
3. 用户输入被错误的处理器处理

## 解决方案

`StdinManager` 提供了一个单例模式的stdin管理器，通过分流机制解决stdin争抢问题。

## 核心API

### 1. NewStdinManager() *StdinManager
获取StdinManager的单例实例。

### 2. PreventDefault() io.Reader
- 阻止其他goroutine读取默认stdin
- 返回一个分流reader，可以安全地传给promptui等库使用
- 此时其他goroutine调用`GetDefaultReader()`会得到空reader

### 3. RecoverDefault()
- 恢复默认的stdin读取
- 其他goroutine可以正常读取stdin

### 4. GetDefaultReader() io.Reader
- 获取当前可用的stdin reader
- 如果被阻止，返回空reader（读取时返回EOF）
- 如果未被阻止，返回原始的os.Stdin

### 5. RegisterReader() *ReaderController
- 注册一个背景reader，返回同步控制器
- 用于强力同步机制，确保完全的暂停/恢复控制

### 6. ReaderController.WaitForSignals() bool
- 等待暂停/恢复信号
- 如果收到暂停信号，会阻塞直到收到恢复信号
- 返回true表示应该继续处理

### 7. ReaderController.Unregister()
- 注销reader，释放资源
- 应该在defer中调用

## 使用示例

### 场景1：在promptui中使用

```go
// 在需要使用promptui的地方
manager := NewStdinManager()
divertedReader := manager.PreventDefault() // 获取分流reader
defer manager.RecoverDefault()            // 确保恢复

// 将divertedReader传给promptui使用
prompt := promptui.Select{
    Label: "请选择选项",
    Items: []string{"选项1", "选项2", "选项3"},
    Stdin: io.NopCloser(divertedReader), // 使用分流reader
}

index, _, err := prompt.Run()
// ... 处理结果
```

### 场景2：在默认输入处理器中使用（强力同步版本）

```go
// 类似SetupSignalHandler的持续读取goroutine（推荐用法）
func setupInputHandler(ctx context.Context) {
    manager := NewStdinManager()
    
    go func() {
        controller := manager.RegisterReader()
        defer controller.Unregister()
        
        for {
            select {
            case <-ctx.Done():
                return
            default:
                // 强力同步：等待暂停/恢复信号
                if !controller.WaitForSignals() {
                    continue
                }
                
                // 获取当前可用的reader
                reader := manager.GetDefaultReader()
                
                line, err := utils.ReadLine(reader)
                if err != nil {
                    if err == io.EOF {
                        time.Sleep(50 * time.Millisecond) // 等待恢复
                        continue
                    }
                    log.Errorf("Failed to read line: %v", err)
                    continue
                }
                
                // 处理输入
                handleInput(string(line))
            }
        }
    }()
}
```

### 场景3：简单版本（向后兼容）

```go
// 简单版本，不使用强力同步
func simpleInputHandler() {
    manager := NewStdinManager()
    
    go func() {
        for {
            reader := manager.GetDefaultReader()
            
            line, err := utils.ReadLine(reader)
            if err == io.EOF {
                time.Sleep(10 * time.Millisecond) // 等待恢复
                continue
            }
            
            if err != nil {
                log.Errorf("Failed to read line: %v", err)
                continue
            }
            
            handleInput(string(line))
        }
    }()
}
```

## 工作原理

### 基础分流机制
1. **正常状态**：`GetDefaultReader()`返回`os.Stdin`，所有goroutine正常读取
2. **分流状态**：
   - 调用`PreventDefault()`时，创建一个管道
   - `os.Stdin`被替换为管道的写入端
   - 返回管道的读取端给调用者
   - 启动一个转发goroutine，将原始stdin数据转发到管道
   - 其他goroutine的`GetDefaultReader()`返回空reader
3. **恢复状态**：调用`RecoverDefault()`时，恢复`os.Stdin`并关闭管道

### 强力同步机制（推荐）
1. **注册阶段**：背景goroutine调用`RegisterReader()`获取控制器
2. **同步控制**：在每次循环中调用`WaitForSignals()`检查状态
3. **暂停机制**：
   - `PreventDefault()`设置暂停标志
   - 所有注册的reader在下次`WaitForSignals()`调用时会被阻塞
4. **恢复机制**：
   - `RecoverDefault()`清除暂停标志并广播恢复信号
   - 所有被阻塞的reader立即恢复运行
5. **资源清理**：reader退出时调用`Unregister()`释放资源

## 注意事项

1. **单例模式**：StdinManager使用单例模式，确保全局只有一个实例
2. **defer恢复**：务必使用`defer manager.RecoverDefault()`确保资源正确释放
3. **线程安全**：所有API都是线程安全的
4. **错误处理**：如果管道创建失败，`PreventDefault()`会返回原始stdin

## 测试

运行测试验证功能：

```bash
go test ./common/ai/aid/aireact/cmd/aireactdeps/ -run TestStdinManager -v
```

## 集成到现有代码

1. 在使用promptui的地方，用`PreventDefault()`获取分流reader
2. 在持续读取stdin的goroutine中，使用`GetDefaultReader()`并处理EOF
3. 确保所有临时接管stdin的代码都使用`defer RecoverDefault()`

这样就可以彻底解决stdin争抢问题，让promptui和其他stdin读取器和谐共存。
