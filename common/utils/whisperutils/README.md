# WhisperUtils

WhisperUtils 是一个用于音频转录和 SRT 字幕管理的 Go 模块。

## 功能特性

### 🎯 核心功能
- **WhisperCli 集成**: 直接调用 whisper-cli 进行音频转录
- **SRT 管理**: 完整的 SRT 字幕文件解析、编辑和生成功能
- **时间点上下文查询**: 根据指定时间点获取前后文本内容
- **转录处理**: 处理 Whisper 转录结果并转换为各种格式

### 🔧 主要组件

#### 1. WhisperCli
- 支持 VAD (Voice Activity Detection)
- 可配置的转录参数（线程数、处理器数、波束大小等）
- 实时流式结果输出
- 支持多种音频格式

#### 2. SRT 管理器 (SRTManager)
- 解析和生成标准 SRT 格式
- 添加、更新、删除字幕条目
- 时间范围查询
- **核心功能**: `GetSRTContextByOffsetSeconds(offsetSeconds, interval)` - 获取指定时间点周围的文本上下文

#### 3. 转录处理器 (TranscriptionProcessor)
- 处理 Whisper JSON 输出
- 转换为 SRT 格式
- 支持分段和单词级别的时间戳

## 使用示例

### 基本 SRT 操作

```go
// 创建 SRT 管理器
manager, err := whisperutils.NewSRTManagerFromContent(srtContent)
if err != nil {
    log.Fatal(err)
}

// 获取 10 秒前后 5 秒的上下文文本
context := manager.GetSRTContextByOffsetSeconds(10.0, 5*time.Second)
fmt.Printf("上下文文本: %s\n", context.ContextText)
fmt.Printf("相关条目数: %d\n", len(context.ContextEntries))
```

### WhisperCli 转录

```go
// 调用 whisper-cli 进行转录
srtTargetPath := audioFile + ".srt"
results, err := whisperutils.InvokeWhisperCli(audioFile, srtTargetPath,
    whisperutils.WithModelPath(modelPath),
    whisperutils.WithVAD(true),
    whisperutils.WithDebug(false),
)

// 处理流式结果
for result := range results {
    fmt.Printf("[%s -> %s] %s\n", result.StartTime, result.EndTime, result.Text)
}
```

### SRT 编辑操作

```go
// 添加新条目
manager.AddEntry(30*time.Second, 35*time.Second, "新的字幕文本")

// 更新现有条目
manager.UpdateEntry(2, "更新后的文本")

// 获取时间范围内的条目
entries := manager.GetEntriesInTimeRange(10*time.Second, 30*time.Second)

// 导出为 SRT 格式
srtOutput := manager.ToSRT()
```

## 核心 API

### SRTManager 主要方法

- `NewSRTManager()` - 创建新的 SRT 管理器
- `NewSRTManagerFromContent(content)` - 从 SRT 内容创建管理器
- `NewSRTManagerFromFile(filePath)` - 从 SRT 文件创建管理器
- `GetSRTContextByOffsetSeconds(offsetSeconds, interval)` - **核心功能**: 获取时间点上下文
- `AddEntry(startTime, endTime, text)` - 添加字幕条目
- `UpdateEntry(index, text)` - 更新字幕条目
- `RemoveEntry(index)` - 删除字幕条目
- `ToSRT()` - 导出为 SRT 格式

### WhisperCli 配置选项

- `WithModelPath(path)` - 设置模型路径
- `WithVAD(enable)` - 启用语音活动检测
- `WithLanguage(lang)` - 设置语言
- `WithThreads(n)` - 设置线程数
- `WithDebug(enable)` - 启用调试模式

## 测试

运行所有测试：
```bash
go test -v ./common/utils/whisperutils
```

运行特定测试：
```bash
go test -v ./common/utils/whisperutils -run TestSRT
```

## 依赖要求

- Go 1.19+
- whisper-cli 二进制文件（用于音频转录）
- Whisper 模型文件（.gguf 格式）
- VAD 模型文件（可选，用于语音活动检测）

## 环境变量

- `YAK_WHISPER_CLI_PATH` - whisper-cli 二进制文件路径
- `YAK_WHISPER_MODEL_PATH` - Whisper 模型文件路径
- `YAK_WHISPER_VAD_MODEL_PATH` - VAD 模型文件路径

## 注意事项

- 此模块专注于 CLI 工具集成，已移除 WhisperServer 相关功能
- SRT 时间格式使用标准的 `HH:MM:SS,mmm` 格式
- 上下文查询功能特别适用于需要获取特定时间点前后文本的场景
- 所有时间计算使用 `time.Duration` 类型确保精确性