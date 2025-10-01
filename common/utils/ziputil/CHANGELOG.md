# ziputil 功能更新日志

## 2025-10-01 - 路径搜索和过滤功能

### 新增功能

#### 1. GrepPath 系列 - 文件路径搜索功能

新增了针对文件路径/文件名的搜索功能，不再局限于文件内容搜索：

- `GrepPathRegexp(zipFile, pattern, opts...)` - 使用正则表达式搜索文件路径
- `GrepPathSubString(zipFile, substring, opts...)` - 使用子字符串搜索文件路径
- `GrepPathRawRegexp(raw, pattern, opts...)` - 从原始数据中使用正则搜索路径
- `GrepPathRawSubString(raw, substring, opts...)` - 从原始数据中使用子串搜索路径

**特点：**
- 返回的 `GrepResult` 中 `LineNumber` 为 0（路径搜索没有行号概念）
- `Line` 字段等于 `FileName`（显示路径本身）
- `ScoreMethod` 以 `path_` 开头（如 `path_regexp:\.go$`）
- 不包含上下文（`ContextBefore` 和 `ContextAfter` 为空）

#### 2. 路径过滤选项

新增了强大的路径过滤功能，可在搜索文件内容时精确控制搜索范围：

- `WithIncludePathSubString(patterns...)` - 只搜索路径包含指定子串的文件（任一匹配）
- `WithExcludePathSubString(patterns...)` - 排除路径包含指定子串的文件（任一匹配）
- `WithIncludePathRegexp(patterns...)` - 只搜索路径匹配指定正则的文件（任一匹配）
- `WithExcludePathRegexp(patterns...)` - 排除路径匹配指定正则的文件（任一匹配）

**过滤规则：**
1. 排除规则优先：如果路径匹配任一排除规则，立即排除
2. 包含规则必须满足：如果设置了包含规则，路径必须匹配至少一个包含规则
3. 组合逻辑：`(不匹配任何排除规则) AND (匹配至少一个包含规则或无包含规则)`

**示例：**
```go
// 只在 src/ 目录的 .go 文件中搜索，排除测试文件
results, err := ziputil.GrepSubString("archive.zip", "TODO",
    ziputil.WithIncludePathSubString("src/"),
    ziputil.WithIncludePathRegexp(`\.go$`),
    ziputil.WithExcludePathRegexp(`_test\.go$`),
)
```

#### 3. ZipGrepSearcher 路径功能

`ZipGrepSearcher` 新增了路径搜索方法：

- `searcher.GrepPathRegexp(pattern, opts...)` - 使用正则搜索文件路径
- `searcher.GrepPathSubString(substring, opts...)` - 使用子串搜索文件路径

所有内容搜索方法（`GrepRegexp`、`GrepSubString`）现在都支持路径过滤选项。

### 改进与优化

#### 1. GrepResult 增强

- 新增 `ScoreMethod` 字段，记录搜索方法（如 `regexp:pattern`, `substring:text`, `path_regexp:\.go$`）
- 改进了 `Score` 计算，基于文件顺序和匹配位置
- 路径搜索结果有专门的标识

#### 2. 内部优化

- 所有 Grep 函数现在都使用统一的路径过滤逻辑 `shouldIncludePath()`
- 改进了并发处理，路径过滤在早期阶段就能跳过不符合条件的文件
- 优化了性能，减少不必要的文件读取

### Yaklang 导出

所有新功能都已导出到 Yaklang 的 `zip` 库：

```yak
// GrepPath 系列
results = zip.GrepPathRegexp("archive.zip", `\.go$`)
results = zip.GrepPathSubString("archive.zip", "test")
results = zip.GrepPathRawRegexp(raw, `_test\.go$`)
results = zip.GrepPathRawSubString(raw, ".txt")

// 路径过滤选项（小写命名）
results = zip.GrepSubString("archive.zip", "TODO",
    zip.grepIncludePathSubString("src/"),
    zip.grepExcludePathSubString("vendor", "test"),
    zip.grepIncludePathRegexp(`\.go$`),
    zip.grepExcludePathRegexp(`_test\.go$`),
)

// 搜索器路径功能
searcher = zip.NewGrepSearcher("archive.zip")~
paths = searcher.GrepPathRegexp(`\.json$`)~
results = searcher.GrepSubString("ERROR",
    zip.grepIncludePathSubString("logs/"),
)~
```

