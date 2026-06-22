# MINI_VECTOR_SCAN_TODO —— mvscan 实施进度与跨环境接力清单

> 配套总方案见 `MINI_VECTOR_SCAN_IMPL.md`。本文件是**接力指南**:换机器/换会话的人或 AI
> 照此即可继续推进,不丢上下文、不破坏已验证的正确性不变量。
>
> 一句话现状(2026-06-22):**M1 已完成并全绿**。纯 Go、rune 级 Glushkov 位并行存在性引擎
> (`BackendMVS`)落地;真实 MITM 规则 88.5% 可编入 NFA;与 stdlib oracle 在 1332 条真实流量上
> 差分逐条一致;纯算法(无 SIMD/CGO)相对 stdlib 逐条 **6.3x**。当前瓶颈是回退正则的 always-on
> 开销(详见第 4 节),下一步是 M2(route-B 字面量门控 / 合并单趟 / Teddy)。

---

## 0. 接力前置:环境准备与一键验证

所有命令默认在仓库根 `/Users/<you>/Projects/yaklang`(或你的克隆根)执行,除生成语料外。

### 0.1 真实流量语料(差分/基准依赖)

`testdata/traffic_corpus.bin`(~5.25MB / ~1332 条)**已在本分支提交并被 git 跟踪**,克隆即带,
一般无需重新生成。没有它时,真实 MITM 差分/基准会 `Skip`(合成差分与随机差分不依赖它,仍可作主护栏)。

仅在以下情况需重新生成(需 CGO + 本地 yakit 库;生成是确定性的,同库同字节):

```bash
cd common/minirehs
CGO_ENABLED=1 go run testdata/gen_corpus.go \
    -db ~/yakit-projects/default-yakit.db \
    -out testdata/traffic_corpus.bin -max 5242880
# 期望输出: corpus written: ... bytes=~5.25MB records=~1332
```

### 0.2 一键回归(改任何 mvs 代码后必须跑)

```bash
# 编译 + 静态检查
go build ./common/minirehs/ && go vet ./common/minirehs/

# NFA 核心正确性(快, 不需语料): 直接对照 + 随机差分(任意字节 vs Go regexp)
go test ./common/minirehs/ -run 'TestMVSNFADirect|TestMVSNFARandomDifferential' -count=1 -v

# 后端级存在性差分(合成, 不需语料)
go test ./common/minirehs/ -run TestMVSExistenceVsOracleSynthetic -count=1 -v

# 真实流量差分(需语料): -short 仅前 200 条(快), 去掉 -short 跑全 1332 条
go test ./common/minirehs/ -run TestMVSExistenceVsOracleMITM -short -count=1 -v

# NFA 覆盖率报告(需语料/规则)
go test ./common/minirehs/ -run TestMVSCoverageMITM -count=1 -v

# 全包短模式回归(确认没碰坏其它后端)
go test ./common/minirehs/ -short -count=1
```

### 0.3 量化性能与定位热点

```bash
# 加速比(纯算法, 需语料, 约 75s 因 stdlib oracle 慢): 打印 mvs / stdlib MB/s 与倍数
go test ./common/minirehs/ -run TestMVSQuantifySpeedup -count=1 -v

# 三方吞吐基准: MVS / Engine / StdlibLoop
go test ./common/minirehs/ -run='^$' -bench 'BenchmarkMVSFullRuleset' -benchtime=3x -count=1

# CPU + 内存 profile(定位瓶颈, 改优化前后都看)
go test ./common/minirehs/ -run='^$' -bench 'BenchmarkMVSFullRuleset/MVS' \
    -benchtime=2x -cpuprofile=/tmp/mvs.prof -memprofile=/tmp/mvs.mem -count=1
go tool pprof -top -nodecount=25 /tmp/mvs.prof
go tool pprof -top -sample_index=alloc_space -nodecount=15 /tmp/mvs.mem
```

---

## 1. 已完成(M1)

- [x] **rune 级 AST + Glushkov 构造** `mvs_glushkov.go`
      `*syntax.Regexp` → `bnode`(rune 级)→ 通用 Glushkov → `mvsNFA`;字母表压缩
      (rune→符号→`reach[sym]` 位集);`{m,n}` 有限展开(上限 `mvsMaxPos=4096`)。
- [x] **位并行存在性执行器** `mvs_exec.go`
      按 Go regexp 同款 `utf8.DecodeRune` 逐 rune(非法字节→RuneError);`existsIn`(多字)
      + `existsIn1`(`nword==1` 零分配快路径)。
- [x] **后端接入** `mvs_backend.go` + `minirehs.go`(`BackendMVS` 枚举/`String`)+ `backend.go`
      (`selectBackend`)。复用现成 AC 字面量预过滤;不可编入 NFA 者回退 `verifier` 兜底。
