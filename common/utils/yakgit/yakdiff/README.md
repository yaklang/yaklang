# YakDiff - 强大的差异比较工具

YakDiff 是一个基于 Git 的高性能差异比较工具，支持文本和文件系统的差异比较，能够生成标准的 unified diff 格式输出。

## 特性

- 🚀 **高性能**: 基于 Git 内核，处理大文件和文件系统速度快
- 🔧 **简单易用**: 简洁的API设计，支持任意数据类型和文件系统
- 🎯 **精确比较**: 生成标准的 unified diff 格式
- 🔄 **向后兼容**: 支持自定义处理器的传统用法
- 🛡️ **并发安全**: 支持多goroutine并发使用
- 🌍 **多数据类型**: 自动转换各种数据类型进行比较
- 📁 **文件系统支持**: 完整的文件系统差异检测（增加、修改、删除）
- 🗂️ **目录结构**: 支持复杂的目录结构和嵌套文件比较

## 快速开始

### 1. 文本字符串比较

```go
package main

import (
    "fmt"
    "github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
)

func main() {
    // 简单字符串比较
    diff, err := yakdiff.Diff("Hello World", "Hello Yaklang")
    if err != nil {
        panic(err)
    }
    fmt.Print(diff)
}
```

输出：
```diff
diff --git a/main/main.txt b/main/main.txt
index 5e1c309dae7f45e0f39b1bf3ac3cd9db12e7d689..8ac4312112bc24c6ff0ca3c98e5f6ad3e965ce4e 100644
--- a/main/main.txt
+++ b/main/main.txt
@@ -1 +1 @@
-Hello World
\ No newline at end of file
+Hello Yaklang
\ No newline at end of file
```

### 2. 文件系统比较

```go
package main

import (
    "fmt"
    "github.com/yaklang/yaklang/common/utils/filesys"
    "github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
)

func main() {
    // 创建第一个虚拟文件系统
    fs1 := filesys.NewVirtualFs()
    fs1.WriteFile("config.json", []byte(`{"port": 8080}`), 0644)
    fs1.WriteFile("app.go", []byte("package main\n\nfunc main() {}"), 0644)
    
    // 创建第二个虚拟文件系统（修改后）
    fs2 := filesys.NewVirtualFs()
    fs2.WriteFile("config.json", []byte(`{"port": 9090}`), 0644)
    fs2.WriteFile("app.go", []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n    fmt.Println(\"Hello!\")\n}"), 0644)
    fs2.WriteFile("README.md", []byte("# New Project"), 0644) // 新文件
    
    // 生成文件系统差异
    diff, err := yakdiff.FileSystemDiff(fs1, fs2)
    if err != nil {
        panic(err)
    }
    fmt.Print(diff)
}
```

输出：
```diff
diff --git a/README.md b/README.md
new file mode 100644
index 0000000000000000000000000000000000000000..8b25206e90253016da35c1ee4a7bd94d6bf747c3
--- /dev/null
+++ b/README.md
@@ -0,0 +1 @@
+# New Project
\ No newline at end of file
diff --git a/app.go b/app.go
index 38c0c6b888b2a09e566e9f4301c64321c6c7f36a..69b0fde62acf1b0e60a7ed1c52c6e2f6f2d50d58 100644
--- a/app.go
+++ b/app.go
@@ -1,3 +1,6 @@
 package main
 
-func main() {}
\ No newline at end of file
+import "fmt"
+
+func main() {
+    fmt.Println("Hello!")
+}
\ No newline at end of file
diff --git a/config.json b/config.json
index 6b4e5c3b6e8f11cee4d1e78b6ae5d00ba68f1e8e..bb7c0bb0c24b0b25b54e5c10d5b35267842c5d2e 100644
--- a/config.json
+++ b/config.json
@@ -1 +1 @@
-{"port": 8080}
\ No newline at end of file
+{"port": 9090}
\ No newline at end of file
```

### 3. 多行文本比较

```go
code1 := `package main

import "fmt"

func main() {
    fmt.Println("Old version")
}`

code2 := `package main

import (
    "fmt"
    "os"
)

