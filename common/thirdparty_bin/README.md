# 第三方二进制工具配置说明

本文档详细介绍如何在 `bin_cfg.yml` 配置文件中添加和配置第三方二进制工具。

## 配置文件结构

配置文件采用 YAML 格式，包含以下主要部分：

```yaml
version: "1.0"
description: "Third-party binary tools configuration"

binaries:
  - name: "tool-name"
    description: "工具描述"
    version: "latest"
    install_type: "bin|archive"
    download_info_map:
      # 平台特定的下载信息
```

## 基本配置字段

### 顶层字段

- `version`: 配置文件版本，固定为 "1.0"
- `description`: 配置文件描述信息
- `binaries`: 二进制工具列表

### 二进制工具配置字段

每个二进制工具包含以下字段：

| 字段名 | 类型 | 必需 | 描述 |
|--------|------|------|------|
| `name` | string | ✅ | 工具名称，作为唯一标识符 |
| `description` | string | ✅ | 工具描述信息 |
| `version` | string | ✅ | 工具版本，通常使用 "latest" |
| `install_type` | string | ✅ | 安装类型：`bin` 或 `archive` |
| `download_info_map` | map | ✅ | 平台下载信息映射 |
| `archive_type` | string | ❌ | 压缩包类型（仅当 install_type 为 archive 时） |
| `dependencies` | []string | ❌ | 依赖的其他二进制工具 |

## 安装类型说明

### 1. `bin` 类型

用于直接可执行的二进制文件，不需要解压，一般download_info_map中只需要配置url、bin_path即可, bin_path可以是带文件夹的路径，管理器会自动创建。

**配置示例：**
```yaml
- name: "vulinbox"
  description: "Yaklang Vulnerability Testing Box"
  version: "latest"
  install_type: "bin"
  download_info_map:
    linux-amd64:
      url: "https://example.com/vulinbox_linux_amd64"
      bin_path: "vulinbox/vulinbox"
      sha256: "a1b2c3d4e5f6..."
    windows-amd64:
      url: "https://example.com/vulinbox_windows_amd64.exe"
      bin_path: "vulinbox.exe"
      md5: "1234567890abcdef..."
```

**字段说明：**
- `url`: 下载链接
- `bin_path`: 安装后的二进制文件路径（相对于安装目录）
- `md5`: 可选，文件MD5校验和
- `sha256`: 可选，文件SHA256校验和

### 2. `archive` 类型

用于需要解压的压缩包文件。

需要注意，如果下载url中不包含文件名和文件后缀，管理器无法识别压缩包类型，需要手动指定archive_type。

**配置示例：**
```yaml
- name: "llama-server"
  description: "Llama Server"
  version: "latest"
  install_type: "archive"
  download_info_map:
    darwin-amd64:
      url: "https://example.com/llama-server.zip"
      pick: "build/bin/llama-server"
      bin_dir: "llama-server"
      bin_path: "llama-server/llama-server"
```

**字段说明：**
- `url`: 压缩包下载链接
- `pick`: 从压缩包中提取的文件/目录路径
- `bin_dir`: 解压到的目录名
- `bin_path`: 最终二进制文件的路径

**bin_dir 使用注意事项：**
- 当 `pick` 参数提取多个文件时，通常需要指定 `bin_dir` 作为解压目录
- ⚠️ **卸载警告**：卸载工具时会删除整个 `bin_dir` 目录
- 如果多个工具安装在同一目录下，请避免使用 `bin_dir`，改为在 `bin_path` 中指定具体的子目录路径
- 推荐为每个工具使用独立的 `bin_dir` 目录，避免意外删除其他工具文件

**重要约束：**
- `pick` 和 `bin_dir` 必须同时存在或同时不存在
- 如果使用 `pick` 和 `bin_dir`，则必须同时配置 `bin_path`

## 平台匹配规则

### 平台标识符格式

平台标识符格式为：`操作系统-架构`

**支持的操作系统：**
- `linux`: Linux 系统
- `darwin`: macOS 系统
- `windows`: Windows 系统

**支持的架构：**
- `amd64`: 64位 x86 架构
- `arm64`: 64位 ARM 架构

### 匹配优先级

1. **精确匹配**：优先匹配完全相同的平台标识符
2. **Glob 模式匹配**：支持通配符模式

**Glob 模式示例：**
```yaml
download_info_map:
  linux-amd64:           # 精确匹配 Linux AMD64
    url: "https://example.com/linux-amd64"
  "darwin-*":            # 匹配所有 macOS 平台
    url: "https://example.com/darwin"
  "windows-*":           # 匹配所有 Windows 平台
    url: "https://example.com/windows"
  "*-arm64":             # 匹配所有 ARM64 架构
    url: "https://example.com/arm64"
  "*":                   # 匹配所有平台（通用）
    url: "https://example.com/universal"
```

## 文件校验和配置

系统支持两种校验和算法来验证下载文件的完整性：

### 支持的校验和类型

- **MD5**: 128位哈希值，格式为32位十六进制字符串
- **SHA256**: 256位哈希值，格式为64位十六进制字符串

### 配置方式

可以为每个平台的下载文件单独配置校验和：

```yaml
download_info_map:
  linux-amd64:
    url: "https://example.com/tool-linux"
    bin_path: "tool"
    sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
  windows-amd64:
    url: "https://example.com/tool-windows.exe"
    bin_path: "tool.exe"
    md5: "d41d8cd98f00b204e9800998ecf8427e"
  darwin-amd64:
    url: "https://example.com/tool-darwin"
    bin_path: "tool"
    # 可以同时配置两种校验和
    md5: "d41d8cd98f00b204e9800998ecf8427e"
    sha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
```

