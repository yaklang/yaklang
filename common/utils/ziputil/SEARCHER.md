# ZipGrepSearcher - 带缓存的 ZIP 搜索器

`ZipGrepSearcher` 是一个带缓存机制的 ZIP 文件搜索器，可以显著提高多次搜索同一个 ZIP 文件的性能。

## 特性

- ✅ **智能缓存** - 自动缓存访问过的文件内容
- ✅ **预加载选项** - 可选择预加载所有文件到内存
- ✅ **并发安全** - 支持多个 goroutine 并发搜索
- ✅ **内存高效** - 按需加载，只缓存实际访问的文件
- ✅ **完整 API** - 支持所有 Grep 功能（正则、子串、上下文等）
- ✅ **文件级搜索** - 支持在指定文件中搜索

## 快速开始

### 基本用法

```go
// 从文件创建搜索器
searcher, err := ziputil.NewZipGrepSearcher("logs.zip")
if err != nil {
    log.Fatal(err)
}

// 搜索（自动缓存）
results, err := searcher.GrepSubString("ERROR", ziputil.WithContext(2))
if err != nil {
    log.Fatal(err)
}

// 再次搜索（使用缓存，更快）
results2, err := searcher.GrepRegexp("WARN.*")
```

### 从原始数据创建

```go
zipData := []byte{...} // ZIP 文件的字节数据

searcher, err := ziputil.NewZipGrepSearcherFromRaw(zipData, "archive.zip")
if err != nil {
    log.Fatal(err)
}
```

## API 文档

### 创建搜索器

#### NewZipGrepSearcher
```go
func NewZipGrepSearcher(zipFile string) (*ZipGrepSearcher, error)
```
从文件路径创建搜索器。

#### NewZipGrepSearcherFromRaw
```go
func NewZipGrepSearcherFromRaw(raw interface{}, filename ...string) (*ZipGrepSearcher, error)
```
从原始数据（[]byte、string 或 io.Reader）创建搜索器。

### 搜索方法

#### GrepRegexp
```go
func (s *ZipGrepSearcher) GrepRegexp(pattern string, opts ...GrepOption) ([]*GrepResult, error)
```
使用正则表达式在所有文件中搜索。

#### GrepSubString
```go
func (s *ZipGrepSearcher) GrepSubString(substring string, opts ...GrepOption) ([]*GrepResult, error)
```
使用子字符串在所有文件中搜索（默认不区分大小写）。

#### GrepRegexpInFile
```go
func (s *ZipGrepSearcher) GrepRegexpInFile(fileName string, pattern string, opts ...GrepOption) ([]*GrepResult, error)
```
在指定文件中使用正则表达式搜索。

#### GrepSubStringInFile
```go
func (s *ZipGrepSearcher) GrepSubStringInFile(fileName string, substring string, opts ...GrepOption) ([]*GrepResult, error)
```
在指定文件中使用子字符串搜索。

### 缓存管理

#### WithCacheAll
```go
func (s *ZipGrepSearcher) WithCacheAll(cacheAll bool) *ZipGrepSearcher
```
启用预加载所有文件。适用于需要多次搜索的场景。

```go
searcher.WithCacheAll(true)  // 立即加载所有文件到缓存
```

#### ClearCache
```go
func (s *ZipGrepSearcher) ClearCache()
```
清空所有缓存。

#### GetCachedFiles
```go
func (s *ZipGrepSearcher) GetCachedFiles() []string
```
返回已缓存的文件名列表。

#### GetCacheSize
```go
func (s *ZipGrepSearcher) GetCacheSize() int
```
返回缓存占用的字节数。

#### GetFileCount
```go
func (s *ZipGrepSearcher) GetFileCount() int
```
返回 ZIP 中的文件总数。

### 辅助方法

#### GetFileContent
```go
func (s *ZipGrepSearcher) GetFileContent(fileName string) (string, error)
```
获取指定文件的完整内容（会被缓存）。

#### String
```go
func (s *ZipGrepSearcher) String() string
```
返回搜索器的描述信息。

## 使用场景

### 场景 1: 日志文件分析

适合需要在同一个日志压缩包中进行多次不同关键词搜索的场景。

