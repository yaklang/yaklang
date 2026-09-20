# 全引擎二进制成分与 slim 减重实测（2026-09-20）

## 结论与版本

测量整个 Yak 可执行文件，覆盖源码子系统、第三方模块、机器码、运行时元数据、静态数据及嵌入资源。

- main 基线：`ab520b4aac60b907285872d304fb9563e68e9db2`。
- 协议 PR #5013：`44085c5025369d7056a9e007ca0d4aa5336ad7b6`。
- 优化版：本 PR 的生产代码变更；报告/审计工具本身不进入引擎。
- Go 1.22.12 / darwin arm64 / CGO_ENABLED=1，`-trimpath -ldflags '-s -w'`。
- 全量 tag 为 `gzip_embed`；slim 为 `gzip_embed,irify_exclude`。没有使用会另行排除 Oracle 的旧 `yakslim` tag。
- 两版用相同生成的 gzip 资源。它们取自基线/协议分支共同的资源源码；保留协议分支已有规则和文档差异。无发布版本注入、无发行签名；这是本机开发构建实测，不是各平台官方安装包承诺。

| 产物 | 字节数 | MiB |
|---|---:|---:|
| main 全量 | 217,527,138 | 207.450 |
| 协议 PR 全量 | 221,494,242 | 211.233 |
| main 原有 slim | 160,071,666 | 152.656 |
| 协议 PR 原有 slim | 164,088,162 | 156.487 |
| 本次清理后的 slim | 162,466,466 | 154.940 |

现有 slim 开关本身已经让协议分支减少 **54.747 MiB**；本次新改动在已有 slim 基础上再减少 **1.547 MiB**。最终相对协议分支全量减少 **56.293 MiB（26.65%）**。不能把已有 slim 的收益全部算成本 PR 新实现。

原协议 PR 全量增量为 **3.783 MiB**。同样使用 slim 参数，协议 PR 原增量为 **3.830 MiB**，清理后仍比 main 的 slim 大 **2.284 MiB**。因此本次没有在“相同 slim 功能基线”下完全抵消协议扩展的成本；全量与 slim 的 56 MiB 级差异包含明确的功能裁剪。

对上述二进制额外做 gzip level 6、mtime=0 的分发压缩实验（不等同于正式安装包）：

| 产物 | gzip MiB |
|---|---:|
| main 原有 slim | 52.769 |
| 协议 PR 原有 slim | 54.406 |
| 本次清理后的 slim | 53.947 |

全量版也重新构建通过，产物为 221,527,186 B（211.265 MiB），比协议 head 多 32.17 KiB；本 PR 的减重作用在 slim，未声称让全量版变小。

## 此次改动与功能边界

1. 在 `irify_exclude` 下不再链接/注册 `SSACompilerCommands`：移除编译、扫描、规则维护等全量版 CLI 命令及其实现。
2. 去掉 slim 中 `scannode` 仅为初始化副作用导入的 `ssa_compile` hook。该 hook 曾通过 `ssa_compile → syntaxflow_scan` 将完整编译/扫描调度链带回 slim；全量版 hook 保持原样。
3. slim 不再嵌入约 250 KB 的 `sfdb/rule_versions.json`，因为对应内置规则语料已由既有 slim 开关排除。共享版本查询 API 仍存在，缺失版本返回错误。
4. 增加 ELF/Mach-O 全二进制审计工具和 CI 依赖边界检查，阻止大型 SSA 前端和编译调度链重新进入 slim。
5. 删除旧注释里无法由当前构建复现的大小、启动和内存百分比，明确保留的内部依赖。

排除的是 C/C++、C#、Go、Java、PHP、Python、TypeScript/JS 的 SSA builder/前端，以及全量编译/扫描调度包。TypeScript 前端是独立实现，不能把全部收益都归因于 ANTLR。