### 校验行为

- 如果配置了校验和，系统会在下载完成后自动验证文件完整性
- 如果同时配置了MD5和SHA256，两个校验和都会被验证
- 如果没有配置校验和，跳过验证步骤
- 校验失败会导致安装过程中断并报错

## Pick 参数详解

`pick` 参数用于指定从压缩包中提取的内容，支持多种模式：

### 1. 全部提取
```yaml
pick: "*"
```
提取压缩包中的所有文件到目标目录。

### 2. 目录内容提取
```yaml
pick: "build/*"
```
提取 `build` 目录下的所有内容到目标目录，但不包含 `build` 目录本身。

### 3. 目录提取
```yaml
pick: "build/"
```
提取整个 `build` 目录到目标目录。

### 4. 单文件提取
```yaml
pick: "build/bin/tool"
```
提取指定的单个文件。

## 支持的压缩格式

系统自动根据文件扩展名识别压缩格式：

- `.zip`: ZIP 压缩包
- `.tar.gz`: Tar + Gzip 压缩包
- `.gz`: Gzip 压缩文件
- `.tar`: Tar 归档文件

如果无法自动识别，可以通过 `archive_type` 字段手动指定：

```yaml
archive_type: ".zip"  # 强制指定为 ZIP 格式
```

## 完整配置示例

### 1. 简单二进制文件
```yaml
- name: "page2image"
  description: "Page2Image - A tool for converting web pages to images"
  version: "latest"
  install_type: "bin"
  download_info_map:
    "darwin-amd64":
      url: "https://oss-qn.yaklang.com/page2img/page2img-darwin-amd64"
      bin_path: "page2img"
    "windows-amd64":
      url: "https://oss-qn.yaklang.com/page2img/page2img-windows-amd64.exe"
      bin_path: "page2img.exe"
    "linux-amd64":
      url: "https://oss-qn.yaklang.com/page2img/page2img-linux-amd64"
      bin_path: "page2img"
```

### 2. 压缩包类型
```yaml
- name: "whisper.cpp"
  description: "Whisper.cpp - A fast and accurate speech-to-text model"
  version: "latest"
  install_type: "archive"
  download_info_map:
    "darwin-amd64":
      url: "https://oss-qn.yaklang.com/whisper.cpp/whisper.cpp-macos-amd64.zip"
      pick: "*"
      bin_dir: "whisper.cpp"
      bin_path: "whisper.cpp/whisper-cli"
    "windows-amd64":
      url: "https://oss-qn.yaklang.com/whisper.cpp/whisper.cpp-windows-amd64.zip"
      pick: "*"
      bin_dir: "whisper.cpp"
      bin_path: "whisper.cpp/whisper-cli.exe"
```

### 3. AI 模型文件
```yaml
- name: "model-whisper-medium-q5"
  description: "Whisper Medium Q5 model"
  version: "latest"
  install_type: "bin"
  download_info_map:
    "*":  # 通用模型，所有平台都使用相同文件
      url: "https://oss-qn.yaklang.com/gguf/whisper-medium-q5.gguf"
      bin_path: "aimodel/whisper-medium-q5.gguf"
```

### 4. 带依赖的工具
```yaml
- name: "complex-tool"
  description: "A complex tool with dependencies"
  version: "latest"
  install_type: "archive"
  dependencies: ["llama-server", "ffmpeg"]  # 依赖其他工具
  download_info_map:
    "linux-amd64":
      url: "https://example.com/complex-tool-linux.tar.gz"
      pick: "bin/*"
      bin_dir: "complex-tool"
      bin_path: "complex-tool/complex-tool"
```

## 最佳实践

### 1. 命名规范
- 工具名称使用小写字母和连字符
- 模型文件使用 `model-` 前缀
- 版本信息包含在名称中（如果有多个版本）

### 2. 平台支持
- 优先支持主流平台：`linux-amd64`、`darwin-amd64`、`windows-amd64`
- 对于 macOS，考虑使用 `darwin-*` 通配符支持多架构
- 对于通用文件（如模型），使用 `*` 通配符

### 3. 路径规范
- 二进制文件直接放在根目录或相应的子目录
- AI 模型统一放在 `aimodel/` 目录下
- 复杂工具使用独立的子目录

### 4. URL 管理
- 使用稳定的下载链接
- 建议使用版本化的 URL 路径
- 确保下载链接的可访问性

### 5. 错误处理
- 提供详细的工具描述
- 确保 `pick` 和 `bin_dir` 的一致性
- 验证压缩包格式和内容结构

## 常见错误及解决方案

### 1. pick 和 bin_dir 不一致
```
错误：pick and bin_dir must both be present or both be absent
解决：确保 pick 和 bin_dir 同时存在或同时省略
```

### 2. 平台不匹配
```
错误：no download info for platform linux-arm64
解决：添加对应平台的配置或使用通配符模式
```

### 3. 文件提取失败
```
错误：file not found in archive
解决：检查 pick 参数是否正确，验证压缩包内容
```

### 4. 不支持的压缩格式
```
错误：unsupported archive format
解决：使用支持的格式或通过 archive_type 手动指定
```

## 配置验证

在添加新配置后，建议进行以下验证：

1. **YAML 语法检查**：确保配置文件语法正确
2. **字段完整性**：验证必需字段都已填写
3. **URL 可访问性**：测试下载链接是否有效
4. **平台兼容性**：在目标平台上测试安装
5. **依赖关系**：验证依赖工具的可用性

通过遵循本文档的规范，可以确保第三方二进制工具的正确配置和稳定运行。 