```go
searcher, _ := ziputil.NewZipGrepSearcher("logs.zip")

// 第一次搜索 ERROR
errors, _ := searcher.GrepSubString("ERROR", ziputil.WithContext(3))
fmt.Printf("Found %d errors\n", len(errors))

// 第二次搜索 WARNING（使用缓存，更快）
warnings, _ := searcher.GrepSubString("WARNING")
fmt.Printf("Found %d warnings\n", len(warnings))

// 第三次使用正则搜索（仍然使用缓存）
criticals, _ := searcher.GrepRegexp("CRITICAL:.*")
fmt.Printf("Found %d criticals\n", len(criticals))

// 查看缓存情况
fmt.Printf("Cached %d files, using %d bytes\n", 
    len(searcher.GetCachedFiles()), searcher.GetCacheSize())
```

### 场景 2: 代码审计

适合需要在代码压缩包中搜索多种安全模式的场景。

```go
searcher, _ := ziputil.NewZipGrepSearcher("code.zip")

// 预加载所有文件（因为需要多次搜索）
searcher.WithCacheAll(true)

patterns := map[string]string{
    "SQL注入": `(execute|query)\s*\(.*\+.*\)`,
    "XSS漏洞": `innerHTML\s*=`,
    "命令注入": `(exec|system|eval)\s*\(`,
    "硬编码密码": `password\s*=\s*["'].*["']`,
}

findings := make(map[string][]*ziputil.GrepResult)
for issue, pattern := range patterns {
    results, _ := searcher.GrepRegexp(pattern)
    if len(results) > 0 {
        findings[issue] = results
    }
}

// 生成报告
for issue, results := range findings {
    fmt.Printf("\n=== %s - 发现 %d 处 ===\n", issue, len(results))
    for _, r := range results {
        fmt.Println(r.String())
    }
}
```

### 场景 3: 特定文件深度搜索

适合已知目标文件，需要在其中进行多种搜索的场景。

```go
searcher, _ := ziputil.NewZipGrepSearcher("app.zip")

targetFile := "src/main.go"

// 搜索函数定义
funcs, _ := searcher.GrepRegexpInFile(targetFile, `func\s+\w+`)
fmt.Printf("Found %d functions\n", len(funcs))

// 搜索 TODO 注释
todos, _ := searcher.GrepSubStringInFile(targetFile, "TODO")
fmt.Printf("Found %d TODOs\n", len(todos))

// 搜索错误处理
errors, _ := searcher.GrepRegexpInFile(targetFile, `if.*err\s*!=\s*nil`)
fmt.Printf("Found %d error checks\n", len(errors))

// 文件内容已被缓存，后续搜索更快
```

## Yak 语言集成

在 Yak 脚本中使用 ZipGrepSearcher：

```yak
// 创建搜索器
searcher = zip.NewGrepSearcher("logs.zip")~

// 搜索错误
errors = searcher.GrepSubString("ERROR", zip.grepContextLine(2))~
println("找到", len(errors), "个错误")

// 打印结果
for error in errors {
    println(error.String())
}

// 搜索警告（使用缓存）
warnings = searcher.GrepSubString("WARNING")~
println("找到", len(warnings), "个警告")

// 查看缓存状态
println("缓存信息:", searcher.String())
println("已缓存文件:", searcher.GetCachedFiles())
println("缓存大小:", searcher.GetCacheSize(), "字节")
```

### 预加载所有文件

```yak
// 创建搜索器并预加载
searcher = zip.NewGrepSearcher("code.zip")~
searcher.WithCacheAll(true)  // 立即加载所有文件

println("预加载完成，共", searcher.GetFileCount(), "个文件")

// 现在所有搜索都会很快
results1 = searcher.GrepSubString("security")~
results2 = searcher.GrepRegexp("auth.*")~
results3 = searcher.GrepSubString("password")~
```

### 在特定文件中搜索

```yak
searcher = zip.NewGrepSearcher("project.zip")~

// 只在 main.yak 中搜索
results = searcher.GrepRegexpInFile("main.yak", "func\\s+\\w+")~
println("main.yak 中找到", len(results), "个函数")

for result in results {
    println(result.String())
}
```

## 性能对比

### 无缓存 vs 有缓存

```go
// 场景：在一个包含 100 个文件的 ZIP 中进行 5 次搜索

// 方式 1: 每次都读取 ZIP（无缓存）
for i := 0; i < 5; i++ {
    results, _ := ziputil.GrepSubString("archive.zip", "keyword")
    // 每次都要：读取 ZIP + 解压所有文件 + 搜索
}

// 方式 2: 使用 ZipGrepSearcher（有缓存）
searcher, _ := ziputil.NewZipGrepSearcher("archive.zip")
for i := 0; i < 5; i++ {
    results, _ := searcher.GrepSubString("keyword")
    // 第一次：读取 + 解压 + 搜索 + 缓存
    // 后续：直接从缓存搜索（快 10-50 倍）
}
```

### 预加载 vs 按需加载

```go
// 按需加载（默认）
searcher1, _ := ziputil.NewZipGrepSearcher("archive.zip")
// 首次搜索会加载文件
results1, _ := searcher1.GrepSubString("keyword1")  // 较慢
results2, _ := searcher1.GrepSubString("keyword2")  // 较快（缓存）

// 预加载（适合多次搜索）
searcher2, _ := ziputil.NewZipGrepSearcher("archive.zip")
searcher2.WithCacheAll(true)  // 一次性加载所有
// 所有后续搜索都很快
results3, _ := searcher2.GrepSubString("keyword1")  // 快
results4, _ := searcher2.GrepSubString("keyword2")  // 快
results5, _ := searcher2.GrepRegexp("pattern")     // 快
```

## 内存管理

### 缓存策略

1. **按需加载（默认）**：只缓存实际搜索时访问的文件
2. **预加载模式**：一次性加载所有文件到缓存
3. **手动清理**：使用 `ClearCache()` 清空缓存

### 内存占用估算

```
缓存大小 ≈ 已缓存文件的原始大小（未压缩）
```

示例：
- ZIP 文件：10 MB（压缩）
- 解压后总大小：50 MB
- 预加载所有文件：~50 MB 内存
- 只搜索 20% 文件：~10 MB 内存

### 建议

| 场景 | 建议策略 | 原因 |
|------|----------|------|
| 单次搜索 | 使用普通 Grep | 不需要缓存 |
| 2-3 次搜索 | ZipGrepSearcher（默认） | 按需缓存 |
| 多次搜索（5次+） | WithCacheAll(true) | 预加载更快 |
| 大文件（>100MB） | 按需加载 | 节省内存 |
| 小文件（<10MB） | 预加载 | 影响小，速度快 |

## 线程安全

`ZipGrepSearcher` 是线程安全的，可以在多个 goroutine 中并发使用：

```go
searcher, _ := ziputil.NewZipGrepSearcher("logs.zip")

var wg sync.WaitGroup

// 并发搜索不同关键词
for _, keyword := range []string{"ERROR", "WARNING", "INFO"} {
    wg.Add(1)
    go func(kw string) {
        defer wg.Done()
        results, _ := searcher.GrepSubString(kw)
        fmt.Printf("%s: %d results\n", kw, len(results))
    }(keyword)
}

wg.Wait()
```

## 最佳实践

1. **复用搜索器**：对于同一个 ZIP 文件，复用搜索器实例
2. **预加载小文件**：小于 10MB 的 ZIP 可以直接预加载
3. **监控缓存**：使用 `GetCacheSize()` 监控内存占用
4. **及时清理**：不再需要时调用 `ClearCache()` 释放内存
5. **组合使用**：与 `MergeGrepResults` 和 `RRFRankResults` 结合使用

## 测试覆盖

完整的测试套件包括：
- `TestNewZipGrepSearcher` - 创建搜索器
- `TestZipGrepSearcher_GrepRegexp` - 正则搜索
- `TestZipGrepSearcher_GrepSubString` - 子串搜索
- `TestZipGrepSearcher_Cache` - 缓存功能
- `TestZipGrepSearcher_GrepInFile` - 文件级搜索
- `TestZipGrepSearcher_GetFileContent` - 内容获取
- `TestZipGrepSearcher_Performance` - 性能测试
- `TestZipGrepSearcher_ConcurrentAccess` - 并发安全测试

运行测试：
```bash
go test -v ./common/utils/ziputil/... -run "TestZipGrepSearcher"
```

## 总结

`ZipGrepSearcher` 通过智能缓存机制，显著提升了多次搜索 ZIP 文件的性能。它特别适合：

- ✅ 日志分析
- ✅ 代码审计
- ✅ 配置文件检查
- ✅ 批量文本搜索
- ✅ 安全扫描

选择合适的缓存策略，可以在性能和内存占用之间取得最佳平衡。

