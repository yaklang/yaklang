# common/utils/subprocess

统一的子进程管理包，提供进程生命周期管理、平台进程组配置、环境变量清理和 stdio 配置。

## 快速开始

### 基本用法

```go
package main

import (
    "context"
    "os/exec"
    "github.com/yaklang/yaklang/common/utils/subprocess"
)

func main() {
    p, err := subprocess.Launch(context.Background(), &subprocess.LaunchConfig{
        Cmd: exec.Command("echo", "hello"),
    })
    if err != nil {
        panic(err)
    }
    <-p.Done() // 等待进程退出
}
```

### 配置 stdio 输出去向

```go
var buf strings.Builder

p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd: exec.Command("my-program"),
    Stdio: subprocess.StdioConfig{
        Stdout: &buf,  // 子进程 stdout 写入 buf
        Stderr: &buf,  // 子进程 stderr 写入 buf
    },
})
<-p.Done()
fmt.Println(buf.String())
```

### 通过 tap reader 读取子进程输出

```go
p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd: exec.Command("my-program"),
    // 不设 Stdout writer → 默认走 pipe，Stdout() 返回 tap reader
})

// 拿到 tap reader，读出来的数据同时 copy 到 StdioConfig.Stdout（如果配了的话）
r := p.Stdout()
if r != nil {
    go func() {
        scanner := bufio.NewScanner(r)
        for scanner.Scan() {
            fmt.Println(scanner.Text())
        }
    }()
}
<-p.Done()
```

### 同时配置 writer 和 tap reader

```go
var fileBuf bytes.Buffer

p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd: exec.Command("my-program"),
    Stdio: subprocess.StdioConfig{
        Stdout: &fileBuf, // 数据 copy 到这里
    },
})

// 同时拿一份 copy
r := p.Stdout()
go func() {
    // r 读到的数据和 fileBuf 收到的是同一份数据
    io.Copy(os.Stderr, r)
}()

<-p.Done()
```

### 往子进程 stdin 写数据

```go
stdinReader, stdinWriter := io.Pipe()

p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd: exec.Command("cat"),
    Stdio: subprocess.StdioConfig{
        Stdin: stdinReader, // 子进程从这里读
    },
})

// 往 stdinWriter 写数据，子进程通过 stdinReader 收到
go func() {
    stdinWriter.Write([]byte("hello\n"))
    stdinWriter.Close()
}()

<-p.Done()
```

### 就绪检测

```go
p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd:            exec.Command("my-server"),
    StartupTimeout: 30 * time.Second,
    Ready: func(ctx context.Context, mp *subprocess.ManagedProcess) error {
        // 在这里检测进程是否就绪，返回 nil 表示就绪
        // 例如等待某个端口可连接、某个输出行出现等
        // 通过 mp.Stdout() 可以拿到 tap reader 来读取输出
        return nil
    },
})
```

### 优雅关闭

```go
p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd:             exec.Command("my-server"),
    ShutdownTimeout: 5 * time.Second,
    GracefulShutdown: func(mp *subprocess.ManagedProcess) error {
        // 在这里执行优雅关闭逻辑
        // 例如发 SIGTERM、写退出命令到 stdin、关闭网络连接等
        // 返回后框架等待 ShutdownTimeout，超时则强杀进程组
        return nil
    },
})

// 调用 Close 触发优雅关闭流程
p.Close()
```

### 完整示例：stdin 写退出命令 + 超时强杀

```go
stdinReader, stdinWriter := io.Pipe()

p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd: exec.Command("ffmpeg", "-i", "input.mp4", "output.mp4"),
    Stdio: subprocess.StdioConfig{
        Stdin:  stdinReader,
        Stdout: log.NewLogWriter(log.InfoLevel),
        Stderr: log.NewLogWriter(log.InfoLevel),
    },
    ShutdownTimeout: 5 * time.Second,
    GracefulShutdown: func(mp *subprocess.ManagedProcess) error {
        // 写 'q' 命令到 ffmpeg stdin 触发优雅退出
        stdinWriter.Write([]byte("q\n"))
        stdinWriter.Close()
        return nil
    },
})

// ... 使用进程 ...

// Close 会：1. 调 GracefulShutdown（写 'q'） 2. 等 5 秒 3. 超时强杀进程组
p.Close()
```

### 环境变量清理

