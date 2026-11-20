# Release-Time Go Linker Reflect-Pruning (`tools/reflect-prune-link`)

本文档用于说明 PR #5488 的实现原理、全平台 CI 实测收益，以及后续**方法级 / Linker 级**进一步缩减二进制体积的候选方案与风险评估，供团队共同评审决策。

---

## 1. 问题背景与核心原理

Go 链接器（`cmd/link/internal/ld/deadcode.go`）在检测到程序任意位置存在动态 `reflect.Value.MethodByName` / `reflect.Type.MethodByName` 调用时，会将全局标志 `d.reflectSeen` 置为 `true`。YakVM 的脚本运行时与 `yak2ssa` 必须使用动态反射，因此 `d.reflectSeen` 在 Yak 二进制中恒为 `true`。

其副作用是：**链接器被迫保留整个二进制中所有可达类型的全部导出方法**（即使它们从未被直接调用、从未通过接口分发、也永远不会被 Yak 脚本反射调用）——例如上万个 ANTLR/AST 生成方法、内部解析器方法、内部编排方法等。

### 我们的非侵入式方案

`tools/reflect-prune-link` 是一个零三方依赖的纯 Go 工具，通过 `go build -overlay` 在临时目录编译一份带极小补丁的 `cmd/link`，仅在 `deadcode.go` 的 `reflectSeen` 保活分支增加过滤条件：

```go
// 原始逻辑：
if (d.reflectSeen && (m.isExported() || d.dynlink)) || d.ifaceMethod[m.m] || d.genericIfaceMethod[m.m.name]

// 补丁后：
if (d.reflectSeen && (m.isExported() || d.dynlink) && !yakReflectFilter.match(d.ldr, m.src)) || d.ifaceMethod[m.m] || d.genericIfaceMethod[m.m.name]
```

关键安全边界：
1. **绝不裁剪直接调用的方法**。
2. **绝不裁剪任何满足接口的方法**（`d.ifaceMethod` / `d.genericIfaceMethod` 100% 保持原样）。
3. **绝不影响日常开发构建**：仅在发布流水线（`reuse-build.yml` / `exp-cross-build.yml` / `build-linux-manylinux.sh`）显式启用，支持 `reflect_prune_profile=off` 一键回退至原生 Go 链接器。
4. **Go 升级安全保护**：`PatchDeadcodeSource` 校验精确代码锚点，若未来升级 Go 导致 `deadcode.go` 结构变更会显式报错（不会静默失效）。
5. **跨平台实现**：
   - **Linux (`manylinux2014`) & macOS**：通过 `-overlay` + `-toolexec` 在 `/tmp` 构建并调用 patched `link`，**完全不修改 `GOROOT`**（兼容 `:ro` 挂载）。
   - **Windows**：由于 `-toolexec` 会导致 `cmd/go` 对 `compile` 禁用 `@argsfile`，使 473 个文件的 `javajive/classparser` 超过 Windows `CreateProcess` 的 32 KB 命令行上限，因此在临时 CI runner 上采用 `try/finally` 临时替换 `GOTOOLDIR\link.exe` 并在构建结束后自动还原的方案。

### 重要运行时约束（为什么不能随意裁剪）

Go 链接器在跳过未达方法时，不会改写 `.rodata` 中类型描述符的 `uncommonType.Mcount`，而是将未保活方法的 `m.Mtyp` 重定位留空。因此：
- 对于**永远不会被 `reflect.Type.Method(i)` 遍历**的内部包：裁剪 100% 透明安全。
- 对于**会被 `yak2ssa`（`common/yak/ssa/extern_instance.go:459`）或 Yak 脚本遍历 `t.Method(i)`** 的类型：若其方法被裁剪，调用 `reflect.(*rtype).Method(i)` 会在 `internal/abi.(*FuncType).NumIn()` 触发空指针 panic。
- **最佳实践**：所有 `yaklib` 导出签名、内置 AI 工具（77 个）及内置核心插件（49 个）可达的类型包，必须保持在保护白名单中，并用全新 `YAKIT_HOME` 启动初始化作为强制回归门禁。

---

## 2. 当前方案（`tier2`）实测收益

### 2.1 全平台 13 个 CI 产物对比（`v1.4.9-beta8` vs PR #5488 `tier2`）

CI Run: https://github.com/yaklang/yaklang/actions/runs/44754788571 （7 Full + 6 Slim 全部构建通过，仅单独 PR #5488，**未叠加 PR #5487**）：