仍保留 Yak VM/ANTLR parser、Yak SSA、静态检查与补全依赖，以及共享的部分 SyntaxFlow internals；因此这不是“移除所有 ANTLR 或所有 SSA”。C 预处理器还用于格式化 RPC，不导入 C ANTLR 编译前端。按既有 slim 设计，SSA/SyntaxFlow 对外相关 RPC 和库能力降级，NASL 库也使用 stub。本次没有通过删 Oracle、Excel、JS、抓包等其他正常能力换体积。

## 1. 整个文件的节组成

此表分区互斥，总和等于文件大小；MiB 四舍五入会有尾差。

| 成分 | 协议分支全量 MiB | 清理后 slim MiB | 减少 MiB |
|---|---:|---:|---:|
| 机器码（含 native C/cgo 与对齐） | 82.663 | 61.791 | 20.872 |
| Go runtime pclntab | 68.031 | 44.441 | 23.590 |
| TEXT 只读数据（字面量、静态表、部分资源） | 28.597 | 23.553 | 5.044 |
| DATA_CONST 只读数据（含类型描述等） | 21.064 | 16.257 | 4.806 |
| 已初始化可写数据 | 8.125 | 6.836 | 1.289 |
| 其他节、头部、对齐、LINKEDIT | 2.754 | 2.063 | 0.691 |

Go 的 `pclntab` 仍有很大占比，因为 `-s -w` 移除的是调试/符号信息，不会删除运行时所需的函数和 PC 数据。不能简单将这部分删掉或压缩而不改变装载器/运行时。移除不可用功能的调用依赖，能同时减少代码、类型和这些元数据。

| pclntab 内部组成（已包含在上表，不能再次相加） | 全量 MiB | slim MiB |
|---|---:|---:|
| `header` | 0.000 | 0.000 |
| `function_names` | 26.022 | 15.370 |
| `compilation_units` | 0.452 | 0.427 |
| `file_names` | 0.448 | 0.427 |
| `pc_data` | 13.673 | 11.424 |
| `function_records` | 27.435 | 16.794 |

`function_records` 包含函数查找/记录及关联数据，`pc_data` 包含运行时 PC 表；不应把它们全部描述为无用的栈追踪字符串。

## 2. 代码子系统排名

以下是运行时函数起止 PC 的机器码跨度，包含函数对齐；**不是包的完整链接体积**。只读数据、共享类型、嵌入资源与 pclntab 不分摊到包。三种代码视图（子系统、包、模块）也是同一批字节，不能相加。

| 子系统 | 全量代码 MiB | slim 代码 MiB | 链接函数数：全量 → slim |
|---|---:|---:|---:|
| `[third-party]` | 19.334 | 19.331 | 54,991 → 54,979 |
| `common/yak/typescript` | 7.026 | 0.000 | 51,341 → 0 |
| `common/ai` | 6.366 | 6.366 | 15,677 → 15,677 |
| `[Go standard library/runtime]` | 5.404 | 5.404 | 14,004 → 14,002 |
| `common/utils` | 5.192 | 5.143 | 20,452 → 20,185 |
| `common/yakgrpc` | 4.446 | 4.348 | 19,594 → 19,422 |
| `common/yak/csharp` | 4.042 | 0.000 | 19,167 → 0 |
| `common/yak/java` | 2.662 | 0.000 | 13,674 → 0 |
| `common/yak/php` | 2.181 | 0.000 | 10,903 → 0 |
| `common/bin-parser` | 1.467 | 1.467 | 1,476 → 1,476 |
| `scannode` | 1.425 | 1.425 | 5,394 → 5,394 |
| `common/lowtun` | 1.382 | 1.382 | 6,079 → 6,079 |
| `common/yak/antlr4yak` | 1.353 | 1.353 | 5,369 → 5,368 |
| `common/yak/ssa` | 1.334 | 1.323 | 6,920 → 6,886 |
| `common/syntaxflow` | 1.327 | 1.225 | 6,025 → 5,833 |
| `common/yak/antlr4c` | 1.055 | 0.000 | 5,222 → 0 |
| `[compiler-generated]` | 0.991 | 0.937 | 5,316 → 4,987 |
| `common/yak/python` | 0.962 | 0.000 | 4,434 → 0 |
| `common/mcp` | 0.936 | 0.936 | 2,064 → 2,064 |
| `common/yak/ssaapi` | 0.925 | 0.830 | 3,095 → 2,870 |
| `common/yak/antlr4go` | 0.878 | 0.000 | 4,886 → 0 |
| `common/yak/yaklib` | 0.853 | 0.847 | 2,554 → 2,538 |
| `common/pcapx` | 0.772 | 0.772 | 925 → 925 |
| `common/yak/antlr4nasl` | 0.712 | 0.003 | 3,150 → 7 |

