# Branch: feature/ssa2llvm/support_ci_and_mustpass_test

> 分支状态（2026-09-30 22:59 +0800 实测更新）。换机器继续时先读本文件。

## 目标

让 `common/yak/yaktest/mustpass/files/*.yak` 全部通过 ssa2llvm AOT 编译 + 原生二进制运行（exit 0、输出不含 `panic`）。

## 实测（2026-09-30）

`TestMustPass_SSA2LLVM_AllScripts` 在 2026-09-30T22:59:28+08:00 结束，`TEST_EXIT:0`。父测试 `--- PASS: TestMustPass_SSA2LLVM_AllScripts (3298.69s)`。目录里 115 个 `.yak` 都有对应的 `--- PASS`，没有 `--- FAIL`。`swagger2_generator.yak` 26.23s。

- HEAD：`69c8445b30749f14742e3aecb031f5a990c1722a`
- 套件日志：`/mnt/data/ssa2llvm-deps/repro-vulinbox/runs/mustpass-all-go127-httpflow.log`
- 驱动日志：`/mnt/data/ssa2llvm-deps/repro-vulinbox/runs/tier-go127/mustpass-httpflow.log`
- 测试进程 `GOROOT=/usr/lib/go`，`go version go1.27.1-X:nodwarf5 linux/amd64`
- 嵌入归档在同日 22:01 用 `/mnt/data/ssa2llvm-deps/go1.27.1-textsect` 重编（core、net、staticanalyze）。系统 `/usr/lib/go` 没有 textsect 补丁。

这一轮之前，同一套脚本在 `swagger2_generator.yak` 停住：AOT 没有打开项目库，`poc.save` 没有 HTTP flow 回调，`openapi.ExtractOpenAPI3Scheme` 返回 `no path item`。`yakit.InitialDatabase` 只写在已经导入 `common/yakgrpc/yakit` 的生成 `yak_register_globals` 里。core 档不导入这个包。

## 环境要求（重要）

- **Go 工具链**：测试和普通编译用系统 `/usr/bin/go`（go 1.27.1），`GOTOOLCHAIN=local`，不要设置 `GOROOT`。不要把 `/mnt/data/ssa2llvm-deps/go1.26.6` 放进 `PATH`。
- **c-archive**：只有编 `libyak.a` 时用补过的 `GOROOT=/mnt/data/ssa2llvm-deps/go1.27.1-textsect`。编完测试前再把 `GOROOT` 取消。不要改 `/usr/lib/go`。
- 2026-09-28 文档里“必须用 go 1.26.6”已经过时。1.26.6 的 textsect 偏移不能拿来编现在的归档。
- **本分支的测试门**是 `TestMustPass_SSA2LLVM_AllScripts`，用 `/usr/bin/go test` 跑 `./common/yak/ssa2llvm/tests/`。不要用 `scripts/ssa-test.sh` 当这个分支的门。
- **YAKIT_HOME**：跑脚本时用 `/mnt/data/ssa2llvm-deps/` 下的独立目录，避免写到用户自己的 yakit 库。

## 已完成（已 push 的 commit 见 git log）

### 环境/基础设施

- go 1.26.6 工具链适配（moduledata 偏移）
- Boehm GC 自动回收禁用（shadow 被过早回收导致随机失败）→ `yak_runtime_gc` 显式 enable/collect/disable
- lld 崩溃修复：`//export` 文件缺 `import "C"` 导致符号丢失（runtime_globals_aot.go）

### 语言语义修复（grammar_test.yak 全量通过）

- OpEq 常量折叠（int vs int64 类型不匹配 → 6==6 折叠成 false）
- float() 类型区分（CreateFloatType，int()/float() 转换方向）
- 类型转换 runtime 函数（to_int/to_float/to_string/bool_to_string/parse_int/parse_float）
- 负索引 key 符号（to_cstring 保留负数）、字符串索引（resolveField string 分支）
- slice 表达式（slice-of-slice 窗口复制 + `a[::-1]` 负步长反转）
- makeInitialMemberCount 过滤 Undefined/phi 占位（a[-1] 读取不膨胀长度）
- buildSliceCall 的 `[a:b]` 定位（colon 计数）
- ellipsis 展开（`b(a...)` 正序 + 过滤读取占位）
- map 闭包调用（`a["c"](1)` 走动态调用而非 method dispatch）
- break 循环 phi 写回（emitLoopExitPhiWriteback）
- float 算术（1.0 * 2 走 runtime float binop，2.0 位模式 tag-bit 解码）

