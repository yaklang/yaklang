# Web API 文档覆盖率基线（Coverage Baseline）

本文件是 Yaklang Web API 文档（`docs/api`）的覆盖率权威基线，由文档生成器在一次"全量重生成"后产出，用于跟踪各库 Go doc 注释的补全进度。文件本身不参与文档站同步（位于 `docs/api` 之外）。

## 生成元信息

- 生成时间：2026-06-19 12:50 CST
- 生成器：`common/yak/yakdoc/generate_web_doc/generate_web_doc.go`
- 生成口径：与 CI（`.github/workflows/generate-web-doc.yml`）完全一致，使用 `-gcflags=all=-l` 禁用内联，保证 go/doc 注释按声明行正确关联。
- 生成器 `main()` 已固化 `debug.SetGCPercent(-1)`，规避 vendored ANTLR4 运行时的堆损坏崩溃；本次两次全量生成均一次通过，无崩溃、无重试。

## 总览数字

- 总函数数（Total functions）：2854
- 有 gap 的函数数（functions with gaps）：961
- 无 gap 的函数数（ok）：1893
- 覆盖率（ok / total）：约 66.3%

一个函数只要缺失"描述 / 参数说明 / 返回值说明 / 示例"中的任意一项，即计为一个 gap。

## 缺失维度分布（按缺失项统计，单函数可命中多项）

| 缺失维度 | 命中函数数 |
|:--|:--|
| 返回值说明（return-explanation） | 924 |
| 参数说明（param-explanation） | 864 |
| 示例（example） | 494 |
| 描述（description） | 361 |

## 按严重程度的 gap 构成（互斥分组，合计 961）

| 分组 | 数量 | 说明 |
|:--|:--|:--|
| 仅缺参数/返回值说明（描述与示例均已具备） | 466 | 函数已有完整描述和示例，仅参数或返回值未逐项标注，多为"软"缺口 |
| 有描述但缺示例 | 134 | 描述齐备、尚缺可运行示例 |
| 缺描述（最严重） | 361 | 描述为空，集中在越界库与结构性丢弃情形 |

## 按库的 gap 明细（降序，共 61 个库存在 gap）

| 库 | gap 数 |
|:--|:--|
| str | 118 |
| yso | 100 |
| nuclei | 67 |
| hids | 63 |
| rag | 59 |
| risk | 48 |
| systemd | 40 |
| ssa | 40 |
| crawler | 37 |
| codec | 36 |
| yakit | 34 |
| servicescan | 26 |
| re | 25 |
| ffmpeg | 22 |
| crawlerx | 22 |
| synscan | 17 |
| db | 16 |
| excel | 14 |
| pprof | 13 |
| subdomain | 12 |
| io | 12 |
| exec | 12 |
| json | 10 |
| tcpmitm | 8 |
| netutils | 8 |
| bufio | 8 |
| amap | 8 |
| netstack | 7 |
| syntaxflow | 6 |
| aiagent | 6 |
| zip | 5 |
| x | 5 |
| math | 5 |
| omnisearch | 4 |
| liteforge | 4 |
| jsonschema | 4 |
| finscan | 4 |
| sfreport | 3 |
| file | 3 |
| aim | 3 |
| pcapx | 2 |
| mmdb | 2 |
| mimetype | 2 |
| filesys | 2 |
| browser | 2 |
| (global) | 2 |
| webforest | 1 |
| toolbox | 1 |
| re2 | 1 |
| pandoc | 1 |
| memeditor | 1 |
| java | 1 |
| git | 1 |
| filescanner | 1 |
| filemonitor | 1 |
| dnslog | 1 |
| diff | 1 |
| cve | 1 |
| cli | 1 |
| bin | 1 |
| ai | 1 |

完整到函数粒度的明细见生成器输出的覆盖率底单（`-coverage-report` 产物，例如 `/tmp/doc_coverage.md`），其中按库列出了每个函数缺失的具体维度。

## 剩余 gap 的分类说明

剩余 gap 主要可归为三类。本轮 synthesis 不对每个函数做逐一改写（属各 backfill 分组的后续工作），此处给出数据驱动的归类与量级估计。

