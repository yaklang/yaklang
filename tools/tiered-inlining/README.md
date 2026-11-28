# Tiered Inlining (`-gcflags`) Binary Size & System Performance Study

本文档为团队讨论 PR 的实测报告与复现工具，专门评估**按包分级内联（`-gcflags="<cold-pkg>=-l"`）**对 Yaklang 全系统性能与二进制体积的真实影响，以决定是否在发布构建中启用。

> **核心结论先行**：
> - **全局禁内联（`-gcflags=all=-l`）不可取**：虽然 Full 省 28.15 MiB / Slim 省 20.45 MiB，但会导致全系统核心负载（YakVM / Java / Go / PHP SSA / SyntaxFlow）**变慢 +22% ～ +40%**。
> - **分级内联（`tierB`：仅对冷包禁用内联，保留全部热路径默认内联）收益极高且几乎零损耗**：
>   - **体积（100% 原生 Go 编译器参数，零 Linker Patch、零反射风险）**：
>     - **Full**：`-16.61 MiB` 原始（`-7.28%`）、**`-6.10 MiB` Zip（`-7.53%`）**
>     - **Slim**：`-9.45 MiB` 原始（`-5.52%`）、**`-3.79 MiB` Zip（`-5.50%`）**
>   - **性能**：6 项真实系统负载（YakVM 密集计算/编解码/JSON、PHP/Java/Go ANTLR+SSA 编译、SyntaxFlow 数据流查询）全部在 **-6.3% ～ +2.8%** 噪声区间内（Java SSA 甚至因指令缓存命中率提升快了 ~4–6%）。

---

## 1. Go `-gcflags` 内联机制说明

`go tool compile` 默认内联级别为标准内联，传 `-l`（即 `-l=0`）会关闭匹配包的内联。通过为不同包传入多条 `-gcflags="<pkg-pattern>=-l"`，可以精确做到：

- **保持默认内联（热路径，单测强制保护）**：
  - Go 标准库 `std` / `runtime`
  - `github.com/yaklang/antlr/v4`（ATN 模拟器与词法热循环）
  - `common/yak/yakvm`、`common/yak/yaklib`
  - `common/yak/ssa`（SSA IR 核心）、`common/syntaxflow/sfvm`（SyntaxFlow 引擎）
  - `common/utils`（含 `lowhttp`、容器、编解码等热工具）
- **禁用内联（`-l`，冷路径 / 生成代码 / 外围重型库）**：
  - **`tierA`（保守档，24 条）**：17 个 ANTLR 生成的 `*/parser` 与 `sf` / `typescript/frontend/ast` + 7 个重型冷三方库（`excelize`、`go-rod`、`blackfriday`、`javajive`、`gvisor`、`grpc`、`gopacket`）。
  - **`tierB`（推荐档，33 条）**：在 `tierA` 基础上扩展到 8 种语言前端目录（`csharp/...`、`rust/...`、`php/...`、`java/...`、`cpp/...`、`kotlin/...`、`scala/...`、`python/...`）及外围业务包（`yakgrpc/...`、`ai/...`、`wsm/...`、`vulinbox/...`、`rpa/...`、`crawlerx/...`、`bin-parser/...`、`sarif/...`、`openapi/...`）。

---

## 2. 二进制体积实测（`main` 基线，`darwin/arm64`，`-trimpath -ldflags "-s -w"`）

### 2.1 Full (`gzip_embed`)

| 策略 | 原始体积 | 原始减少 | Zip 体积 | Zip 减少 | `__text` | `__gopclntab` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| `default`（默认全内联） | 228.10 MiB | — | 81.01 MiB | — | 92.13 MiB | 68.10 MiB |
| `tierA`（冷 parser + 7 三方库） | 217.95 MiB | **-10.15 MiB (-4.45%)** | 77.28 MiB | **-3.73 MiB (-4.60%)** | 88.32 MiB | 65.66 MiB |
| **`tierB`（推荐分级档）** | **211.49 MiB** | **-16.61 MiB (-7.28%)** | **74.91 MiB** | **-6.10 MiB (-7.53%)** | **85.11 MiB** | **63.71 MiB** |
| `all`（`-gcflags=all=-l`） | 199.96 MiB | -28.15 MiB (-12.34%) | 69.64 MiB | -11.37 MiB (-14.04%) | 79.76 MiB | 60.88 MiB |

### 2.2 Slim (`gzip_embed,irify_exclude`)

