# JSON Extractor - StreamExtractor

`StreamExtractor` 是 YakLang 中强大的 JSON 流式解析器，支持高效的流式 JSON 数据提取和处理。它不仅能处理标准JSON，还能容错处理各种格式的类JSON数据。

## 🚀 核心特性

### 流式处理
- **内存高效**: 不需要一次性加载整个 JSON 数据到内存
- **实时处理**: 数据边解析边处理，支持大文件处理
- **低延迟**: 即时响应，无需等待完整解析
- **字符级流式**: 支持字符级的实时数据流输出

### 灵活的回调机制
- **多种回调类型**: 支持对象、数组、键值对等不同粒度的回调
- **条件回调**: 基于特定条件触发的智能回调
- **流式字段处理**: 为特定字段提供实时流式数据处理

## 📚 API 文档

### 主要函数

#### ExtractStructuredJSON
```go
func ExtractStructuredJSON(jsonString string, options ...CallbackOption) error
```
从字符串解析JSON数据，支持多种回调选项。

**参数：**
- `jsonString`: 要解析的JSON字符串
- `options`: 可变参数，支持多种回调选项

**返回值：**
- `error`: 解析错误，如果成功则返回nil

#### ExtractStructuredJSONFromStream  
```go
func ExtractStructuredJSONFromStream(reader io.Reader, options ...CallbackOption) error
```
从流中解析JSON数据，适合处理大文件或网络流。

**参数：**
- `reader`: 实现了io.Reader接口的数据源
- `options`: 可变参数，支持多种回调选项

**返回值：**
- `error`: 解析错误，如果成功则返回nil

### 回调选项

#### 基础回调

##### WithObjectKeyValue
```go
func WithObjectKeyValue(callback func(key string, data any)) CallbackOption
```
监听对象的键值对，当解析到对象的属性时触发。

##### WithArrayCallback
```go
func WithArrayCallback(callback func(data []any)) CallbackOption
```
监听数组完成解析，当整个数组解析完成时触发。

##### WithObjectCallback
```go
func WithObjectCallback(callback func(data map[string]any)) CallbackOption
```
监听对象完成解析，当整个对象解析完成时触发。

##### WithRootMapCallback
```go
func WithRootMapCallback(callback func(data map[string]any)) CallbackOption
```
监听根级对象解析完成。

##### WithRawKeyValueCallback
```go
func WithRawKeyValueCallback(callback func(key, data any)) CallbackOption
```
监听原始的键值对，包含未处理的原始字符串数据。

#### 高级回调

##### WithRegisterConditionalObjectCallback
```go
func WithRegisterConditionalObjectCallback(keys []string, callback func(data map[string]any)) CallbackOption
```
条件回调，只有当对象包含指定的所有键时才触发。

##### WithRegisterFieldStreamHandler ⭐ 新功能
```go
func WithRegisterFieldStreamHandler(fieldName string, handler func(reader io.Reader)) CallbackOption
```
**字段流式处理器** - 这是最强大的新功能，为特定字段提供字符级的实时流式处理。

**特性：**
- **实时流式**: 解析过程中逐字符写入流，无需等待字段解析完成
- **内存高效**: 不缓存字段内容，直接流式传输
- **并发处理**: 在独立的goroutine中处理流数据
- **多字段支持**: 可同时为多个字段注册不同的处理器

## 🎯 使用示例

### 基础用法

```go
package main

import (
    "fmt"
    "io"
    "github.com/yaklang/yaklang/common/jsonextractor"
)

func main() {
    jsonData := `{
        "name": "John Doe",
        "age": 30,
        "skills": ["Go", "Python", "JavaScript"],
        "profile": {
            "bio": "Software Engineer",
            "location": "San Francisco"
        }
    }`

    // 基础对象回调
    err := jsonextractor.ExtractStructuredJSON(jsonData,
        jsonextractor.WithObjectCallback(func(data map[string]any) {
            fmt.Printf("解析到对象: %+v\n", data)
        }),
        jsonextractor.WithArrayCallback(func(data []any) {
            fmt.Printf("解析到数组: %+v\n", data)
        }),
    )

    if err != nil {
        fmt.Printf("解析失败: %v\n", err)
    }
}
```

