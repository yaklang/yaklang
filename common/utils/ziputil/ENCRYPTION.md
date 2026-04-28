# ZIP 加密支持（密码 / AES）

## 概述

本文档介绍 `common/utils/ziputil`、`common/utils/filesys.ZipFS` 与 `common/yak/yaklib/zip` 在带密码 zip 上的能力。

底层实现位于 `common/utils/zipx`（hard fork 自 [yeka/zip](https://github.com/yeka/zip)），保持与 Go 标准库 `archive/zip` 完全兼容的 API，并叠加：

- 读：`(*File).IsEncrypted()`、`(*File).SetPassword(string)`
- 写：`(*Writer).Encrypt(name, password string, method EncryptionMethod)`

## 加密算法

| 常量 | 含义 | 何时使用 |
|------|------|----------|
| `ziputil.StandardEncryption` | PKWARE / ZipCrypto | 仅与系统自带 `zip/unzip` 工具兼容时使用，已知不安全 |
| `ziputil.AES128Encryption` | WinZip AES-128 | |
| `ziputil.AES192Encryption` | WinZip AES-192 | |
| `ziputil.AES256Encryption` | WinZip AES-256 | **推荐默认值**，需要 `7z` / 较新的 `unzip` |

## 增量 API（向后兼容）

所有原始公开签名保持不变。新增以 `*WithOptions` 后缀的入口接受 Option：

### 解压

```go
// 文件 -> 目录
err := ziputil.DeCompressWithOptions(
    "encrypted.zip", "/tmp/out",
    ziputil.WithDecompressPassword("p@ss"),
)

// 字节 -> 目录（HTTP 下载场景）
err := ziputil.DeCompressFromRawWithOptions(zipBytes, "/tmp/out",
    ziputil.WithDecompressPassword("p@ss"),
)
```

### 提取

```go
content, err := ziputil.ExtractFileFromRawWithOptions(zipBytes, "config.ini",
    ziputil.WithExtractPassword("p@ss"),
)

results, err := ziputil.ExtractByPatternFromRawWithOptions(zipBytes, "*.txt",
    ziputil.WithExtractPassword("p@ss"),
)
```

### Grep

```go
results, err := ziputil.GrepRawSubString(zipBytes, "needle",
    ziputil.WithGrepPassword("p@ss"),
)

searcher, _ := ziputil.NewZipGrepSearcherFromRaw(zipBytes)
searcher.SetPassword("p@ss")
results, _ := searcher.GrepRegexp(`secret`)
```

### 压缩

```go
err := ziputil.CompressByNameWithOptions(
    []string{"a.txt", "b.txt"}, "out.zip",
    ziputil.WithCompressPassword("p@ss"),
    ziputil.WithCompressEncryption(ziputil.AES256Encryption), // 默认 AES256
)

mem, err := ziputil.CompressRawMapWithOptions(map[string]interface{}{
    "a.txt": "hello",
    "b.txt": []byte("world"),
}, ziputil.WithCompressPassword("p@ss"))
```

## ZipFS 加密读

`filesys.ZipFS` 现在可以挂载到带密码的 zip：

```go
zfs, err := filesys.NewZipFSFromLocalWithOptions("encrypted.zip",
    filesys.WithZipFSPassword("p@ss"),
)
defer zfs.Close()

data, _ := zfs.ReadFile("config.ini")
entries, _ := zfs.ReadDir("docs")
```

也可以延后设置密码：`zfs.SetPassword("p@ss")`。

## yak DSL 入口

通过 `zip` 模块直接使用：

```yak
// 简易：明文 API
zip.DecompressWithPassword("/tmp/in.zip", "/tmp/out", "p@ss")~
zip.CompressWithPassword("/tmp/out.zip", "p@ss", "/tmp/a.txt", "/tmp/b.txt")~
data = zip.CompressRawWithPassword({"a.txt": "hello"}, "p@ss")~

// 选项 API
zip.DecompressWithOptions("/tmp/in.zip", "/tmp/out",
    zip.decompressPassword("p@ss"),
)~

zip.CompressByNameWithOptions(["/tmp/a.txt"], "/tmp/out.zip",
    zip.compressPassword("p@ss"),
    zip.compressEncryption(zip.AES256),
)~

content = zip.ExtractFileFromRawWithOptions(raw, "config.ini",
    zip.extractPassword("p@ss"),
)~

results = zip.GrepRawSubString(raw, "marker", zip.grepPassword("p@ss"))~
```

加密常量：`zip.StandardEncryption`、`zip.AES128`、`zip.AES192`、`zip.AES256`。

## 行为约束

- **未加密 zip + 任意密码**：调用 `*WithOptions` 携带密码读取未加密 zip，密码会被忽略，行为与旧 API 完全一致（向后兼容）。
- **加密 zip + 缺失密码**：返回明确错误，错误信息包含 `encrypted but no password supplied`。
- **加密 zip + 错密码**：底层返回 `zip: invalid password`，调用方应捕获 error。
- **错密码不会破坏其它条目**：每个文件 `SetPassword` 是 per-entry 的，错误仅影响该条目。

## 并发约束

`zipx`（即 yeka/zip fork）的 `*zip.File.SetPassword` 是 per-instance 状态：

- 同一个 `*zip.File` 实例不能同时被两个 goroutine 用不同密码并发打开。
- 当前代码（`grep.go`、`extract_options.go`）对 `Reader.File` 切片做 fan-out 时，每个 goroutine 拿到的是不同的 `*zip.File`，因此 `SetPassword` + `Open` 是安全的。
- `ZipGrepSearcher` 缓存的是已读取出的字节，不会重复 `Open` 同一个条目，安全。

如果调用方自己持有 `*Reader.File` 并发执行解密，**必须保证不同 goroutine 之间不共享同一个 `*zip.File` 指针**。

## 互联网下载安装场景

典型流程：

```go
// 1. HTTP 下载
resp, _ := http.Get("https://example.com/pkg.zip")
defer resp.Body.Close()
buf := new(bytes.Buffer)
buf.ReadFrom(resp.Body)

// 2a. 走 ziputil 直接解压到目录
ziputil.DeCompressFromRawWithOptions(buf.Bytes(), "/opt/install",
    ziputil.WithDecompressPassword("p@ss"))

// 2b. 或者走 thirdparty_bin 的 install 入口
os.WriteFile("/tmp/pkg.zip", buf.Bytes(), 0o644)
thirdparty_bin.ExtractFileWithPassword(
    "/tmp/pkg.zip", "/opt/install", ".zip", "build/*", true, "p@ss",
)
```