| 策略 | 原始体积 | 原始减少 | Zip 体积 | Zip 减少 | `__text` | `__gopclntab` |
| :--- | ---: | ---: | ---: | ---: | ---: | ---: |
| `default`（默认全内联） | 171.27 MiB | — | 68.97 MiB | — | 71.10 MiB | 46.54 MiB |
| `tierA`（冷 parser + 7 三方库） | 167.57 MiB | **-3.70 MiB (-2.16%)** | 67.25 MiB | **-1.72 MiB (-2.49%)** | 69.56 MiB | 45.58 MiB |
| **`tierB`（推荐分级档）** | **161.82 MiB** | **-9.45 MiB (-5.52%)** | **65.18 MiB** | **-3.79 MiB (-5.50%)** | **66.64 MiB** | **43.80 MiB** |
| `all`（`-gcflags=all=-l`） | 150.82 MiB | -20.45 MiB (-11.94%) | 60.16 MiB | -8.81 MiB (-12.77%) | 61.51 MiB | 41.13 MiB |

> **亮点**：分级内联消除的是高熵机器码（`__text`）和内联树（`__gopclntab`），**Zip 压缩包转化率高达 ~37–40%**（`tierB` 直接把 Full Zip 压掉 **6.10 MiB**、Slim Zip 压掉 **3.79 MiB**）。

---

## 3. 全系统真实负载性能实测（每项 3 轮取中位数，预热 `YAKIT_HOME`）

| 负载场景 | `default` | `tierA` | `tierB`（推荐） | `all`（`-gcflags=all=-l`） |
| :--- | ---: | ---: | ---: | ---: |
| **YakVM 综合**（50k 循环 + 5k Base64 + 3k JSON） | 181 ms | **179 ms (-1.1%)** | **186 ms (+2.8%)** | 254 ms (**+40.3% 变慢**) |
| **YakVM 纯计算**（200k 位运算紧循环） | 481 ms | **477 ms (-0.8%)** | **480 ms (-0.2%)** | 634 ms (**+31.8% 变慢**) |
| **PHP ANTLR + SSA**（15x 完整解析+SSA） | 83 ms | **80 ms (-3.6%)** | **82 ms (-1.2%)** | 109 ms (**+31.3% 变慢**) |
| **Java ANTLR + SSA**（20x 完整解析+SSA） | 143 ms | **134 ms (-6.3%)** | **137 ms (-4.2%)** | 176 ms (**+23.1% 变慢**) |
| **Go ANTLR + SSA**（40x 完整解析+SSA） | 189 ms | **188 ms (-0.5%)** | **187 ms (-1.1%)** | 252 ms (**+33.3% 变慢**) |
| **SyntaxFlow 查询**（30x 污点流规则匹配） | 55 ms | **56 ms (+1.8%)** | **53 ms (-3.6%)** | 71 ms (**+29.1% 变慢**) |

### 为什么 `tierB` 连 Java/PHP 解析反而略快了一点？
1. ANTLR 解析真正的 CPU 热点在 `github.com/yaklang/antlr/v4` 的 ATN 模拟器和词法扫描循环（以及 `common/yak/ssa` 核心），这些热路径在 `tierB` 中**保持 100% 默认内联**；
2. 生成的 `parser` 与 `*2ssa` 则是体积庞大、分支稀疏的规则函数，关闭它们的内联减少了 7 MiB 机器码膨胀，反而提升了 CPU L1/L2 指令缓存（I-Cache）命中率。

---

## 4. 本地一键复现

```bash
# 校验热路径保护单测
go test -v ./tools/tiered-inlining/...

# 查看 tierB 生成的 -gcflags 列表
go run ./tools/tiered-inlining --profile=tierB

# 复跑完整的 4 策略体积 + 性能基准对比
bash ./tools/tiered-inlining/bench.sh
```

---

## 5. 团队评审讨论点

本 PR **尚未修改任何 CI 发布脚本**，仅包含工具代码、热路径保护单测、复现基准脚本与本报告，供大家评审后决定：

1. **是否采纳 `tierB` 接入发布构建（`reuse-build.yml` / `exp-cross-build.yml` / `build-linux-manylinux.sh`）？**
   - 零工具链 Patch、零反射风险、跨 Windows/macOS/Linux 原生支持（标准 Go `-gcflags`）。
   - 单独收益即达 **Full -16.6 MiB（Zip -6.1 MiB）/ Slim -9.5 MiB（Zip -3.8 MiB）**，且几乎零性能损耗。
2. **是否还有其他业务热路径需要加入 `gcflags_test.go` 的永久保护白名单？**（例如 `common/yakgrpc` 的高频流式接口、或 `common/ai` 的某些热路径）
3. **与其他优化 PR 的叠加顺序**：本方案与 #5487（源码/泛型瘦身）、#5489（ANTLR `-no-visitor`）及 #5488（Linker 反射裁剪）均可独立正交叠加。