### 流式处理大字段

```go
package main

import (
    "fmt"
    "io"
    "strings"
    "github.com/yaklang/yaklang/common/jsonextractor"
)

func main() {
    // 模拟包含大字段的JSON
    largeContent := strings.Repeat("这是一段很长的文本内容。", 10000)
    jsonData := fmt.Sprintf(`{
        "id": 12345,
        "title": "大文档",
        "content": "%s",
        "summary": "文档摘要"
    }`, largeContent)

    fmt.Println("开始流式处理大字段...")

    err := jsonextractor.ExtractStructuredJSON(jsonData,
        // 为content字段注册流式处理器
        jsonextractor.WithRegisterFieldStreamHandler("content", func(reader io.Reader) {
            fmt.Println("开始接收content字段的流式数据...")
            
            buffer := make([]byte, 1024)
            totalBytes := 0
            chunkCount := 0
            
            for {
                n, err := reader.Read(buffer)
                if err == io.EOF {
                    break
                }
                if err != nil {
                    fmt.Printf("读取错误: %v\n", err)
                    return
                }
                
                totalBytes += n
                chunkCount++
                fmt.Printf("接收到第%d块数据，大小: %d 字节\n", chunkCount, n)
                
                // 在这里可以实时处理数据块
                // 例如：写入文件、计算哈希、发送到其他服务等
            }
            
            fmt.Printf("content字段处理完成! 总共接收: %d 字节，%d 个数据块\n", totalBytes, chunkCount)
        }),
        
        // 为其他字段注册普通回调
        jsonextractor.WithRegisterFieldStreamHandler("title", func(reader io.Reader) {
            data, _ := io.ReadAll(reader)
            fmt.Printf("文档标题: %s\n", string(data))
        }),
    )

    if err != nil {
        fmt.Printf("解析失败: %v\n", err)
    }
}
```

### 多字段并发流式处理

```go
package main

import (
    "fmt"
    "io"
    "sync"
    "time"
    "github.com/yaklang/yaklang/common/jsonextractor"
)

func main() {
    jsonData := `{
        "field1": "` + strings.Repeat("A", 5000) + `",
        "field2": "` + strings.Repeat("B", 3000) + `",
        "field3": "` + strings.Repeat("C", 4000) + `",
        "metadata": {"created": "2024-01-01"}
    }`

    var wg sync.WaitGroup
    var mu sync.Mutex
    results := make(map[string]int)

    // 为多个字段注册并发流式处理器
    err := jsonextractor.ExtractStructuredJSON(jsonData,
        jsonextractor.WithRegisterFieldStreamHandler("field1", func(reader io.Reader) {
            defer wg.Done()
            processField("field1", reader, &mu, results)
        }),
        
        jsonextractor.WithRegisterFieldStreamHandler("field2", func(reader io.Reader) {
            defer wg.Done()
            processField("field2", reader, &mu, results)
        }),
        
        jsonextractor.WithRegisterFieldStreamHandler("field3", func(reader io.Reader) {
            defer wg.Done()
            processField("field3", reader, &mu, results)
        }),
    )

    wg.Add(3) // 等待3个字段处理完成

    if err != nil {
        fmt.Printf("解析失败: %v\n", err)
        return
    }

    // 等待所有字段处理完成
    done := make(chan bool)
    go func() {
        wg.Wait()
        done <- true
    }()

    select {
    case <-done:
        fmt.Println("所有字段处理完成!")
        for field, size := range results {
            fmt.Printf("%s: %d 字节\n", field, size)
        }
    case <-time.After(5 * time.Second):
        fmt.Println("处理超时!")
    }
}

func processField(fieldName string, reader io.Reader, mu *sync.Mutex, results map[string]int) {
    fmt.Printf("开始处理字段: %s\n", fieldName)
    
    buffer := make([]byte, 512)
    totalSize := 0
    
    for {
        n, err := reader.Read(buffer)
        if err == io.EOF {
            break
        }
        if err != nil {
            fmt.Printf("处理%s时出错: %v\n", fieldName, err)
            return
        }
        totalSize += n
    }
    
    mu.Lock()
    results[fieldName] = totalSize
    mu.Unlock()
    
    fmt.Printf("字段%s处理完成，大小: %d 字节\n", fieldName, totalSize)
}
```