func main() {
    fmt.Println("New version")
    os.Exit(0)
}`

diff, err := yakdiff.Diff(code1, code2)
if err != nil {
    panic(err)
}
fmt.Print(diff)
```

## API 参考

### 文本字符串差异函数

#### `Diff(raw1, raw2 any, handler ...DiffHandler) (string, error)`

主要的文本差异比较函数。

**参数：**
- `raw1`: 第一个比较对象（任意类型）
- `raw2`: 第二个比较对象（任意类型）  
- `handler`: 可选的自定义处理器

**返回值：**
- `string`: diff结果字符串
- `error`: 错误信息

#### `DiffToString(raw1, raw2 any) (string, error)`

专门用于生成diff字符串的函数。

#### `DiffToStringContext(ctx context.Context, raw1, raw2 any) (string, error)`

带上下文的diff字符串生成函数，支持取消操作。

#### `DiffContext(ctx context.Context, raw1, raw2 any, handler ...DiffHandler) error`

带上下文的传统diff函数，保持向后兼容。

### 文件系统差异函数

#### `FileSystemDiff(fs1, fs2 fi.FileSystem, handler ...DiffHandler) (string, error)`

主要的文件系统差异比较函数。

**参数：**
- `fs1`: 第一个文件系统
- `fs2`: 第二个文件系统
- `handler`: 可选的自定义处理器

**返回值：**
- `string`: diff结果字符串
- `error`: 错误信息

**功能：**
- ✅ 检测文件新增
- ✅ 检测文件修改
- ✅ 检测文件删除
- ✅ 支持目录结构变化
- ✅ 支持嵌套文件和目录

#### `FileSystemDiffToString(fs1, fs2 fi.FileSystem) (string, error)`

专门用于生成文件系统diff字符串的函数。

#### `FileSystemDiffToStringContext(ctx context.Context, fs1, fs2 fi.FileSystem) (string, error)`

带上下文的文件系统diff字符串生成函数，支持取消操作。

#### `FileSystemDiffContext(ctx context.Context, fs1, fs2 fi.FileSystem, handler ...DiffHandler) error`

带上下文的传统文件系统diff函数，保持向后兼容。

### 自定义处理器

```go
type DiffHandler func(*object.Commit, *object.Change, *object.Patch) error
```

如果需要自定义处理逻辑，可以提供处理器函数：

```go
customHandler := func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
    if patch != nil {
        fmt.Printf("Change detected: %s\n", patch.String())
    }
    return nil
}

_, err := yakdiff.Diff("old", "new", customHandler)
```

## 使用场景

### 文本字符串差异场景

#### 1. 代码变更检测

```go
// 比较两个代码文件的差异
oldCode := `func add(a, b int) int {
    return a + b
}`

newCode := `func add(a, b int) int {
    result := a + b
    return result
}`

diff, _ := yakdiff.Diff(oldCode, newCode)
fmt.Print(diff)
```

#### 2. 配置文件变更

```go
// 比较配置文件差异
oldConfig := `{
    "port": 8080,
    "debug": false
}`

newConfig := `{
    "port": 9090,
    "debug": true,
    "timeout": 30
}`

diff, _ := yakdiff.Diff(oldConfig, newConfig)
fmt.Print(diff)
```

#### 3. 数据结构比较

```go
// 支持各种数据类型
slice1 := []string{"apple", "banana"}
slice2 := []string{"apple", "orange", "banana"}

diff, _ := yakdiff.Diff(slice1, slice2)
fmt.Print(diff)
```

#### 4. 二进制数据比较

```go
// 二进制数据也能处理
binary1 := []byte{0x01, 0x02, 0x03}
binary2 := []byte{0x01, 0x04, 0x03}

diff, _ := yakdiff.Diff(binary1, binary2)
fmt.Print(diff)
```

### 文件系统差异场景

#### 1. 项目版本比较

```go
// 比较两个项目版本的文件系统差异
func compareProjectVersions(version1, version2 fi.FileSystem) {
    diff, err := yakdiff.FileSystemDiff(version1, version2)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Version differences:\n%s", diff)
}
```

#### 2. 配置目录变更检测

