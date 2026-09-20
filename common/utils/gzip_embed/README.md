# 使用说明

## 0. 简介
此工具用于解决embed明文打包的问题，可以将静态资源文件压缩后打包到二进制文件中，运行时自动解压文件。

## 1. 安装

需要安装项目中的`gzip-embed`工具，可以使用以下命令安装：
```bash
go install ./common/utils/gzip_embed/gzip-embed
```

## 2. 使用

`gzip-embed`工具会自动读取工作目录下的static目录，将其中的文件进行压缩后生成static.tar.gz和embed.go文件。embed.go文件中定义了变量FS，可以读取并自动解压文件。

### 2.1 编写init.go

避免每次生成压缩文件时都要手动执行gzip-embed工具，可以在static的同级目录中编写init.go文件，添加generate指令，内容如下：
```go
package xxx

//go:generate gzip-embed -cache
```

默认启用懒加载和驻留缓存，`-cache` 可以省略。构造时不读取、解码或解压归档；首次 `ReadFile`、`Open`、`Stat`、`ReadDir` 或 `GetHash` 时解码并完整解压一次，然后驻留文件内容与目录元数据。并发首次访问共享同一次加载，加载失败也会保留错误，避免重复消耗资源。显式 `--cache=false` 才使用每次访问重新扫描的非驻留模式。

默认使用共享 XOR key 对压缩字节做混淆，不增加归档长度；只有首次加载时才执行 XOR。旧的普通 gzip 归档仍兼容。`--xor-key=自定义值` 保留独立 key，`--xor-key=` 关闭混淆。XOR 是资源混淆，不是保密加密。

### 2.2 生成压缩文件

在static目录的同级目录下执行以下命令：
```bash
go generate .
```
也可以在项目根目录下执行以下命令：
```bash
go generate ./...
```
执行后会自动在init.go同级生成embed.go和static.tar.gz文件。

### 2.3 读取文件

在代码中使用embed.go中的FS变量读取文件，示例如下：
```go
package main

import (
    "fmt"
    "log"
    "xxx"
)

func main() {
    data, err := xxx.FS.ReadFile("test.txt")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(string(data))
}
```
具体案例可以参考测试文件：common/utils/gzip_embed/test/fs_test.go

## 本地开发与发布构建

沿用现有 `!gzip_embed` / `gzip_embed` 双入口：普通本地构建直接嵌入原始目录或文件，修改资源后重新编译即可，不需要手工生成或提交压缩副本。发布前使用本工具重新打包，再加 `gzip_embed` 标签构建。解压后的路径和文件内容保持一致，`Asset` 对已有 `.gz` / `.gzip` 的解压语义不变。

公共数据、MITM 静态资源、二进制配置、标准映射和 full 规则版本的发布准备入口：

```bash
GZIP_EMBED="$(bash scripts/build-gzip-embed.sh)"
export GZIP_EMBED
bash scripts/generate-compressed-assets.sh
```

该脚本已接入 `reuse-build.yml`、`exp-cross-build.yml` 和本地发布安装脚本；继续使用各入口原有的其他资源压缩步骤。新增归档均在 `.gitignore` 中，不提交到仓库。full 和 slim 的发布均使用压缩公共资源；slim 仍排除 full 规则版本。CI 使用独立的 Linux / Windows 任务验证普通构建与发布构建的资源内容一致性和并发缓存行为，不等待引擎准备任务。发布脚本每次从当前检出编译一次工具，整批归档复用同一工具；支持 Git Bash / Windows `.exe` 和含空格路径，避免误用 PATH 中的旧工具。Windows ARM64 发布沿用现有原生 runner 和 llvm-mingw 配置。

本工具使用 gzip level 9 并清除 tar 中的时间戳及 UID/GID，同样的输入可重复生成；也支持以 `--source` 指定单个文件。生成失败会返回非零退出码，避免 CI 继续使用旧归档。`PreprocessingEmbed` 支持目录打开、排序遍历及 `Seek`/HTTP Range；新增资源使用默认懒加载驻留模式。驻留以整个归档为单位，不是按单个文件加载；首次访问后持有整个归档的解压内容。`Open` 使用独立游标直接读取不可变缓存，`ReadFile` 返回副本以兼容调用方修改返回值的行为。