### fuzztag 模板（x\`...\`）

- 内联 fuzztag 引擎（fuzztag_rt.go）：trim/substr/gb18030/gb18030toUTF8/hexd/crlf/list/list:comma/list:auto/int，嵌套 {{...}} 笛卡尔组合
- 不用 common/mutate（其 consts 依赖污染 runtime init 图导致崩溃）

### 其他

- VULINBOX/VULINBOX_HOST 编译期注入（extern value，mustpass_all_test 用 t.Setenv）
- string/bytes 按内容比较（runtimeValuesEqual）
- 闭包 slice 写回（支配检查 + 变量名匹配 + 懒解析 + entry alloca 中转）
- mustpass_simple/ 回归套件（5 个最小用例 + mustpass_simple_test.go）

## 历史状态（2026-09-29 11:37 CST，已被上面的 115/115 取代）

- **59/115 通过，56 失败**（当时实测；此前为 54/115）。下面的失败清单是这次全量通过之前的记录。
  修复了 5 个原先编译失败的脚本（`poc_download`、`nuclei_network_runtime`、
  `udp`、`waitAllAsyncCallFinish`×2）以及负数 int/float 显示错误。
- 失败分类：崩溃 crash(exit -1) 24 个、exit 255 20 个、超时 9 个、
  runtime panic 3 个。**本轮无新增失败脚本**（逐项对比失败清单确认）。
- 单轮测试的临时磁盘占用从约 60 GB 降到约 27 MB（见下节）。

- 历史基线（2026-09-28）：54/115 通过，61 失败（含 5 个编译失败）。
- 回归测试（mustpass_simple + closure + DualRun + ZeroDep）全绿

### 2026-09-28 rebase + 实测基线（历史记录，不要再按这里执行）

当时的记录认为系统 go 1.27.1 会产生 `final textsectmap len/cap mismatch`，
所以用了 1.26.6。2026-09-30 起改用系统 go 1.27.1，c-archive 用补过的 1.27 GOROOT。下面的命令只作当时的记录：

```sh
export PATH=/mnt/data/ssa2llvm-deps/go1.26.6/go/bin:$PATH GOTOOLCHAIN=local
bash common/yak/ssa2llvm/scripts/build_tiers.sh /mnt/data/ssa2llvm-deps/tiers
CGO_ENABLED=1 go build -o /mnt/data/ssa2llvm-deps/ssa2llvm ./common/yak/ssa2llvm/cmd/ssa2llvm

mkdir -p /mnt/data/ssa2llvm-deps/tmp /mnt/data/ssa2llvm-deps/yakit-all
TMPDIR=/mnt/data/ssa2llvm-deps/tmp YAKIT_HOME=/mnt/data/ssa2llvm-deps/yakit-all \
  bash scripts/ssa-test.sh ./common/yak/ssa2llvm/tests/ \
  -run '^TestMustPass_SSA2LLVM_AllScripts$' -count=1 -timeout 3h -v
```

全量耗时约 39 分钟（2331s）。分层结果、失败清单与分类存放在
`/mnt/data/ssa2llvm-deps/mustpass-all.log`、`fail-map.tsv`、`fail-list.txt`。

- 本机没有静态 `libgc.a`（Arch 的 gc 包只提供 `.so`），已从 bdwgc 8.2.12
  现场编译并放在 `/mnt/data/ssa2llvm-deps/gc-prefix/lib/libgc.a`，
  再拷回 `common/yak/ssa2llvm/runtime/runtime_go/libs/libgc.a`。
- rebase 引入的集成问题已修：`common/notify/drivers/feishu` 的 init 会调用
  shared 组里的 protobuf，原先未被任何分组认领；已把它归入 shared 组，
  否则 `staticanalyze` 层会因 elfsplit 启动路径泄漏而构建失败。

## 历史剩余工作（2026-09-29；2026-09-30 全量已通过，不再是待办）

### P0-P2（runtime 语义，可继续修）