### 从流中解析数据

```go
package main

import (
    "bytes"
    "fmt"
    "io"
    "github.com/yaklang/yaklang/common/jsonextractor"
)

func main() {
    // 模拟从网络或文件读取的数据流
    jsonData := `{
        "users": [
            {"name": "Alice", "age": 25},
            {"name": "Bob", "age": 30},
            {"name": "Charlie", "age": 35}
        ],
        "total": 3,
        "description": "用户列表数据"
    }`

    // 创建一个io.Reader
    reader := bytes.NewBufferString(jsonData)

    fmt.Println("从流中解析JSON数据...")

    err := jsonextractor.ExtractStructuredJSONFromStream(reader,
        jsonextractor.WithArrayCallback(func(data []any) {
            fmt.Printf("解析到数组，长度: %d\n", len(data))
            for i, item := range data {
                if user, ok := item.(map[string]any); ok {
                    fmt.Printf("用户%d: %s, 年龄: %.0f\n", i+1, user["name"], user["age"])
                }
            }
        }),
        
        jsonextractor.WithRegisterFieldStreamHandler("description", func(reader io.Reader) {
            data, _ := io.ReadAll(reader)
            fmt.Printf("描述: %s\n", string(data))
        }),
    )

    if err != nil {
        fmt.Printf("解析失败: %v\n", err)
    }
}
```

### 条件回调示例

```go
package main

import (
    "fmt"
    "github.com/yaklang/yaklang/common/jsonextractor"
)

func main() {
    jsonData := `{
        "user": {
            "name": "Alice",
            "email": "alice@example.com",
            "profile": {
                "age": 25,
                "city": "New York"
            }
        },
        "settings": {
            "theme": "dark",
            "notifications": true
        }
    }`

    err := jsonextractor.ExtractStructuredJSON(jsonData,
        // 只有当对象同时包含name和email字段时才触发
        jsonextractor.WithRegisterConditionalObjectCallback(
            []string{"name", "email"}, 
            func(data map[string]any) {
                fmt.Printf("发现用户对象: %s (%s)\n", data["name"], data["email"])
            },
        ),
        
        // 只有当对象同时包含age和city字段时才触发
        jsonextractor.WithRegisterConditionalObjectCallback(
            []string{"age", "city"}, 
            func(data map[string]any) {
                fmt.Printf("发现档案信息: 年龄%.0f, 城市%s\n", data["age"], data["city"])
            },
        ),
    )

    if err != nil {
        fmt.Printf("解析失败: %v\n", err)
    }
}
```

## 🔧 高级特性

### 容错解析

StreamExtractor 具有强大的容错能力，能够处理各种非标准的JSON格式：

```go
// 支持处理格式不规范的JSON
malformedJSON := `{
    "name": "test",
    "data": "some"incomplete"json",
    "array": [1, 2, 3,],  // 尾随逗号
    "number": 123.45e10
}`

jsonextractor.ExtractStructuredJSON(malformedJSON,
    jsonextractor.WithObjectCallback(func(data map[string]any) {
        fmt.Printf("即使格式有问题，也能解析: %+v\n", data)
    }),
)
```

### 性能优化建议

