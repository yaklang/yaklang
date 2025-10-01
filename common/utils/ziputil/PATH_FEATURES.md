# ZIP 路径搜索和过滤功能

## 概述

本文档介绍 `ziputil` 包中的路径搜索和路径过滤功能，这些功能让 ZIP 文件操作更加强大和灵活。

## 功能特性

### 1. GrepPath 系列函数

用于搜索 ZIP 文件中的文件路径/文件名，而不是文件内容。

#### GrepPathRegexp

使用正则表达式搜索文件路径：

```go
// 在文件中搜索
results, err := ziputil.GrepPathRegexp("archive.zip", `\.go$`)

// 从原始数据中搜索
results, err := ziputil.GrepPathRawRegexp(zipData, `_test\.go$`)

// 使用选项
results, err := ziputil.GrepPathRegexp("archive.zip", `^src/`, 
    ziputil.WithGrepLimit(10),
    ziputil.WithGrepCaseSensitive(),
)
```

#### GrepPathSubString

使用子字符串搜索文件路径：

```go
// 搜索包含 "test" 的文件（不区分大小写）
results, err := ziputil.GrepPathSubString("archive.zip", "test")

// 区分大小写搜索
results, err := ziputil.GrepPathSubString("archive.zip", "TEST",
    ziputil.WithGrepCaseSensitive(),
)
```

### 2. 路径过滤选项

在搜索文件内容时，可以使用路径过滤来限制搜索范围。

#### WithIncludePathSubString

只搜索路径包含指定子串的文件（任一匹配即可）：

```go
// 只搜索 src/ 目录下的文件
results, err := ziputil.GrepRegexp("archive.zip", "TODO",
    ziputil.WithIncludePathSubString("src/"),
)

// 可以指定多个包含条件（任一满足即可）
results, err := ziputil.GrepRegexp("archive.zip", "ERROR",
    ziputil.WithIncludePathSubString("src/", "lib/"),
)
```

#### WithExcludePathSubString

排除路径包含指定子串的文件（任一匹配即排除）：

```go
// 排除 test 和 vendor 目录
results, err := ziputil.GrepSubString("archive.zip", "TODO",
    ziputil.WithExcludePathSubString("test", "vendor"),
)
```

#### WithIncludePathRegexp

只搜索路径匹配指定正则的文件：

```go
// 只搜索 .go 文件
results, err := ziputil.GrepSubString("archive.zip", "ERROR",
    ziputil.WithIncludePathRegexp(`\.go$`),
)

// 可以指定多个正则（任一满足即可）
results, err := ziputil.GrepRegexp("archive.zip", "TODO",
    ziputil.WithIncludePathRegexp(`\.go$`, `\.rs$`),
)
```

#### WithExcludePathRegexp

排除路径匹配指定正则的文件：

```go
// 排除测试文件
results, err := ziputil.GrepSubString("archive.zip", "ERROR",
    ziputil.WithExcludePathRegexp(`_test\.go$`),
)
```

### 3. 组合使用

路径过滤选项可以组合使用：

```go
// 只搜索 src/ 目录下的 .go 文件，但排除测试文件
results, err := ziputil.GrepRegexp("archive.zip", "TODO",
    ziputil.WithIncludePathSubString("src/"),
    ziputil.WithIncludePathRegexp(`\.go$`),
    ziputil.WithExcludePathRegexp(`_test\.go$`),
)

// 搜索所有 .go 文件，但排除 vendor 和 test 目录
results, err := ziputil.GrepSubString("archive.zip", "ERROR",
    ziputil.WithIncludePathRegexp(`\.go$`),
    ziputil.WithExcludePathSubString("vendor", "test"),
)
```

### 4. ZipGrepSearcher 支持

`ZipGrepSearcher` 也支持所有路径搜索和过滤功能：

```go
searcher, err := ziputil.NewZipGrepSearcher("archive.zip")
if err != nil {
    return err
}

// 搜索文件路径
paths, err := searcher.GrepPathRegexp(`\.go$`)

// 在内容中搜索时使用路径过滤
results, err := searcher.GrepSubString("TODO",
    ziputil.WithIncludePathSubString("src/"),
    ziputil.WithExcludePathSubString("vendor"),
)
```

## API 参考

### GrepPath 函数

| 函数名 | 描述 | 搜索目标 |
|--------|------|----------|
| `GrepPathRegexp` | 正则表达式搜索文件路径 | ZIP 文件 |
| `GrepPathSubString` | 子字符串搜索文件路径 | ZIP 文件 |
| `GrepPathRawRegexp` | 正则表达式搜索文件路径 | 原始数据 |
| `GrepPathRawSubString` | 子字符串搜索文件路径 | 原始数据 |

### 路径过滤选项

| 选项名 | 类型 | 描述 |
|--------|------|------|
| `WithIncludePathSubString` | 包含过滤 | 路径必须包含指定子串（任一） |
| `WithExcludePathSubString` | 排除过滤 | 排除包含指定子串的路径（任一） |
| `WithIncludePathRegexp` | 包含过滤 | 路径必须匹配指定正则（任一） |
| `WithExcludePathRegexp` | 排除过滤 | 排除匹配指定正则的路径（任一） |