| 产物 | 基线原始体积 | `tier2` 原始体积 | 原始体积减少 | 基线 Zip | `tier2` Zip | Zip 减少 |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| `yak_darwin_amd64` | 235M | **221M** | **-14M (-6.0%)** | 85.84 MiB | **83.73 MiB** | -2.11 MiB (-2.46%) |
| `yak_darwin_arm64` | 229M | **214M** | **-15M (-6.6%)** | 82.65 MiB | **80.32 MiB** | -2.33 MiB (-2.82%) |
| `yak_linux_amd64` | 234M | **221M** | **-13M (-5.6%)** | 85.65 MiB | **83.54 MiB** | -2.12 MiB (-2.47%) |
| `yak_linux_arm64` | 227M | **213M** | **-14M (-6.2%)** | 80.74 MiB | **78.54 MiB** | -2.20 MiB (-2.73%) |
| `yak_windows_amd64.exe` | 237M | **223M** | **-14M (-5.9%)** | 86.19 MiB | **83.90 MiB** | -2.28 MiB (-2.65%) |
| `yak_windows_arm64.exe` | 227M | **213M** | **-14M (-6.2%)** | 79.97 MiB | **77.74 MiB** | -2.23 MiB (-2.79%) |
| `yak_windows_legacy_amd64.exe` | 237M | **223M** | **-14M (-5.9%)** | 86.19 MiB | **83.91 MiB** | -2.28 MiB (-2.65%) |
| `yak-slim_darwin_amd64` | 178M | **174M** | **-4M (-2.2%)** | 73.18 MiB | **72.34 MiB** | -0.85 MiB (-1.16%) |
| `yak-slim_darwin_arm64` | 172M | **168M** | **-4M (-2.3%)** | 70.18 MiB | **69.35 MiB** | -0.82 MiB (-1.18%) |
| `yak-slim_linux_amd64` | 178M | **173M** | **-5M (-2.8%)** | 73.02 MiB | **72.18 MiB** | -0.84 MiB (-1.15%) |
| `yak-slim_linux_arm64` | 170M | **166M** | **-4M (-2.4%)** | 68.73 MiB | **67.95 MiB** | -0.78 MiB (-1.14%) |
| `yak-slim_windows_amd64.exe` | 179M | **175M** | **-4M (-2.2%)** | 73.19 MiB | **72.29 MiB** | -0.89 MiB (-1.22%) |
| `yak-slim_windows_arm64.exe` | 170M | **166M** | **-4M (-2.4%)** | 67.90 MiB | **67.12 MiB** | -0.78 MiB (-1.15%) |

> 注：叠加 PR #5487（代码与内联/泛型瘦身：Full -9.03 MiB / Slim -5.23 MiB）后，Full 预计降至 **~204–214M（总省 ~22–24M）**，Slim 预计降至 **~161–170M（总省 ~9–10M）**。

---

## 3. 方法级进一步缩减空间（同一 `main` 基线实测全景）

以下为同一 `main` 提交在 `darwin/arm64`（`-trimpath -ldflags "-s -w"`）下，不同裁剪档位的精确字节数对比：

### 3.1 Full (`gzip_embed`) 各档位实测

| 档位 | 规则规模 | 裁剪方法数 | 原始体积 | 相对 `main` 减少 | 叠加 PR #5487 预计 |
| :--- | :--- | ---: | ---: | ---: | ---: |
| `off` (`main` 基线) | 0 | 0 | 228.10 MiB (`239,184,210` B) | — | ~219.1 MiB |
| `tier1`（纯 ANTLR/AST） | 4 条 | 31,791 | 216.33 MiB (`226,839,234` B) | **-11.77 MiB (-5.16%)** | ~207.3 MiB |
| **`tier2`（当前 PR 默认，已验证）** | **9 条 + 6 白名单** | **38,099** | **213.90 MiB (`224,286,402` B)** | **-14.21 MiB (-6.23%)** | **~204.9 MiB** |
| 方案 A（`tier3` 扩展黑名单候选） | 25 条 + 8 白名单 | 47,008 | 207.64 MiB (`217,730,530` B) | **-20.46 MiB (-8.97%)** | **~198.6 MiB** |
| 方案 B（反向白名单，预估） | `ALL` + `!yaklib` | ~65,000+ | ~198–202 MiB | **~-26 ～ -30 MiB** | **~189–193 MiB** |
| `ALL`（理论极限，不可直接运行脚本） | 全部 | 78,069 | 191.36 MiB (`200,651,970` B) | **-36.75 MiB (-16.11%)** | ~182.3 MiB |

### 3.2 Slim (`gzip_embed,irify_exclude`) 各档位实测