1. **选择合适的缓冲区大小**：
```go
jsonextractor.WithRegisterFieldStreamHandler("largefield", func(reader io.Reader) {
    buffer := make([]byte, 8192) // 8KB缓冲区，根据数据特点调整
    for {
        n, err := reader.Read(buffer)
        if err == io.EOF {
            break
        }
        // 处理数据...
    }
})
```

2. **避免在回调中进行阻塞操作**：
```go
jsonextractor.WithRegisterFieldStreamHandler("data", func(reader io.Reader) {
    // 好的做法：异步处理
    go func() {
        // 在独立的goroutine中处理耗时操作
        processDataAsync(reader)
    }()
})
```

3. **合理使用条件回调**：
```go
// 避免为每个对象都注册回调，使用条件回调提高效率
jsonextractor.WithRegisterConditionalObjectCallback(
    []string{"type", "id"}, // 只处理包含这些字段的对象
    handleSpecificObjects,
)
```

## 📊 性能对比

| 场景 | 传统JSON解析 | StreamExtractor |
|------|-------------|-----------------|
| 小文件 (< 1MB) | 快速 | 快速 |
| 大文件 (> 100MB) | 内存溢出风险 | 稳定的常量内存使用 |
| 选择性字段解析 | 解析全部后筛选 | 只解析目标字段 |
| 实时数据处理 | 需要等待完整解析 | 边解析边处理 |
| 内存占用 | O(n) 完整加载 | O(1) 流式处理 |
| 处理延迟 | 高（需要完整解析） | 低（即时响应） |

## ⚠️ 注意事项

### 并发安全
```go
var mu sync.Mutex
sharedData := make(map[string]int)

jsonextractor.WithRegisterFieldStreamHandler("field", func(reader io.Reader) {
    // 访问共享资源时需要加锁
    mu.Lock()
    sharedData["field"]++
    mu.Unlock()
})
```

### 错误处理
```go
jsonextractor.WithRegisterFieldStreamHandler("field", func(reader io.Reader) {
    buffer := make([]byte, 1024)
    for {
        n, err := reader.Read(buffer)
        if err == io.EOF {
            break // 正常结束
        }
        if err != nil {
            // 妥善处理读取错误
            log.Printf("读取字段数据失败: %v", err)
            return
        }
        // 处理数据...
    }
})
```

### 资源管理
```go
jsonextractor.WithRegisterFieldStreamHandler("field", func(reader io.Reader) {
    file, err := os.Create("output.txt")
    if err != nil {
        return
    }
    defer file.Close() // 确保资源被释放
    
    io.Copy(file, reader)
})
```

## 🚀 最佳实践

1. **组合使用多种回调类型**：
```go
jsonextractor.ExtractStructuredJSON(data,
    jsonextractor.WithRegisterFieldStreamHandler("content", handleLargeContent),
    jsonextractor.WithObjectCallback(handleObjects),
    jsonextractor.WithArrayCallback(handleArrays),
)
```

2. **使用流式处理处理大字段**：
```go
// 对于大字段，优先使用流式处理器
jsonextractor.WithRegisterFieldStreamHandler("largeField", func(reader io.Reader) {
    // 分块处理，避免内存溢出
    chunk := make([]byte, 4096)
    for {
        n, err := reader.Read(chunk)
        if err == io.EOF {
            break
        }
        processChunk(chunk[:n])
    }
})
```

3. **合理设计错误恢复机制**：
```go
err := jsonextractor.ExtractStructuredJSON(data, callbacks...)
if err != nil {
    if err == io.EOF {
        // 正常结束，部分数据可能已经被处理
        log.Println("数据处理完成")
    } else {
        // 处理其他错误
        log.Printf("解析错误: %v", err)
    }
}
```

这个强大的JSON流式解析器能够满足从简单的数据提取到复杂的实时流处理的各种需求，是处理JSON数据的理想选择。