### 过滤规则

1. **排除规则优先**：如果路径匹配任一排除规则，立即排除
2. **包含规则必须满足**：如果设置了包含规则，路径必须匹配至少一个包含规则
3. **组合逻辑**：`(不匹配任何排除规则) AND (匹配至少一个包含规则或无包含规则)`

## 使用场景

### 1. 在代码库中搜索

```go
// 只在源码中搜索 TODO
results, err := ziputil.GrepRegexp("codebase.zip", "TODO",
    ziputil.WithIncludePathRegexp(`\.(go|rs|py)$`),
    ziputil.WithExcludePathSubString("vendor", "node_modules", "test"),
)
```

### 2. 列出特定类型的文件

```go
// 列出所有配置文件
configs, err := ziputil.GrepPathRegexp("project.zip", `\.(json|yaml|yml|toml)$`)

// 列出所有 test 文件
tests, err := ziputil.GrepPathRegexp("project.zip", `_test\.go$`)
```

### 3. 在特定目录搜索

```go
// 只在 api/ 目录下搜索
results, err := ziputil.GrepSubString("backend.zip", "API_KEY",
    ziputil.WithIncludePathSubString("api/"),
)
```

### 4. 排除特定目录

```go
// 搜索所有文件但排除测试和文档
results, err := ziputil.GrepRegexp("project.zip", "error",
    ziputil.WithExcludePathSubString("test", "docs", ".git"),
)
```

## GrepResult 特殊字段

对于 GrepPath 搜索，`GrepResult` 有以下特点：

- `LineNumber` 为 0（路径搜索没有行号概念）
- `Line` 等于 `FileName`（显示路径本身）
- `ScoreMethod` 以 `path_` 开头（如 `path_regexp:\.go$` 或 `path_substring:test`）
- `ContextBefore` 和 `ContextAfter` 为空（路径搜索不需要上下文）

示例：

```go
results, err := ziputil.GrepPathSubString("archive.zip", ".go")
for _, r := range results {
    fmt.Printf("File: %s (Score: %.4f, Method: %s)\n", 
        r.FileName, r.Score, r.ScoreMethod)
}
// 输出：
// File: src/main.go (Score: 1.0000, Method: path_substring:.go)
// File: src/utils.go (Score: 0.5000, Method: path_substring:.go)
```

## 性能优化建议

1. **使用路径过滤减少搜索范围**：在搜索大型 ZIP 文件时，使用路径过滤可以显著提升性能

2. **使用 ZipGrepSearcher 进行多次搜索**：如果需要多次搜索，使用 `ZipGrepSearcher` 可以缓存文件内容

3. **优先使用排除过滤**：排除过滤在早期就能跳过文件，比包含过滤更高效

4. **合理使用正则表达式**：简单的子串匹配比复杂的正则表达式更快

## 示例：综合应用

```go
package main

import (
    "fmt"
    "github.com/yaklang/yaklang/common/utils/ziputil"
)

func main() {
    // 创建带缓存的搜索器
    searcher, err := ziputil.NewZipGrepSearcher("large-project.zip")
    if err != nil {
        panic(err)
    }

    // 1. 列出所有 Go 源文件
    goFiles, _ := searcher.GrepPathRegexp(`\.go$`,
        ziputil.WithExcludePathSubString("vendor"),
    )
    fmt.Printf("Found %d Go files\n", len(goFiles))

    // 2. 在源码中搜索 TODO（排除测试文件）
    todos, _ := searcher.GrepRegexp("TODO",
        ziputil.WithIncludePathRegexp(`\.go$`),
        ziputil.WithExcludePathRegexp(`_test\.go$`),
        ziputil.WithContext(2),
    )
    fmt.Printf("Found %d TODOs in source code\n", len(todos))

    // 3. 搜索 API 密钥（只在配置文件中）
    apiKeys, _ := searcher.GrepRegexp(`API[_-]?KEY`,
        ziputil.WithIncludePathRegexp(`\.(json|yaml|yml|env)$`),
        ziputil.WithGrepCaseSensitive(),
    )
    fmt.Printf("Found %d potential API keys\n", len(apiKeys))

    // 4. 列出所有测试文件
    testFiles, _ := searcher.GrepPathRegexp(`_test\.go$`)
    fmt.Printf("Found %d test files\n", len(testFiles))
}
```

## 在 Yaklang 中使用

所有功能都已导出到 Yaklang 的 `zip` 库：

```yak
// GrepPath 功能
results = zip.GrepPathRegexp("archive.zip", `\.go$`)
results = zip.GrepPathSubString("archive.zip", "test")

// 路径过滤选项
results = zip.GrepRegexp("archive.zip", "TODO",
    zip.grepIncludePathSubString("src/"),
    zip.grepExcludePathSubString("vendor", "test"),
    zip.grepIncludePathRegexp(`\.go$`),
    zip.grepExcludePathRegexp(`_test\.go$`),
)

// 搜索器支持
searcher = zip.NewGrepSearcher("archive.zip")
paths = searcher.GrepPathSubString(".go")
results = searcher.GrepSubString("ERROR",
    zip.grepIncludePathSubString("src/"),
)
```