| 档位 | 规则规模 | 裁剪方法数 | 原始体积 | 相对 `main` 减少 | 叠加 PR #5487 预计 |
| :--- | :--- | ---: | ---: | ---: | ---: |
| `off` (`main` 基线) | 0 | 0 | 171.27 MiB (`179,590,114` B) | — | ~166.0 MiB |
| `tier1`（纯 ANTLR/AST） | 4 条 | 4,477 | 169.82 MiB (`178,065,074` B) | **-1.45 MiB (-0.85%)** | ~164.6 MiB |
| **`tier2`（当前 PR 默认，已验证）** | **9 条 + 6 白名单** | **11,333** | **167.22 MiB (`175,346,658` B)** | **-4.05 MiB (-2.36%)** | **~162.0 MiB** |
| 方案 A（`tier3` 扩展黑名单候选） | 25 条 + 8 白名单 | 19,003 | 161.38 MiB (`169,222,098` B) | **-9.89 MiB (-5.77%)** | **~156.2 MiB** |
| 方案 B（反向白名单，预估） | `ALL` + `!yaklib` | ~33,000+ | ~153–157 MiB | **~-14 ～ -18 MiB** | **~148–152 MiB** |
| `ALL`（理论极限，不可直接运行脚本） | 全部 | 47,394 | 147.16 MiB (`154,311,570` B) | **-24.11 MiB (-14.08%)** | ~141.9 MiB |

---

## 4. `tier2` 之后的剩余可裁剪分布（`go tool nm -size` 实测）

`tier2` → `ALL` 之间剩余 **37,393 个反射保活方法（分布在 1,390 个包）**，仅 `.text` 代码段占 **8.55 MiB**（叠加 `.rodata`/类型表/`pclntab` 后约 **22.5 MiB Full / 20.1 MiB Slim**）。Top 包级分布及 `yaklib` 暴露审计如下：

| 剩余 `.text` (KiB) | 剩余方法数 | 包路径 | `yaklib` 引用 | 评估备注 |
| ---: | ---: | :--- | :---: | :--- |
| 1,442.7 | 1,197 | `github.com/xuri/excelize/v2` | 0 | 单体最大漏网包（总二进制 ~2.5–3 MiB），可优先纳入 |
| 412.2 | 6,075 | `common/yakgrpc/ypb` | 16 | **不可整包裁剪**（`yaklib` 引用），需方案 C 方法级处理 |
| 288.3 | 1,628 | `common/ai/aid/aicommon` | 0 | 内部 AI 编排包，可纳入 |
| 264.9 | 1,222 | `common/yakgrpc` | 0 | gRPC 服务端内部包（注意排除 `ypb`） |
| 199.8 | 1,038 | `common/utils` | 多处 | 通用工具包，建议保留 |
| 149.0 | 995 | `common/yak/ssaapi` | 多处 | **必须保护**（Yak 脚本反射 `*ssaapi.Program` 等） |
| 143.3 ~ 33.0 | ~2,200 | 各语言 `*/parser`（csharp/rust/php/cpp/java/scala/kotlin/go/c/yak） | 0 | **传递保活漏洞**：因 `*2ssa` 嵌入 `*parser.BaseVisitor` 被反向拖活 |
| 137.4 | 737 | `common/sarif` | 0 | 内部 SARIF 报告模型，可纳入 |
| 127.1 | 266 | `common/openapi/openapi3` | 0 | 内部 OpenAPI 解析，可纳入 |
| 120.5 | 218 | `common/pcapx/pcapdb` | 16 | **必须保护**（内置 AI 工具 `pcap/load_pcapfile.yak` 使用） |
| 100.0 | 684 | `common/ai/aiforge` | 0 | 内部 AI Forge，可纳入 |
| 98.0 | 214 | `github.com/ProtonMail/go-crypto/openpgp/packet` | 0 | 内部加密包，可纳入 |
| 95.1 | 195 | `go/parser` | 0 | 内部 Go 源码解析，可纳入 |
| 92.8 | 626 | `common/ai/aid/coordinator` | 0 | 内部 AI 协调器，可纳入 |
| 83.8 | 165 | `debug/dwarf` | 0 | 内部 DWARF 调试解析，可纳入 |
| 76.6 | 128 | `github.com/russross/blackfriday/v2` | 0 | Markdown 渲染库，可纳入 |
| 71.1 | 311 | `common/ai/aid/aireact` | 0 | 内部 AI ReAct，可纳入 |
| 70.1 | 41 | `golang.org/x/image/draw` | 0 | 图像处理库，可纳入 |
| 69.8 | 158 | `common/browser` | 0* | **需保护**（实测内置 AI 工具 `browser/use_browser.yak` 反射调用） |
| 45.0 | 65 | `github.com/google/pprof/profile` | 0 | 性能剖析包，可纳入 |
| 44.4 | 113 | `github.com/go-git/go-git/v5` | 4 | **必须保护**（`yaklib` 引用） |

