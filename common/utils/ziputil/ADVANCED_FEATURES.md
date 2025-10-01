# ZipUtil Advanced Features

本文档介绍 ziputil 包的高级功能，包括 GrepResult 的人类可读输出、结果合并和 RRF 排序。

## GrepResult 增强功能

### 1. String() 方法 - 人类可读输出

`GrepResult` 实现了 `String()` 方法，可以以人类可读的格式展示搜索结果。

#### 输出格式

```
文件名:行号 [合并的行号列表]
上下文行号  | 上下文内容
主行号 > | 匹配的行内容
上下文行号  | 上下文内容
上下文行号 * | 另一个匹配行（如果是合并结果）
Score: 得分 (Method: 方法名)
```

#### 示例

```go
result := &GrepResult{
    FileName:      "test.go",
    LineNumber:    42,
    Line:          "func main() {",
    ContextBefore: []string{"package main", ""},
    ContextAfter:  []string{"    fmt.Println(\"hello\")", "}"},
    Score:         0.95,
    ScoreMethod:   "regex",
}

fmt.Println(result.String())
```

输出：
```
test.go:42
    40  | package main
    41  |
    42 > | func main() {
    43  |     fmt.Println("hello")
    44  | }
Score: 0.9500 (Method: regex)
```

### 2. 结果合并功能

#### CanMerge() - 判断是否可以合并

判断两个 `GrepResult` 是否可以合并。合并条件：
- 同一个文件
- 一个匹配行在另一个的上下文范围内

```go
r1 := &GrepResult{
    FileName:      "test.txt",
    LineNumber:    10,
    ContextBefore: []string{"line 8", "line 9"},
    ContextAfter:  []string{"line 11", "line 12"},
}

r2 := &GrepResult{
    FileName:      "test.txt",
    LineNumber:    12,
    ContextBefore: []string{"line 10", "line 11"},
    ContextAfter:  []string{"line 13", "line 14"},
}

if r1.CanMerge(r2) {
    fmt.Println("可以合并")
}
```

#### Merge() - 合并两个结果

合并两个 `GrepResult`，将重叠的上下文合并为一个结果。

```go
merged := r1.Merge(r2)
// merged.LineNumber == 10 (使用较小的行号)
// merged.MatchedLines == []int{10, 12} (记录所有匹配行)
```

#### MergeGrepResults() - 批量合并

自动合并一组搜索结果中所有可以合并的项。

```go
results := []*GrepResult{r1, r2, r3, r4}
merged := MergeGrepResults(results)
// 将相邻或重叠的结果自动合并
```

### 3. RRF (Reciprocal Rank Fusion) 排序

`GrepResult` 实现了 `RRFScoredData` 接口，可以使用 RRF 算法进行多方法结果融合排序。

#### RRF 接口实现

```go
// GetUUID 返回唯一标识
func (g *GrepResult) GetUUID() string

// GetScore 返回得分
func (g *GrepResult) GetScore() float64

// GetScoreMethod 返回评分方法
func (g *GrepResult) GetScoreMethod() string
```

#### 使用示例

```go
import "github.com/yaklang/yaklang/common/utils"

// 使用不同方法进行多次搜索
results1 := GrepRegexp("file.zip", "pattern1")
results2 := GrepSubString("file.zip", "keyword")

// 为每组结果设置不同的方法标识
for _, r := range results1 {
    r.ScoreMethod = "regex"
    r.Score = calculateScore(r)
}

for _, r := range results2 {
    r.ScoreMethod = "substring"
    r.Score = calculateScore(r)
}

// 合并结果
allResults := append(results1, results2...)

// 使用 RRF 排序
ranked := utils.RRFRankWithDefaultK(allResults)

// ranked 中的结果按照融合后的得分排序
// 同时在多个方法中得分高的结果会排在前面
```

## Yak 语言集成

在 Yak 语言中使用这些功能：

```yak
// 搜索并打印结果
results = zip.GrepRegexp("logs.zip", "ERROR.*", zip.grepContextLine(2))~
for result in results {
    println(result.String())  // 使用 String() 方法输出
}

// 合并搜索结果
merged = zip.MergeGrepResults(results)~
println("合并后结果数:", len(merged))

// RRF 多方法融合排序
results1 = zip.GrepRegexp("file.zip", "pattern1")~
results2 = zip.GrepSubString("file.zip", "keyword")~

// 设置不同的方法标识和得分
for result in results1 {
    result.ScoreMethod = "regex"
    result.Score = 0.9
}
for result in results2 {
    result.ScoreMethod = "substring"
    result.Score = 0.8
}

allResults = append(results1, results2...)
ranked = zip.RRFRankResults(allResults)~

println("RRF 排序后的最佳匹配:")
for i, result in ranked[:10] {  // 显示前10个结果
    println(sprintf("%d. %s", i+1, result.GetUUID()))
    println(result.String())
}
```