最大的可排除前端成本来自 TypeScript、C#、Java、PHP 等，其中大量生成方法和泛型实例也扩大元数据。AI、utils、gRPC、网络栈和协议分析在 slim 中仍占有真实代码体积。

## 3. slim 第三方模块代码排名

| 模块 | 代码 MiB | 链接函数数 |
|---|---:|---:|
| `github.com/yaklang/goja` | 2.003 | 7,279 |
| `github.com/xuri/excelize/v2` | 1.830 | 1,733 |
| `github.com/yaklang/javajive` | 1.338 | 3,364 |
| `google.golang.org/grpc` | 0.993 | 5,208 |
| `github.com/gopacket/gopacket` | 0.890 | 2,128 |
| `github.com/go-rod/rod` | 0.874 | 4,672 |
| `google.golang.org/protobuf` | 0.750 | 2,327 |
| `github.com/jellydator/ttlcache/v3` | 0.655 | 3,071 |
| `github.com/go-git/go-git/v5` | 0.654 | 1,949 |
| `golang.org/x/net` | 0.433 | 1,267 |
| `github.com/itchyny/gojq` | 0.427 | 637 |
| `github.com/refraction-networking/utls` | 0.423 | 799 |
| `github.com/quic-go/quic-go` | 0.421 | 1,266 |
| `github.com/cloudflare/circl` | 0.417 | 1,569 |
| `github.com/sijms/go-ora/v2` | 0.377 | 512 |

这些都是已链接代码而非 go.mod 列出的依赖总大小。模块还可能带静态表和类型/函数元数据，不能仅凭代码列判断完整可节约空间。例如 Oracle 的主要体积在字符转换静态表，不在驱动机器码。

## 4. slim 嵌入资源

依赖图中的 `go:embed` 输入总共 **8.072 MiB**；这不是独立于节组成的附加体积。普通 Go 字符串、生成表和 protobuf 描述符不属于该清单。多数大文件已经压缩。

| 资源 | 输入 MiB |
|---|---:|
| `common/yak/yakdoc/doc/doc.gob.zst` | 1.235 |
| `common/ai/ytoken/qwen.tiktoken.gz` | 1.082 |
| `common/wsm/payloads/payloads.tar.gz` | 0.610 |
| `embed/data/nfp.gz` | 0.581 |
| `embed/data/geo/city2coord.json` | 0.533 |
| `common/fp/fingerprint/rule_resources/static.tar.gz` | 0.473 |
| `common/crep/static/css/bootstrap.min.css` | 0.211 |
| `common/bin-parser/rules/rules.tar.zst` | 0.194 |
| `common/ai/aid/aitool/buildinaitools/yakscripttools/yakscriptforai.tar.gz` | 0.191 |
| `embed/data/anti-crawler/stealth.min.js` | 0.178 |
| `embed/data/user-wfp-rules.zip` | 0.145 |
| `embed/data/user-wfp-rules/custom.yml.gzip` | 0.142 |
| `common/coreplugin/base-yak-plugin.tar.gz` | 0.098 |
| `common/yso/resources/static.tar.gz` | 0.097 |

