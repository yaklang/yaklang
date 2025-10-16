# Yak Script Tools for AI

这个目录包含了用于 AI 工具的 Yak 脚本资源，这些脚本被打包成 tar.gz 格式并嵌入到 Go 程序中。

## 目录结构

```
yakscriptforai/
├── README.md              # 本文件
├── init.go                # 包初始化文件，包含 go:generate 指令
├── embed.go               # 自动生成的嵌入文件
├── yakscriptforai.tar.gz  # 自动生成的压缩文件
├── amap/                  # 高德地图相关脚本
├── codec/                 # 编解码相关脚本
├── dns/                   # DNS 相关脚本
├── doc/                   # 文档处理相关脚本
├── fp/                    # 指纹识别相关脚本
├── fs/                    # 文件系统相关脚本
├── git/                   # Git 相关脚本
├── http/                  # HTTP 相关脚本
├── mock/                  # Mock 相关脚本
├── net/                   # 网络相关脚本
├── pcap/                  # 数据包分析相关脚本
├── pentest/               # 渗透测试相关脚本
├── risk/                  # 风险检测相关脚本
├── ssa/                   # SSA 相关脚本
├── system/                # 系统相关脚本
├── tls/                   # TLS 相关脚本
├── yakplugin/             # Yak 插件相关脚本
└── zip/                   # ZIP 相关脚本
```

## 构建说明

### 前置条件

确保你在项目根目录 `/Users/z3/Code/yaklang2/` 下操作。

### 步骤 1: 安装 gzip-embed 工具

首先需要安装 `gzip-embed` 命令行工具：

```bash
go install ./common/utils/gzip_embed/gzip-embed
```

这会将 `gzip-embed` 工具安装到你的 `$GOPATH/bin` 目录中。

### 步骤 2: 生成压缩文件和嵌入代码

安装完成后，执行以下命令来生成 `yakscriptforai.tar.gz` 压缩文件和 `embed.go` 嵌入代码：

```bash
go generate -run="gzip-embed" -v -x ./common/ai/aid/aitool/buildinaitools/yakscripttools/...
```

或者使用更精确的匹配：

```bash
go generate -run="^gzip-embed" -v -x ./common/ai/aid/aitool/buildinaitools/yakscripttools/...
```

### 步骤 3: 验证生成结果

生成完成后，应该会看到以下文件：
- `yakscriptforai.tar.gz` - 压缩后的脚本文件
- `embed.go` - 自动生成的 Go 嵌入代码

## 工作原理

1. **gzip-embed** 工具会：
   - 扫描当前目录下的所有文件（排除 `.tar.gz` 文件）
   - 将它们打包成 tar 格式
   - 使用 gzip 压缩
   - 生成 `embed.go` 文件，其中包含 `//go:embed` 指令

2. **embed.go** 文件会：
   - 使用 Go 的 `embed.FS` 嵌入 tar.gz 文件
   - 提供 `FS` 变量，类型为 `*gzip_embed.PreprocessingEmbed`
   - 在 `init()` 函数中初始化文件系统并启用缓存

3. **缓存机制**：
   - `PreprocessingEmbed` 实现了 `fi.FileSystem` 接口
   - 启用缓存后，所有文件在初始化时会被解压并缓存到内存
   - 提供 `GetHash()` 方法用于检测文件是否有变动

## 使用示例

在代码中使用嵌入的文件系统：

```go
import "github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"

// 读取文件
content, err := yakscripttools.FS.ReadFile("amap/get_location.yak")
if err != nil {
    log.Fatal(err)
}

// 列出目录
entries, err := yakscripttools.FS.ReadDir("amap")
if err != nil {
    log.Fatal(err)
}

// 获取文件哈希（用于检测变动）
hash, err := yakscripttools.FS.GetHash()
if err != nil {
    log.Fatal(err)
}
```

## 添加新脚本

如果需要添加新的脚本文件：

1. 在对应的目录下创建或修改 `.yak` 文件
2. 重新运行 `go generate` 命令（参见步骤 2）
3. 重新编译项目

## 注意事项

- 所有的 `.tar.gz` 文件会被自动排除，不会被重复打包
- 生成的文件系统是只读的
- 缓存在程序启动时创建，之后读取性能极高
- `GetHash()` 方法的结果会被缓存，除非调用 `InvalidateHash()`

## 故障排查

### gzip-embed 命令找不到

确保 `$GOPATH/bin` 在你的 `PATH` 环境变量中：

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```