```go
// 检测配置目录的变更
func detectConfigChanges() {
    configV1 := filesys.NewVirtualFs()
    configV1.WriteFile("nginx.conf", []byte("server { listen 80; }"), 0644)
    configV1.WriteFile("ssl.conf", []byte("ssl_protocols TLSv1.2;"), 0644)
    
    configV2 := filesys.NewVirtualFs()
    configV2.WriteFile("nginx.conf", []byte("server { listen 8080; }"), 0644)
    configV2.WriteFile("ssl.conf", []byte("ssl_protocols TLSv1.3;"), 0644)
    configV2.WriteFile("cache.conf", []byte("expires 1h;"), 0644) // 新增文件
    
    diff, _ := yakdiff.FileSystemDiff(configV1, configV2)
    fmt.Print(diff)
}
```

#### 3. 代码重构检测

```go
// 检测代码重构的文件变化
func detectRefactoring(beforeRefactor, afterRefactor fi.FileSystem) {
    diff, err := yakdiff.FileSystemDiff(beforeRefactor, afterRefactor)
    if err != nil {
        log.Fatal(err)
    }
    
    // 分析diff结果
    if strings.Contains(diff, "deleted file") {
        fmt.Println("有文件被删除")
    }
    if strings.Contains(diff, "new file") {
        fmt.Println("有新文件被创建")
    }
    
    fmt.Print(diff)
}
```

#### 4. 部署前后对比

```go
// 部署前后的文件系统对比
func compareDeployment(preDeployment, postDeployment fi.FileSystem) error {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    diff, err := yakdiff.FileSystemDiffToStringContext(ctx, preDeployment, postDeployment)
    if err != nil {
        return err
    }
    
    // 生成部署报告
    if strings.TrimSpace(diff) == "" {
        fmt.Println("部署没有发生文件变化")
    } else {
        fmt.Printf("部署变化报告:\n%s", diff)
    }
    
    return nil
}
```

#### 5. 自定义文件系统处理器

```go
import (
    "fmt"
    "log"
    
    "github.com/go-git/go-git/v5/plumbing/object"
    "github.com/go-git/go-git/v5/plumbing/filemode"
    "github.com/yaklang/yaklang/common/utils/yakgit/yakdiff"
    fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

// 使用自定义处理器分析文件系统变化
func analyzeFileSystemChanges(fs1, fs2 fi.FileSystem) {
    var addedFiles, modifiedFiles, deletedFiles []string
    
    customHandler := func(commit *object.Commit, change *object.Change, patch *object.Patch) error {
        if change.From.Name == "" {
            // 新增文件
            addedFiles = append(addedFiles, change.To.Name)
        } else if change.To.Name == "" {
            // 删除文件
            deletedFiles = append(deletedFiles, change.From.Name)
        } else {
            // 修改文件
            modifiedFiles = append(modifiedFiles, change.To.Name)
        }
        return nil
    }
    
    _, err := yakdiff.FileSystemDiff(fs1, fs2, customHandler)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("新增文件: %v\n", addedFiles)
    fmt.Printf("修改文件: %v\n", modifiedFiles) 
    fmt.Printf("删除文件: %v\n", deletedFiles)
}
```

## 性能特性

### 文本字符串差异性能
- **小文本**: 通常在 87μs 内完成
- **中等文件** (100行): 通常在 234μs 内完成  
- **大文件** (1000+行): 通常在 5ms 内完成

### 文件系统差异性能  
- **小文件系统** (10个文件): 通常在 50ms 内完成
- **中等文件系统** (50个文件): 通常在 100ms 内完成
- **大文件系统** (100个文件): 通常在 200ms 内完成
- **复杂结构**: 支持嵌套目录和大量文件

### 通用特性
- **并发安全**: 支持多个goroutine同时使用
- **内存优化**: 基于Git的增量处理
- **格式标准**: 生成标准unified diff格式

## 高级用法

### 文本diff上下文取消

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

diff, err := yakdiff.DiffToStringContext(ctx, largeText1, largeText2)
if err != nil {
    // 处理超时或取消
}
```

### 文件系统diff上下文取消

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

diff, err := yakdiff.FileSystemDiffToStringContext(ctx, largeFS1, largeFS2)
if err != nil {
    // 处理超时或取消
}
```