- [x] **测试与基准** `mvs_test.go`:直接对照、随机差分(1 万+ 条 × 任意字节)、合成 ID 差分、
      真实 MITM ID 差分(1332 条 + 整段)、覆盖率、加速比、三方基准。
- [x] **验收**:`build`/`vet`/全包 `-short` 全绿;真实流量全量差分逐条一致;早期字节级
      `mvs_utf8.go` 已删除(被 rune 级取代)。

### 关键结果快照

| 指标 | 值 |
|---|---|
| 真实 MITM 规则 NFA 覆盖率 | 88.5%(77/87);兜底 10 |
| 真实流量差分 | 1332 条 + 整段拼接,与 stdlib oracle 逐条一致 |
| 随机差分 | 10289 条 NFA × 6 组任意字节输入,与 `regexp.Match` 一致 |
| 吞吐(纯 Go) | mvs 1.02 MB/s,engine 0.35,stdlib 0.16 → **mvs/stdlib 6.3x** |

---

## 2. 必须遵守的正确性不变量(改代码前务必读)

1. **存在性语义**:命中一律上报 `Match{ID, From:-1, To:-1}`。差分**只能按命中 ID 集合**比较
   (用 `mvs_test.go` 的 `mvsExistIDs`/`mvsAssertSameIDSet`),**不能按偏移**比较。
2. **预过滤只能放过更多,绝不能漏**:任何字面量/预过滤是"必要条件"(允许假阳,真伪由验证判定)。
   route-B 提取的字面量必须保证"命中必含其一",否则漏报(假阴)=正确性事故。
3. **rune 解码必须与 Go regexp 逐位一致**:用 `utf8.DecodeRune`。任何触及解码/类/符号映射的改动,
   都要跑随机差分(任意字节)+ 1332 条真实差分。
4. **回退不丢规则**:不能编入 NFA 的 pattern 必须落 `verifier`(`mvs_backend.go: tryCompileNFA`
   返回 nil 即兜底)。
5. **锚点策略**:仅剥离顶层 concat 两端的 `^/\A`(anchoredStart)与 `$/\z`(requireEnd);
   其它位置锚点 / 词边界 `\b\B` / 行锚 `(?m)^$` / backref / lookaround → 兜底。
   **可空根(能匹配空串)与空 first → 兜底**(避免存在性边界歧义)。
6. **不删 `vectorscan_bridge.go` / `vectorscan_stub.go`**:作为性能/正确性对照基线,
   直到 mvscan 全部验收通过(见 IMPL 第 10/13 节)。

---

## 3. 代码坐标(接力时快速定位)

- `mvs_glushkov.go`
  - `synToRune`:支持的 RE2 op 列表(改这里扩/缩 NFA 适用范围)。
  - `compileMVSNFA`:总入口——锚点剥离 → 构造 → 可空根/空 first/`over` 兜底 → 字母表 → 快路径表。
  - `buildAlphabet` / `symIndex` / `symbolOf`:字母表压缩与 rune→符号映射。
  - `stripEndAnchors`:顶层首尾锚处理。
- `mvs_exec.go`:`existsIn`(多字)/`existsIn1`(单字快路径)。位并行递推公式见文件头注释。
- `mvs_backend.go`
  - `tryCompileNFA`:regexp2-only / parse 失败 / `!ok` → nil(兜底)。
  - `mvsDB.scan`:① 字面量预过滤门控 → ② always-on(无字面量)→ 每条 `verifyOne`。
  - `verifyOne`:有 NFA 走 `existsIn`,否则 `verifier`;命中 `Match{ID,-1,-1}`。
- 复用件:`prefilter.go`(`buildLiteralIndex`/`newPrefilter`/`litHit`)、`ahocorasick.go`、
  `literal.go`(`extractRequiredLiterals`,RE2 树版,route-B 可借鉴/改造)、`verifier.go`、
  `width.go`、`feature_gate.go`。
- 测试复用:`consistency_test.go`(`fixedPatterns`/`randomCorpus`/`compilableMITMPatterns`)、
  `testdata_helper_test.go`(`mitmPatterns`/`loadCorpus`)。注意 `existIDs`/`assertSameIDSet` 带
  `minirehs_vectorscan` tag 不可见,故 mvs 自带 `mvsExistIDs`/`mvsAssertSameIDSet`。

---

## 4. 当前瓶颈(数据驱动,profile 已确认)

NFA 本身几乎免费(`existsIn` 合计 ~31% CPU 且低分配)。热点在**那 10 条回退正则的 always-on 全量扫描**:

- regexp2-only(回溯引擎):~**32% CPU、~75% 分配**(`regexp2.getRunes` / `Regexp2Wrapper.Match`)。
- stdlib regexp 回退:~**25% CPU**(`regexp.(*machine).match`)。

这是现有 engine 也背的"regexp2 always-on 税"。**纯 Go 再上台阶,核心是让回退正则别每条都全量跑**,
而不是继续优化 NFA。

---

## 5. 待办(按优先级 / M2→M5 路线)

> 工作法:每完成一步,先跑第 0.2 节回归(尤其 1332 条真实差分 + 随机差分),再跑 0.3 节看数字。
> **先写护栏,再动优化。**

### P1 — route-B 近似字面量门控(高收益,⚠ 有漏报风险)
- 目标:给 regexp2-only / 无字面量的回退项提取"必需字面量",纳入现有 AC 预过滤,命中才验证;
  直接压掉第 4 节的 57% always-on 开销。
- 做什么:对 regexp2 pattern 文本剥离 `(?=...)`/`(?<=...)`/`\1` 等包装后,用 `regexp/syntax`
  解析可解析骨架,复用/改造 `literal.go` 的 `extractRequiredLiterals` 思路提"必需字面量 OR 集";
  提不出就保持 always-on。集成点:`mvsBackend.compile` 里把这些字面量也喂给 `buildLiteralIndex`。
- ⚠ 风险:**只要提出一个非必需字面量就会漏报**。务必先加单测:对一批 regexp2 pattern 断言
  "提取的字面量确实是任意命中的必要条件"(可用真实流量 + 随机串双向验证)。
- 验收:1332 条真实差分 + 合成差分 + 随机差分全绿;吞吐显著上升;新增字面量必需性单测通过。

### P2 — 合并单趟 always-on NFA(中收益,低风险)
- 目标:把 always-on 的多条 NFA 合并为一个大位集自动机,一次扫描代替多趟(当前 11 条 always-on
  每条全量扫)。需全局字母表 + accept 位置→patternID 映射 + 处理混合 anchoredStart/requireEnd。
- 验收:差分全绿;always-on 部分吞吐上升。

### P3 — Teddy SIMD 多字面量预过滤(高收益,M2 主力)
- 目标:把"逐字面量 AC"升级为 Teddy(nibble 指纹 + PSHUFB,一次 16/32 字节),大幅降低预过滤成本;
  这是逼近 100x 的主力。参照 `native/teddy.c` 与 BurntSushi `aho-corasick` 的 `src/packed/teddy`。
- 验收:差分全绿(预过滤只增假阳);吞吐向 vectorscan(bridge 基线 ~87x)靠拢。

### P4 — 纯 C 运行期内核 + blob + SIMD 分发 + amalgamation(M3/M4)
- 目标:把热路径(Teddy + bit-NFA)落成纯 C99,经平台无关 blob 与 Go 前端解耦;运行期 CPU 特性
  探测 + 函数指针分发(SSSE3/AVX2/NEON/scalar),每个 SIMD kernel 配标量孪生逐位一致;
  `tools/amalgamate` 合并单文件。build tag `minirehs_mvs` + stub 退化(见 IMPL 第 8/9/15 节)。
- 验收:四平台编译测试绿;C 各档与 Go 参考执行器逐位一致;达 ~100x。

### P5 — 平台收敛 + 接入切换(M4/M5)
- MSVC/MinGW + 跨架构 CI + ASan/UBSan/fuzz;`BackendMVS` 切到 C 加速档,纯 Go 作退化与差分 oracle;
  **全部验收通过后**移除 vectorscan bridge。

### 可选 — 扩 NFA 覆盖率
- 词边界 `\b\B` / 行锚 `(?m)^$` 的零宽断言支持(Glushkov 加 assertion-closure),把覆盖率从 88.5%
  继续往上推,进一步缩小回退集。

---

## 6. 与总方案的偏差记录(供 IMPL 文档对齐)

- **NFA 改为 rune 级**(IMPL 原文偏字节级):因 Go regexp 是逐 rune 解码、非法 UTF-8 当 RuneError,
  字节级在非法 UTF-8(真实流量常见)上对 `.`/负类与 oracle 分歧。rune 级 + 字母表压缩既正确又紧凑。
  Teddy/字面量预过滤仍是字节级(literal 本就是字节串),C 内核做 bit-NFA 时同样可做 rune 解码。
- 早期"ASCII 闸 + 字节级 UTF-8 展开"方案已废弃(`mvs_utf8.go` 删除),覆盖率由此从 33% 升到 88.5%。

> 建议接力者在动 P3/P4 前,把本节并入 `MINI_VECTOR_SCAN_IMPL.md` 的对应小节,保持设计记录一致。
