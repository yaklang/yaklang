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
***注意***：`-cache`参数表示生成的embed.go文件中会缓存文件内容，如果文件较大，可以不加此参数，每次读取文件时都会重新解压。
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
go install ./common/utils/gzip_embed/gzip-embed
bash scripts/generate-compressed-assets.sh
```

该脚本已接入 `reuse-build.yml`、`exp-cross-build.yml` 和本地发布安装脚本；继续使用各入口原有的其他资源压缩步骤。新增归档均在 `.gitignore` 中，不提交到仓库。full 和 slim 的发布均使用压缩公共资源；slim 仍排除 full 规则版本。CI 分别验证普通构建与发布构建的资源内容一致性。

本工具使用 gzip level 9 并清除 tar 中的时间戳及 UID/GID，同样的输入可重复生成；也支持以 `--source` 指定单个文件。生成失败会返回非零退出码，避免 CI 继续使用旧归档。`PreprocessingEmbed` 支持目录打开、排序遍历及 `Seek`/HTTP Range；新增资源使用非缓存模式按需读取，不在初始化时展开全部归档。此模式降低常驻解压内存，但读取文件时需要扫描归档。
