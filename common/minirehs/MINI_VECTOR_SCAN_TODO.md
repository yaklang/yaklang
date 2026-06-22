# MINI_VECTOR_SCAN_TODO —— mvscan 实施进度与跨环境接力清单

> 配套总方案见 `MINI_VECTOR_SCAN_IMPL.md`。本文件是**接力指南**:换机器/换会话的人或 AI
> 照此即可继续推进,不丢上下文、不破坏已验证的正确性不变量。
>
> 一句话现状(2026-06-22):**M1 + 匹配定位/内容 + P1(route-B) + P2(合并 always-on) +
> 零宽断言 NFA 扩展 + P3(Teddy SIMD) + P4 全部(M2 标量内核 / M3 SIMD bit-NFA / M4 单文件
> amalgamation + 跨平台矩阵) 已完成并全绿;P5 接入切换完成;vectorscan/libhs 对照桥已整体移除
> (4 文件删 + 枚举/选择/API/scratch 清理),mvscan 自此唯一高性能后端、全程零外部依赖**。
> **Phase 4:断言路径边界共享 + Rose-lite 左右截窗本地化已落地(C 内核存在性 2.13→2.44 MB/s、
> 纯 RE2 子集 3.21→3.83 MB/s、该路径内存降 65x),差分 oracle + 对抗性窗口测试全过。**
> 纯 Go、rune 级 Glushkov 位并行引擎(`BackendMVS`)落地;**真实 MITM 规则 NFA 覆盖 88.5%→96.6%
> (84/87:lean 77 + 断言 7,兜底 10→3)**;与 stdlib oracle 在 1332 条真实流量上存在性逐条一致;
> NFA 由自身算出**精确字节偏移与匹配内容**(leftmost-longest),与 `regexp.Longest().FindAllIndex`
> 逐字节一致;route-B 把 regexp2-only 必需字面量纳入预过滤,合并 always-on NFA 单趟扫描。
> **存在性热路径落成纯 C99 运行期内核 `native/mvscan/`,经平台无关小端 blob 与 Go 前端解耦,
> `-tags minirehs_mvs` 接入,内置 SSE2/NEON 多字 SIMD 加速档 + 标量孪生逐位一致;并由 `tools/amalgamate`
> 拼为"丢两个文件即可编"的单文件发行,在 darwin/arm64(clang/NEON)、linux/amd64(gcc/SSE2)、
> linux/arm64(gcc/NEON,qemu) 真实运行 + ASan/UBSan 干净,Windows/x64(MinGW) 编译链接通过。**
> **已落地(2026-06)**:Teddy SIMD 默认化(替标量 AC)、Rose-lite 双向锚定(前向 ∪ 反向锚定,消除"双无界
> keyword 类"整段扫;`mvs_reverse.go`/`mvs_bicover.go`,见 IMPL 0'.5)。cgocall 28%→11%、RE2-only 存在性
> +25%(CGO)/+32.5%(NoCGO)、零假阴(差分 oracle 全 1332 + 反向独立护栏)。
> **已落地(2026-06-23)**:regexp2→go-pcre2-lite **全局迁移**(yaklang 整库,原 ~40% CPU 绝对瓶颈消除),
> `MVS_Exist` 全规则存在性 3.86→**5.79 MB/s(+50%)**;minirehs 测试提速 477s→36s(差分迭代/语料/诊断
> 分档,见 `diff_iters_test.go` + 环境变量 `MINIREHS_DIFF_ITERS`/`MINIREHS_FULL_CORPUS`/`MINIREHS_DIAG`)。
> **当前性能(实测,vs Go RE2 逐条 0.18 MB/s 基线)**:存在性 **32x**(全规则)/**42x**(纯 RE2 子集)、定位 **16x**;
> dlopen 真 hyperscan 历史天花板 87x,故现实目标 = 存在性冲 80x(再 ~2x)。详见第 4' 节倍数评估与路线。
> 下一步(按收益):**R1 字面量门控合并单趟**(现最大头,结构性)→ R2 断言 NFA 合并 → R3 AVX2 → R5 定位 C 内核。
> 详见 IMPL 第 0'.4 / 0'.5 节 + 本文件第 4' 节。

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

## 1b. 已完成(匹配定位 / 内容上报)—— 2026-06-22

> 需求:除了判定"哪些规则命中",还要给出"匹配的内容是什么、匹配到哪里"。

- [x] **NFA 自定位执行器** `mvs_exec.go`:`findLocFrom`(带绝对起点的 leftmost-longest 单匹配)+
      `findAllLoc`(非重叠枚举)。在位并行递推上为每个活跃 position 维护"起点字节偏移"
      (后继继承、汇聚取最小、命中取最小起点/最大终点),所有"起点≤best 起点"线程消亡即停。
      不依赖 stdlib,由 NFA 自身算偏移;混合锚点(`^` 仅绝对 0 注入 / `$` 仅末尾接受)正确。
- [x] **后端上报偏移与内容** `mvs_backend.go`:NFA pattern 命中后用 `findAllLoc` 上报
      `Match{ID,From,To}`,内容即 `data[From:To]`;regexp2-only 兜底无法可靠给字节偏移,
      仍以 `-1/-1` 上报。新增 `WithReportLocation(bool)`(默认 true;false 回到纯存在性快路径)。
- [x] **定位差分验证** `mvs_location_test.go`:直接用例 + 随机(1 万+ NFA × 6 组任意字节,
      6 万+ 次)+ 真实流量(77 条 NFA 规则 × 1332 条记录)逐字节对照
      `regexp.Longest().FindAllIndex`;端到端复核 `data[From:To]` 确属该规则的真实匹配。

## 1c. 已完成(P1 route-B 字面量门控 / P2 合并 always-on)—— 2026-06-22

- [x] **P1 route-B** `literal_routeb.go`:对 regexp2-only pattern 做"语言只增不减"的 RE2 超集
      改写(移除零宽断言 / `\uXXXX`→`\x{XXXX}` / backref→`[\s\S]*` / 原子组·命名捕获→非捕获,
      无法确定为超集即 bail),再用现成 `extractRequiredLiterals` 提必需字面量并入预过滤。
      因 R_super⊇R_orig,提出的字面量必为 R_orig 命中的必要条件 → **绝不漏报**。always-on 11→9。
      护栏:`literal_routeb_test.go` 改写器单测 + 必需性属性测试(真实流量 + 4000 随机/结构化输入,
      "regexp2 命中⇒字面量出现")+ 既有 mvs/engine 全量差分。
- [x] **P2 合并 always-on NFA** `mvs_merged.go`:把无字面量且可编入 NFA 的 always-on 合并为
      一个全局字母表 + 单一活跃位集的不相交并集自动机,单趟扫描得命中集合(MITM 5 条→1 趟),
      命中后用各自 `findAllLoc` 定位。护栏:`mvs_merged_test.go` 合并命中集合 == 各成员单独
      `existsIn`(随机 + 真实流量,覆盖混合锚点 / 多字 / 多字节)。

### 关键结果快照

| 指标 | 值 |
|---|---|
| 真实 MITM 规则 NFA 覆盖率 | **96.6%(84/87:lean 77 + 断言 7);兜底 10→3**(2 条 regexp2-only URL + 1 条复杂 \b 多分支) |
| 存在性差分 | 1332 条 + 整段拼接,mvs/engine 与 stdlib oracle 逐条一致 |
| 定位差分 | 77 条 NFA × 1332 条记录,与 `regexp.Longest().FindAllIndex` 逐字节一致 |
| 随机差分 | 存在性 10289 条 + 定位 10241 条 NFA × 6 组任意字节,与 stdlib 一致 |
| **断言 NFA 差分** | `existsInAssert` == stdlib:定向用例 + 随机断言正则 × 任意输入 + 7 条真实恢复规则 × 1332 真实流量;带 tag oracle 全量复核 |
| route-B always-on | 11 → 9(`://`、`http` 两条 regexp2-only URL 规则被门控);必需性属性测试全过 |
| 合并 always-on | 5 条 NFA always-on → 单趟自动机(npos≈139,nword=3) |
| 吞吐(纯 Go,**带定位**) | mvs 0.96 MB/s,stdlib 0.17 → **mvs/stdlib 5.7x**(纯存在性档约 6.3x) |
| **C 内核差分(P4-M2)** | C existsIn == Go: 随机 10294 NFA×6 任意字节 + 77 NFA×1332 真实记录(10万+次);合并 1333 输入 C==Go;端到端 == oracle |
| **Teddy SIMD(P3)** | SIMD == 标量孪生 == 纯 Go AC:随机字面量集 × 任意输入 + 真实规则字面量 × 真实流量;teddyM 激活逻辑单测 |
| **bit-NFA SIMD(P4-M3)** | SSE2/NEON 多字(nword≥2)分发 == 标量孪生 == Go == stdlib:6 多字 NFA × 1200 检查 + 真实合并(nword=3)1333 输入逐位一致(arm64+amd64) |
| **单文件 amalgamation(P4-M4)** | `tools/amalgamate` 拼单文件 + 漂移护栏(产物 == 现场重生);真实流量 1333×87 校验器在 clang/NEON、gcc/SSE2、aarch64/qemu 上结果 == Go(767 merged / 2305 exists) |
| **C 内核跨平台矩阵** | darwin/arm64(clang/NEON)、linux/amd64(gcc12/SSE2)、linux/arm64(gcc12/NEON,qemu) 编译运行全绿 + ASan/UBSan 干净;windows/x64(MinGW gcc12) 编译链接通过(无 wine 未执行) |

---

## 1d. 已完成(P4 里程碑 M2:纯 C99 标量运行期内核 + 平台无关 blob)—— 2026-06-22

> 目标(IMPL 第 1/2/9 节):把存在性热路径落成**纯 C99、平台/CPU 无关(兼容+退化)**的运行期内核,
> 像 sqlite 一样只认 `blob + data`,经平台无关 blob 与 Go 前端解耦。本轮完成 M2(标量基线档),
> 与 Go 参考执行器逐位一致,并在 macOS/clang 与 linux/gcc 上编译运行 + ASan 验证。

- [x] **纯 C99 标量内核** `native/mvscan/mvscan.{h,c}`:零依赖(只用 `stdint/stddef/stdlib/string`),
      blob 零依赖解析 → rune 级 Glushkov 位并行 NFA;实现 `existsIn`(per-pattern 存在性)与
      `scanExist`(合并 always-on 单趟,发射 `posPat` 成员)。UTF-8 解码**逐字节复刻 Go
      `unicode/utf8.DecodeRune`**(first/acceptRanges 表照搬,非法→RuneError 单字节),字母表二分
      `symbolOf` 复刻 Go。`ctz64` 跨编译器(GCC/Clang `__builtin_ctzll` / MSVC `_BitScanForward64` /
      标量兜底)。当前为标量(任意架构可编可跑);SIMD 加速档(M3)将以函数指针分发叠加,配标量孪生。
- [x] **平台无关 blob 契约** `mvs_blob.go`(纯 Go,无 build tag):把 per-pattern NFA 与合并 NFA
      统一序列化为 `unit`(firstUnanchored/firstAnchored 承载锚点语义、posPat 承载命中位置→成员),
      小端显式编码、与机器字节序/对齐无关;头 `magic="MVS1"+version=1`,C 侧 `mvscan_db_open` 校验。
- [x] **cgo 接入 + 退化** `mvs_cgo.go`(`cgo && minirehs_mvs`)/ `mvs_stub.go`(否则):范式同
      `prefilter_cgo.go`(Go 序列化 blob、C 出力)。`mvsDB` 持 `kernel`;**有内核时存在性走 C
      (existsIn / merged scanExist),定位(`findAllLoc`)始终走已验证的 Go 路径**(符合 IMPL "存在性
      运行期内核" 定位);CGO_ENABLED=0 或不带 tag 时 `newMVSKernel` 返回 nil,全程纯 Go,零开销。
- [x] **差分护栏** `mvs_cgo_test.go`:① C `nfaExists` == Go `existsIn` == stdlib(随机 10294 NFA ×
      6 组任意字节含非法 UTF-8/多字节;真实 77 NFA × 1332 记录 + joined 共 10 万+ 次);② C
      `mergedScan` 命中集合 == Go `scanExist`(真实 1333 输入 + 4000 随机合并);③ 端到端
      `-tags minirehs_mvs`(C 内核驱动) == stdlib oracle(1332 记录)。
- [x] **跨平台 / 内存安全验证**:本机 `darwin/arm64`(clang) 带 tag 全包回归全绿;OrbStack docker
      `linux/amd64`(gcc 12) 带 tag 全包短回归 + 内核差分全绿;`linux/amd64` `go test -asan`(ASan+UBSan)
      跑内核差分干净(无越界/泄漏/UB)。`linux/arm64` 因本机 docker 凭证助手拉取多架构镜像挂起
      (环境问题,非代码)未能在容器内跑;arm64 指令集由原生 darwin/arm64 覆盖。big-endian 无可用镜像
      运行,但 blob 按字节显式小端解码、**设计上与字节序无关**(`le_u32/le_u64` 逐字节)。

### 验证命令(接力可复跑)

```bash
# 本机 (darwin/arm64, clang): 编译 + 带 tag 差分
CGO_ENABLED=1 go build -tags minirehs_mvs ./common/minirehs/
CGO_ENABLED=1 go test  -tags minirehs_mvs ./common/minirehs/ \
    -run 'TestMVSKernel' -count=1 -v        # 全部 C 内核差分
CGO_ENABLED=1 go test  -tags minirehs_mvs ./common/minirehs/ -short -count=1  # 整包

# 跨平台 (OrbStack docker, linux/amd64, gcc). 挂载主机 mod 缓存避免重新下载:
docker run --rm --platform linux/amd64 -e GOPROXY=off \
  -v $(go env GOPATH)/../Projects/yaklang:/src -v $(go env GOMODCACHE):/go/pkg/mod:ro \
  -v mvscache_amd64:/root/.cache/go-build -w /src golang:1.22 \
  sh -c 'CGO_ENABLED=1 go test -tags minirehs_mvs ./common/minirehs/ -run TestMVSKernel -count=1 -v'

# 内存安全 (linux/amd64 + ASan/UBSan):
docker run ... golang:1.22 \
  sh -c 'CGO_ENABLED=1 go test -asan -tags minirehs_mvs ./common/minirehs/ -run TestMVSKernel -count=1 -v'
```

---

## 1e. 已完成(零宽断言扩展 + P3 Teddy + P4-M3 SIMD bit-NFA + P4-M4 amalgamation/跨平台)—— 2026-06-22

### 零宽断言 NFA 扩展(覆盖率 88.5%→96.6%)
> 动机:`mvs_diag_test.go` 诊断 10 条兜底里 **7 条只因零宽断言**(`\b \B`、行锚 `^ $ \A \z`、
> `(?m)^ (?m)$`)而无法编入 NFA,是最高性价比的覆盖提升点。

- [x] **守卫式位并行自动机** `mvs_assert.go`:把 Glushkov 的 `first/follow/accept` 转移带上**边界守卫**
      (条件位掩码:文本始/末、行始/末、词边界/非词边界)。`existsInAssert` 在位并行递推每步按当前
      边界条件求值守卫,条件成立的转移才生效。`condBeginText/EndText/BeginLine/EndLine/WordBoundary/
      NoWordBoundary` + `guard/guardSet` + `boundaryConds/guardHolds` 实现;`compileMVSNFAAssert` 编译,
      `buildCondBits` 分离无条件/有条件位集。
- [x] **集成与边界** `mvs_glushkov.go`(`mvsNFA` 加 `hasAssert/condFirst/condFollow/condAccept`、
      `bnode` 加 `bAssert/acond`)、`mvs_exec.go`(`existsIn` 按 `hasAssert` 分发 `existsInAssert`)、
      `mvs_backend.go`(`tryCompileNFA` 先试 lean、失败再试断言;`assertAlwaysOn` 分类;命中后
      `reportViaVerifier` 用已验证 verifier 定位)。**断言 NFA 仅走 Go 存在性门;不入 C blob**
      (`mvs_blob.go` 对 `hasAssert` 置 `slotUnit=-1`),保持 C 内核简单。
- [x] **差分护栏** `mvs_assert_test.go`:`existsInAssert` == stdlib(定向用例 + 随机断言正则 × 任意输入
      `TestMVSAssertRandomDifferential` + 7 条真实恢复规则 × 1332 真实流量 `TestMVSAssertRecoversFallback`)。
      `mvs_test.go: TestMVSCoverageMITM` 现报 `lean=77 assert=7 fallback=3`。
- 结果:**total=87 nfa=84(96.6%)兜底 3**(2 条 regexp2-only URL 含 lookahead/unicode 区间 + 1 条
      超大 `\b` 多分支)。定位走 verifier 保证与 oracle 一致。

### P3 — 真正的 Teddy SIMD 多字面量预过滤
- [x] **Teddy 滤波器** `native/teddy.c`:重写为指纹(M=min(最短字面量,4))+ nibble 掩码 + 8 桶,
      SIMD(SSSE3 `PSHUFB` / NEON `TBL`,重叠非对齐载入)一次 16 字节算候选桶位掩码,候选位再 `memcmp`
      精确确认;`M<2` 或无字面量退 AC。`build_teddy`/`teddy_scan_{ssse3,neon,scalar}`/`teddy_scan`(运行期
      分发)/`mrehs_pf_scan`(Teddy 或 AC)。
- [x] **Go 接入** `prefilter_cgo.go`:`flattenLiterals` 把 `[]string` 摊成字节 + 偏移传 C;`useTeddy/teddyM`
      自省;`scanHitsScalar` 调标量孪生供差分。
- [x] **差分护栏** `teddy_cgo_test.go`:`TestTeddyEnabled`(激活逻辑 + teddyM)、`TestTeddyDifferentialRandom`
      (SIMD == 标量孪生 == 纯 Go AC,随机字面量集 × 任意输入)、`TestTeddyRealTrafficVsGoAC`(真实规则字面量
      × 真实流量)。**预过滤只增假阳,绝不漏**——命中集合与 Go AC 完全一致。

### P4-M3 — bit-NFA 多字 SIMD(SSE2/NEON)+ 标量孪生 + 逐位差分
- [x] **SIMD 字向量原语** `native/mvscan/mvscan.c`:`row_copy/or/and` 标量(`_s`)与 SIMD(`_v`,SSE2
      `_mm_*` / NEON `v*q_u64`,一次 2 字 = 128 位)两套;`mvscan_run.inc` 用 `ROW_*`/`NFA_RUN_NAME` 宏
      模板把递推体生成 `nfa_run_scalar` 与 `nfa_run_simd` 两份(其余 rune 解码/接受/早停完全同源)。
- [x] **运行期分发** `nfa_run_dispatch`:`nword>=2` 且编入 SIMD 档走 SIMD(一次 2 字),否则标量;
      `forceScalar` 强制标量孪生。新增 C API `mvscan_db_{nfa_exists,merged_scan}_scalar` +
      `mvscan_simd_enabled`,Go 侧 `mvs_cgo.go` 暴露 `nfaExistsScalar/mergedScanScalar/simdEnabled`。
- [x] **逐位差分** `mvs_cgo_test.go`:`TestMVSKernelSIMDScalarTwin`(6 多字 NFA × 1200 检查,SIMD==标量==Go)、
      `TestMVSKernelMergedSIMDScalarTwin`(真实合并 nword=3,1333 输入逐位一致)。arm64(NEON)+ amd64(SSE2)
      均逐位一致。

### P4-M4 — 单文件 amalgamation + 跨平台编译矩阵
- [x] **拼接工具** `tools/amalgamate/`:`amalgamate.go`(库 `Build`:把 `mvscan_run.inc` 的两处 `#include`
      就地内联两份、保留对公共头 `mvscan.h` 的 include,消除项目内 include)+ `cmd/amalgamate`(写
      `native/mvscan/amalgamation/{mvscan.c,mvscan.h}`)。产物"宿主丢两个文件 + 一条命令即可编"。
- [x] **漂移护栏** `mvs_amalgamation_test.go`(纯 Go,任意构建都跑):提交的单文件必须与从 `native/mvscan`
      源现场重生**逐字节一致**,否则失败并提示重生命令——杜绝改源忘重生。
- [x] **真实负载校验器** `native/mvscan/amalgamation/fixture_check.c` + `mvs_fixture_emit_test.go`
      (`MVS_EMIT_FIXTURE=1` 导出真实 blob + 1333 真实流量 + Go 期望):独立 C 程序(只编 `mvscan.c` + 校验器,
      零依赖)对真实流量跑合并扫描 + 逐 pattern 存在性,累计命中 == Go 参考(**767 merged / 2305 exists**)。
- [x] **跨平台矩阵**(均通过):
      - darwin/arm64 **clang/NEON**:smoke + 校验器 OK;**ASan+UBSan 干净**;全 Go 内核套件(amalg tag)全绿。
      - linux/amd64 **gcc12/SSE2**(docker):smoke + 校验器(-O1/-O3)OK;**ASan+UBSan 干净**;全 Go 内核套件
        (amalg tag,含 36s 端到端 oracle)全绿。
      - linux/arm64 **gcc12/NEON**(docker + qemu-user):校验器 OK + UBSan 干净(绕开此前多架构镜像凭证挂起)。
      - windows/x64 **MinGW gcc12**(docker 交叉):`smoke.exe`/`fixcheck.exe` 编译链接通过(无 wine,未执行运行)。
- [x] **真实运行期验证而非仅编译**:`mvs_cgo.go` 的 C include 条件化(`MVS_USE_AMALGAMATION`),
      `mvs_cgo_amalg.go`(`minirehs_mvs_amalg` tag 注入 `-DMVS_USE_AMALGAMATION`)使**同一套差分/oracle 矩阵
      直接打单文件发行件**:`go test -tags 'minirehs_mvs minirehs_mvs_amalg'` 在 arm64 + amd64 全绿。
- **诚实记录的限制**:① MSVC 原生未机验(用 MinGW 覆盖 Windows/x64 ABI + SSE2 + 链接;`_MSC_VER`
      `_BitScanForward64` 分支按文档书写但无 MSVC 实测);② Windows 运行未执行(仅编译链接,无 wine);
      ③ big-endian 无可用镜像运行,但 blob 逐字节显式小端解码、**设计上与字节序无关**。

### 验证命令(1e,接力可复跑)

```bash
# 断言扩展 + 覆盖率 + 回退原因(纯 Go)
go test ./common/minirehs/ -run 'TestMVSAssert|TestMVSCoverageMITM|TestMVSFallbackReasons' -count=1 -v

# P3 Teddy / P4-M3 SIMD bit-NFA 孪生差分(本机 darwin/arm64, NEON)
CGO_ENABLED=1 go test -tags minirehs_cgo  ./common/minirehs/ -run 'TestTeddy' -count=1 -v
CGO_ENABLED=1 go test -tags minirehs_mvs  ./common/minirehs/ -run 'TestMVSKernelSIMDScalarTwin|TestMVSKernelMergedSIMDScalarTwin' -count=1 -v

# P4-M4 单文件:重生 + 漂移护栏 + 单文件运行期(同一差分/oracle 矩阵)
go run ./common/minirehs/tools/amalgamate/cmd/amalgamate         # 重生发行单文件
go test ./common/minirehs/ -run TestMVSAmalgamationFresh -count=1 -v
CGO_ENABLED=1 go test -tags 'minirehs_mvs minirehs_mvs_amalg' ./common/minirehs/ -run TestMVSKernel -count=1

# 单文件独立编译 + 真实负载校验器(零依赖, 丢两个文件即可编)
MVS_EMIT_FIXTURE=1 MVS_FIXTURE_DIR=/tmp/mvs_fixture \
  go test ./common/minirehs/ -run TestMVSEmitKernelFixture -count=1   # 导出 blob+真实流量+期望
cd common/minirehs/native/mvscan/amalgamation
clang -O2 -std=c99 -Wall -Wextra mvscan.c fixture_check.c -o /tmp/fc && /tmp/fc /tmp/mvs_fixture
clang -O1 -g  -std=c99 -fsanitize=address,undefined -fno-sanitize-recover=all \
      mvscan.c fixture_check.c -o /tmp/fc_san && /tmp/fc_san /tmp/mvs_fixture   # ASan+UBSan

# 跨平台 (OrbStack docker). amd64/SSE2 真实负载 + ASan/UBSan:
docker run --rm --platform linux/amd64 \
  -v $PWD:/amalg:ro -v /tmp/mvs_fixture:/fixture:ro -w /tmp golang:1.22 bash -c '
  cp /amalg/mvscan.c /amalg/mvscan.h /amalg/fixture_check.c .
  gcc -O1 -g -std=c99 -fsanitize=address,undefined -fno-sanitize-recover=all mvscan.c fixture_check.c -o fc && ./fc /fixture'
# linux/arm64 (gcc/NEON, 经 qemu-user, 绕开多架构镜像凭证挂起):
docker run --rm --platform linux/amd64 -v $PWD:/amalg:ro -v /tmp/mvs_fixture:/fixture:ro -w /tmp golang:1.22 bash -c '
  apt-get update -qq && apt-get install -y -qq gcc-aarch64-linux-gnu libc6-dev-arm64-cross qemu-user-static
  cp /amalg/mvscan.c /amalg/mvscan.h /amalg/fixture_check.c .
  aarch64-linux-gnu-gcc -O2 -std=c99 -static mvscan.c fixture_check.c -o fc_arm64 && qemu-aarch64-static ./fc_arm64 /fixture'
# windows/x64 (MinGW, 编译链接):
#   apt-get install -y gcc-mingw-w64-x86-64 && x86_64-w64-mingw32-gcc -O2 -std=c99 mvscan.c fixture_check.c -o fc.exe
```

---

## 2. 必须遵守的正确性不变量(改代码前务必读)

1. **命中语义(已扩展为可定位)**:默认对可编入 NFA 的 pattern 上报精确字节区间
   `Match{ID, From, To}`(`data[From:To]` 即匹配内容,leftmost-longest 语义);regexp2-only 兜底
   仍报 `Match{ID, -1, -1}`。`WithReportLocation(false)` 可整体退回纯存在性(全报 -1/-1)。
   - **存在性差分**(`mvsExistIDs`/`mvsAssertSameIDSet`):只按命中 ID 集合比较,跨 NFA/兜底/定位
     开关都成立,是主护栏。
   - **定位差分**:NFA 路径的 `findAllLoc` 必须与 `regexp.Longest().FindAllIndex` 逐字节一致
     (`mvs_location_test.go`)。触及 `findLocFrom`/锚点/解码者必须重跑定位随机差分 + 真实流量定位差分。
2. **预过滤只能放过更多,绝不能漏**:任何字面量/预过滤是"必要条件"(允许假阳,真伪由验证判定)。
   route-B 提取的字面量必须保证"命中必含其一",否则漏报(假阴)=正确性事故。
3. **rune 解码必须与 Go regexp 逐位一致**:用 `utf8.DecodeRune`。任何触及解码/类/符号映射的改动,
   都要跑随机差分(任意字节)+ 1332 条真实差分。
4. **回退不丢规则**:不能编入 NFA 的 pattern 必须落 `verifier`(`mvs_backend.go: tryCompileNFA`
   返回 nil 即兜底)。
5. **锚点 / 断言策略**:lean NFA 仅剥离顶层 concat 两端的 `^/\A`(anchoredStart)与 `$/\z`
   (requireEnd)。**零宽断言(`\b \B`、中缀/多行 `^ $`、`\A \z`、`(?m)^ (?m)$`)现由
   `compileMVSNFAAssert` 的守卫式自动机支持(`hasAssert`),走 Go `existsInAssert`、定位走 verifier、
   且不入 C blob**(`mvs_blob.go` 置 `slotUnit=-1`)。仍兜底:backref / lookaround / 仍编不出的复杂式。
   **可空根(能匹配空串)与空 first → 兜底**。触及 `mvs_assert.go`/守卫求值者必须重跑
   `mvs_assert_test.go`(随机断言差分 + 真实恢复规则)。
6. **SIMD 必须配标量孪生且逐位一致**:任何 SIMD 档(Teddy `native/teddy.c`、bit-NFA 多字
   `nfa_run_simd`)都有等价标量孪生(`*_scalar` / `teddy_scan_scalar`),改动后必跑孪生逐位差分
   (`mvs_cgo_test.go` SIMD twin、`teddy_cgo_test.go`)。SIMD 只能加速、不得改变命中。
7. **单文件 amalgamation 不得手改、不得漂移**:`native/mvscan/amalgamation/{mvscan.c,mvscan.h}` 是
   `tools/amalgamate` 从 `native/mvscan` 源生成的产物。改源后必重生(`go run
   ./common/minirehs/tools/amalgamate/cmd/amalgamate`),否则 `TestMVSAmalgamationFresh` 失败。
8. **vectorscan/libhs 对照桥已整体移除(2026-06)**:按"只允许自研、不加载任何动态库"的硬要求,
   `vectorscan_bridge.go` / `vectorscan_stub.go` / `vectorscan_test.go` / `bench_vectorscan_test.go`
   已删除,`BackendVectorscan` 枚举与 `rehs.backend("vectorscan")` 入口同步清除。**禁止再引入任何
   外部动态库依赖**(链接期或 dlopen 皆禁);mvscan 内核源码随仓库分发。
9. **存在性本地化(Rose-lite 左右截窗)必须零假阴**:`mvs_window.go` 仅对 RE2-exact、非锚定的
   字面量门控 pattern 收窄 `nfaExists` 窗口;锚定 / 尾部无界者安全退回整段。改动 `mvs_window.go` 或
   `scan` 窗口逻辑者必跑 `mvs_window_test.go`(对抗性窗口健全性 + 长随机填充)防本地化漏报。

---

## 3. 代码坐标(接力时快速定位)

- `mvs_glushkov.go`
  - `synToRune`:支持的 RE2 op 列表(改这里扩/缩 NFA 适用范围)。
  - `compileMVSNFA`:总入口——锚点剥离 → 构造 → 可空根/空 first/`over` 兜底 → 字母表 → 快路径表。
  - `buildAlphabet` / `symIndex` / `symbolOf`:字母表压缩与 rune→符号映射。
  - `stripEndAnchors`:顶层首尾锚处理。
- `mvs_exec.go`:`existsIn`/`existsIn1`(存在性,可早停);`findLocFrom`/`findAllLoc`(**定位**,
  leftmost-longest,带绝对起点与起点追踪)。位并行递推公式见各函数头注释。
- `mvs_merged.go`(**P2**):`buildMergedNFA`(成员不相交并集 + 全局字母表)、`mvsMergedNFA.scanExist`
  (单趟得命中集合)、`mvsNFA.posRanges`(从压缩字母表反推位置类)。
- `literal_routeb.go`(**P1**):`extractRequiredLiteralsApprox`(入口)、`re2Superset`(超集改写,
  bail-heavy)、`rewriteGroupOpen`/`rewriteEscape`/`copyClass`/`findGroupClose`。
- `mvs_backend.go`
  - `tryCompileNFA`:regexp2-only / parse 失败 / `!ok` → nil(兜底)。
  - `mvsBackend.compile`:有字面量→预过滤;无字面量 NFA→合并(`merged`);无字面量兜底→`otherAlwaysOn`。
  - `mvsDB.scan`:① 字面量预过滤门控 → ② 合并 always-on NFA 单趟 → ③ 兜底逐条。
  - `verifyOne` / `reportLocated`:`reportLoc` 为真时 `findAllLoc` 报 `Match{ID,From,To}`;否则
    `existsIn` 报 `Match{ID,-1,-1}`;兜底走 `verifier`。**有 `kernel` 时存在性走 C,定位仍走 Go。**
- **P4-M2 纯 C 内核**:
  - `native/mvscan/mvscan.h`:C API(`mvscan_db_open/close/nfa_exists/merged_scan/npat/has_merged`)。
  - `native/mvscan/mvscan.c`:blob 解析(`parse_unit`/`mvscan_db_open`)、UTF-8 解码(`mvs_decode_rune`,
    复刻 Go)、字母表二分(`sym_index`/`symbol_of`)、位并行递推(`nfa_run`,mode 0 存在性 / 1 合并发射)。
  - `mvs_blob.go`(纯 Go,无 tag):`buildMVSBlob`(整库)、`unitFromNFA`/`unitFromMerged`、`encodeUnit`
    (布局须与 `parse_unit` 严格对齐,改一处必改两处)、小端 `putU32/putU64/...`。
  - `mvs_cgo.go`(`cgo && minirehs_mvs`):`mvsKernel`(持 `*C.mvscan_db`)、`newMVSKernel`/`openMVSKernel`、
    `nfaExists`/`mergedScan`/`close`(范式同 `prefilter_cgo.go`)。
  - `mvs_stub.go`(`!cgo || !minirehs_mvs`):`mvsKernel` 空类型 + `newMVSKernel` 返回 nil,纯 Go 退化。
  - `mvs_cgo_test.go`(tag):C==Go==oracle 三类差分护栏(`getMVSDB` 取内部 `*mvsDB`)+ SIMD/标量孪生逐位差分。
- **零宽断言扩展(1e)**:
  - `mvs_assert.go`:守卫式 Glushkov 构造 + 执行(`compileMVSNFAAssert`/`existsInAssert`/`boundaryConds`/
    `guard`/`buildCondBits`)。边界条件:文本始末、行始末、词边界/非词边界。
  - `mvs_glushkov.go` 增量:`mvsNFA.{hasAssert,condFirst,condFollow,condAccept}`、`bnode.{bAssert,acond}`。
  - `mvs_diag_test.go`:`TestMVSFallbackReasons` 按未支持算子分类兜底(诊断高性价比扩展点)。
  - `mvs_assert_test.go`:断言差分(定向 + 随机断言正则 + 真实恢复规则)。
- **P3 Teddy SIMD**:`native/teddy.c`(`build_teddy`/`teddy_scan_{ssse3,neon,scalar}`/`teddy_scan`)、
  `prefilter_cgo.go`(`flattenLiterals`/`useTeddy`/`teddyM`/`scanHitsScalar`)、`teddy_cgo_test.go`。
- **P4-M3 SIMD bit-NFA**:`native/mvscan/mvscan_run.inc`(递推体模板,`ROW_*`/`NFA_RUN_NAME` 宏)、
  `mvscan.c` 的 `row_*_{s,v}`/`nfa_run_{scalar,simd}`/`nfa_run_dispatch`、`mvscan.h` +
  `mvs_cgo.go` 的 `*_scalar`/`mvscan_simd_enabled` 孪生入口。
- **P4-M4 amalgamation**:`tools/amalgamate/{amalgamate.go,cmd/amalgamate/main.go}`(生成器)、
  `native/mvscan/amalgamation/{mvscan.c,mvscan.h,example_smoke.c,fixture_check.c}`(发行单文件 +
  独立校验器)、`mvs_amalgamation_test.go`(漂移护栏,纯 Go)、`mvs_fixture_emit_test.go`
  (`MVS_EMIT_FIXTURE=1` 导出 sanitizer fixture)、`mvs_cgo.go`(`MVS_USE_AMALGAMATION` 条件 include)+
  `mvs_cgo_amalg.go`(`minirehs_mvs_amalg` tag 注入 `-DMVS_USE_AMALGAMATION`)。
- 复用件:`prefilter.go`(`buildLiteralIndex`/`newPrefilter`/`litHit`)、`ahocorasick.go`、
  `literal.go`(`extractRequiredLiterals`,RE2 树版,route-B 可借鉴/改造)、`verifier.go`、
  `width.go`、`feature_gate.go`。
- 测试复用:`consistency_test.go`(`fixedPatterns`/`randomCorpus`/`compilableMITMPatterns`)、
  `testdata_helper_test.go`(`mitmPatterns`/`loadCorpus`)。mvs 自带 `mvsExistIDs`/`mvsAssertSameIDSet`
  存在性差分辅助(原 vectorscan 测试随 bridge 移除)。

---

## 4. 当前瓶颈(数据驱动,profile 已确认)

> **2026-06 更新**:Teddy 默认化 + Rose-lite 双向锚定(见 0'.5 节 / `mvs_reverse.go`)已落地。后者把
> `runtime.cgocall`(C 内核整段扫)从 ~28% 砍到 ~11%、C `nfaExists` 扫描字节 16.35MB→3.93MB(降 76%)。
> **2026-06-23 更新**:regexp2→go-pcre2-lite 全局迁移已落地(原 ~40% CPU 绝对瓶颈消除),`MVS_Exist`
> 全规则存在性 3.86→**5.79 MB/s(+50%)**。此后瓶颈重新分布如下(见 4' 节实测)。

- **regexp2 前端**(原最大头 ~40% CPU,**已解决**):**【2026-06-23 已落地】** yaklang 全局把 regexp2 后端从
  `dlclark/regexp2`(.NET 回溯)切换为 `go-pcre2-lite/regexp2`(PCRE2,线性时间)。`regexp2Verifier` 经
  `regexp_utils.YakRegexpUtils.Match()` 自动享有该加速,无需 minirehs 内旁路。
- **字面量门控 pattern 逐条 existsIn(现最大头,结构性)**:每条字面量命中的 lean pattern 各自调一次
  C `nfaExists`/Go `existsIn`。合并 NFA(`mvs_merged.go`)目前只覆盖"无字面量 always-on";门控 pattern
  尚未合并 ⇒ N 条门控 = N 次 cgo 跨界 + N 趟位递推。这是逼近 80x 的下一个主战场(见 4' 节 R1)。
- 断言 NFA always-on(`existsInAssertShared` ~9.5% + `existsInAssertAnchored` ~4%):IMPL 0'.4 #2 的"断言 NFA 合并"。
- 前向/反向锚定(`existsInAnchored*` ~15% + `existsInReverseAnchored` ~5%):双向锚定本体,已是收窄后的局部扫描。
- **定位档(`findAllLoc`)始终走 Go**:C 内核只加速存在性;`MVS_Located` 16x 的天花板在此(见 4' 节 R5)。

---

## 4'. 性能倍数评估与冲击 80x/100x 的路线(2026-06-23,实测)

> 基线 `StdlibLoop` = Go RE2 逐条扫 = **0.18 MB/s**(即"N 条正则逐条匹配"的等价基线)。
> 实测条件:全量 5.2MB / 1332 条真实流量 + 87 条真实 MITM 规则,C 内核档(`-tags minirehs_mvs`),
> `MINIREHS_FULL_CORPUS=1`,`benchtime=3x`。复现:见 0.3 节命令 + 本节脚注。

| 模式 | 吞吐 | vs 基线 | 场景 |
|---|---|---|---|
| `MVS_Exist_RE2only`(纯 RE2 子集,存在性) | **7.60 MB/s** | **≈42x** | 最干净对比 |
| `MVS_Exist`(全 87 规则,存在性) | **5.79 MB/s** | **≈32x** | MITM 打标 |
| `MVS_Located`(全规则,精确字节偏移) | **2.95 MB/s** | **≈16x** | 替换/标注 |

**诚实结论**:
- 当前**存在性 32~42x、定位 16x**。这是"纯算法 + 零外部依赖 + 全平台可移植"下的数字。
- 历史上 dlopen 真 vectorscan 曾测得 **87x**(已移除)——那是本类的**实际天花板参照**。即 "80x ≈ 追平真
  hyperscan","100x+ 需超越简化移植难以企及","200x 在本语料(平均 ~3.9KB/记录,每记录 Go 侧编译/分发
  开销占比高)上不现实"。
- 故现实目标设为 **存在性冲 80x(再 ~2x)**,定位档单独补 C 内核可从 16x 提到 ~30x。

**到 80x 的杠杆(按收益/确定性排序,均未落地)**:

- **R1 — 字面量门控 pattern 合并单趟(最大头,结构性,预期最高收益)**:把所有"字面量门控的 lean NFA"
  并入一台带 per-position 接受标注的合并自动机(扩 `mvs_merged.go` 现仅覆盖 always-on 的能力),用一次
  Teddy 预过滤的命中位置集合驱动单趟位递推,消除 N 条门控 = N 次 cgo + N 趟扫的结构性开销。难点:门控
  语义(命中字面量才激活该成员)要映射成合并自动机的"位置注入掩码";护栏:合并命中集合 == 各成员单独
  existsIn(仿 `mvs_merged_test.go`)。预期把 `MVS_Exist` 推向 ~10+ MB/s 量级。
- **R2 — 断言 NFA 合并单趟(中等,确定性)**:IMPL 0'.4 #2。带 guard(`condFirst/condFollow/condAccept`)
  的断言 NFA 仿合并法收成一趟,削 ~13% 断言路径 CPU。护栏:guard 合并专项差分。
- **R3 — AVX2 档(纯吞吐,平台相关)**:现 SSE2/NEON 一次 16 字节;AVX2 一次 32 字节,宽扫面预期 +1.5~2x。
  需新增 AVX2 标量孪生差分 + 运行期 `cpuid` 探测(`mvscan_simd_enabled` 扩展)。
- **R4 — Rose 子串图多跳链(>2 字面量,工程量大)**:双无界已由双向锚定吃掉;剩"中间子串级编排"(>2
  必需字面量按图边逐跳验证)压低误触发率。收益视规则膨胀程度。
- **R5 — 定位档 C 内核(独立收益,把 16x→~30x)**:现 `findAllLoc` 始终走 Go。给 leftmost-longest 定位
  补一条 C 路径(位递推 + 起点追踪),需与 Go oracle 逐字节差分(`mvs_location_test.go` 已有护栏框架)。
  仅替换场景受益;打标场景用存在性档即可,无需此项。

**脚注(复现命令)**:
```bash
# 存在性三档(C 内核, 全量语料)
CGO_ENABLED=1 MINIREHS_FULL_CORPUS=1 go test -tags minirehs_mvs ./common/minirehs/ \
    -run '^$' -bench BenchmarkMVSExistence -benchtime=3x -count=1
# 基线(StdlibLoop, 全量语料)
CGO_ENABLED=1 MINIREHS_FULL_CORPUS=1 go test -tags minirehs_mvs ./common/minirehs/ \
    -run '^$' -bench BenchmarkMITMRealTraffic -benchtime=3x -count=1
```

---

## 5. 待办(按优先级 / M2→M5 路线)

> 工作法:每完成一步,先跑第 0.2 节回归(尤其 1332 条真实差分 + 随机差分),再跑 0.3 节看数字。
> **先写护栏,再动优化。**

### P1 — route-B 近似字面量门控 ✅ 已完成(2026-06-22,见 1c 节)
- 实现:`literal_routeb.go` 用"语言只增不减"的 RE2 超集改写 + `extractRequiredLiterals`;集成点在
  `database.go: Compile` 的 regexp2-only 分支(`extractRequiredLiteralsApprox`),对 mvs 与 engine 同时生效。
- 结果:always-on 11→9(`参数-URL设计` 提到 `://`、`Url信息` 提到 `http`);必需性属性测试 +
  全量差分全绿。
- 漏报风险已用"超集证明 + 必需性属性测试 + 双后端全量差分"三重护栏覆盖。Email 规则因唯一定长字符
  `@` 长度 1<minLen 仍 always-on(预期)。

### P2 — 合并单趟 always-on NFA ✅ 已完成(2026-06-22,见 1c 节)
- 实现:`mvs_merged.go` 把无字面量且可编入 NFA 的 always-on 合并为单一全局字母表 + 单活跃位集的
  不相交并集自动机(`posRanges` 反推位置类、混合锚点拆 firstUnanchored/firstAnchored 与 lastAny/lastEnd、
  `posPat` 映射命中位置→成员)。`mvsBackend.compile`/`scan` 接入,命中后各自 `findAllLoc` 定位。
- 结果:MITM 5 条 always-on NFA 合并为 1 趟;`mvs_merged_test.go` 合并命中集合 == 各成员单独 existsIn。

### P3 — Teddy SIMD 多字面量预过滤 ✅ 已完成(2026-06-22,见 1e 节)
- 实现:`native/teddy.c` 重写为真 Teddy(指纹 + nibble 掩码 + 8 桶,SSSE3 `PSHUFB`/NEON `TBL` 一次
  16 字节 + `memcmp` 确认,`M<2` 退 AC);`prefilter_cgo.go` 接入 + `flattenLiterals`。
- 护栏:`teddy_cgo_test.go` SIMD == 标量孪生 == 纯 Go AC(随机 + 真实流量),命中集合完全一致。

### P4 — 纯 C 运行期内核 + blob + SIMD 分发 + amalgamation ✅ 全部完成
- **M2(标量内核 + 平台无关 blob + cgo 接入 + 退化 + 差分,见 1d 节)**:`native/mvscan/mvscan.{h,c}`、
  `mvs_blob.go`、`mvs_cgo.go`、`mvs_stub.go`、`mvs_cgo_test.go`。与 Go 参考执行器逐位一致。
- **M3(SIMD 加速档,见 1e 节)**:bit-NFA 多字(`nword>=2`)SSE2/NEON 字向量 OR/AND/COPY,`mvscan_run.inc`
  模板生成 `nfa_run_scalar/simd` + `nfa_run_dispatch` 运行期分发 + 标量孪生逐位差分(arm64+amd64)。
  注:Teddy 预过滤 SIMD(P3,`native/teddy.c`)与 mvscan bit-NFA SIMD 现为两个 TU,各自配标量孪生差分;
  二者尚未合并到同一 `dispatch.c` 框架(非必需,功能与护栏均已就位)。AVX2 档未做(SSE2/NEON 已覆盖
  目标平台基线;AVX2 可作后续纯吞吐优化,需新增标量孪生差分)。
- **M4(单文件 amalgamation + 跨平台矩阵,见 1e 节)**:`tools/amalgamate` 生成单文件 + 漂移护栏
  `mvs_amalgamation_test.go` + 真实负载校验器 `fixture_check.c`;矩阵 darwin/arm64(clang/NEON)、
  linux/amd64(gcc/SSE2)、linux/arm64(gcc/NEON,qemu)运行 + ASan/UBSan 干净,windows/x64(MinGW)
  编译链接通过。`minirehs_mvs_amalg` tag 让同一差分/oracle 矩阵直接验证单文件运行期。
- 文件结构偏差:IMPL 第 15 节规划 `native/mvscan/{blob.c,nfa_exec.c,teddy.c,dispatch.c,...}` 多文件;
  实际以单文件 `mvscan.c`(+ `mvscan_run.inc` 模板 + `mvscan.h`)+ 独立 `native/teddy.c` 落地,并由
  `tools/amalgamate` 产出发行单文件——等价达成 amalgamation 目标,结构更简。

### P5 — 平台收敛 + 接入切换(M4/M5)— ✅ 完成(bridge 已移除)
- **接入切换现状(已实现并文档化)**:
  - 默认构建(无 tag / `CGO_ENABLED=0`):纯 Go `BackendMVS`,`newMVSKernel` 返回 nil,零 C 依赖。
  - `-tags minirehs_mvs`(+ cgo):存在性热路径走纯 C99 内核(per-pattern existsIn + 合并 scanExist),
    内置 SSE2/NEON SIMD 加速档;**定位始终走已验证 Go `findAllLoc`**;纯 Go 同时作退化与差分 oracle。
  - `-tags 'minirehs_mvs minirehs_mvs_amalg'`:同上,但 C 侧编入单文件 amalgamation 发行件(验证发行件运行期)。
  - 断言 NFA 与 regexp2-only 兜底始终走 Go(不入 C)。
- 平台收敛:SSE2(x86_64 基线)+ NEON(arm64 基线)无需运行期探测;其它架构自动退标量孪生
  (`mvscan_simd_enabled()` 自省)。跨架构矩阵见 1e 节。
- **bridge 移除已完成(2026-06)**:`vectorscan_bridge.go`/`vectorscan_stub.go`/`vectorscan_test.go`/
  `bench_vectorscan_test.go` 全部删除,枚举/选择/API/scratch 字段同步清理;三档构建(CGO 默认 /
  `CGO_ENABLED=0` / `-tags minirehs_mvs`)回归全绿。
- **剩余(需人工验收决策,非代码)**:在目标 CI 固化跨架构编译/测试任务;MSVC 原生编译与
  Windows 运行需有 Windows 环境时补机验(代码 `_MSC_VER` 分支已就位)。

### Phase 4 — 断言边界共享 + Rose-lite 本地化 ✅ 已落地(2026-06-22)
- **断言路径边界共享**(`mvs_assert.go` `existsInAssertShared` + `scratch.assertBound`):每报文惰性算一次
  `computeBoundaries`,多条零宽断言 NFA 复用,省去逐 pattern 重复 rune 解码/边界计算。
- **存在性本地化 / Rose-lite 左右截窗**(`mvs_window.go` `computeLitWindow` + `scan` 改造):RE2-exact、
  非锚定的字面量门控 pattern 命中后,按 AST 上下文宽度算 `[head,tail]` 界,整段 `nfaExists` 收窄到
  命中点邻域的 union 窗口;锚定 / 尾部无界者安全退回整段(零假阴,`mvs_window_test.go` 对抗护栏)。
- 实测(darwin/arm64,C 内核):存在性 2.13→**2.44 MB/s**、定位 1.52→**1.75**、纯 RE2 子集 3.21→**3.83**;
  纯 RE2 子集该路径内存 29.5→**0.45 MB/op(降 65x)**。
### Phase 5 — Rose-lite 双向锚定(前向 ∪ 反向)✅ 已落地(2026-06)
- **反向 NFA**(`mvs_reverse.go` `reverseBnode`/`compileReverseExprToNFA`):结构反转 rune 级 `bnode` 树后复用
  `glushkovNFA`,得反转语言的同构自动机;`existsInReverseAnchored`(`utf8.DecodeLastRune` 自尾向头扫 + rune-end
  注入 + 提前消亡)。
- **可救性分析**(`mvs_bicover.go` `computeLitBiCover`):per-occurrence 两侧界,判"每出现处 head 或 tail 至少一侧
  有界";头有界出现处→前向锚定、尾有界出现处→反向锚定,并集 = 全部匹配(零假阴代数保证)。
- **接线**(`mvs_backend.go` `biAnchorable`/`revNFAs`/`litBiHead`/`litBiTail` + `scan` 双向锚定批处理):替换
  双无界 keyword 类(`参数-用户名/敏感参数/密码泄露` 等 9 条,含 3 大热点)的整段 C 扫。
- **零假阴护栏**(`mvs_reverse_test.go`):反向 NFA(全区间注入)== 正向 `existsIn`,6407 随机正则×含非法 UTF-8 +
  74 真实 MITM×1332 记录,零 mismatch(证 `DecodeLastRune` 与 `DecodeRune` 切分一致)。
- 实测(A/B `biAnchorEnabled`):RE2-only 存在性 +25%(CGO 6.04→7.55)/+32.5%(NoCGO 5.66→7.50);`runtime.cgocall`
  28%→11%;C `nfaExists` 字节 16.35MB→3.93MB(降 76%);差分 oracle 全 1332(定位+存在性)+ 一致性 87 条全绿。
- **下一步(未落地,按收益)**:前端 regexp2→go-pcre2-lite(现 ~40% CPU 绝对瓶颈,**单独 PR**)→ 断言 NFA 合并单趟
  → Rose 子串图多跳链。详见 IMPL 第 0'.4 / 0'.5 节。

### 可选 — 扩 NFA 覆盖率 ✅ 已完成(2026-06-22,见 1e 节零宽断言扩展)
- 词边界 `\b\B` / 行锚 `(?m)^$` / 中缀 `^$` / `\A\z` 已由守卫式自动机 `mvs_assert.go` 支持,
  覆盖率 88.5%→**96.6%**(兜底 10→3)。余 3 条为 2 条 regexp2-only URL(lookahead/unicode 区间)+
  1 条超大 `\b` 多分支(断言编译仍 bail),保留兜底。

### Phase 6 — regexp2→go-pcre2-lite 全局迁移 ✅ 已落地(2026-06-23)
- **范围**:yaklang 整库(~20 个 .go 文件)把 `github.com/dlclark/regexp2` 换成
  `github.com/VillanCh/go-pcre2-lite/regexp2`(PCRE2,线性时间)。`go.mod` 锁 `v0.1.0`(正式 release,无 replace)。
- **minirehs 受益**:`regexp2Verifier` 经 `regexp_utils.YakRegexpUtils.Match()` 自动享有,无需内部旁路;
  实验期的 `verifier_pcre2*.go`/`mvs_pcre2_test.go` 已回收。
- **实测**:`MVS_Exist` 全规则存在性 3.86→**5.79 MB/s(+50%)**,消除原 ~40% CPU 的 regexp2 回溯瓶颈。
- **行为变化**:PCRE2 支持 POSIX 字符类 `[[:alpha:]]`(dlclark/.NET 不支持),已更新 regexp-utils 相应测试期望。
- **遗留**:`dlclark/regexp2` 仅作 `dop251/goja`(JS 引擎)传递依赖残留 go.mod `// indirect`,与 yaklang 正则路径无关。

### Phase 7 — 冲击 80x(存在性)路线 — 待落地(按收益/确定性排序,详见第 4' 节)
- **R1 字面量门控 pattern 合并单趟**(现最大头,结构性,预期最高收益):扩 `mvs_merged.go` 覆盖门控 lean NFA,
  Teddy 命中位置驱动单趟位递推,消除 N 条门控 = N 次 cgo + N 趟扫。护栏:合并命中集 == 各成员 existsIn。
- **R2 断言 NFA 合并单趟**(中等,确定性):guard 断言 NFA 仿合并法收一趟,削 ~13% 断言路径 CPU。
- **R3 AVX2 档**(纯吞吐):一次 32 字节,需 AVX2 标量孪生差分 + 运行期探测。
- **R4 Rose 子串图多跳链**(>2 字面量,工程量大):中间子串级编排压低误触发率。
- **R5 定位档 C 内核**(独立,把 `MVS_Located` 16x→~30x):仅替换场景受益,需与 Go oracle 逐字节差分。

---

## 6. 与总方案的偏差记录(供 IMPL 文档对齐)

- **NFA 改为 rune 级**(IMPL 原文偏字节级):因 Go regexp 是逐 rune 解码、非法 UTF-8 当 RuneError,
  字节级在非法 UTF-8(真实流量常见)上对 `.`/负类与 oracle 分歧。rune 级 + 字母表压缩既正确又紧凑。
  Teddy/字面量预过滤仍是字节级(literal 本就是字节串),C 内核做 bit-NFA 时同样可做 rune 解码。
- 早期"ASCII 闸 + 字节级 UTF-8 展开"方案已废弃(`mvs_utf8.go` 删除),覆盖率由此从 33% 升到 88.5%。

> 建议接力者在动 P3/P4 前,把本节并入 `MINI_VECTOR_SCAN_IMPL.md` 的对应小节,保持设计记录一致。