- `db_query_plugin.yak`：Test 1.3（float ID 查询）—— float binop 已修，需重验
- `expect_100_continue.yak`、`fuzz-http-request-value.yak`、`fuzz_mutate_post_json_params.yak`、`git_test.yak`、`git_to_sca.yak`、`githack_test.yak`、`java-decompiler.yak`、`jsonschema_builder.yak`、`noautodecode-fuzz.yak`、`poc_no_redirect.yak`、`suricata_match.yak`、`tlsinspect.yak`、`yaklang_programming_complex.yak` 等（exit 255，部分输出后失败）
- `dictutil.yak`（挂起 timeout）
- `fuzz_json_params_no_escape_html.yak`（回归？之前通过，需确认）

### P3（SIGSEGV，约 35 个）

- `jwt`/`jwt_order`/`mixcaller*`/`crawlerx`/`http_lowhttp`/`mitm_*`/`nuclei_*`/`mock_*`/`portscan`/`syntaxflow`/`poc_params_fuzz`/`risk_*`/`browser_brute`/`build_kb_from_file`/`head_chunked_test`/`hook_load_plugin_by_id`/`httpserver_allbasic`/`lowhttp_poc`/`plugin_inherit_proxy`/`rag_question_index`/`yakpoc-cookie-and-ua`/`zip` 等
- 主要模式：调用帧/指针表示问题（crypto/hmac 函数表间接调用崩溃、小地址对象指针错解引用）

### P4（网络/复杂模块）

- `mitm_*`（10 个）、`udp*`（3 个，含 lld 链接崩溃）、`waitAllAsyncCallFinish*`（lld 崩溃）、`rag*`、`omnisearch`、`sandbox`、`nuclei_network*`、`poc_download`（编译失败）等

## 如何重跑

在 worktree 里，`GOROOT` 不设置，`GOTOOLCHAIN=local`，`GOWORK=off`，`SSA2LLVM_TIER_DIR` 不设置：

```sh
/usr/bin/go test -v -count=1 -timeout 150m -failfast \
  -run 'TestMustPass_SSA2LLVM_AllScripts$' ./common/yak/ssa2llvm/tests/
```

`runtime/libyak.a` 必须新于 `runtime/runtime_go` 里的源文件，否则测试会用系统 GOROOT 重编归档。归档要用补过的 1.27 GOROOT，按 core、net、staticanalyze 的顺序编，一次只跑一个链接。

## 注意事项

- 这个分支的 mustpass 用上面的 `/usr/bin/go test`，不要把 `scripts/ssa-test.sh` 当作本分支的门
- 不要清理 GOCACHE（构建很慢）
- 构建 CLI 覆盖 `build/ssa2llvm`，不要留版本号
- 提交前检查无 `Co-authored-by:` 行

## 测试磁盘占用（2026-09-29 修复）

一次 ssa2llvm 编译会在 `$TMPDIR` 下留下约 600 MB 临时数据：运行时归档的私有
副本（staticanalyze 层最大约 390 MB）加上链接产物，而编译器会保留这些确定性
work dir（`yakssa-compile-*`）以便复用。mustpass 逐脚本强制 `-a` 重建，复用
不可能发生，于是 115 个脚本累积约 60 GB 残留，CI runner 的磁盘放不下。

**修复后实测：** 完整 115 脚本一轮结束后，共享 temp 目录只剩 27 MB
（修复前为 63 GB），峰值约 6 GB，且不残留任何 `yakssa-compile-*`。

修复只动测试侧，生产缓存语义不变：

- `tests/main_test.go` 的 `TestMain` 为整轮测试建立一个 run root 并把它设为
  `TMPDIR`/`TMP`/`TEMP`，跑完统一删除；`SSA2LLVM_TEST_KEEP_TMP=1` 可保留排查。
- mustpass 编译改用 `runSSA2LLVMCLIInDirScratch`，每次调用单独 `TMPDIR`，脚本
  结束即刻回收，峰值从约 60 GB 降到单个脚本量级（约 0.6 GB）。

验证：指定 2 个脚本运行后共享 temp 目录不再产生任何 `yakssa-compile-*`，run
root 也不残留；`SSA2LLVM_TEST_KEEP_TMP=1` 单脚本运行时，临时数据确实全部落在
run root 内。