```go
// 从父进程环境中过滤掉敏感变量，追加 worker 标识
env := subprocess.BuildChildEnvironment(
    os.Environ(),
    []string{"SECRET", "API_KEY", "WORKER_TOKEN"}, // 过滤这些（大小写不敏感）
    []string{"WORKER=1", "MODE=isolated"},          // 追加这些
)
```

### *os.File 直接继承（fd 继承）

```go
// 当 StdioConfig.Stdout 是 *os.File 时，框架直接把 fd 传给子进程
// 不创建 pipe，不启动 drain goroutine
// 此时 Stdout() 返回 nil
logFile, _ := os.OpenFile("child.log", os.O_WRONLY|os.O_CREATE, 0644)

p, _ := subprocess.Launch(ctx, &subprocess.LaunchConfig{
    Cmd: exec.Command("my-program"),
    Stdio: subprocess.StdioConfig{
        Stdout: logFile, // *os.File → fd 继承
        Stderr: logFile,
    },
})
```

## API 概览

### StdioConfig

| 字段 | 类型 | 说明 |
|---|---|---|
| `Stdin` | `io.Reader` | 子进程 stdin 来源。需要写入时传 `io.Pipe()` 的 reader 端 |
| `Stdout` | `io.Writer` | 子进程 stdout 去向。`*os.File` 走 fd 继承，其他走 pipe+drain |
| `Stderr` | `io.Writer` | 子进程 stderr 去向。同上 |

### ManagedProcess 方法

| 方法 | 说明 |
|---|---|
| `Stdout() io.Reader` | 返回 tap reader，读取子进程 stdout 的副本。`*os.File` 模式返回 nil |
| `Stderr() io.Reader` | 返回 tap reader，读取子进程 stderr 的副本。同上 |
| `Done() <-chan struct{}` | 进程退出时关闭的 channel |
| `WaitError() error` | 返回 `cmd.Wait()` 的错误，进程未退出时返回 nil |
| `PID() int` | 返回进程 PID |
| `Cmd() *exec.Cmd` | 返回底层的 `exec.Cmd` |
| `Close()` | 优雅关闭：调 GracefulShutdown → 等 ShutdownTimeout → 强杀进程组 |
| `Kill()` | 立即强杀进程组并等待退出 |

### LaunchConfig 字段

| 字段 | 类型 | 说明 |
|---|---|---|
| `Cmd` | `*exec.Cmd` | 要启动的命令，调用方设置 Path/Args/Dir |
| `EnvExclude` | `[]string` | 从子进程环境中过滤的变量名（大小写不敏感） |
| `EnvExtra` | `[]string` | 追加到子进程环境的变量 |
| `Stdio` | `StdioConfig` | stdio 配置 |
| `StartupTimeout` | `time.Duration` | 就绪检测超时，零值默认 60s |
| `ShutdownTimeout` | `time.Duration` | 优雅关闭超时，零值默认 5s |
| `Ready` | `func(ctx, *ManagedProcess) error` | 就绪检测回调，返回 nil 表示就绪 |
| `GracefulShutdown` | `func(*ManagedProcess) error` | 优雅关闭回调，在强杀前调用 |

## 平台支持

| 功能 | Unix (darwin/linux/freebsd/...) | 其他平台 (windows 等) |
|---|---|---|
| `ConfigureProcessGroup` | `Setpgid: true` | no-op |
| `KillProcessGroup` | `Kill(-pid, SIGKILL)` + fallback | `Process.Kill()` |

## 设计约束

- **禁止 `any` 类型**：stdio 配置完全通过类型化字段（`io.Reader` / `io.Writer`）和方法暴露
- **tap reader 懒创建**：`Stdout()` / `Stderr()` 在首次调用时创建 tap pipe，之前的输出不会回放
- **pipe 生命周期**：stdin pipe 由 `Close()` 关闭；stdout/stderr pipe 由 `cmd.Wait()` 内部关闭
- **drain goroutine 安全**：`Close()` / `Kill()` 会 join drain goroutine，确保所有缓冲输出已 flush

## 已接入的使用点

| 使用点 | 接入方式 |
|---|---|
| `mcp/stdio` | 平台原语（`ConfigureProcessGroup` / `KillProcessGroup` / `BuildChildEnvironment`） |
| `memfit-cli` | 完整生命周期（`Launch` + `Ready` + `GracefulShutdown` + tap reader） |
| `screcorder` | 完整生命周期（`Launch` + `GracefulShutdown` + stdin pipe） |
| `yaklib` | 平台原语（`ConfigureProcessGroup`，保留 `cmd.Cancel` override） |