协议规则归档只有 203,533 字节。它无法解释整个引擎的大小，也无法通过继续压缩它收回 3.78 MiB。

## 5. 可进一步压缩或治理的部分

### 原始文本资源（已做离线压缩实验，未改加载接口）

对实际 slim 嵌入输入使用 gzip level 9、mtime=0 的结果：

| 输入 | 原始字节 | gzip 字节 | 数据本身减少字节 |
|---|---:|---:|---:|
| `data/geo/city2coord.json` | 559,195 | 69,420 | 489,775 |
| `static/css/bootstrap.min.css` | 220,735 | 29,792 | 190,943 |
| `data/anti-crawler/stealth.min.js` | 186,516 | 14,887 | 171,629 |
| `static/js/jquery-3.6.0.min.js` | 89,500 | 30,871 | 58,629 |
| `data/rss/cyber_security_rss.opml` | 63,269 | 12,138 | 51,131 |
| `static/js/bootstrap.min.js` | 60,554 | 16,126 | 44,428 |
| `data/anti-crawler/sannysoft.html` | 54,071 | 11,964 | 42,107 |

这些是数据压缩实验，**不是已实现的二进制节约量**。优先级最高的是地理坐标 JSON、静态 CSS 和 anti-crawler JS；落地需保持 `embed.FS`/Asset/HTTP 静态资源行为、内容类型、读取路径与并发安全。现有压缩文件再次压缩通常收益很小。

### 大型非 embed 静态表

同源 `-w` 诊断构建的命名静态符号跨度显示，`go-ora/v2/converters` 约 **4.06 MiB**，`golang.org/x/text/collate.mainValues` 约 **0.96 MiB**。这类表可以研究按需压缩/解码，但需要上游或本地 fork，验证全部字符集行为与启动/堆内存成本；本 PR 保留完整驱动，不把删除字符集支持算作压缩。数字是符号跨度估计，不是可直接兑现的节约量。

### 生成方法与泛型实例

Excel、Goja、gRPC/protobuf、AI、omap 等仍有较大代码或元数据成本。后续应以调用链与导出合同为依据分离可选功能、减少重复泛型实例和反射保留的方法。禁止仅凭文件大小删除功能或使用不受支持的链接器改写破坏 runtime 表。

### 不应算作文件浪费的内存

`gopacket` 的 PPP/Ethernet metadata 大数组等属于 BSS/zero-fill，`nm` 可显示几 MiB 的虚拟空间，但它们不占相同数量的磁盘字节。另一个常见误差是叠加 `runtime.pclntab`、`runtime.funcnametab`、`runtime.functab` 的重叠符号跨度；本工具按实际节和运行时边界计数。

## 验证与复现

命令和统计口径见 [scripts/binary-size](../scripts/binary-size/README.md)。完整 JSON 输出包含全部包和资源，不限于本文排名前列。

本机验证范围：

- 全量与 slim 构建；slim `--help` / `--version`，使用独立 `YAKIT_HOME`。
- `go test ./scripts/binary-size`：当前 Go 可执行文件的节/代码/元数据分区总和及依赖边界；额外交叉编译了 stripped Linux/amd64 工具二进制，在 macOS 上读取其 ELF 成分成功。
- `go run ./scripts/binary-size -deps SLIM_DEPS -check-slim`：大型外部语言 SSA 前端及完整编译/扫描调度包不再出现在正式入口依赖图。
- slim gRPC 补全/降级合同、CLI 命令组与内置版本资源 gate。
- 全量扫描节点 compiler hook 和已有 CLI 配置/别名回归。
- slim `common/bin-parser/...`、`pcaputil`、`pcap-inspect` 协议回归。

未将本次文件大小结果外推为运行内存、启动速度、持续抓包吞吐收益，也未声称 Windows/Linux 发布包大小与此相同。工具已支持 ELF，跨平台数值应在对应的真实发布构建上重新测量。