### 错误处理

#### 文本diff错误处理

```go
diff, err := yakdiff.Diff(data1, data2)
if err != nil {
    log.Printf("Text diff failed: %v", err)
    return
}

if strings.TrimSpace(diff) == "" {
    log.Println("No text differences found")
} else {
    log.Printf("Found text differences:\n%s", diff)
}
```

#### 文件系统diff错误处理

```go
diff, err := yakdiff.FileSystemDiff(fs1, fs2)
if err != nil {
    log.Printf("FileSystem diff failed: %v", err)
    return
}

if strings.TrimSpace(diff) == "" || strings.Contains(diff, ".gitkeep") {
    log.Println("No filesystem differences found")
} else {
    log.Printf("Found filesystem differences:\n%s", diff)
    
    // 分析变化类型
    if strings.Contains(diff, "new file") {
        log.Println("- Contains new files")
    }
    if strings.Contains(diff, "deleted file") {
        log.Println("- Contains deleted files")
    }
    if strings.Contains(diff, "index ") && !strings.Contains(diff, "new file") && !strings.Contains(diff, "deleted file") {
        log.Println("- Contains modified files")
    }
}
```

## 注意事项

### 文本字符串差异注意事项

1. **内存使用**: 大文件比较时会消耗相应内存
2. **数据类型**: 输入会自动转换为字节数组进行比较
3. **Git格式**: 输出遵循标准的Git diff格式
4. **并发安全**: 可以在多个goroutine中安全使用

### 文件系统差异注意事项

1. **文件系统接口**: 需要实现 `fi.FileSystem` 接口
2. **虚拟文件系统**: 推荐使用 `filesys.NewVirtualFs()` 创建测试文件系统
3. **空文件系统**: 空文件系统可能包含 `.gitkeep` 文件的差异信息
4. **文件删除检测**: 使用正确的Git工作流确保删除操作被检测
5. **目录结构**: 支持嵌套目录，自动处理目录创建和删除
6. **性能考虑**: 大型文件系统比较需要更多时间和内存
7. **路径格式**: 文件路径使用Unix风格的正斜杠分隔符

## 测试覆盖

YakDiff 包含了全面的测试套件：

### 文本字符串差异测试
- ✅ 基础字符串比较测试
- ✅ 边界情况测试（空字符串、相同内容等）
- ✅ 多行文本测试
- ✅ 二进制数据测试
- ✅ 性能基准测试
- ✅ 并发安全测试
- ✅ 错误处理测试
- ✅ 特殊字符处理测试
- ✅ 大文件处理测试
- ✅ 数据类型转换测试
- ✅ 上下文取消测试

### 文件系统差异测试
- ✅ 基础文件系统比较测试
- ✅ 文件新增、修改、删除检测
- ✅ 目录结构变化检测
- ✅ 嵌套文件和目录测试
- ✅ 空文件系统处理测试
- ✅ 大型文件系统性能测试
- ✅ 自定义处理器测试
- ✅ 上下文取消测试
- ✅ 并发安全测试
- ✅ 错误处理测试

运行所有测试：
```bash
go test ./common/utils/yakgit/yakdiff/ -v
```

运行文本diff测试：
```bash
go test ./common/utils/yakgit/yakdiff/ -run "TestBasic|TestEdge|TestMulti|TestBinary|TestPerformance" -v
```

运行文件系统diff测试：
```bash
go test ./common/utils/yakgit/yakdiff/ -run "TestFileSystemDiff" -v
```

运行基准测试：
```bash
go test ./common/utils/yakgit/yakdiff/ -bench=. -benchtime=5s
```

运行并发测试：
```bash
go test ./common/utils/yakgit/yakdiff/ -run "TestConcurrency" -v -count=10
```

## 贡献

欢迎提交 issue 和 pull request 来改进这个模块。在提交代码前，请确保：

1. 所有测试通过
2. 新功能包含相应测试
3. 代码遵循项目规范
4. 更新相关文档

## 许可证

本项目遵循 Yaklang 项目的许可证。