### 1. 结构性 go/doc 限制 / 可接受的软缺口（约 466 + 大量 option-setter）

- go/doc 单文件解析会把"返回某命名类型（或其切片/指针）"的函数当作该类型的构造器；当该类型声明在同包另一文件时，函数会被整体丢弃，表现为"有注释却空描述/缺失"。
- 另有大量函数已具备完整描述与示例，仅参数/返回值未逐项标注（共 466 个），多源于：
  - functional-option 变参（如 `yso.Generate*` 系列的 `options`、各库的 `opt ...Option`），go/doc 难以对变参逐项展开；
  - 标准 `(result, error)` 返回，未单独为 `error` 写返回说明。
- 典型代表：`str`（118，仅 5 个缺描述，其余均为参数/返回软缺口）、`yso`（100，全部仅缺参数/返回说明）、`hids`（63，多为 `Xxx`/`XxxCallback` 配对的返回/回调参数软缺口）。

### 2. 规则明确允许跳过（builder 闭包绑定 / 外部包别名）

- builder 闭包绑定：option-setter 函数返回在别处声明的 option 类型，被 go/doc 视作构造器而丢弃。典型如 `systemd.service_*`/`systemd.install_wanted_by` 等服务配置项、`risk` 的 `Save`/`NewRisk`/`Check*` 等闭包绑定。
- 外部包函数别名：库导出实际指向其他包的函数，源处无独立注释。典型如 `dnslog.LookupFirst`（= `netx.LookupFirst`）。
- 这类按既定规则允许跳过，不计入"真实待补"。

### 3. 真实待补（out-of-scope 库的无注释函数）

- 集中在本轮 backfill 未覆盖的越界库，函数完全无 Go doc 注释（描述/参数/返回/示例全缺）。典型如 `nuclei`（67，核心 `Scan`/`AllPoC`/`PullDatabase` 等全无注释）、`yakit`（34）、部分 `ssa`/`rag`/`db`/`pprof`。
- 缺描述总计 361 个，是后续补全的主要目标池；扣除其中属第 1、2 类（结构性丢弃 + option-setter 闭包，如 `systemd` 40、部分 `codec`/`db`）后，纯"越界库真实待补"约在 200 量级。

> 说明：上述各类量级为基于缺失维度与库归属的数据驱动估计，精确到函数的归类需结合各 backfill 分组逐函数审计，超出本次 synthesis（重生成 + 同步 + 验证 + 留档）的范围。

## 如何重生成与验证

在 yaklang 主仓根目录执行（注意 flag 必须放在位置参数 `web_doc/` 之前，Go flag 包在首个位置参数处停止解析）：

```bash
cd /Users/v1ll4n/Projects/yaklang
# 1) 确认生成器可编译
go build ./common/yak/yakdoc/generate_web_doc/

# 2) 全量重生成（与 CI 同口径，并产出覆盖率底单到 docs/api 之外）
rm -rf web_doc
go run -gcflags=all=-l common/yak/yakdoc/generate_web_doc/generate_web_doc.go \
    -coverage-report /tmp/doc_coverage.md web_doc/ > /tmp/gen.log 2>&1

# 3) 抓取覆盖率总览与各库明细
grep "doc coverage summary" /tmp/gen.log
grep "function(s) with gaps" /tmp/gen.log

# 4) 同步到文档站（与 CI 一致：--delete 加三个 exclude）
rsync -ai --delete \
    --exclude='_category_.json' --exclude='__global__.md' --exclude='global_buildin_ops.md' \
    web_doc/ /Users/v1ll4n/Projects/yaklang.github.io/docs/api/
```

示例校验（在文档站目录）：

```bash
cd /Users/v1ll4n/Projects/yaklang.github.io
python3 scripts/verify-manual-examples.py
# 通过时以 "ALL verifiable blocks passed"（exit 0）结束
```

若生成器仍偶发 `fatal error: found bad pointer in Go heap`，直接重试即可（最多 5 次）；根治需单独 PR 升级 vendored ANTLR4 运行时（本轮不处理，且不得改动 `vendor/`、`go.mod`）。