---

## 5. 三种后续演进方案对比（供团队评审）

### 方案 A：扩充黑名单（`tier2` → `tier3`）
- **做法**：
  1. 补上 `*2ssa`（如 `csharp2ssa.`/`rust2ssa.`/`php2ssa.`/`java2ssa.`/`cpp2ssa.`/`scala2ssa.`/`kotlin2ssa.`/`go2ssa.`/`py2ssa.`/`ts2ssa.`/`js2ssa.`）以切断 `BaseVisitor` 传递保活链；
  2. 纳入 `excelize/v2`、`common/ai/aid/`、`common/ai/aiforge`、`common/sarif`、`common/openapi/`、`ProtonMail/go-crypto`、`blackfriday/v2`、`debug/dwarf`、`go/parser`、`x/image/`、`pprof/profile`、`common/netstackvm` 等已验证非脚本反射的大包（注意避开实测被内置 AI 工具触发的 `common/browser` 与 `ssa_compile`）。
- **收益**：Full **-20.5 MiB（总降 ~9%）**，Slim **-9.9 MiB（总降 ~5.8%）**；叠加 PR #5487 后 Full 可压至 **~199 MiB**、Slim 压至 **~156 MiB**。
- **风险与成本**：**低**。不改工具架构，仅增补 `profiles.go` 列表，配合全新 `YAKIT_HOME`（77 AI 工具 + 49 核心插件）+ `Essential Tests` 即可验证。

### 方案 B：反向白名单模式（`ALL` + 自动生成 `yaklib` 可达包白名单）
- **做法**：
  - 剩余 8.55 MiB `.text` 高度长尾（1,390 个包），手工维护黑名单边际递减。
  - 改为默认开启 `ALL` 裁剪，并通过 Go AST/类型检查脚本自动扫描 `common/yak/yaklib` 导出函数/常量的全部参数与返回值类型所属包 + Go 标准库 + 已知动态插件包，生成 `!` 白名单并接入 CI 校验。
- **收益**：Full **~-26 ～ -30 MiB**，Slim **~-14 ～ -18 MiB**；叠加 PR #5487 后 Full 可压至 **~190 MiB**、Slim 压至 **~149 MiB**（**对 Slim 收益翻倍以上**）。
- **风险与成本**：**中**。需要维护白名单生成/校验脚本，并用裁剪后的 `yak` 二进制跑全量集成测试，防止遗漏某个冷门 Yak 插件动态返回的三方类型。

### 方案 C：Linker 方法表压缩 + 方法名级细粒度裁剪
- **做法**：
  - 针对 `common/yakgrpc/ypb`（6,075 个未用 `Get*`/`ProtoReflect`/`Descriptor` 方法，约 2.5–3 MiB）这类“包本身被 `yaklib` 引用、但 90% 方法无人反射”的包，在 `deadcode.go` 裁剪未达方法的同时，同步压缩 `uncommonType.methods` 并更新 `Mcount`（避免 `reflect.Type.Method(i)` 遇到空 `Mtyp` panic）。
- **收益**：在方案 A/B 基础上额外再省 **~2.5–3 MiB**（Full & Slim 均有收益）。
- **风险与成本**：**高**。直接操作 Go 运行时 `abi.UncommonType` 二进制布局，跨 Go 大版本维护成本明显高于方案 A/B，建议仅在方案 A/B 落地后仍有强需求时再考虑。

---

## 6. 团队评审建议 Checklist

1. **当前 PR #5488（`tier2`）是否先合入？**
   - 已有全平台 13 产物 CI 实测（Full -13~15M / Slim -4~5M），77 AI 工具 + 49 核心插件全绿，且保留 `reflect_prune_profile=off` 一键回退开关。
2. **下一阶段优先选 方案 A 还是 方案 B？**
   - 追求最稳最省心：先推 **方案 A（`tier3`）**，把 Full 压到 **< 200M**、Slim 压到 **~156M**。
   - 追求 Slim 极致瘦身（< 150M）：投入一次自动化 AST 白名单扫描，直接上 **方案 B**。
3. **是否需要在 CI 中增加一条“使用裁剪后产物跑 `YAKIT_HOME` 初始化 + 关键 Yak 脚本冒烟”的门禁步骤？**（强烈建议，可提前拦截类似 `pcapx` / `browser` / `javajive` 被误裁的问题。）