## 实战场景

### 场景 1: 日志分析与错误聚合

```yak
// 搜索所有错误
errors = zip.GrepRegexp("logs.zip", `\[ERROR\]`, zip.grepContextLine(3))~

// 合并相邻的错误（同一个错误场景）
merged = zip.MergeGrepResults(errors)~

// 输出合并后的结果
for error in merged {
    println("\n=== 错误场景 ===")
    println(error.String())
    if len(error.MatchedLines) > 1 {
        println("相关错误行:", error.MatchedLines)
    }
}
```

### 场景 2: 多关键词融合搜索

```yak
// 使用多个关键词搜索
keywords = ["security", "auth", "password", "token"]
allResults = []

for keyword in keywords {
    results = zip.GrepSubString("code.zip", keyword)~
    for r in results {
        r.ScoreMethod = keyword
        r.Score = calculateRelevance(r, keyword)
    }
    allResults = append(allResults, results...)
}

// RRF 融合排序
ranked = zip.RRFRankResults(allResults)~

// 输出最相关的结果
println("最相关的安全代码位置:")
for i := 0; i < min(20, len(ranked)); i++ {
    println(sprintf("\n%d. %s", i+1, ranked[i].GetUUID()))
    println(ranked[i].String())
}
```

### 场景 3: 代码审计报告生成

```yak
// 搜索多种安全问题
patterns = {
    "SQL注入": `(execute|query)\s*\(.*\+.*\)`,
    "XSS漏洞": `innerHTML\s*=`,
    "命令注入": `(exec|system|eval)\s*\(`,
}

findings = {}
for issue, pattern in patterns {
    results = zip.GrepRegexp("webapp.zip", pattern, zip.grepContextLine(5))~
    for r in results {
        r.ScoreMethod = issue
        r.Score = calculateSeverity(r, issue)
    }
    findings[issue] = results
}

// 生成报告
println("=== 代码安全审计报告 ===\n")
for issue, results in findings {
    if len(results) > 0 {
        println(sprintf("\n## %s - 发现 %d 处\n", issue, len(results)))
        
        merged = zip.MergeGrepResults(results)~
        for i, finding in merged {
            println(sprintf("\n### 位置 %d", i+1))
            println(finding.String())
        }
    }
}
```

## API 参考

### GrepResult 结构

```go
type GrepResult struct {
    FileName      string   // 文件名
    LineNumber    int      // 行号
    Line          string   // 匹配的行
    ContextBefore []string // 前置上下文
    ContextAfter  []string // 后置上下文
    
    // RRF 相关字段
    Score       float64 // 搜索得分
    ScoreMethod string  // 搜索方法
    
    // 合并相关字段
    MatchedLines []int // 合并后的所有匹配行号
}
```

### 方法

- `String() string` - 返回人类可读的格式化字符串
- `GetUUID() string` - 返回唯一标识 (实现 RRFScoredData)
- `GetScore() float64` - 返回得分 (实现 RRFScoredData)
- `GetScoreMethod() string` - 返回评分方法 (实现 RRFScoredData)
- `CanMerge(*GrepResult) bool` - 判断是否可以合并
- `Merge(*GrepResult) *GrepResult` - 合并两个结果

### 函数

- `MergeGrepResults([]*GrepResult) []*GrepResult` - 批量合并结果

## 性能考虑

1. **合并效率**: `MergeGrepResults` 会先对结果排序，时间复杂度 O(n log n)
2. **RRF 排序**: RRF 算法的时间复杂度为 O(n log n)，适合中等规模的结果集
3. **String() 方法**: 格式化输出涉及字符串拼接，对于大量结果建议按需调用

## 测试覆盖

所有新功能都有完整的单元测试：
- `TestGrepResult_String` - String() 方法测试
- `TestGrepResult_RRFInterface` - RRF 接口测试
- `TestGrepResult_CanMerge` - 合并判断测试
- `TestGrepResult_Merge` - 合并功能测试
- `TestMergeGrepResults` - 批量合并测试
- `TestGrepResult_RRFRanking` - RRF 排序测试

运行测试：
```bash
go test -v ./common/utils/ziputil/... -run "TestGrepResult"
```

