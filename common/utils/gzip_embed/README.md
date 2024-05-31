# 使用说明

## 0. 简介
此工具用于解决embed明文打包的问题，可以将静态资源文件压缩后打包到二进制文件中，运行时自动解压文件。

## 1. 安装

需要安装项目中的`gzip-embed`工具，可以使用以下命令安装：
```bash
go build -ldflags="-s -w" -o ~/.local/bin common/utils/gzip_embed/cmd/gzip-embed.go && chmod +x ~/.local/bin/gzip-embed
```

## 2. 使用

`gzip-embed`工具会自动读取工作目录下的static目录，将其中的文件进行压缩后生成static.tar.gz文件，然后将static.tar.gz文件，并生成embed.go文件。embed.go文件中定义了变量FS，可以读取并自动解压文件。

### 2.1 编写init.go

避免每次生成压缩文件时都要手动执行gzip-embed工具，可以在static的同级目录中编写init.go文件，添加generate指令，内容如下：
```go
package xxx

//go:generate gzip-embed -cache
```
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