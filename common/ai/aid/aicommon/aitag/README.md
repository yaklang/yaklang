# AITAG - 流式标签解析器

AITAG 是一个专门用于解析流式文本中特定格式标签的工具，能够从 `io.Reader` 中提取标签内容并触发相应的回调函数。

## 特性

- **流式解析**: 支持从 `io.Reader` 进行流式解析，适合处理大型数据流
- **标签格式**: 解析形如 `<|TAGNAME_{{ .Nonce }}|>` 和 `<|TAGNAME_END_{{ .Nonce }}|>` 的标签对
- **回调机制**: 为每个标签-nonce组合注册回调函数，在解析到内容时触发
- **错误容忍**: 能够处理格式错误的标签，跳过无效内容继续解析
- **高性能**: 基于简化的状态机设计，避免复杂的嵌套处理
- **并发安全**: 支持并发处理多个独立的流

## 重要限制

**不支持嵌套标签**: 本解析器明确不支持标签嵌套。如果在一个标签内遇到另一个标签，内层标签会被当作普通文本内容处理。

## 使用方法

### 基本用法

```go
package main

import (
    "io"
    "strings"
    "fmt"
    
    "github.com/yaklang/yaklang/common/ai/aid/aicommon/aitag"
)

func main() {
    input := `前面一些内容
<|CODE_abc123|>
package main

import "fmt"

func main() {
    fmt.Println("Hello World")
}
<|CODE_END_abc123|>
后面一些内容`

    err := aitag.Parse(strings.NewReader(input),
        aitag.WithCallback("CODE", "abc123", func(reader io.Reader) {
            content, _ := io.ReadAll(reader)
            fmt.Printf("捕获到代码:\n%s\n", string(content))
        }),
    )
    
    if err != nil {
        panic(err)
    }
}
```

### 多标签处理

```go
input := `开始处理
<|REQUEST_req001|>
{"method": "POST", "url": "/api/data"}
<|REQUEST_END_req001|>

<|RESPONSE_req001|>
{"status": 200, "data": "success"}
<|RESPONSE_END_req001|>
处理完成`

err := aitag.Parse(strings.NewReader(input),
    aitag.WithCallback("REQUEST", "req001", func(reader io.Reader) {
        content, _ := io.ReadAll(reader)
        fmt.Printf("请求: %s\n", string(content))
    }),
    aitag.WithCallback("RESPONSE", "req001", func(reader io.Reader) {
        content, _ := io.ReadAll(reader)
        fmt.Printf("响应: %s\n", string(content))
    }),
)
```

### 使用回调映射

```go
callbacks := map[string]map[string]aitag.CallbackFunc{
    "CODE": {
        "block1": func(reader io.Reader) {
            // 处理代码块1
        },
        "block2": func(reader io.Reader) {
            // 处理代码块2
        },
    },
    "DATA": {
        "dataset1": func(reader io.Reader) {
            // 处理数据集1
        },
    },
}

err := aitag.ParseWithCallbacks(strings.NewReader(input), callbacks)
```

## 标签格式

### 开始标签
```
<|TAGNAME_NONCE|>
```

### 结束标签
```
<|TAGNAME_END_NONCE|>
```

### 规则
- `TAGNAME`: 标签名，可包含字母、数字、下划线
- `NONCE`: 标识符，用于区分同名标签的不同实例
- 标签名和 nonce 通过最后一个下划线分隔
- 不支持标签嵌套

## API 参考

### 核心函数

#### `Parse(reader io.Reader, options ...ParseOption) error`
解析输入流中的标签内容。

#### `WithCallback(tagName, nonce string, callback CallbackFunc) ParseOption`
注册标签回调函数。

#### `ParseWithCallbacks(reader io.Reader, callbacks map[string]map[string]CallbackFunc) error`
使用回调映射进行解析的便利函数。

### 类型定义

#### `CallbackFunc`
```go
type CallbackFunc func(reader io.Reader)
```
标签内容处理函数，接收包含标签内容的 `io.Reader`。

## 性能特点

- **内存效率**: 将整个流读入内存后进行解析，适合大多数用例
- **处理速度**: 基于简化状态机，避免复杂的栈操作
- **错误恢复**: 遇到格式错误的标签时能够快速跳过并继续解析
- **大数据支持**: 成功测试了 14MB+ 的输入数据

## 测试覆盖

项目包含全面的测试套件，覆盖：
- 基本标签解析
- 多标签顺序处理
- 格式错误处理
- 边界条件测试
- 大数据流处理
- 并发安全性
- 流中断处理
- Unicode 字符支持

## 日志输出

使用 yaklang 项目的 `common/log` 包进行日志输出，所有日志内容均为英文。调试信息使用 `log.Debugf`，普通信息使用 `log.Infof`，警告使用 `log.Warnf`。

## 限制和注意事项

1. **不支持嵌套**: 嵌套标签会被当作文本内容处理
2. **内存使用**: 当前实现会将整个输入读入内存
3. **标签跨行**: 标签本身不应跨越多行
4. **字符编码**: 假设输入为 UTF-8 编码

## 示例输出

```
[INFO] 2025-09-25 12:10:35 [example:96] 收到完整请求数据
[INFO] 2025-09-25 12:10:35 [example:101] 收到请求头数据
[INFO] 2025-09-25 12:10:35 [example:106] 收到请求体数据
[WARN] 2025-09-25 12:10:35 [extractor:160] stream ended with unclosed tag: INCOMPLETE_test
```