### 测试覆盖

新增了全面的测试：

1. **Unit Tests (Go)**
   - `grep_path_test.go` - GrepPath 功能测试
   - 路径过滤功能测试
   - `shouldIncludePath()` 逻辑测试
   - ZipGrepSearcher 路径功能测试

2. **Integration Tests (Yaklang)**
   - `zip.yak` - 基础功能测试（已通过）
   - `zip-advance.yak` - 高级功能和实战案例测试（已通过）

3. **CI/CD**
   - 已集成到 `essential-tests.yml` workflow

### 使用场景

#### 1. 代码库分析
```go
// 找到所有 Go 源文件（排除 vendor）
goFiles, _ := ziputil.GrepPathRegexp("codebase.zip", `\.go$`,
    ziputil.WithExcludePathSubString("vendor"),
)

// 在源码中搜索 TODO（排除测试和vendor）
todos, _ := ziputil.GrepSubString("codebase.zip", "TODO",
    ziputil.WithIncludePathRegexp(`\.go$`),
    ziputil.WithExcludePathRegexp(`_test\.go$`),
    ziputil.WithExcludePathSubString("vendor"),
)
```

#### 2. 安全审计
```go
// 在代码中搜索敏感信息（只在源码，排除测试和配置）
secrets, _ := ziputil.GrepRegexp("app.zip", `(password|api_key|secret)`,
    ziputil.WithIncludePathRegexp(`\.(go|py|js)$`),
    ziputil.WithExcludePathSubString("test", "config"),
    ziputil.WithGrepCaseSensitive(false),
)
```

#### 3. 日志分析
```go
// 只在日志文件中搜索错误
errors, _ := ziputil.GrepRegexp("logs.zip", "ERROR",
    ziputil.WithIncludePathRegexp(`\.log$`),
    ziputil.WithContext(2),
)
```

#### 4. 快速定位
```go
searcher, _ := ziputil.NewZipGrepSearcher("project.zip")

// 步骤1: 找到所有 HTTP 相关文件
httpFiles, _ := searcher.GrepPathSubString("http")

// 步骤2: 在这些文件中搜索函数
functions, _ := searcher.GrepRegexp("func ",
    ziputil.WithIncludePathSubString("http"),
)
```

### 性能提升

- 路径过滤在文件打开前就能排除不符合条件的文件，显著提升搜索效率
- `ZipGrepSearcher` 的缓存机制让重复搜索更快
- 并发处理保持高效（默认 CPU 核心数，最多 8 个并发）

### 向后兼容

所有现有功能保持完全兼容，新功能为可选项。

### 文档

新增文档：
- `PATH_FEATURES.md` - 路径搜索和过滤功能详细说明
- `ADVANCED_FEATURES.md` - 高级功能（GrepResult 合并、RRF）
- `SEARCHER.md` - ZipGrepSearcher 使用指南
- `README.md` - 完整 API 参考

### 验证的功能列表

✅ 34项核心功能：
1. 基础 Grep 功能（4项）
2. Extract 功能（6项）
3. 配置选项（3项）
4. GrepPath 功能（4项）
5. 路径过滤选项（4项）
6. ZipGrepSearcher 功能（3项）
7. 实战场景（7项）
8. 高级应用（2项）
9. 性能优化（2项）

## 贡献者

- 实现：v1ll4n
- 测试：完整的单元测试和集成测试覆盖
- 文档：完整的中英文文档

## 相关 PR/Issues

- 增强 ziputil 包的搜索和过滤能力
- 添加路径搜索功能
- 支持高级路径过滤
- 完善测试覆盖率

